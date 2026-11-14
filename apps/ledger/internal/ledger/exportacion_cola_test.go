package ledger_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/jackc/pgx/v5/pgxpool"

	"ledger/internal/cola"
	"ledger/internal/ledger"
	"ledger/internal/objetos"
	"ledger/internal/pg"
	"ledger/internal/proveedor"
)

const varSQS = "SQS_ENDPOINT_TEST"

type entornoCola struct {
	cli   *sqs.Client
	svc   *ledger.Servicio
	pool  *pgxpool.Pool
	cola  *cola.Cola
	dlq   *cola.Cola
	desde string
	a, b  int64
}

func colaDePrueba(t *testing.T, cliente *sqs.Client, nombre string) *cola.Cola {
	t.Helper()
	ctx := context.Background()
	salida, err := cliente.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(fmt.Sprintf("%s-%d", nombre, time.Now().UnixNano())),
	})
	if err != nil {
		t.Fatalf("crear la cola: %v", err)
	}
	url := aws.ToString(salida.QueueUrl)
	t.Cleanup(func() { cliente.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{QueueUrl: aws.String(url)}) })
	c, err := cola.Nueva(ctx, "us-east-1", url)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func conCola(t *testing.T, alm func(objetos.Almacen) objetos.Almacen) *entornoCola {
	t.Helper()
	endpoint := os.Getenv(varSQS)
	if endpoint == "" {
		t.Skipf("sin %s: se saltea la prueba contra SQS", varSQS)
	}
	_, _, pool := entorno(t)
	t.Setenv("AWS_ENDPOINT_URL_SQS", endpoint)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"))
	if err != nil {
		t.Fatal(err)
	}
	cliente := sqs.NewFromConfig(cfg)

	disco, err := objetos.Desde(ctx, "", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := ledger.Nuevo(ctx, pool, proveedor.Nuevo("http://localhost:1", time.Second), alm(disco), nil, time.Minute, "http://localhost:1")
	if err != nil {
		t.Fatal(err)
	}
	e := &entornoCola{cli: cliente, svc: svc, pool: pool, cola: colaDePrueba(t, cliente, "exports"), dlq: colaDePrueba(t, cliente, "exports-dlq")}
	svc.ConCola(e.cola, e.dlq, 3, 30*time.Second)

	a, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "a", Tipo: "usuario", SaldoInicialCentavos: 1000})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "b", Tipo: "usuario"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Transferir(ctx, ledger.EntradaTransferencia{
		IdempotencyKey: "cola-1", OrigenID: a.ID, DestinoID: b.ID, MontoCentavos: 500}); err != nil {
		t.Fatal(err)
	}
	e.a, e.b = a.ID, b.ID
	e.desde = pg.InicioDeMes(time.Now()).Format("2006-01")
	return e
}

func sinCambios(alm objetos.Almacen) objetos.Almacen { return alm }

func (e *entornoCola) fila(t *testing.T, id int64) (estado string, intentos int, tomada *string) {
	t.Helper()
	if err := e.pool.QueryRow(context.Background(),
		`SELECT estado, intentos, tomada_por FROM solicitudes_exportacion WHERE id = $1`, id).Scan(&estado, &intentos, &tomada); err != nil {
		t.Fatal(err)
	}
	return
}

func vacia(t *testing.T, c *cola.Cola) {
	t.Helper()
	if m, err := c.Recibir(context.Background(), time.Second, 30*time.Second); err != nil || m != nil {
		t.Fatalf("la cola tendria que estar vacia: %+v, %v", m, err)
	}
}

func TestExportacionPorColaQuedaListaYBorraElMensaje(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()

	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	if hubo, err := e.svc.AtenderMensajeDeExportacion(ctx); !hubo || err != nil {
		t.Fatalf("atender: hubo=%v, %v", hubo, err)
	}
	if estado, intentos, tomada := e.fila(t, x.ID); estado != ledger.ExportLista || intentos != 1 {
		t.Fatalf("estado %q, intentos %d, tomada %v", estado, intentos, tomada)
	}
	vacia(t, e.cola)
	vacia(t, e.dlq)
}

func TestMensajeDuplicadoNoArmaDosVeces(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()

	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.cola.Enviar(ctx, strconv.FormatInt(x.ID, 10)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if hubo, err := e.svc.AtenderMensajeDeExportacion(ctx); !hubo || err != nil {
			t.Fatalf("atender %d: hubo=%v, %v", i, hubo, err)
		}
	}
	if estado, intentos, _ := e.fila(t, x.ID); estado != ledger.ExportLista || intentos != 1 {
		t.Fatalf("el duplicado no tendria que sumar un intento: estado %q, intentos %d", estado, intentos)
	}
	vacia(t, e.cola)
}

func TestDuplicadoConDuenioVivoSeVuelveAOcultar(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()

	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `
		UPDATE solicitudes_exportacion SET tomada_por = 'otra-tarea', tomada_en = now(), intentos = 1 WHERE id = $1`, x.ID); err != nil {
		t.Fatal(err)
	}
	if hubo, err := e.svc.AtenderMensajeDeExportacion(ctx); !hubo || err != nil {
		t.Fatalf("atender: hubo=%v, %v", hubo, err)
	}
	estado, intentos, tomada := e.fila(t, x.ID)
	if estado != ledger.ExportPendiente || intentos != 1 || tomada == nil || *tomada != "otra-tarea" {
		t.Fatalf("no tendria que tocar una exportacion tomada: estado %q, intentos %d, tomada %v", estado, intentos, tomada)
	}
	vacia(t, e.cola)
	if ocultos := e.atributo(t, e.cola, types.QueueAttributeNameApproximateNumberOfMessagesNotVisible); ocultos != "1" {
		t.Fatalf("el mensaje tiene que quedar oculto, no borrado: %s ocultos", ocultos)
	}
}

func TestReintentaYAlAgotarLaMandaALaDLQ(t *testing.T) {
	e := conCola(t, func(alm objetos.Almacen) objetos.Almacen { return &almacenQueFalla{Almacen: alm, fallas: 99} })
	ctx := context.Background()

	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	if hubo, err := e.svc.AtenderMensajeDeExportacion(ctx); !hubo || err == nil {
		t.Fatalf("el primer intento tenia que fallar: hubo=%v, %v", hubo, err)
	}
	estado, intentos, tomada := e.fila(t, x.ID)
	if estado != ledger.ExportPendiente || intentos != 1 || tomada != nil {
		t.Fatalf("tras un error tiene que quedar pendiente y suelta: estado %q, intentos %d, tomada %v", estado, intentos, tomada)
	}
	vacia(t, e.cola)

	if err := e.cola.Enviar(ctx, strconv.FormatInt(x.ID, 10)); err != nil {
		t.Fatal(err)
	}
	if hubo, err := e.svc.AtenderMensajeDeExportacion(ctx); !hubo || err != nil {
		t.Fatalf("una copia durante la espera no tiene que fallar: hubo=%v, %v", hubo, err)
	}
	if _, intentos, _ := e.fila(t, x.ID); intentos != 1 {
		t.Fatalf("una copia durante la espera no gasta un intento: intentos %d", intentos)
	}

	for intento := 2; intento <= 3; intento++ {
		if _, err := e.pool.Exec(ctx, `UPDATE solicitudes_exportacion SET reintentar_desde = NULL WHERE id = $1`, x.ID); err != nil {
			t.Fatal(err)
		}
		if err := e.cola.Enviar(ctx, strconv.FormatInt(x.ID, 10)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.AtenderMensajeDeExportacion(ctx); err == nil {
			t.Fatalf("el intento %d tenia que fallar", intento)
		}
	}
	if estado, intentos, _ := e.fila(t, x.ID); estado != ledger.ExportFallida || intentos != 3 {
		t.Fatalf("al tercer intento tiene que quedar fallida: estado %q, intentos %d", estado, intentos)
	}
	m, err := e.dlq.Recibir(ctx, time.Second, 30*time.Second)
	if err != nil || m == nil || m.Cuerpo != strconv.FormatInt(x.ID, 10) {
		t.Fatalf("la DLQ tenia que recibir la exportacion %d: %+v, %v", x.ID, m, err)
	}
}

func TestElBarridoReencolaLasPendientesViejas(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()

	var id int64
	if err := e.pool.QueryRow(ctx, `
		INSERT INTO solicitudes_exportacion (cuenta_id, desde, hasta, estado, solicitada_en)
		VALUES ($1, $2, $2, 'pendiente', now() - interval '1 hour') RETURNING id`,
		e.a, pg.InicioDeMes(time.Now())).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if n, err := e.svc.ReencolarExportaciones(ctx); err != nil || n != 1 {
		t.Fatalf("reencolar: %d, %v", n, err)
	}
	if n, err := e.svc.ReencolarExportaciones(ctx); err != nil || n != 0 {
		t.Fatalf("recien reencolada no se vuelve a mandar: %d, %v", n, err)
	}
	m, err := e.cola.Recibir(ctx, time.Second, 30*time.Second)
	if err != nil || m == nil || m.Cuerpo != strconv.FormatInt(id, 10) {
		t.Fatalf("la cola tenia que recibir la exportacion %d: %+v, %v", id, m, err)
	}
}

func TestArchivadoEnCursoNoGastaIntentos(t *testing.T) {
	svc, _, pool := entorno(t)
	ctx := context.Background()

	a, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "a", Tipo: "usuario"})
	if err != nil {
		t.Fatal(err)
	}
	mes := pg.InicioDeMes(time.Now())
	if _, err := pool.Exec(ctx, `INSERT INTO periodos_archivados (periodo) VALUES ($1)`, mes); err != nil {
		t.Fatal(err)
	}
	x, err := svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: a.ID, Desde: mes.Format("2006-01"), Hasta: mes.Format("2006-01")})
	if err != nil {
		t.Fatal(err)
	}
	if hubo, err := svc.ResolverExportacionPendiente(ctx); !hubo || err != nil {
		t.Fatalf("resolver: hubo=%v, %v", hubo, err)
	}
	var estado string
	var intentos int
	var tomada *string
	if err := pool.QueryRow(ctx, `SELECT estado, intentos, tomada_por FROM solicitudes_exportacion WHERE id = $1`, x.ID).Scan(&estado, &intentos, &tomada); err != nil {
		t.Fatal(err)
	}
	if estado != ledger.ExportPendiente || intentos != 0 || tomada != nil {
		t.Fatalf("un mes archivandose no gasta el intento: estado %q, intentos %d, tomada %v", estado, intentos, tomada)
	}
	if hubo, err := svc.ResolverExportacionPendiente(ctx); hubo || err != nil {
		t.Fatalf("mientras se archiva no se vuelve a tomar enseguida: hubo=%v, %v", hubo, err)
	}
}

func TestPorTablaReintentaTrasUnError(t *testing.T) {
	_, _, pool := entorno(t)
	ctx := context.Background()
	disco, err := objetos.Desde(ctx, "", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := ledger.Nuevo(ctx, pool, proveedor.Nuevo("http://localhost:1", time.Second),
		&almacenQueFalla{Almacen: disco, fallas: 1}, nil, time.Minute, "http://localhost:1")
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "a", Tipo: "usuario"})
	if err != nil {
		t.Fatal(err)
	}
	mes := pg.InicioDeMes(time.Now()).Format("2006-01")
	x, err := svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: a.ID, Desde: mes, Hasta: mes})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolverExportacionPendiente(ctx); err == nil {
		t.Fatal("el primer intento tenia que fallar")
	}
	if v, _ := svc.VerExportacion(ctx, x.ID); v.Estado != ledger.ExportPendiente {
		t.Fatalf("tras un error tiene que seguir pendiente, esta %q", v.Estado)
	}
	if hubo, err := svc.ResolverExportacionPendiente(ctx); hubo || err != nil {
		t.Fatalf("antes de la espera no se reintenta: hubo=%v, %v", hubo, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE solicitudes_exportacion SET reintentar_desde = now() - interval '1 second' WHERE id = $1`, x.ID); err != nil {
		t.Fatal(err)
	}
	if hubo, err := svc.ResolverExportacionPendiente(ctx); !hubo || err != nil {
		t.Fatalf("segundo intento: hubo=%v, %v", hubo, err)
	}
	if v, _ := svc.VerExportacion(ctx, x.ID); v.Estado != ledger.ExportLista || v.Motivo != "" {
		t.Fatalf("el segundo intento tenia que dejarla lista y sin motivo: %+v", v)
	}
}

func (e *entornoCola) atributo(t *testing.T, c *cola.Cola, nombre types.QueueAttributeName) string {
	t.Helper()
	salida, err := e.cli.GetQueueAttributes(context.Background(), &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(c.URL()), AttributeNames: []types.QueueAttributeName{nombre}})
	if err != nil {
		t.Fatal(err)
	}
	return salida.Attributes[string(nombre)]
}

func TestArchivadoEnCursoPorColaReencolaConDemora(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()

	mes := pg.InicioDeMes(time.Now())
	if _, err := e.pool.Exec(ctx, `INSERT INTO periodos_archivados (periodo) VALUES ($1)`, mes); err != nil {
		t.Fatal(err)
	}
	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	if hubo, err := e.svc.AtenderMensajeDeExportacion(ctx); !hubo || err != nil {
		t.Fatalf("atender: hubo=%v, %v", hubo, err)
	}
	if estado, intentos, tomada := e.fila(t, x.ID); estado != ledger.ExportPendiente || intentos != 0 || tomada != nil {
		t.Fatalf("estado %q, intentos %d, tomada %v", estado, intentos, tomada)
	}
	vacia(t, e.cola)
	if demorados := e.atributo(t, e.cola, types.QueueAttributeNameApproximateNumberOfMessagesDelayed); demorados != "1" {
		t.Fatalf("tendria que haber un mensaje demorado, hay %s", demorados)
	}
}

func TestSiYaAgotoLosIntentosQuedaFallidaConSuMotivo(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()

	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE solicitudes_exportacion SET intentos = 3, motivo = 'PUT cortado' WHERE id = $1`, x.ID); err != nil {
		t.Fatal(err)
	}
	if hubo, err := e.svc.AtenderMensajeDeExportacion(ctx); !hubo || err != nil {
		t.Fatalf("atender: hubo=%v, %v", hubo, err)
	}
	v, err := e.svc.VerExportacion(ctx, x.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Estado != ledger.ExportFallida || v.Motivo != "PUT cortado" {
		t.Fatalf("tiene que quedar fallida con el ultimo error: %+v", v)
	}
	if m, err := e.dlq.Recibir(ctx, time.Second, 30*time.Second); err != nil || m == nil || m.Cuerpo != strconv.FormatInt(x.ID, 10) {
		t.Fatalf("la DLQ tenia que recibir la exportacion: %+v, %v", m, err)
	}
	vacia(t, e.cola)
}

func TestElBarridoNoTocaLasVivasNiLasQueEsperan(t *testing.T) {
	e := conCola(t, sinCambios)
	ctx := context.Background()

	for _, extra := range []string{
		`tomada_por = 'otra-tarea', tomada_en = now()`,
		`reintentar_desde = now() + interval '1 hour'`,
	} {
		if _, err := e.pool.Exec(ctx, `
			INSERT INTO solicitudes_exportacion (cuenta_id, desde, hasta, estado, solicitada_en)
			VALUES ($1, $2, $2, 'pendiente', now() - interval '1 hour')`, e.a, pg.InicioDeMes(time.Now())); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, `UPDATE solicitudes_exportacion SET `+extra+` WHERE id = (SELECT max(id) FROM solicitudes_exportacion)`); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := e.svc.ReencolarExportaciones(ctx); err != nil || n != 0 {
		t.Fatalf("no tenia que reencolar ninguna: %d, %v", n, err)
	}
}

type almacenQueEspera struct {
	objetos.Almacen
	empezo chan struct{}
}

func (a *almacenQueEspera) Subir(ctx context.Context, _, _ string, _ func(io.Writer) error) (objetos.Subido, error) {
	select {
	case a.empezo <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return objetos.Subido{}, ctx.Err()
}

func TestSiOtraTareaLaTomaElLatidoCortaSinCerrar(t *testing.T) {
	espera := &almacenQueEspera{empezo: make(chan struct{}, 1)}
	e := conCola(t, func(alm objetos.Almacen) objetos.Almacen { espera.Almacen = alm; return espera })
	ctx := context.Background()

	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	listo := make(chan error, 1)
	go func() {
		_, err := e.svc.AtenderMensajeDeExportacion(ctx)
		listo <- err
	}()
	<-espera.empezo
	if _, err := e.pool.Exec(ctx, `UPDATE solicitudes_exportacion SET tomada_por = 'otra-tarea', tomada_en = now() WHERE id = $1`, x.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-listo:
		if err != nil {
			t.Fatalf("atender: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("el latido no corto el armado")
	}
	estado, intentos, tomada := e.fila(t, x.ID)
	if estado != ledger.ExportPendiente || intentos != 1 || tomada == nil || *tomada != "otra-tarea" {
		t.Fatalf("no tenia que tocar la exportacion ajena: estado %q, intentos %d, tomada %v", estado, intentos, tomada)
	}
	vacia(t, e.cola)
}

func TestSiSeCancelaDevuelveElIntentoYElMensaje(t *testing.T) {
	espera := &almacenQueEspera{empezo: make(chan struct{}, 1)}
	e := conCola(t, func(alm objetos.Almacen) objetos.Almacen { espera.Almacen = alm; return espera })
	ctx := context.Background()

	x, err := e.svc.Exportar(ctx, ledger.EntradaExportacion{CuentaID: e.a, Desde: e.desde, Hasta: e.desde})
	if err != nil {
		t.Fatal(err)
	}
	apagado, cancelar := context.WithCancel(ctx)
	listo := make(chan error, 1)
	go func() {
		_, err := e.svc.AtenderMensajeDeExportacion(apagado)
		listo <- err
	}()
	<-espera.empezo
	cancelar()
	if err := <-listo; err != nil {
		t.Fatalf("atender: %v", err)
	}
	if estado, intentos, tomada := e.fila(t, x.ID); estado != ledger.ExportPendiente || intentos != 0 || tomada != nil {
		t.Fatalf("tiene que devolver el intento y soltarla: estado %q, intentos %d, tomada %v", estado, intentos, tomada)
	}
	m, err := e.cola.Recibir(ctx, 2*time.Second, 30*time.Second)
	if err != nil || m == nil || m.Cuerpo != strconv.FormatInt(x.ID, 10) || m.Recibido != 1 {
		t.Fatalf("el mensaje tiene que volver como uno nuevo: %+v, %v", m, err)
	}
}
