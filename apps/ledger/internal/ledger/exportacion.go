package ledger

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"ledger/internal/apierr"
	"ledger/internal/cola"
	"ledger/internal/objetos"
	"ledger/internal/pg"
)

const (
	ExportPendiente = "pendiente"
	ExportLista     = "lista"
	ExportFallida   = "fallida"
)

const prefijoExportaciones = "exports/"

const maxPeriodosExportables = 24

type EntradaExportacion struct {
	CuentaID int64  `json:"cuenta_id,omitempty"`
	Desde    string `json:"desde"`
	Hasta    string `json:"hasta"`
}

type Exportacion struct {
	ID              int64      `json:"id"`
	CuentaID        *int64     `json:"cuenta_id,omitempty"`
	Desde           string     `json:"desde"`
	Hasta           string     `json:"hasta"`
	Estado          string     `json:"estado"`
	Objeto          string     `json:"objeto,omitempty"`
	URI             string     `json:"uri,omitempty"`
	Filas           *int64     `json:"filas,omitempty"`
	PeriodosBase    *int       `json:"periodos_base,omitempty"`
	PeriodosArchivo *int       `json:"periodos_archivo,omitempty"`
	Motivo          string     `json:"motivo,omitempty"`
	SolicitadaEn    time.Time  `json:"solicitada_en"`
	ResueltaEn      *time.Time `json:"resuelta_en,omitempty"`
}

func (s *Servicio) Exportar(ctx context.Context, e EntradaExportacion) (*Exportacion, error) {
	desde, err := time.Parse("2006-01", e.Desde)
	if err != nil {
		return nil, apierr.Invalido("desde tiene que tener formato AAAA-MM")
	}
	hasta, err := time.Parse("2006-01", e.Hasta)
	if err != nil {
		return nil, apierr.Invalido("hasta tiene que tener formato AAAA-MM")
	}
	desde, hasta = pg.InicioDeMes(desde), pg.InicioDeMes(hasta)
	if hasta.Before(desde) {
		return nil, apierr.Invalido("hasta no puede ser anterior a desde")
	}

	periodos := []time.Time{}
	for p := desde; !p.After(hasta); p = p.AddDate(0, 1, 0) {
		periodos = append(periodos, p)
	}
	if len(periodos) > maxPeriodosExportables {
		return nil, apierr.Invalido("el rango es de %d meses y el maximo es %d: pedilo en tramos",
			len(periodos), maxPeriodosExportables)
	}

	var cuenta *int64
	if e.CuentaID != 0 {
		var existe bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM cuentas WHERE id = $1)`, e.CuentaID).Scan(&existe); err != nil {
			return nil, err
		}
		if !existe {
			return nil, apierr.NoEncontrado("no existe la cuenta %d", e.CuentaID)
		}
		cuenta = &e.CuentaID
	}

	var id int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO solicitudes_exportacion (cuenta_id, desde, hasta, estado)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		cuenta, desde, hasta, ExportPendiente).Scan(&id); err != nil {
		return nil, err
	}

	s.encolarExportacion(ctx, id)
	return s.VerExportacion(ctx, id)
}

const (
	vigenciaDescarga   = 15 * time.Minute
	esperaExportacion  = time.Minute
	esperaCola         = 20 * time.Second
	barridoCola        = 5 * time.Minute
	esperaArchivado    = 5 * time.Minute
	toleranciaArchivo  = 24 * time.Hour
	tiempoLimiteAviso  = 10 * time.Second
	tiempoLimiteCola   = 5 * time.Second
	maxIntentosSoltado = 10
)

var errExportacionAjena = errors.New("otra tarea tomo la exportacion")

type reclamo struct {
	id         int64
	token      string
	intentos   int
	cuenta     *int64
	periodos   []time.Time
	solicitada time.Time
}

func (s *Servicio) plazoReclamo() time.Duration { return s.visibilidadCola / 2 }

func (s *Servicio) umbralBarrido() time.Duration { return max(barridoCola, 3*s.visibilidadCola) }

func (s *Servicio) despertarExportaciones() {
	select {
	case s.exportaciones <- struct{}{}:
	default:
	}
}

func (s *Servicio) encolarExportacion(ctx context.Context, id int64) {
	if !s.cola.Habilitada() {
		s.despertarExportaciones()
		return
	}
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(ctx), tiempoLimiteCola)
	defer cancelar()
	if err := s.cola.Enviar(ctx, strconv.FormatInt(id, 10)); err != nil {
		slog.Warn("encolar exportacion: la retoma el barrido", "exportacion", id, "error", err)
	}
}

func (s *Servicio) AtenderExportaciones(ctx context.Context, consumidores int) {
	consumidores = max(consumidores, 1)
	var grupo sync.WaitGroup
	for range consumidores {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			if s.cola.Habilitada() {
				s.consumirCola(ctx)
			} else {
				s.consumirTabla(ctx)
			}
		}()
	}
	grupo.Wait()
}

func (s *Servicio) consumirTabla(ctx context.Context) {
	tic := time.NewTicker(esperaExportacion)
	defer tic.Stop()
	for {
		for {
			hubo, err := s.ResolverExportacionPendiente(ctx)
			manejado := esIntentoFallido(err)
			if err != nil && manejado {
				slog.Warn("exportacion fallida, se reintenta", "error", err)
			} else if err != nil {
				slog.Error("resolver exportacion", "error", err)
			}
			if !hubo || (err != nil && !manejado) || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tic.C:
		case <-s.exportaciones:
		}
	}
}

func (s *Servicio) BarrerExportaciones(ctx context.Context) {
	tic := time.NewTicker(barridoCola)
	defer tic.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tic.C:
			if n, err := s.ReencolarExportaciones(ctx); err != nil {
				slog.Error("reencolar exportaciones", "error", err)
			} else if n > 0 {
				slog.Info("exportaciones reencoladas", "cantidad", n)
			}
		}
	}
}

func (s *Servicio) consumirCola(ctx context.Context) {
	for ctx.Err() == nil {
		hubo, err := s.AtenderMensajeDeExportacion(ctx)
		if err != nil && esIntentoFallido(err) {
			slog.Warn("exportacion fallida, se reintenta", "error", err)
			continue
		}
		if err != nil && ctx.Err() == nil {
			slog.Error("atender la cola de exportaciones", "recibio", hubo, "error", err)
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (s *Servicio) AtenderMensajeDeExportacion(ctx context.Context) (bool, error) {
	m, err := s.cola.Recibir(ctx, esperaCola, s.visibilidadCola)
	if err != nil || m == nil {
		return false, err
	}
	id, err := strconv.ParseInt(m.Cuerpo, 10, 64)
	if err != nil {
		return true, errors.Join(fmt.Errorf("mensaje de exportacion invalido %q", m.Cuerpo), s.cola.Borrar(ctx, m))
	}

	r, err := s.reclamar(ctx, id)
	if err != nil {
		err = fmt.Errorf("reclamar la exportacion %d (recepcion %d): %w", id, m.Recibido, err)
		if ctx.Err() != nil {
			return true, errors.Join(err, s.devolverMensaje(ctx, m))
		}
		return true, err
	}
	if r == nil {
		espera, err := s.esperaPendiente(ctx, id)
		if err != nil {
			return true, err
		}
		if espera > 0 {
			return true, s.cola.Ocultar(ctx, m, espera)
		}
		return true, s.cola.Borrar(ctx, m)
	}

	res, err := s.intentarExportacion(ctx, r, m)
	switch s.desenlace(ctx, r, err) {
	case exportacionLista:
		errBorrar := s.borrarMensaje(ctx, m)
		s.avisarExportacionLista(ctx, res)
		return true, errBorrar
	case exportacionAjena:
		return true, s.borrarMensaje(ctx, m)
	case exportacionCancelada:
		return true, errors.Join(s.soltarExportacion(ctx, r, nil, true, 0), s.reencolarMensaje(ctx, m, 0))
	case exportacionEnEspera:
		return true, errors.Join(s.soltarExportacion(ctx, r, err, true, esperaArchivado), s.reencolarMensaje(ctx, m, esperaArchivado))
	case exportacionAgotada:
		return true, errors.Join(s.errorDeIntento(r, err), s.agotarExportacion(ctx, r, err), s.borrarMensaje(ctx, m))
	default:
		espera := s.esperaReintento(r.intentos)
		return true, errors.Join(s.errorDeIntento(r, err), s.soltarExportacion(ctx, r, err, false, espera), s.ocultarMensaje(ctx, m, espera))
	}
}

type desenlaceExportacion int

const (
	exportacionLista desenlaceExportacion = iota
	exportacionAjena
	exportacionCancelada
	exportacionEnEspera
	exportacionAgotada
	exportacionReintento
)

var errIntentosAgotados = errors.New("agoto los reintentos")

func (s *Servicio) intentarExportacion(ctx context.Context, r *reclamo, m *cola.Mensaje) (*Exportacion, error) {
	if r.intentos > s.intentosExportacion {
		return nil, errIntentosAgotados
	}
	return s.armarExportacion(ctx, r, m)
}

func (s *Servicio) desenlace(ctx context.Context, r *reclamo, err error) desenlaceExportacion {
	switch {
	case err == nil:
		return exportacionLista
	case errors.Is(err, errExportacionAjena):
		return exportacionAjena
	case errors.Is(err, errIntentosAgotados):
		return exportacionAgotada
	case ctx.Err() != nil:
		return exportacionCancelada
	case s.esperarArchivado(r, err):
		return exportacionEnEspera
	case r.intentos >= s.intentosExportacion:
		return exportacionAgotada
	default:
		return exportacionReintento
	}
}

type intentoFallido struct{ error }

func (e intentoFallido) Unwrap() error { return e.error }

func (s *Servicio) errorDeIntento(r *reclamo, err error) error {
	if errors.Is(err, errIntentosAgotados) {
		return nil
	}
	return intentoFallido{fmt.Errorf("exportacion %d, intento %d: %w", r.id, r.intentos, err)}
}

func esIntentoFallido(err error) bool {
	if err == nil {
		return false
	}
	if varios, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range varios.Unwrap() {
			if !esIntentoFallido(e) {
				return false
			}
		}
		return true
	}
	_, ok := err.(intentoFallido)
	return ok
}

func (s *Servicio) esperaPendiente(ctx context.Context, id int64) (time.Duration, error) {
	var segundos *float64
	err := s.pool.QueryRow(ctx, `
		SELECT greatest(
		         extract(epoch FROM reintentar_desde - now()),
		         CASE WHEN tomada_en > now() - make_interval(secs => $3) THEN $4 ELSE 0 END)
		FROM solicitudes_exportacion
		WHERE id = $1 AND estado = $2`,
		id, ExportPendiente, s.plazoReclamo().Seconds(), s.visibilidadCola.Seconds()).Scan(&segundos)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (segundos == nil || *segundos <= 0)) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return time.Duration(*segundos*float64(time.Second)) + time.Second, nil
}

func (s *Servicio) reencolarMensaje(ctx context.Context, m *cola.Mensaje, espera time.Duration) error {
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(ctx), tiempoLimiteCola)
	defer cancelar()
	if err := s.cola.EnviarMasTarde(ctx, m.Cuerpo, espera); err != nil {
		return errors.Join(err, s.cola.Ocultar(ctx, m, espera))
	}
	return s.cola.Borrar(ctx, m)
}

func (s *Servicio) devolverMensaje(ctx context.Context, m *cola.Mensaje) error {
	return s.ocultarMensaje(ctx, m, 0)
}

func (s *Servicio) ocultarMensaje(ctx context.Context, m *cola.Mensaje, durante time.Duration) error {
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(ctx), tiempoLimiteCola)
	defer cancelar()
	return s.cola.Ocultar(ctx, m, durante)
}

func (s *Servicio) borrarMensaje(ctx context.Context, m *cola.Mensaje) error {
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(ctx), tiempoLimiteCola)
	defer cancelar()
	return s.cola.Borrar(ctx, m)
}

func (s *Servicio) esperaReintento(intentos int) time.Duration {
	return min(time.Duration(1<<min(intentos-1, maxIntentosSoltado))*time.Minute, s.umbralBarrido()-time.Minute)
}

func (s *Servicio) esperarArchivado(r *reclamo, err error) bool {
	var e *apierr.Error
	return errors.As(err, &e) && e.Estado == http.StatusServiceUnavailable && time.Since(r.solicitada) < toleranciaArchivo
}

func (s *Servicio) ReencolarExportaciones(ctx context.Context) (int, error) {
	filas, err := s.pool.Query(ctx, `
		UPDATE solicitudes_exportacion SET encolada_en = now()
		WHERE id IN (
			SELECT id FROM solicitudes_exportacion
			WHERE estado = $1
			  AND greatest(solicitada_en, coalesce(encolada_en, solicitada_en), coalesce(tomada_en, solicitada_en))
			      < now() - make_interval(secs => $2)
			  AND (reintentar_desde IS NULL OR reintentar_desde <= now())
			ORDER BY id
			LIMIT 100
			FOR UPDATE SKIP LOCKED)
		RETURNING id`, ExportPendiente, s.umbralBarrido().Seconds())
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(filas, pgx.RowTo[int64])
	if err != nil {
		return 0, err
	}
	var errs []error
	var fallidos []int64
	for _, id := range ids {
		if err := s.cola.Enviar(ctx, strconv.FormatInt(id, 10)); err != nil {
			errs = append(errs, err)
			fallidos = append(fallidos, id)
		}
	}
	if len(fallidos) > 0 {
		_, err := s.pool.Exec(context.WithoutCancel(ctx),
			`UPDATE solicitudes_exportacion SET encolada_en = NULL WHERE id = ANY($1)`, fallidos)
		errs = append(errs, err)
	}
	return len(ids) - len(fallidos), errors.Join(errs...)
}

const columnasReclamo = `id, tomada_por, intentos, cuenta_id, desde, hasta, solicitada_en`

func (s *Servicio) ResolverExportacionPendiente(ctx context.Context) (bool, error) {
	r, err := s.escanearReclamo(s.pool.QueryRow(ctx, `
		UPDATE solicitudes_exportacion SET tomada_por = $3, tomada_en = now(), intentos = intentos + 1
		WHERE id = (
			SELECT id FROM solicitudes_exportacion
			WHERE estado = $1
			  AND (tomada_en IS NULL OR tomada_en < now() - make_interval(secs => $2))
			  AND (reintentar_desde IS NULL OR reintentar_desde <= now())
			ORDER BY id
			LIMIT 1
			FOR UPDATE SKIP LOCKED)
		RETURNING `+columnasReclamo,
		ExportPendiente, s.plazoReclamo().Seconds(), nuevoToken()))
	if r == nil || err != nil {
		return false, err
	}

	res, err := s.intentarExportacion(ctx, r, nil)
	switch s.desenlace(ctx, r, err) {
	case exportacionLista:
		s.avisarExportacionLista(ctx, res)
		return true, nil
	case exportacionAjena:
		return true, nil
	case exportacionCancelada:
		return true, s.soltarExportacion(ctx, r, nil, true, 0)
	case exportacionEnEspera:
		return true, s.soltarExportacion(ctx, r, err, true, esperaArchivado)
	case exportacionAgotada:
		return true, errors.Join(s.errorDeIntento(r, err), s.agotarExportacion(ctx, r, err))
	default:
		return true, errors.Join(s.errorDeIntento(r, err), s.soltarExportacion(ctx, r, err, false, s.esperaReintento(r.intentos)))
	}
}

func (s *Servicio) reclamar(ctx context.Context, id int64) (*reclamo, error) {
	return s.escanearReclamo(s.pool.QueryRow(ctx, `
		UPDATE solicitudes_exportacion SET tomada_por = $3, tomada_en = now(), intentos = intentos + 1
		WHERE id = $1 AND estado = $2
		  AND (tomada_en IS NULL OR tomada_en < now() - make_interval(secs => $4))
		  AND (reintentar_desde IS NULL OR reintentar_desde <= now())
		RETURNING `+columnasReclamo,
		id, ExportPendiente, nuevoToken(), s.plazoReclamo().Seconds()))
}

func (s *Servicio) escanearReclamo(fila pgx.Row) (*reclamo, error) {
	var r reclamo
	var desde, hasta time.Time
	err := fila.Scan(&r.id, &r.token, &r.intentos, &r.cuenta, &desde, &hasta, &r.solicitada)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for p := desde; !p.After(hasta); p = p.AddDate(0, 1, 0) {
		r.periodos = append(r.periodos, p)
	}
	return &r, nil
}

func nuevoToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Servicio) armarExportacion(ctx context.Context, r *reclamo, m *cola.Mensaje) (*Exportacion, error) {
	trabajo, cancelar := context.WithCancelCause(ctx)
	latido := make(chan struct{})
	defer func() {
		cancelar(nil)
		<-latido
	}()
	go func() {
		defer close(latido)
		tic := time.NewTicker(s.visibilidadCola / 4)
		defer tic.Stop()
		for {
			select {
			case <-trabajo.Done():
				return
			case <-tic.C:
				if m != nil {
					if err := s.cola.Ocultar(trabajo, m, s.visibilidadCola); err != nil && trabajo.Err() == nil {
						slog.Warn("extender la visibilidad de la exportacion", "exportacion", r.id, "error", err)
					}
				}
				base, cancelarBase := context.WithTimeout(trabajo, tiempoLimiteCola)
				tag, err := s.pool.Exec(base, `
					UPDATE solicitudes_exportacion SET tomada_en = now() WHERE id = $1 AND tomada_por = $2`,
					r.id, r.token)
				cancelarBase()
				if trabajo.Err() != nil {
					return
				}
				if err == nil && tag.RowsAffected() == 0 {
					cancelar(errExportacionAjena)
					return
				}
				if err != nil {
					slog.Warn("renovar la exportacion tomada", "exportacion", r.id, "error", err)
				}
			}
		}
	}()
	res, err := s.resolverExportacion(trabajo, r)
	if err != nil && ctx.Err() == nil && errors.Is(context.Cause(trabajo), errExportacionAjena) {
		return nil, errExportacionAjena
	}
	return res, err
}

func (s *Servicio) soltarExportacion(ctx context.Context, r *reclamo, causa error, devolverIntento bool, espera time.Duration) error {
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelar()
	var motivo *string
	if causa != nil {
		m := causa.Error()
		motivo = &m
	}
	devolver := 0
	if devolverIntento {
		devolver = 1
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE solicitudes_exportacion
		SET tomada_por = NULL, tomada_en = NULL, encolada_en = now(),
		    reintentar_desde = now() + make_interval(secs => $6),
		    intentos = intentos - $3, motivo = coalesce($4, motivo)
		WHERE id = $1 AND tomada_por = $2 AND estado = $5`,
		r.id, r.token, devolver, motivo, ExportPendiente, espera.Seconds())
	return err
}

func (s *Servicio) agotarExportacion(ctx context.Context, r *reclamo, causa error) error {
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelar()
	var motivo *string
	if causa != nil && !errors.Is(causa, errIntentosAgotados) {
		m := causa.Error()
		motivo = &m
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE solicitudes_exportacion
		SET estado = $3, motivo = coalesce($4, motivo, 'agoto los reintentos'), resuelta_en = now(), tomada_por = NULL
		WHERE id = $1 AND tomada_por = $2 AND estado = $5`,
		r.id, r.token, ExportFallida, motivo, ExportPendiente)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	var errDLQ error
	if s.dlq.Habilitada() {
		if err := s.dlq.Enviar(ctx, strconv.FormatInt(r.id, 10)); err != nil {
			errDLQ = fmt.Errorf("mandar la exportacion %d a la DLQ: %w", r.id, err)
		}
	}
	s.avisarExportacionFallida(ctx, r)
	return errDLQ
}

func (s *Servicio) pantallaExportaciones() string {
	return strings.TrimSuffix(s.appURL, "/") + "/#/exportaciones"
}

func (s *Servicio) avisarExportacionLista(ctx context.Context, e *Exportacion) {
	mensaje := fmt.Sprintf("Tu exportación N.º %d (%s a %s, %d movimientos) está lista.\n\n"+
		"Para descargarla, entrá a la app: %s\n\nEl archivo queda disponible unos días.",
		e.ID, e.Desde, e.Hasta, valor(e.Filas), s.pantallaExportaciones())
	s.avisarTitular(ctx, e.ID, e.CuentaID, "lista", "Tu exportación está lista", mensaje)
}

func (s *Servicio) avisarExportacionFallida(ctx context.Context, r *reclamo) {
	desde, hasta := r.periodos[0].Format("2006-01"), r.periodos[len(r.periodos)-1].Format("2006-01")
	mensaje := fmt.Sprintf("No pudimos armar tu exportación N.º %d (%s a %s): la intentamos varias veces y "+
		"siguió fallando.\n\nPodés pedirla de nuevo desde la app: %s",
		r.id, desde, hasta, s.pantallaExportaciones())
	s.avisarTitular(ctx, r.id, r.cuenta, "fallida", "No pudimos armar tu exportación", mensaje)
}

func (s *Servicio) avisarTitular(ctx context.Context, id int64, cuenta *int64, estado, asunto, mensaje string) {
	if !s.avisos.Habilitado() || cuenta == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(ctx), tiempoLimiteAviso)
	defer cancelar()
	var titular *string
	if err := s.pool.QueryRow(ctx, `SELECT titular FROM cuentas WHERE id = $1`, *cuenta).Scan(&titular); err != nil {
		slog.Error("buscar el titular para avisar la exportacion", "exportacion", id, "error", err)
		return
	}
	if titular == nil {
		return
	}
	if err := s.avisos.Avisar(ctx, *titular, asunto, mensaje); err != nil {
		slog.Error("avisar la exportacion", "exportacion", id, "estado", estado, "error", err)
		return
	}
	slog.Info("exportacion avisada", "exportacion", id, "estado", estado)
}

func valor(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

type Descarga struct {
	URL     string    `json:"url"`
	VenceEn time.Time `json:"vence_en"`
}

func (s *Servicio) DescargarExportacion(ctx context.Context, e *Exportacion) (*Descarga, error) {
	if e.Estado != ExportLista || e.Objeto == "" {
		return nil, apierr.New(http.StatusConflict, "exportacion_no_lista",
			"la exportacion %d esta %s: se puede descargar cuando este lista", e.ID, e.Estado)
	}
	url, err := s.obj.URLDescarga(ctx, e.Objeto, fmt.Sprintf("exportacion-%d.csv", e.ID), vigenciaDescarga)
	if errors.Is(err, objetos.ErrNoExiste) {
		return nil, apierr.New(http.StatusGone, "exportacion_vencida",
			"el archivo de la exportacion %d ya no esta: pedila de nuevo", e.ID)
	}
	if err != nil {
		return nil, err
	}
	return &Descarga{URL: url, VenceEn: time.Now().Add(vigenciaDescarga).UTC()}, nil
}

var operacionPorTramo = map[string]string{
	TramoTransferencia:       "Transferencia",
	TramoSemilla:             "Transferencia",
	TramoReserva:             "Retiro",
	TramoLiquidacion:         "Liquidación de retiro",
	TramoReversa:             "Devolución de retiro",
	TramoIngresoBanco:        "Ingreso desde el banco",
	TramoIngresoAcreditacion: "Ingreso",
}

var horaArgentina = func() *time.Location {
	if z, err := time.LoadLocation("America/Argentina/Buenos_Aires"); err == nil {
		return z
	}
	return time.FixedZone("ART", -3*60*60)
}()

func enPesos(centavos int64) string {
	signo := ""
	if centavos < 0 {
		signo, centavos = "-", -centavos
	}
	return fmt.Sprintf("%s%d,%02d", signo, centavos/100, centavos%100)
}

func (s *Servicio) resolverExportacion(ctx context.Context, r *reclamo) (*Exportacion, error) {
	id, cuenta, periodos := r.id, r.cuenta, r.periodos
	var filas int64
	var deBase, deArchivo int

	aliases := map[int64]string{}
	alias := func(cuentaID int64, resuelto string) (string, error) {
		if resuelto != "" {
			return resuelto, nil
		}
		if a, ok := aliases[cuentaID]; ok {
			return a, nil
		}
		var a string
		err := s.pool.QueryRow(ctx, `SELECT alias FROM cuentas WHERE id = $1`, cuentaID).Scan(&a)
		if errors.Is(err, pgx.ErrNoRows) {
			a, err = fmt.Sprintf("#%d", cuentaID), nil
		}
		if err != nil {
			return "", err
		}
		aliases[cuentaID] = a
		return a, nil
	}

	clave := fmt.Sprintf("%ssolicitud=%d/%s/movimientos.csv", prefijoExportaciones, id, r.token)
	subido, err := s.obj.Subir(ctx, clave, "text/csv; charset=utf-8", func(w io.Writer) error {
		if _, err := io.WriteString(w, "\ufeff"); err != nil {
			return err
		}
		salida := csv.NewWriter(w)
		salida.Comma = ';'
		if err := salida.Write([]string{"Fecha", "Cuenta", "Operación", "Contraparte", "Monto", "Orden"}); err != nil {
			return err
		}
		for _, p := range periodos {
			origen, _, err := s.recorrerPeriodo(ctx, cuenta, p, func(m movimientoConAlias) error {
				propia, err := alias(m.CuentaID, m.alias)
				if err != nil {
					return err
				}
				otra, err := alias(m.ContraparteID, m.aliasContraparte)
				if err != nil {
					return err
				}
				operacion := operacionPorTramo[m.Tramo]
				if operacion == "" {
					operacion = m.Tramo
				}
				filas++
				return salida.Write([]string{
					m.CreadoEn.In(horaArgentina).Format("2006-01-02 15:04:05"),
					propia, operacion, otra, enPesos(m.MontoCentavos), strconv.FormatInt(m.OrdenID, 10),
				})
			})
			if err != nil {
				return err
			}
			if origen == OrigenBase {
				deBase++
			} else {
				deArchivo++
			}
		}
		salida.Flush()
		return salida.Error()
	})
	if err != nil {
		return nil, err
	}

	res, err := escanearExportacion(s.pool.QueryRow(ctx, `
		UPDATE solicitudes_exportacion
		SET estado = $2, clave = $3, uri = $4, filas = $5,
		    periodos_base = $6, periodos_archivo = $7, motivo = NULL, resuelta_en = now(),
		    tomada_por = NULL, reintentar_desde = NULL
		WHERE id = $1 AND tomada_por = $8 AND estado = $9
		RETURNING `+columnasExportacion,
		id, ExportLista, clave, subido.URI, filas, deBase, deArchivo, r.token, ExportPendiente))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errExportacionAjena
	}
	return res, err
}

const columnasExportacion = `id, cuenta_id, desde, hasta, estado, clave, uri, filas,
	periodos_base, periodos_archivo, motivo, solicitada_en, resuelta_en`

func escanearExportacion(fila pgx.Row) (*Exportacion, error) {
	var e Exportacion
	var desde, hasta time.Time
	var clave, uri, motivo *string
	if err := fila.Scan(&e.ID, &e.CuentaID, &desde, &hasta, &e.Estado, &clave, &uri, &e.Filas,
		&e.PeriodosBase, &e.PeriodosArchivo, &motivo, &e.SolicitadaEn, &e.ResueltaEn); err != nil {
		return nil, err
	}

	e.Desde, e.Hasta = desde.Format("2006-01"), hasta.Format("2006-01")
	if clave != nil {
		e.Objeto = *clave
	}
	if uri != nil {
		e.URI = *uri
	}
	if motivo != nil {
		e.Motivo = *motivo
	}
	return &e, nil
}

func (s *Servicio) VerExportacion(ctx context.Context, id int64) (*Exportacion, error) {
	e, err := escanearExportacion(s.pool.QueryRow(ctx,
		`SELECT `+columnasExportacion+` FROM solicitudes_exportacion WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NoEncontrado("no existe la exportacion %d", id)
	}
	return e, err
}

func (s *Servicio) ListarExportaciones(ctx context.Context, cuenta *int64, limite int) ([]*Exportacion, error) {
	if limite <= 0 || limite > 100 {
		limite = 20
	}
	filas, err := s.pool.Query(ctx, `SELECT `+columnasExportacion+` FROM solicitudes_exportacion
		WHERE ($1::bigint IS NULL OR cuenta_id = $1)
		ORDER BY id DESC
		LIMIT $2`, cuenta, limite)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	lista := []*Exportacion{}
	for filas.Next() {
		e, err := escanearExportacion(filas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, e)
	}
	return lista, filas.Err()
}
