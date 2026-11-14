package webhook

import (
	"context"
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
