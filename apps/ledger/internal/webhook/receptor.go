package webhook

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ledger/internal/apierr"
)

type Receptor struct {
	pool    *pgxpool.Pool
	secreto string
}

func NuevoReceptor(pool *pgxpool.Pool, secreto string) *Receptor {
	return &Receptor{pool: pool, secreto: secreto}
}

type AvisoRecibido struct {
	ID            int64           `json:"id"`
	OrdenID       int64           `json:"orden_id"`
	Tipo          string          `json:"tipo"`
	Estado        string          `json:"estado"`
	MontoCentavos int64           `json:"monto_centavos"`
	Referencia    string          `json:"referencia,omitempty"`
	Cuerpo        json.RawMessage `json:"cuerpo"`
	FirmaValida   bool            `json:"firma_valida"`
	Origen        string          `json:"origen,omitempty"`
	RecibidoEn    time.Time       `json:"recibido_en"`
}

const columnasAviso = `id, orden_id, tipo, estado, monto_centavos, coalesce(referencia, ''),
	cuerpo, firma_valida, coalesce(origen, ''), recibido_en`

func (r *Receptor) Recibir(ctx context.Context, cuerpo []byte, firma, origen string) (*AvisoRecibido, error) {
	if len(cuerpo) == 0 {
		return nil, apierr.Invalido("el aviso viene vacio")
	}

	var a Aviso
	if err := json.Unmarshal(cuerpo, &a); err != nil {
		return nil, apierr.Invalido("el aviso no es JSON valido: %v", err)
	}

	valida := r.secreto != "" && hmac.Equal([]byte(firma), []byte(Firmar(cuerpo, r.secreto)))

	// SQS entrega al menos una vez: un evento_id repetido devuelve el aviso que ya estaba.
	// Sólo cuenta si la firma es válida; si no, cualquiera podría ocupar el evento_id de un
	// aviso legítimo antes de que llegue.
	var evento *string
	if valida && a.EventoID != "" {
		evento = &a.EventoID
	}

	guardado := &AvisoRecibido{}
	destino := []any{&guardado.ID, &guardado.OrdenID, &guardado.Tipo, &guardado.Estado,
		&guardado.MontoCentavos, &guardado.Referencia, &guardado.Cuerpo,
		&guardado.FirmaValida, &guardado.Origen, &guardado.RecibidoEn}

	err := r.pool.QueryRow(ctx, `
		INSERT INTO avisos_recibidos
			(orden_id, tipo, estado, monto_centavos, referencia, cuerpo, firma_valida, origen, evento_id)
		VALUES ($1, $2, $3, $4, nullif($5, ''), $6, $7, nullif($8, ''), $9)
		ON CONFLICT (evento_id) DO NOTHING
		RETURNING `+columnasAviso,
		a.OrdenID, a.Tipo, a.Estado, a.MontoCentavos, a.Referencia, cuerpo, valida, origen, evento).
		Scan(destino...)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.pool.QueryRow(ctx, `SELECT `+columnasAviso+` FROM avisos_recibidos WHERE evento_id = $1`, evento).
			Scan(destino...)
	}
	if err != nil {
		return nil, err
	}

	if !valida {
		motivo := "la firma no coincide"
		if r.secreto == "" {
			motivo = "este despliegue no tiene WEBHOOK_SECRET configurado"
		}
		return guardado, apierr.New(http.StatusUnauthorized, "firma_invalida",
			"%s: %s", CabeceraFirma, motivo)
	}
	return guardado, nil
}

func (r *Receptor) Listar(ctx context.Context, limite int) ([]AvisoRecibido, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	filas, err := r.pool.Query(ctx, `
		SELECT `+columnasAviso+`
		FROM avisos_recibidos ORDER BY recibido_en DESC, id DESC LIMIT $1`, limite)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	avisos := []AvisoRecibido{}
	for filas.Next() {
		var a AvisoRecibido
		if err := filas.Scan(&a.ID, &a.OrdenID, &a.Tipo, &a.Estado, &a.MontoCentavos,
			&a.Referencia, &a.Cuerpo, &a.FirmaValida, &a.Origen, &a.RecibidoEn); err != nil {
			return nil, err
		}
		avisos = append(avisos, a)
	}
	if err := filas.Err(); err != nil {
		return nil, fmt.Errorf("leer avisos recibidos: %w", err)
	}
	return avisos, nil
}
