package cola

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type clienteSQS interface {
	SendMessage(ctx context.Context, in *sqs.SendMessageInput, opts ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	ReceiveMessage(ctx context.Context, in *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, in *sqs.DeleteMessageInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
	ChangeMessageVisibility(ctx context.Context, in *sqs.ChangeMessageVisibilityInput, opts ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityOutput, error)
}

type Cola struct {
	sqs clienteSQS
	url string
}

type Mensaje struct {
	Cuerpo   string
	Recibido int
	recibo   string
}

func Nueva(ctx context.Context, region, url string) (*Cola, error) {
	if url == "" {
		return &Cola{}, nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("cargar credenciales de AWS: %w", err)
	}
	return &Cola{sqs: sqs.NewFromConfig(cfg), url: url}, nil
}

func (c *Cola) Habilitada() bool { return c != nil && c.url != "" }

func (c *Cola) URL() string { return c.url }

func (c *Cola) Enviar(ctx context.Context, cuerpo string) error {
	_, err := c.sqs.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(c.url), MessageBody: aws.String(cuerpo)})
	if err != nil {
		return fmt.Errorf("encolar en %s: %w", c.url, err)
	}
	return nil
}

func (c *Cola) EnviarMasTarde(ctx context.Context, cuerpo string, espera time.Duration) error {
	_, err := c.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:     aws.String(c.url),
		MessageBody:  aws.String(cuerpo),
		DelaySeconds: int32(min(espera, 15*time.Minute) / time.Second),
	})
	if err != nil {
		return fmt.Errorf("encolar en %s: %w", c.url, err)
	}
	return nil
}

func (c *Cola) Recibir(ctx context.Context, espera, visibilidad time.Duration) (*Mensaje, error) {
	salida, err := c.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:                    aws.String(c.url),
		MaxNumberOfMessages:         1,
		WaitTimeSeconds:             int32(espera / time.Second),
		VisibilityTimeout:           int32(visibilidad / time.Second),
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{types.MessageSystemAttributeNameApproximateReceiveCount},
	})
	if err != nil {
		return nil, fmt.Errorf("recibir de %s: %w", c.url, err)
	}
	if len(salida.Messages) == 0 {
		return nil, nil
	}
	m := salida.Messages[0]
	recibido, _ := strconv.Atoi(m.Attributes[string(types.MessageSystemAttributeNameApproximateReceiveCount)])
	return &Mensaje{Cuerpo: aws.ToString(m.Body), Recibido: recibido, recibo: aws.ToString(m.ReceiptHandle)}, nil
}

func (c *Cola) Borrar(ctx context.Context, m *Mensaje) error {
	_, err := c.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(c.url), ReceiptHandle: aws.String(m.recibo)})
	if err != nil {
		return fmt.Errorf("borrar el mensaje de %s: %w", c.url, err)
	}
	return nil
}

func (c *Cola) Ocultar(ctx context.Context, m *Mensaje, durante time.Duration) error {
	_, err := c.sqs.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(c.url),
		ReceiptHandle:     aws.String(m.recibo),
		VisibilityTimeout: int32(durante / time.Second),
	})
	if err != nil {
		return fmt.Errorf("cambiar la visibilidad en %s: %w", c.url, err)
	}
	return nil
}
