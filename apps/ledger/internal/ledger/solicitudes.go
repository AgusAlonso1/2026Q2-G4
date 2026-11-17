package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"net/http"

	"ledger/internal/apierr"
	"ledger/internal/proveedor"
)

type EntradaSolicitud struct {
	IdempotencyKey string
	DestinoID      int64  `json:"destino_id"`
	De             string `json:"de"`
	MontoCentavos  int64  `json:"monto_centavos"`
}

type PagoDeSolicitud struct {
	Solicitud proveedor.Solicitud `json:"solicitud"`
	Orden     *Orden              `json:"orden"`
}

const (
	SolicitudPendiente = "pendiente"
	SolicitudAceptada  = "aceptada"
	SolicitudRechazada = "rechazada"
)

func (s *Servicio) PedirDinero(ctx context.Context, e EntradaSolicitud) (*proveedor.Solicitud, error) {
	if e.MontoCentavos <= 0 {
		return nil, apierr.Invalido("monto_centavos tiene que ser positivo")
	}
	if problema := s.rechazarInterna(e.DestinoID); problema != nil {
		return nil, problema
	}
	alias, ok, err := s.aliasLocal(ctx, e.DestinoID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierr.NoEncontrado("no existe la cuenta %d", e.DestinoID)
	}
	pagador, err := s.BuscarDestino(ctx, e.De, e.DestinoID)
	if err != nil {
		return nil, err
	}
	if pagador.Entidad == "" {
		return nil, apierr.Invalido("solo se le puede pedir plata a un alias o un CVU de nucleo, no a una CBU bancaria")
	}

	referencia := nuevaReferencia("SOL")
	if e.IdempotencyKey != "" {
		suma := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", s.entidad, e.DestinoID, e.IdempotencyKey)))
		referencia = "SOL-" + base32.StdEncoding.EncodeToString(suma[:])[:10]
	}
	x, err := s.prov.CrearSolicitud(ctx, proveedor.Solicitud{
		Referencia:    referencia,
		CVUPagador:    pagador.CBU,
		AliasPagador:  pagador.Alias,
		CVUCobrador:   s.CVU(e.DestinoID),
		AliasCobrador: s.AliasPublico(alias),
		MontoCentavos: e.MontoCentavos,
	})
	if err != nil {
		return nil, errorDelProveedor(err, "registrar la solicitud")
	}
	return &x, nil
}

func (s *Servicio) Solicitudes(ctx context.Context, cuentaID int64, rol string) ([]proveedor.Solicitud, error) {
	var lista []proveedor.Solicitud
	var err error
	switch rol {
	case "recibidas":
		lista, err = s.prov.Solicitudes(ctx, s.CVU(cuentaID), "", SolicitudPendiente)
	case "enviadas":
		lista, err = s.prov.Solicitudes(ctx, "", s.CVU(cuentaID), "")
	default:
		return nil, apierr.Invalido("rol tiene que ser recibidas o enviadas")
	}
	if err != nil {
		return nil, errorDelProveedor(err, "leer las solicitudes")
	}
	return lista, nil
}

func (s *Servicio) AceptarSolicitud(ctx context.Context, cuentaID int64, referencia string) (*PagoDeSolicitud, error) {
	x, err := s.solicitudPendiente(ctx, cuentaID, referencia)
	if err != nil {
		return nil, err
	}
	saldo, err := s.Saldo(ctx, cuentaID)
	if err != nil {
		return nil, err
	}
	if saldo.VigenteCentavos < x.MontoCentavos {
		return nil, apierr.Rechazado("saldo_insuficiente",
			"saldo %d insuficiente para pagar %d centavos", saldo.VigenteCentavos, x.MontoCentavos)
	}

	o, err := s.Egresar(ctx, EntradaEgreso{
		IdempotencyKey: "solicitud:" + referencia,
		OrigenID:       cuentaID,
		CBUDestino:     x.CVUCobrador,
		MontoCentavos:  x.MontoCentavos,
	})
	if o == nil {
		return nil, err
	}
	estado, motivo := "", ""
	switch o.Estado {
	case EstadoAcreditada:
		estado = SolicitudAceptada
	case EstadoRechazada, EstadoDevuelta:
		estado, motivo = SolicitudRechazada, "no se pudo pagar: "+o.Motivo
	}
	if estado != "" {
		resuelta, errResolver := s.prov.ResolverSolicitud(ctx, referencia, estado, motivo)
		if errResolver != nil {
			return nil, errorDelProveedor(errResolver, "resolver la solicitud")
		}
		x = &resuelta
	}
	return &PagoDeSolicitud{Solicitud: *x, Orden: o}, err
}

func (s *Servicio) RechazarSolicitud(ctx context.Context, cuentaID int64, referencia string) (*proveedor.Solicitud, error) {
	if _, err := s.solicitudPendiente(ctx, cuentaID, referencia); err != nil {
		return nil, err
	}
	x, err := s.prov.ResolverSolicitud(ctx, referencia, SolicitudRechazada, "la rechazo quien tenia que pagar")
	if err != nil {
		return nil, errorDelProveedor(err, "resolver la solicitud")
	}
	return &x, nil
}

func (s *Servicio) solicitudPendiente(ctx context.Context, cuentaID int64, referencia string) (*proveedor.Solicitud, error) {
	x, err := s.prov.VerSolicitud(ctx, referencia)
	if proveedor.EsNoEncontrado(err) {
		return nil, apierr.NoEncontrado("no existe la solicitud %s", referencia)
	}
	if err != nil {
		return nil, errorDelProveedor(err, "leer la solicitud")
	}
	if x.CVUPagador != s.CVU(cuentaID) {
		return nil, apierr.New(http.StatusForbidden, "solicitud_ajena",
			"la solicitud %s no se le pide a la cuenta %d", referencia, cuentaID)
	}
	if x.Estado != SolicitudPendiente {
		return nil, apierr.New(http.StatusConflict, "solicitud_resuelta", "la solicitud %s ya esta %s", referencia, x.Estado)
	}
	return &x, nil
}

func errorDelProveedor(err error, accion string) error {
	if pe, ok := proveedor.EsRechazo(err); ok {
		return apierr.New(http.StatusBadGateway, "proveedor_rechazo", "el proveedor no dejo %s (%d): %s", accion, pe.Codigo, pe.Cuerpo)
	}
	return apierr.New(http.StatusBadGateway, "proveedor_no_disponible", "no se pudo %s en el proveedor: %v", accion, err)
}
