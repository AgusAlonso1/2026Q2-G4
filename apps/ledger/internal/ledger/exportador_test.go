package ledger_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/jackc/pgx/v5/pgxpool"

	"ledger/internal/avisos"
	"ledger/internal/ledger"
	"ledger/internal/objetos"
	"ledger/internal/pg"
)

type almacenQueSeJunta struct {
	objetos.Almacen
	esperan int
	plazo   time.Duration

	mu      sync.Mutex
	adentro int
	todos   chan struct{}
}

type escritorQueSeJunta struct {
	w       io.Writer
	alm     *almacenQueSeJunta
	escrito int
	junto   bool
}

func (e *escritorQueSeJunta) Write(p []byte) (int, error) {
	e.escrito += len(p)
	if !e.junto && e.escrito > 1024 {
		e.junto = true
		e.alm.mu.Lock()
		e.alm.adentro++
		if e.alm.adentro == e.alm.esperan {
			close(e.alm.todos)
		}
		e.alm.mu.Unlock()
		select {
		case <-e.alm.todos:
		case <-time.After(e.alm.plazo):
		}
	}
	return e.w.Write(p)
}

func (a *almacenQueSeJunta) Subir(ctx context.Context, clave, tipo string, escribir func(io.Writer) error) (objetos.Subido, error) {
	return a.Almacen.Subir(ctx, clave, tipo, func(w io.Writer) error {
		return escribir(&escritorQueSeJunta{w: w, alm: a})
	})
}

func poolChico(t *testing.T, conexiones string) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(varURL)
	separador := "?"
	if strings.Contains(url, "?") {
		separador = "&"
	}
	pool, err := pg.Abrir(context.Background(), url+separador+"pool_max_conns="+conexiones)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestElExportadorArmaDosALaVezConUnaConexionCadaUna(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()

	viejo := periodoViejo.Format("2006-01")
	if _, err := e.svc.SembrarPeriodo(ctx, ledger.EntradaSemilla{
		Periodo: viejo, OrigenID: e.a, DestinoID: e.b, Cantidad: 400, MontoCentavos: 1}); err != nil {
		t.Fatal(err)
	}

	disco, err := objetos.Desde(ctx, "", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	junta := &almacenQueSeJunta{Almacen: disco, esperan: 2, plazo: 15 * time.Second, todos: make(chan struct{})}
	exportador, err := ledger.Nuevo(ctx, poolChico(t, "2"), nil, junta, nil, time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	exportador.ConCola(e.cola, e.dlq, 3, 30*time.Second)

	var ids []int64
	for _, cuenta := range []int64{0, e.a} {
		x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: cuenta, Desde: viejo, Hasta: viejo})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, x.ID)
	}

	apagado, apagar := context.WithCancel(ctx)
	defer apagar()
	termino := make(chan struct{})
	go func() {
		defer close(termino)
		exportador.AtenderExportaciones(apagado, 2)
	}()

	limite := time.Now().Add(30 * time.Second)
	for _, id := range ids {
		for {
			estado, _, _ := e.fila(t, id)
			if estado == ledger.ExportLista {
				break
			}
			if time.Now().After(limite) {
				t.Fatalf("la exportacion %d sigue %q", id, estado)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	select {
	case <-junta.todos:
	default:
		t.Fatal("las dos exportaciones tenian que armarse a la vez")
	}

	for _, id := range ids {
		x, err := e.svc.VerExportacion(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if x.CuentaID == nil && valorDe(x.Filas) != 800 || x.CuentaID != nil && valorDe(x.Filas) != 400 {
			t.Fatalf("exportacion %d: %d filas", id, valorDe(x.Filas))
		}
	}

	apagar()
	select {
	case <-termino:
	case <-time.After(10 * time.Second):
		t.Fatal("el exportador no se detuvo al cancelar el contexto")
	}
	vacia(t, e.cola)
}

func valorDe(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

type almacenQueCopia struct {
	objetos.Almacen
	copia bytes.Buffer
}

func (a *almacenQueCopia) Subir(ctx context.Context, clave, tipo string, escribir func(io.Writer) error) (objetos.Subido, error) {
	return a.Almacen.Subir(ctx, clave, tipo, func(w io.Writer) error {
		return escribir(io.MultiWriter(w, &a.copia))
	})
}

func TestLaExportacionLlevaLosAliasDeLasDosCuentas(t *testing.T) {
	copia := &almacenQueCopia{}
	e := conCola(t, func(alm objetos.Almacen) objetos.Almacen { copia.Almacen = alm; return copia })
	ctx := context.Background()

	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AtenderMensajeDeExportacion(ctx); err != nil {
		t.Fatal(err)
	}
	if estado, _, _ := e.fila(t, x.ID); estado != ledger.ExportLista {
		t.Fatalf("estado %q", estado)
	}
	if contenido := copia.copia.String(); !strings.Contains(contenido, ";a;Transferencia;b;-5,00;") {
		t.Fatalf("la fila tiene que llevar los alias de las dos cuentas:\n%s", contenido)
	}
}

func TestAlAgotarLosIntentosAvisaAlTitular(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()
	t.Setenv("AWS_ENDPOINT_URL_SNS", os.Getenv(varSQS))

	cfg := e.cli.Options()
	snsCli := sns.New(sns.Options{Region: cfg.Region, Credentials: cfg.Credentials, BaseEndpoint: aws.String(os.Getenv(varSQS))})
	topic, err := snsCli.CreateTopic(ctx, &sns.CreateTopicInput{Name: aws.String(fmt.Sprintf("titulares-%d", time.Now().UnixNano()))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { snsCli.DeleteTopic(context.Background(), &sns.DeleteTopicInput{TopicArn: topic.TopicArn}) })
	buzon := colaDePrueba(t, e.cli, "buzon")
	atributos, err := e.cli.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(buzon.URL()), AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snsCli.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: topic.TopicArn, Protocol: aws.String("sqs"),
		Endpoint: aws.String(atributos.Attributes[string(types.QueueAttributeNameQueueArn)]),
		Attributes: map[string]string{
			"RawMessageDelivery": "true",
			"FilterPolicy":       `{"titular": ["sub-a"]}`,
		},
	}); err != nil {
		t.Fatal(err)
	}
	avisador, err := avisos.Nuevo(ctx, "us-east-1", aws.ToString(topic.TopicArn))
	if err != nil {
		t.Fatal(err)
	}
	e.svc.ConAvisos(avisador, "http://app.example/")

	if _, err := e.pool.Exec(ctx, `UPDATE cuentas SET titular = 'sub-a' WHERE id = $1`, e.a); err != nil {
		t.Fatal(err)
	}
	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE solicitudes_exportacion SET intentos = 3 WHERE id = $1`, x.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AtenderMensajeDeExportacion(ctx); err != nil {
		t.Fatal(err)
	}
	if estado, _, _ := e.fila(t, x.ID); estado != ledger.ExportFallida {
		t.Fatalf("estado %q", estado)
	}

	m, err := buzon.Recibir(ctx, 2*time.Second, 30*time.Second)
	if err != nil || m == nil {
		t.Fatalf("el titular tenia que recibir un aviso: %+v, %v", m, err)
	}
	if !strings.Contains(m.Cuerpo, "No pudimos armar tu exportación N.º") ||
		!strings.Contains(m.Cuerpo, "http://app.example/#/exportaciones") {
		t.Fatalf("aviso inesperado: %q", m.Cuerpo)
	}
}
