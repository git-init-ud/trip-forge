package clients

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

const (
	// LimitePagina es el tamaño de página que usa el MID al recorrer el CRUD.
	LimitePagina = 100
	// MaximoPaginas acota el recorrido para que un CRUD defectuoso no deje al
	// MID paginando sin fin.
	MaximoPaginas = 100
)

// FiltrosRutas reproduce los filtros de GET /rutas del CRUD. Las fechas se
// envían en UTC.
type FiltrosRutas struct {
	ConductorID string
	Estado      string
	Desde       *time.Time
	Hasta       *time.Time
	Limit       int
	Offset      int
}

// PaginaRutas es la respuesta paginada del CRUD.
type PaginaRutas struct {
	Items  []rutas.Ruta `json:"items"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// ClienteRutas consulta el CRUD de rutas por HTTP.
type ClienteRutas struct {
	base *clienteBase
}

// NuevoClienteRutas construye el cliente del CRUD. secreto es el valor de
// GATEWAY_SECRET; vacío desactiva la cabecera compartida.
func NuevoClienteRutas(baseURL string, plazo time.Duration, secreto string) (*ClienteRutas, error) {
	base, err := nuevoClienteBase("rutas-crud", baseURL, plazo, secreto)
	if err != nil {
		return nil, err
	}
	return &ClienteRutas{base: base}, nil
}

// ListarRutas pide una página de GET /rutas.
func (c *ClienteRutas) ListarRutas(ctx context.Context, filtros FiltrosRutas) (PaginaRutas, error) {
	var pagina PaginaRutas
	estado, err := c.base.obtener(ctx, "/rutas", consultaRutas(filtros), &pagina)
	if err != nil {
		return PaginaRutas{}, err
	}
	if estado != 200 {
		return PaginaRutas{}, errorDeEstado("rutas-crud", estado)
	}
	if pagina.Total < 0 || pagina.Limit < 0 || pagina.Offset < 0 {
		return PaginaRutas{}, fmt.Errorf("%w: rutas-crud devolvió una página inválida", ErrNoDisponible)
	}
	if pagina.Items == nil {
		pagina.Items = []rutas.Ruta{}
	}
	return pagina, nil
}

// ListarTodas recorre GET /rutas con páginas de 100 hasta cubrir total. Falla
// si se supera el tope de seguridad de 100 páginas.
func (c *ClienteRutas) ListarTodas(ctx context.Context, filtros FiltrosRutas) ([]rutas.Ruta, error) {
	filtros.Limit = LimitePagina
	filtros.Offset = 0

	todas := make([]rutas.Ruta, 0, LimitePagina)
	for pagina := 1; pagina <= MaximoPaginas; pagina++ {
		resultado, err := c.ListarRutas(ctx, filtros)
		if err != nil {
			return nil, err
		}
		todas = append(todas, resultado.Items...)
		if len(resultado.Items) == 0 || resultado.Total <= len(todas) {
			return todas, nil
		}
		filtros.Offset += len(resultado.Items)
	}
	return nil, fmt.Errorf("%w: rutas-crud superó el tope de %d páginas", ErrNoDisponible, MaximoPaginas)
}

func consultaRutas(filtros FiltrosRutas) url.Values {
	valores := url.Values{}
	if filtros.ConductorID != "" {
		valores.Set("conductorId", filtros.ConductorID)
	}
	if filtros.Estado != "" {
		valores.Set("estado", filtros.Estado)
	}
	if filtros.Desde != nil {
		valores.Set("desde", filtros.Desde.UTC().Format(time.RFC3339Nano))
	}
	if filtros.Hasta != nil {
		valores.Set("hasta", filtros.Hasta.UTC().Format(time.RFC3339Nano))
	}
	if filtros.Limit > 0 {
		valores.Set("limit", strconv.Itoa(filtros.Limit))
	}
	if filtros.Offset > 0 {
		valores.Set("offset", strconv.Itoa(filtros.Offset))
	}
	return valores
}
