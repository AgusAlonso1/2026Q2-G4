package webhook

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledger/db"
	"ledger/internal/pg"
)

func base(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DB_URL_TEST")
	if url == "" {
		t.Skip("sin DB_URL_TEST: se saltea la prueba de integracion")
	}
	ctx := context.Background()
	pool, err := pg.Abrir(ctx, url)
	if err != nil {
		t.Fatalf("abrir la base: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("limpiar el esquema: %v", err)
	}
	if _, err := db.Aplicar(ctx, pool); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	return pool
}

func TestElEventoLlevaLosSubDeLosTitulares(t *testing.T) {
	pool := base(t)
	ctx := context.Background()

	var origen, destino, sinTitular int64
	if err := pool.QueryRow(ctx, `
		WITH c AS (
			INSERT INTO cuentas (alias, tipo, titular) VALUES
				('ana', 'usuario', 'sub-ana'), ('beto', 'usuario', 'sub-beto'), ('caja', 'usuario', NULL)
			RETURNING id, alias)
		SELECT (SELECT id FROM c WHERE alias = 'ana'), (SELECT id FROM c WHERE alias = 'beto'),
		       (SELECT id FROM c WHERE alias = 'caja')`).Scan(&origen, &destino, &sinTitular); err != nil {
		t.Fatal(err)
	}

	falso := &snsFalso{}
	n := &Notificador{pool: pool, sns: falso, topic: "arn"}
	n.Notificar(Aviso{OrdenID: 1, Tipo: "transferencia", Estado: "liquidada", OrigenID: &origen, DestinoID: &destino})
	n.Notificar(Aviso{OrdenID: 2, Tipo: "ingreso", Estado: "atribuida", DestinoID: &destino})
	n.Notificar(Aviso{OrdenID: 3, Tipo: "ingreso", Estado: "atribuida", DestinoID: &sinTitular})
	n.Esperar(ctx)

	titulares := map[int64]*Titulares{}
	for _, in := range falso.recibidos {
		var ev Evento
		if err := json.Unmarshal([]byte(*in.Message), &ev); err != nil {
			t.Fatal(err)
		}
		titulares[ev.OrdenID] = ev.Titulares
	}
	if got := titulares[1]; got == nil || got.Origen != "sub-ana" || got.Destino != "sub-beto" {
		t.Fatalf("transferencia: titulares = %+v", got)
	}
	if got := titulares[2]; got == nil || got.Origen != "" || got.Destino != "sub-beto" {
		t.Fatalf("ingreso: titulares = %+v", got)
	}
	if got := titulares[3]; got != nil {
		t.Fatalf("una cuenta sin titular no lleva titulares: %+v", got)
	}
}

func TestElReceptorDescartaLosDuplicados(t *testing.T) {
	pool := base(t)
	ctx := context.Background()
	r := NuevoReceptor(pool, "s3cr3t")

	cuerpo := []byte(`{"evento_id":"7:liquidada","orden_id":7,"tipo":"transferencia","estado":"liquidada","monto_centavos":100}`)
	firma := Firmar(cuerpo, "s3cr3t")

	primero, err := r.Recibir(ctx, cuerpo, firma, "test")
	if err != nil {
		t.Fatal(err)
	}
	segundo, err := r.Recibir(ctx, cuerpo, firma, "test")
	if err != nil {
		t.Fatalf("un duplicado no es un error: %v", err)
	}
	if segundo.ID != primero.ID {
		t.Fatalf("el duplicado se guardó dos veces: %d y %d", primero.ID, segundo.ID)
	}

	// Una firma inválida no ocupa el evento_id: el aviso legítimo que llega después se guarda.
	falso := []byte(`{"evento_id":"8:liquidada","orden_id":8,"tipo":"transferencia","estado":"liquidada"}`)
	if _, err := r.Recibir(ctx, falso, "firma-falsa", "test"); err == nil {
		t.Fatal("una firma inválida tiene que dar error")
	}
	legitimo, err := r.Recibir(ctx, falso, Firmar(falso, "s3cr3t"), "test")
	if err != nil || !legitimo.FirmaValida {
		t.Fatalf("el aviso legítimo no se guardó: %+v, %v", legitimo, err)
	}

	// Sin evento_id (otro equipo con la versión vieja) no se deduplica.
	viejo := []byte(`{"orden_id":9,"tipo":"transferencia","estado":"liquidada"}`)
	a, _ := r.Recibir(ctx, viejo, Firmar(viejo, "s3cr3t"), "test")
	b, _ := r.Recibir(ctx, viejo, Firmar(viejo, "s3cr3t"), "test")
	if a.ID == b.ID {
		t.Fatal("sin evento_id cada aviso se guarda")
	}

	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM avisos_recibidos`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("se esperaban 5 avisos guardados (1 + 2 del 8 + 2 sin evento_id), hay %d", total)
	}
}
