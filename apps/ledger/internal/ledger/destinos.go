package ledger

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"ledger/internal/apierr"
	"ledger/internal/cvu"
	"ledger/internal/proveedor"
)

type Destino struct {
	CBU          string `json:"cbu"`
	Alias        string `json:"alias,omitempty"`
	Entidad      string `json:"entidad,omitempty"`
	Billetera    string `json:"billetera,omitempty"`
	MismaEntidad bool   `json:"misma_entidad"`
}

func (s *Servicio) PublicarAlias(ctx context.Context) error {
	activo, err := s.proveedorVigente(ctx)
	if err != nil {
		return err
	}
	marca := activo + "#" + s.billetera
	filas, err := s.pool.Query(ctx, `
		SELECT id, alias FROM cuentas
		WHERE tipo <> 'interna' AND alias_publicado_en IS DISTINCT FROM $1
		ORDER BY id LIMIT 200`, marca)
	if err != nil {
		return err
	}
	var ids []int64
	var cuentas []proveedor.CuentaAlias
	for filas.Next() {
		var id int64
		var alias string
		if err := filas.Scan(&id, &alias); err != nil {
			filas.Close()
			return err
		}
		ids = append(ids, id)
		cuentas = append(cuentas, proveedor.CuentaAlias{Alias: s.AliasPublico(alias), CVU: s.CVU(id)})
	}
	filas.Close()
	if err := filas.Err(); err != nil || len(cuentas) == 0 {
		return err
	}

	ocupados, err := s.prov.PublicarAlias(ctx, s.billetera, cuentas)
	if _, rechazo := proveedor.EsRechazo(err); err != nil && !rechazo {
		return err
	}
	if err != nil {
		slog.Error("el proveedor no acepto los alias: no se los vuelve a mandar", "proveedor", activo,
			"billetera", s.billetera, "error", err)
	}
	if len(ocupados) > 0 {
		slog.Warn("alias que en el proveedor ya son de otra entidad", "proveedor", activo, "alias", ocupados)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE cuentas SET alias_publicado_en = $1 WHERE id = ANY($2)`, marca, ids)
	return err
}

func (s *Servicio) BuscarDestino(ctx context.Context, clave string, origenID int64) (*Destino, error) {
	clave = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(clave), "@"))
	if clave == "" {
		return nil, apierr.Invalido("falta el alias, la CBU o el CVU de destino")
	}

	var d *Destino
	var err error
	if numeros := strings.ReplaceAll(clave, " ", ""); len(numeros) == cvu.Largo && soloNumeros(numeros) {
		d, err = s.destinoDeLaCBU(ctx, numeros)
	} else {
		d, err = s.destinoDelAlias(ctx, clave)
	}
	if err != nil {
		return nil, err
	}

	if origenID != 0 && d.CBU == s.CVU(origenID) {
		return nil, apierr.Invalido("es el CVU de la misma cuenta de origen")
	}
	d.MismaEntidad = d.Entidad != "" && d.Entidad == s.entidad
	return d, nil
}

func (s *Servicio) destinoDeLaCBU(ctx context.Context, numeros string) (*Destino, error) {
	if err := cvu.Validar(numeros); err != nil {
		return nil, apierr.Invalido("la CBU o el CVU no es valido: %v", err)
	}
	d := &Destino{CBU: numeros}
	entidad, id, esCVU := cvu.Partes(numeros)
	if !esCVU {
		return d, nil
	}
	d.Entidad = entidad
	if entidad == s.entidad {
		if alias, ok, err := s.aliasLocal(ctx, id); err != nil {
			return nil, err
		} else if ok {
			d.Alias, d.Billetera = s.AliasPublico(alias), s.billetera
		}
		return d, nil
	}
	c, err := s.prov.BuscarCuenta(ctx, numeros)
	if err == nil {
		d.Alias, d.Billetera = c.Alias, c.Billetera
	} else if !proveedor.EsNoEncontrado(err) {
		slog.Warn("no se pudo buscar el CVU en el proveedor", "error", err)
	}
	return d, nil
}

func (s *Servicio) destinoDelAlias(ctx context.Context, clave string) (*Destino, error) {
	if local, ok := strings.CutPrefix(clave, s.billetera+"."); ok {
		return s.destinoLocal(ctx, local, clave)
	}

	c, err := s.prov.BuscarCuenta(ctx, clave)
	if err == nil {
		if entidad, _, ok := cvu.Partes(c.CVU); !ok || entidad != c.Entidad {
			return nil, apierr.New(http.StatusBadGateway, "proveedor_respuesta_invalida",
				"el proveedor devolvio un CVU invalido para el alias %s", clave)
		}
		return &Destino{CBU: c.CVU, Alias: c.Alias, Entidad: c.Entidad, Billetera: c.Billetera}, nil
	}
	if !proveedor.EsNoEncontrado(err) {
		slog.Warn("no se pudo buscar el alias en el proveedor: se busca entre las cuentas propias", "error", err)
	}
	d, errLocal := s.destinoLocal(ctx, clave, clave)
	if errLocal != nil && err != nil && !proveedor.EsNoEncontrado(err) {
		return nil, apierr.New(http.StatusBadGateway, "proveedor_no_disponible",
			"no se pudo buscar el alias en el proveedor: %v", err)
	}
	return d, errLocal
}

func (s *Servicio) destinoLocal(ctx context.Context, alias, pedido string) (*Destino, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM cuentas WHERE alias = $1 AND tipo <> 'interna'`, alias).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierr.NoEncontrado("no hay ninguna cuenta con el alias %s", pedido)
	}
	if err != nil {
		return nil, err
	}
	return &Destino{CBU: s.CVU(id), Alias: s.AliasPublico(alias), Entidad: s.entidad, Billetera: s.billetera}, nil
}

func (s *Servicio) aliasLocal(ctx context.Context, id int64) (string, bool, error) {
	var alias string
	err := s.pool.QueryRow(ctx,
		`SELECT alias FROM cuentas WHERE id = $1 AND tipo <> 'interna'`, id).Scan(&alias)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return alias, err == nil, err
}

func soloNumeros(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
