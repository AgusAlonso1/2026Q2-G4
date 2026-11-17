package ledger_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledger/internal/apierr"
	"ledger/internal/cvu"
	"ledger/internal/ledger"
	"ledger/internal/mock"
	"ledger/internal/objetos"
	"ledger/internal/proveedor"
)

func entornoConBanco(t *testing.T) (*ledger.Servicio, *pgxpool.Pool, string) {
	t.Helper()
	_, _, pool := entorno(t)
	ctx := context.Background()

	mux := http.NewServeMux()
	mock.Nuevo(pool, mock.Opciones{}).Registrar(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	banco := srv.URL + mock.Prefijo

	alm, err := objetos.Desde(ctx, "", "", t.TempDir())
	if err != nil {
		t.Fatalf("almacen: %v", err)
	}
	svc, err := ledger.Nuevo(ctx, pool, proveedor.Nuevo(banco, 5*time.Second), alm, nil, time.Minute, banco)
	if err != nil {
		t.Fatalf("servicio: %v", err)
	}
	return svc.ConEntidad("4233"), pool, banco
}

func conciliar(t *testing.T, svc *ledger.Servicio, pool *pgxpool.Pool) ledger.ResumenConciliacion {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE conciliacion SET ultima_consulta = NULL WHERE id = 1`); err != nil {
		t.Fatalf("reiniciar la conciliacion: %v", err)
	}
	res, err := svc.Conciliar(ctx)
	if err != nil {
		t.Fatalf("conciliar: %v", err)
	}
	return res
}

func TestRetiroAUnCVUSeAcreditaEnLaCuentaDeDestino(t *testing.T) {
	svc, pool, _ := entornoConBanco(t)
	ctx := context.Background()

	ana, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "ana", Tipo: "usuario", SaldoInicialCentavos: 100000})
	if err != nil {
		t.Fatalf("crear ana: %v", err)
	}
	beto, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "beto", Tipo: "usuario"})
	if err != nil {
		t.Fatalf("crear beto: %v", err)
	}

	retiro, err := svc.Egresar(ctx, ledger.EntradaEgreso{
		IdempotencyKey: "retiro-1", OrigenID: ana.ID, CBUDestino: svc.CVU(beto.ID), MontoCentavos: 25000})
	if err != nil {
		t.Fatalf("egresar: %v", err)
	}
	if retiro.Estado != ledger.EstadoAcreditada {
		t.Fatalf("el retiro quedo %s (%s)", retiro.Estado, retiro.Motivo)
	}
	if retiro.NombreDestino != svc.AliasPublico("beto") {
		t.Errorf("nombre_destino del retiro = %q", retiro.NombreDestino)
	}
	enviados, err := svc.Movimientos(ctx, ana.ID, 1, 0)
	if err != nil || len(enviados) != 1 || enviados[0].NombreDestino != svc.AliasPublico("beto") {
		t.Errorf("el movimiento de ana no dice a quien le transfirio: %+v, %v", enviados, err)
	}

	res := conciliar(t, svc, pool)
	if res.Movimientos != 1 || res.Atribuidos != 1 {
		t.Fatalf("conciliacion = %+v, se esperaba un movimiento atribuido", res)
	}

	saldo, err := svc.Saldo(ctx, beto.ID)
	if err != nil {
		t.Fatalf("saldo: %v", err)
	}
	if saldo.VigenteCentavos != 25000 || !saldo.Coincide {
		t.Fatalf("saldo de beto = %+v", saldo)
	}

	movs, err := svc.Movimientos(ctx, beto.ID, 10, 0)
	if err != nil || len(movs) != 1 {
		t.Fatalf("movimientos de beto = %v, %v", movs, err)
	}
	if movs[0].NombreOrigen != svc.AliasPublico("ana") {
		t.Errorf("nombre_origen = %q", movs[0].NombreOrigen)
	}
	ingreso, err := svc.Consultar(ctx, ledger.TipoIngreso, movs[0].OrdenID)
	if err != nil {
		t.Fatalf("leer el ingreso: %v", err)
	}
	if ingreso.Estado != ledger.EstadoAtribuida || ingreso.CBUOrigen != svc.CVU(ana.ID) {
		t.Errorf("ingreso = %+v", ingreso)
	}

	if res := conciliar(t, svc, pool); res.Movimientos != 0 {
		t.Errorf("la segunda conciliacion volvio a traer %d movimientos", res.Movimientos)
	}
}

func TestRetiroAUnCVUDeOtraEntidadSoloLoVeEsaEntidad(t *testing.T) {
	svc, pool, banco := entornoConBanco(t)
	ctx := context.Background()

	ana, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "ana", Tipo: "usuario", SaldoInicialCentavos: 100000})
	if err != nil {
		t.Fatalf("crear ana: %v", err)
	}
	ajeno := cvu.Armar("0042", 7)
	retiro, err := svc.Egresar(ctx, ledger.EntradaEgreso{
		IdempotencyKey: "retiro-1", OrigenID: ana.ID, CBUDestino: ajeno, MontoCentavos: 30000})
	if err != nil || retiro.Estado != ledger.EstadoAcreditada {
		t.Fatalf("egresar: %+v, %v", retiro, err)
	}

	if res := conciliar(t, svc, pool); res.Movimientos != 0 {
		t.Fatalf("esta entidad vio %d movimientos de otra", res.Movimientos)
	}

	otra := proveedor.Nuevo(banco, 5*time.Second)
	otra.ConEntidad("0042")
	movs, _, _, err := otra.Movimientos(ctx, "")
	if err != nil {
		t.Fatalf("movimientos de la otra entidad: %v", err)
	}
	if len(movs) != 1 || movs[0].CVUDestino != ajeno || movs[0].NombreOrigen != svc.AliasPublico("ana") ||
		movs[0].CBUOrigen != svc.CVU(ana.ID) || movs[0].MontoCentavos != 30000 {
		t.Fatalf("movimientos de la otra entidad = %+v", movs)
	}
}

func TestRetiroAUnCVUSinCuentaQuedaSinAtribuir(t *testing.T) {
	svc, pool, _ := entornoConBanco(t)
	ctx := context.Background()

	ana, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "ana", Tipo: "usuario", SaldoInicialCentavos: 100000})
	if err != nil {
		t.Fatalf("crear ana: %v", err)
	}
	if _, err := svc.Egresar(ctx, ledger.EntradaEgreso{
		IdempotencyKey: "retiro-1", OrigenID: ana.ID, CBUDestino: svc.CVU(9999), MontoCentavos: 10000}); err != nil {
		t.Fatalf("egresar: %v", err)
	}

	if res := conciliar(t, svc, pool); res.NoAtribuidos != 1 {
		t.Fatalf("conciliacion = %+v, se esperaba un movimiento sin atribuir", res)
	}
}

func TestRetiroRechazaUnaCBUMalTipeadaOElCVUPropio(t *testing.T) {
	svc, _, _ := entornoConBanco(t)
	ctx := context.Background()

	ana, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "ana", Tipo: "usuario", SaldoInicialCentavos: 100000})
	if err != nil {
		t.Fatalf("crear ana: %v", err)
	}
	mal := []byte(svc.CVU(2))
	mal[20] = '0' + (mal[20]-'0'+1)%10

	for i, destino := range []string{string(mal), svc.CVU(ana.ID), "123"} {
		_, err := svc.Egresar(ctx, ledger.EntradaEgreso{
			IdempotencyKey: "retiro-" + destino, OrigenID: ana.ID, CBUDestino: destino, MontoCentavos: 1000})
		var e *apierr.Error
		if !errors.As(err, &e) || e.Estado != http.StatusBadRequest {
			t.Errorf("caso %d (%s): se esperaba un 400, vino %v", i, destino, err)
		}
	}
}

func TestLosAliasSePublicanConElPrefijoDeLaBilleteraYResuelvenElDestino(t *testing.T) {
	svc, _, banco := entornoConBanco(t)
	ctx := context.Background()

	ana, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "ana", Tipo: "usuario", SaldoInicialCentavos: 100000})
	if err != nil {
		t.Fatalf("crear ana: %v", err)
	}
	beto, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "beto", Tipo: "usuario"})
	if err != nil {
		t.Fatalf("crear beto: %v", err)
	}
	if err := svc.PublicarAlias(ctx); err != nil {
		t.Fatalf("publicar alias: %v", err)
	}

	otra := proveedor.Nuevo(banco, 5*time.Second)
	otra.ConEntidad("0042")
	carla := cvu.Armar("0042", 3)
	if _, err := otra.PublicarAlias(ctx, "otra", []proveedor.CuentaAlias{{Alias: "otra.carla", CVU: carla}}); err != nil {
		t.Fatalf("publicar alias de la otra billetera: %v", err)
	}
	if _, err := otra.PublicarAlias(ctx, svc.Billetera(), []proveedor.CuentaAlias{
		{Alias: svc.AliasPublico("ana"), CVU: cvu.Armar("0042", 4)}}); err == nil {
		t.Error("otra entidad pudo publicar con el nombre de billetera de esta")
	}
	misma := proveedor.Nuevo(banco, 5*time.Second)
	misma.ConEntidad(svc.Entidad())
	if _, err := misma.PublicarAlias(ctx, "intrusa", []proveedor.CuentaAlias{
		{Alias: "intrusa.eva", CVU: svc.CVU(99)}}); err == nil {
		t.Error("otra billetera pudo publicar con la entidad de esta")
	}

	d, err := svc.BuscarDestino(ctx, "@otra.carla", ana.ID)
	if err != nil || d.CBU != carla || d.Alias != "otra.carla" || d.Billetera != "otra" || d.MismaEntidad {
		t.Fatalf("destino otra.carla = %+v, %v", d, err)
	}
	for _, clave := range []string{"beto", svc.AliasPublico("beto"), svc.CVU(beto.ID)} {
		d, err = svc.BuscarDestino(ctx, clave, ana.ID)
		if err != nil || d.CBU != svc.CVU(beto.ID) || d.Alias != svc.AliasPublico("beto") || !d.MismaEntidad {
			t.Fatalf("destino %s = %+v, %v", clave, d, err)
		}
	}
	d, err = svc.BuscarDestino(ctx, carla, ana.ID)
	if err != nil || d.Alias != "otra.carla" || d.Billetera != "otra" {
		t.Fatalf("destino por CVU = %+v, %v", d, err)
	}
	if b, err := otra.BuscarCuenta(ctx, svc.AliasPublico("beto")); err != nil || b.CVU != svc.CVU(beto.ID) || b.Billetera != svc.Billetera() {
		t.Fatalf("la otra entidad no encuentra a beto: %+v, %v", b, err)
	}

	for clave, estado := range map[string]int{
		"ana": http.StatusBadRequest, "nadie": http.StatusNotFound, "otra.nadie": http.StatusNotFound,
	} {
		_, err := svc.BuscarDestino(ctx, clave, ana.ID)
		var e *apierr.Error
		if !errors.As(err, &e) || e.Estado != estado {
			t.Errorf("BuscarDestino(%s): se esperaba %d, vino %v", clave, estado, err)
		}
	}

	retiro, err := svc.Egresar(ctx, ledger.EntradaEgreso{
		IdempotencyKey: "retiro-1", OrigenID: ana.ID, AliasDestino: "otra.carla", MontoCentavos: 20000})
	if err != nil || retiro.Estado != ledger.EstadoAcreditada || retiro.CBUDestino != carla || retiro.NombreDestino != "otra.carla" {
		t.Fatalf("retiro por alias = %+v, %v", retiro, err)
	}
}

func TestUnaSolicitudAceptadaLePagaAlQueLaPidio(t *testing.T) {
	svc, pool, _ := entornoConBanco(t)
	ctx := context.Background()

	ana, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "ana", Tipo: "usuario"})
	if err != nil {
		t.Fatalf("crear ana: %v", err)
	}
	beto, err := svc.CrearCuenta(ctx, ledger.EntradaCuenta{Alias: "beto", Tipo: "usuario", SaldoInicialCentavos: 50000})
	if err != nil {
		t.Fatalf("crear beto: %v", err)
	}

	pedido := ledger.EntradaSolicitud{IdempotencyKey: "k1", DestinoID: ana.ID, De: "beto", MontoCentavos: 30000}
	x, err := svc.PedirDinero(ctx, pedido)
	if err != nil || x.Estado != ledger.SolicitudPendiente || x.AliasCobrador != svc.AliasPublico("ana") {
		t.Fatalf("pedir = %+v, %v", x, err)
	}
	if otra, err := svc.PedirDinero(ctx, pedido); err != nil || otra.Referencia != x.Referencia {
		t.Fatalf("el reintento creo otra solicitud: %+v, %v", otra, err)
	}

	recibidas, err := svc.Solicitudes(ctx, beto.ID, "recibidas")
	if err != nil || len(recibidas) != 1 || recibidas[0].Referencia != x.Referencia {
		t.Fatalf("recibidas de beto = %+v, %v", recibidas, err)
	}
	var e *apierr.Error
	if _, err := svc.AceptarSolicitud(ctx, ana.ID, x.Referencia); !errors.As(err, &e) || e.Estado != http.StatusForbidden {
		t.Fatalf("ana pudo aceptar su propia solicitud: %v", err)
	}

	pago, err := svc.AceptarSolicitud(ctx, beto.ID, x.Referencia)
	if err != nil || pago.Solicitud.Estado != ledger.SolicitudAceptada || pago.Orden.Estado != ledger.EstadoAcreditada {
		t.Fatalf("aceptar = %+v, %v", pago, err)
	}
	if _, err := svc.AceptarSolicitud(ctx, beto.ID, x.Referencia); !errors.As(err, &e) || e.Estado != http.StatusConflict {
		t.Fatalf("se pudo aceptar dos veces: %v", err)
	}

	conciliar(t, svc, pool)
	saldo, err := svc.Saldo(ctx, ana.ID)
	if err != nil || saldo.VigenteCentavos != 30000 {
		t.Fatalf("saldo de ana = %+v, %v", saldo, err)
	}

	sinFondos, err := svc.PedirDinero(ctx, ledger.EntradaSolicitud{IdempotencyKey: "k2", DestinoID: ana.ID, De: "beto", MontoCentavos: 90000})
	if err != nil {
		t.Fatalf("pedir de nuevo: %v", err)
	}
	if _, err := svc.AceptarSolicitud(ctx, beto.ID, sinFondos.Referencia); !errors.As(err, &e) || e.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se acepto sin fondos: %v", err)
	}
	rechazada, err := svc.RechazarSolicitud(ctx, beto.ID, sinFondos.Referencia)
	if err != nil || rechazada.Estado != ledger.SolicitudRechazada {
		t.Fatalf("rechazar = %+v, %v", rechazada, err)
	}
	enviadas, err := svc.Solicitudes(ctx, ana.ID, "enviadas")
	if err != nil || len(enviadas) != 2 {
		t.Fatalf("enviadas de ana = %+v, %v", enviadas, err)
	}
}
