package mock

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"ledger/internal/cvu"
)

type Solicitud struct {
	Referencia    string     `json:"referencia"`
	CVUPagador    string     `json:"cvu_pagador"`
	AliasPagador  string     `json:"alias_pagador,omitempty"`
	CVUCobrador   string     `json:"cvu_cobrador"`
	AliasCobrador string     `json:"alias_cobrador,omitempty"`
	MontoCentavos int64      `json:"monto_centavos"`
	Estado        string     `json:"estado"`
	Motivo        string     `json:"motivo,omitempty"`
	CreadaEn      time.Time  `json:"creada_en"`
	ResueltaEn    *time.Time `json:"resuelta_en,omitempty"`
}

const columnasSolicitud = `referencia, cvu_pagador, coalesce(alias_pagador, ''), cvu_cobrador,
	coalesce(alias_cobrador, ''), monto_centavos, estado, coalesce(motivo, ''), creada_en, resuelta_en`

func escanearSolicitud(fila pgx.Row) (*Solicitud, error) {
	var x Solicitud
	err := fila.Scan(&x.Referencia, &x.CVUPagador, &x.AliasPagador, &x.CVUCobrador, &x.AliasCobrador,
		&x.MontoCentavos, &x.Estado, &x.Motivo, &x.CreadaEn, &x.ResueltaEn)
	return &x, err
}

func (s *Servicio) registrarSolicitudes(mux *http.ServeMux) {
	mux.HandleFunc("POST "+Prefijo+"/solicitudes", s.protegido(s.crearSolicitud))
	mux.HandleFunc("GET "+Prefijo+"/solicitudes", s.protegido(s.listarSolicitudes))
	mux.HandleFunc("GET "+Prefijo+"/solicitudes/{referencia}", s.protegido(s.verSolicitud))
	mux.HandleFunc("POST "+Prefijo+"/solicitudes/{referencia}/resolver", s.protegido(s.resolverSolicitud))
}

func (s *Servicio) crearSolicitud(w http.ResponseWriter, r *http.Request) {
	var x Solicitud
	if err := json.NewDecoder(r.Body).Decode(&x); err != nil {
		escribir(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_, _, okPagador := cvu.Partes(x.CVUPagador)
	_, _, okCobrador := cvu.Partes(x.CVUCobrador)
	switch {
	case x.Referencia == "":
		escribir(w, http.StatusBadRequest, map[string]string{"error": "falta referencia"})
		return
	case !okPagador || !okCobrador:
		escribir(w, http.StatusBadRequest, map[string]string{"error": "cvu_pagador y cvu_cobrador tienen que ser CVU validos"})
		return
	case x.CVUPagador == x.CVUCobrador:
		escribir(w, http.StatusBadRequest, map[string]string{"error": "el pagador y el cobrador son la misma cuenta"})
		return
	case x.MontoCentavos <= 0:
		escribir(w, http.StatusBadRequest, map[string]string{"error": "monto_centavos tiene que ser positivo"})
		return
	}

	if _, err := s.pool.Exec(r.Context(), `
		INSERT INTO mock_solicitudes (referencia, cvu_pagador, alias_pagador, cvu_cobrador,
		                              alias_cobrador, monto_centavos)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6)
		ON CONFLICT (referencia) DO NOTHING`,
		x.Referencia, x.CVUPagador, x.AliasPagador, x.CVUCobrador, x.AliasCobrador, x.MontoCentavos); err != nil {
		escribir(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.responderSolicitud(w, r.Context(), x.Referencia, http.StatusCreated)
}

func (s *Servicio) listarSolicitudes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("cvu_pagador") == "" && q.Get("cvu_cobrador") == "" {
		escribir(w, http.StatusBadRequest, map[string]string{"error": "falta cvu_pagador o cvu_cobrador"})
		return
	}
	filas, err := s.pool.Query(r.Context(), `SELECT `+columnasSolicitud+` FROM mock_solicitudes
		WHERE ($1 = '' OR cvu_pagador = $1) AND ($2 = '' OR cvu_cobrador = $2) AND ($3 = '' OR estado = $3)
		ORDER BY creada_en DESC LIMIT 20`, q.Get("cvu_pagador"), q.Get("cvu_cobrador"), q.Get("estado"))
	if err != nil {
		escribir(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer filas.Close()
	lista := []Solicitud{}
	for filas.Next() {
		x, err := escanearSolicitud(filas)
		if err != nil {
			escribir(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		lista = append(lista, *x)
	}
	if err := filas.Err(); err != nil {
		escribir(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	escribir(w, http.StatusOK, map[string]any{"solicitudes": lista})
}

func (s *Servicio) verSolicitud(w http.ResponseWriter, r *http.Request) {
	s.responderSolicitud(w, r.Context(), r.PathValue("referencia"), http.StatusOK)
}

func (s *Servicio) resolverSolicitud(w http.ResponseWriter, r *http.Request) {
	var pedido struct {
		Estado string `json:"estado"`
		Motivo string `json:"motivo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&pedido); err != nil {
		escribir(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if pedido.Estado != "aceptada" && pedido.Estado != "rechazada" {
		escribir(w, http.StatusBadRequest, map[string]string{"error": "estado tiene que ser aceptada o rechazada"})
		return
	}
	if _, err := s.pool.Exec(r.Context(), `
		UPDATE mock_solicitudes SET estado = $2, motivo = NULLIF($3, ''), resuelta_en = now()
		WHERE referencia = $1 AND estado = 'pendiente'`,
		r.PathValue("referencia"), pedido.Estado, pedido.Motivo); err != nil {
		escribir(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.responderSolicitud(w, r.Context(), r.PathValue("referencia"), http.StatusOK)
}

func (s *Servicio) responderSolicitud(w http.ResponseWriter, ctx context.Context, referencia string, estado int) {
	x, err := escanearSolicitud(s.pool.QueryRow(ctx,
		`SELECT `+columnasSolicitud+` FROM mock_solicitudes WHERE referencia = $1`, referencia))
	if errors.Is(err, pgx.ErrNoRows) {
		escribir(w, http.StatusNotFound, map[string]string{"error": "no existe la solicitud"})
		return
	}
	if err != nil {
		escribir(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	escribir(w, estado, x)
}
