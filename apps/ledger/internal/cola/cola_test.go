package cola

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

const varEndpoint = "SQS_ENDPOINT_TEST"

func crearDePrueba(t *testing.T, nombre string) string {
	t.Helper()
	endpoint := os.Getenv(varEndpoint)
	if endpoint == "" {
		t.Skipf("sin %s: se saltea la prueba contra SQS", varEndpoint)
	}
	t.Setenv("AWS_ENDPOINT_URL_SQS", endpoint)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"))
	if err != nil {
		t.Fatal(err)
	}
	cliente := sqs.NewFromConfig(cfg)
	salida, err := cliente.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(fmt.Sprintf("%s-%d", nombre, time.Now().UnixNano())),
	})
	if err != nil {
		t.Fatalf("crear la cola: %v", err)
	}
	url := aws.ToString(salida.QueueUrl)
	t.Cleanup(func() { cliente.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{QueueUrl: aws.String(url)}) })
	return url
}

func TestEnviarRecibirOcultarYBorrar(t *testing.T) {
	url := crearDePrueba(t, "cola")
	ctx := context.Background()
	c, err := Nueva(ctx, "us-east-1", url)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Habilitada() {
		t.Fatal("con URL tendria que estar habilitada")
	}

	if err := c.Enviar(ctx, "42"); err != nil {
		t.Fatal(err)
	}
	m, err := c.Recibir(ctx, time.Second, 30*time.Second)
	if err != nil || m == nil {
		t.Fatalf("recibir: %v, %v", m, err)
	}
	if m.Cuerpo != "42" || m.Recibido != 1 {
		t.Fatalf("mensaje inesperado: %+v", m)
	}
	if otro, err := c.Recibir(ctx, time.Second, 30*time.Second); err != nil || otro != nil {
		t.Fatalf("mientras esta oculto no tendria que volver: %+v, %v", otro, err)
	}

	if err := c.Ocultar(ctx, m, 0); err != nil {
		t.Fatal(err)
	}
	deNuevo, err := c.Recibir(ctx, time.Second, 30*time.Second)
	if err != nil || deNuevo == nil || deNuevo.Recibido != 2 {
		t.Fatalf("con la visibilidad en 0 tendria que volver como segunda entrega: %+v, %v", deNuevo, err)
	}

	if err := c.Borrar(ctx, deNuevo); err != nil {
		t.Fatal(err)
	}
	if fin, err := c.Recibir(ctx, time.Second, 30*time.Second); err != nil || fin != nil {
		t.Fatalf("despues de borrarlo no tendria que volver: %+v, %v", fin, err)
	}
}

func TestSinURLEstaDeshabilitada(t *testing.T) {
	c, err := Nueva(context.Background(), "us-east-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Habilitada() {
		t.Fatal("sin URL no tendria que estar habilitada")
	}
	var nula *Cola
	if nula.Habilitada() {
		t.Fatal("una cola nil no esta habilitada")
	}
}
