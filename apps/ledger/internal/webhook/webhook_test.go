package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sns"
)

type snsFalso struct {
	mu        sync.Mutex
	recibidos []*sns.PublishInput
	err       error
}

func (f *snsFalso) Publish(_ context.Context, in *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recibidos = append(f.recibidos, in)
	return &sns.PublishOutput{}, f.err
}

func TestFirmarEsSobreLosBytesEnviados(t *testing.T) {
	a := Aviso{OrdenID: 7, Tipo: "transferencia", Estado: "liquidada", MontoCentavos: 100}

	cuerpo, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := Firmar(cuerpo, "s3cr3t"), Firmar(cuerpo, "s3cr3t"); got != want {
		t.Fatalf("la firma no es determinista: %q vs %q", got, want)
	}
	if Firmar(cuerpo, "s3cr3t") == Firmar(cuerpo, "otro") {
		t.Fatal("la firma no depende del secreto")
	}
	if Firmar(cuerpo, "s3cr3t") == Firmar(append(cuerpo, ' '), "s3cr3t") {
		t.Fatal("la firma no depende del cuerpo")
	}
}

func TestFirmaCompatibleConLaLambda(t *testing.T) {
	const deNode = "2a02d86e5904f86e69632e216f6ab6410a43c83044939371076f02a227683d97"
	if got := Firmar([]byte(`{"orden_id":7,"tipo":"transferencia"}`), "s3cr3to"); got != deNode {
		t.Fatalf("la firma no coincide con la de la Lambda: %q", got)
	}
}

func TestSinTopicNoPublica(t *testing.T) {
	n, err := Nuevo(context.Background(), nil, "us-east-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if n.Habilitado() {
		t.Fatal("sin topic no deberia estar habilitado")
	}
	n.Notificar(Aviso{OrdenID: 1, Estado: "liquidada"})

	var nulo *Notificador
	nulo.Notificar(Aviso{OrdenID: 1, Estado: "liquidada"})
}

func TestPublicaElEventoConSusAtributos(t *testing.T) {
	falso := &snsFalso{}
	n := &Notificador{sns: falso, topic: "arn:aws:sns:us-east-1:1:ledger-events"}

	origen, destino := int64(3), int64(4)
	n.Notificar(Aviso{OrdenID: 7, Tipo: "transferencia", Estado: "liquidada",
		OrigenID: &origen, DestinoID: &destino, MontoCentavos: 150050})
	n.Esperar(context.Background())

	if len(falso.recibidos) != 1 {
		t.Fatalf("se esperaba una publicación, hubo %d", len(falso.recibidos))
	}
	in := falso.recibidos[0]
	if *in.TopicArn != n.topic {
		t.Fatalf("topic equivocado: %s", *in.TopicArn)
	}
	if *in.MessageAttributes["tipo"].StringValue != "transferencia" || *in.MessageAttributes["estado"].StringValue != "liquidada" {
		t.Fatalf("faltan los atributos que usa el filtro del mailer: %+v", in.MessageAttributes)
	}

	var ev Evento
	if err := json.Unmarshal([]byte(*in.Message), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.EventoID != "7:liquidada" {
		t.Fatalf("evento_id = %q, se esperaba 7:liquidada", ev.EventoID)
	}
	if ev.MontoCentavos != 150050 || *ev.OrigenID != 3 || *ev.DestinoID != 4 {
		t.Fatalf("el evento llegó alterado: %+v", ev)
	}
}

func TestUnErrorDeSNSNoFrenaNada(t *testing.T) {
	n := &Notificador{sns: &snsFalso{err: errors.New("sin red")}, topic: "arn"}
	n.Notificar(Aviso{OrdenID: 1, Tipo: "egreso", Estado: "acreditada"})
	n.Esperar(context.Background())
}

func TestElAvisoPublicoNoLlevaTitulares(t *testing.T) {
	ev := Evento{Aviso: Aviso{OrdenID: 1}, Titulares: &Titulares{Destino: "sub-b"}}
	conTitulares, _ := json.Marshal(ev)
	if !strings.Contains(string(conTitulares), `"titulares":{"destino":"sub-b"}`) {
		t.Fatalf("el evento tiene que llevar los titulares: %s", conTitulares)
	}
	sinTitulares, _ := json.Marshal(ev.Aviso)
	if strings.Contains(string(sinTitulares), "titulares") {
		t.Fatalf("el aviso no tiene que llevar los titulares: %s", sinTitulares)
	}
}
