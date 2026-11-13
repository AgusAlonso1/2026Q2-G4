package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

const CabeceraFirma = "X-Ledger-Signature"

const tiempoLimite = 10 * time.Second

type Aviso struct {
	EventoID      string `json:"evento_id,omitempty"`
	OrdenID       int64  `json:"orden_id"`
	Tipo          string `json:"tipo"`
	Estado        string `json:"estado"`
	Motivo        string `json:"motivo,omitempty"`
	OrigenID      *int64 `json:"origen_id,omitempty"`
	DestinoID     *int64 `json:"destino_id,omitempty"`
	MontoCentavos int64  `json:"monto_centavos"`
	Referencia    string `json:"referencia,omitempty"`
}

type Evento struct {
	Aviso
	Titulares *Titulares `json:"titulares,omitempty"`
}

type Titulares struct {
	Origen  string `json:"origen,omitempty"`
	Destino string `json:"destino,omitempty"`
}

type publicador interface {
	Publish(ctx context.Context, in *sns.PublishInput, opts ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type Notificador struct {
	pool  *pgxpool.Pool
	sns   publicador
	topic string

	pendientes sync.WaitGroup
}

func Nuevo(ctx context.Context, pool *pgxpool.Pool, region, topic string) (*Notificador, error) {
	if topic == "" {
		return &Notificador{}, nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("cargar credenciales de AWS: %w", err)
	}
	return &Notificador{pool: pool, sns: sns.NewFromConfig(cfg), topic: topic}, nil
}

func (n *Notificador) Habilitado() bool { return n != nil && n.topic != "" }

func (n *Notificador) Topic() string { return n.topic }

func (n *Notificador) Notificar(a Aviso) {
	if !n.Habilitado() {
		return
	}
	n.pendientes.Add(1)
	go func() {
		defer n.pendientes.Done()
		ctx, cancelar := context.WithTimeout(context.Background(), tiempoLimite)
		defer cancelar()
		if err := n.publicar(ctx, a); err != nil {
			slog.Error("publicar evento", "orden", a.OrdenID, "estado", a.Estado, "error", err)
		}
	}()
}

func (n *Notificador) Esperar(ctx context.Context) {
	if !n.Habilitado() {
		return
	}
	listo := make(chan struct{})
	go func() { n.pendientes.Wait(); close(listo) }()
	select {
	case <-listo:
	case <-ctx.Done():
	}
}

func (n *Notificador) publicar(ctx context.Context, a Aviso) error {
	a.EventoID = fmt.Sprintf("%d:%s", a.OrdenID, a.Estado)
	ev := Evento{Aviso: a, Titulares: n.titulares(ctx, a)}

	cuerpo, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = n.sns.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(n.topic),
		Message:  aws.String(string(cuerpo)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"tipo":   {DataType: aws.String("String"), StringValue: aws.String(a.Tipo)},
			"estado": {DataType: aws.String("String"), StringValue: aws.String(a.Estado)},
		},
	})
	if err != nil {
		return err
	}
	slog.Info("evento publicado", "evento", a.EventoID)
	return nil
}

func (n *Notificador) titulares(ctx context.Context, a Aviso) *Titulares {
	if n.pool == nil {
		return nil
	}
	var t Titulares
	err := n.pool.QueryRow(ctx, `
		SELECT coalesce((SELECT titular FROM cuentas WHERE id = $1), ''),
		       coalesce((SELECT titular FROM cuentas WHERE id = $2), '')`,
		a.OrigenID, a.DestinoID).Scan(&t.Origen, &t.Destino)
	if err != nil {
		slog.Warn("leer titulares del evento", "orden", a.OrdenID, "error", err)
		return nil
	}
	if t.Origen == "" && t.Destino == "" {
		return nil
	}
	return &t
}

func Firmar(cuerpo []byte, secreto string) string {
	mac := hmac.New(sha256.New, []byte(secreto))
	mac.Write(cuerpo)
	return hex.EncodeToString(mac.Sum(nil))
}
