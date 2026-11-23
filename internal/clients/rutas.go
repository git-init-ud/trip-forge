package clients

import (
	"context"
	"encoding/json"
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
// GATEWAY_SECRET y token el token de servicio opcional; cualquiera de los dos
// vacío desactiva su cabecera.
func NuevoClienteRutas(baseURL string, plazo time.Duration, secreto, token string) (*ClienteRutas, error) {
	base, err := nuevoClienteBase("rutas-crud", baseURL, plazo, secreto, token)
	if err != nil {
		return nil, err
	}
	return &ClienteRutas{base: base}, nil
}

// paginaCruda conserva la presencia de los campos: un JSON null o un objeto
// sin "items" no se puede confundir con una página vacía legítima.
type paginaCruda struct {
	Items  json.RawMessage `json:"items"`
	Total  *int            `json:"total"`
	Limit  *int            `json:"limit"`
	Offset *int            `json:"offset"`
}

// ListarRutas pide una página de GET /rutas y valida la forma completa de la
// respuesta. Una página que no cumple el contrato se trata como dependencia no
// disponible: el MID no puede calcular nada con datos parciales.
func (c *ClienteRutas) ListarRutas(ctx context.Context, filtros FiltrosRutas) (PaginaRutas, error) {
	var cruda paginaCruda
	estado, err := c.base.obtener(ctx, "/rutas", consultaRutas(filtros), &cruda)
	if err != nil {
		return PaginaRutas{}, err
	}
	if estado != 200 {
		return PaginaRutas{}, errorDeEstado("rutas-crud", estado)
	}
	return interpretarPagina(cruda, filtros.Offset)
}

func interpretarPagina(cruda paginaCruda, offsetSolicitado int) (PaginaRutas, error) {
	if len(cruda.Items) == 0 || string(cruda.Items) == "null" {
		return PaginaRutas{}, paginaInvalida("falta items")
	}
	var items []rutas.Ruta
	if err := json.Unmarshal(cruda.Items, &items); err != nil || items == nil {
		return PaginaRutas{}, paginaInvalida("items no es un arreglo")
	}
	if cruda.Total == nil || cruda.Limit == nil || cruda.Offset == nil {
		return PaginaRutas{}, paginaInvalida("faltan total, limit u offset")
	}
	total, limit, offset := *cruda.Total, *cruda.Limit, *cruda.Offset
	switch {
	case total < 0:
		return PaginaRutas{}, paginaInvalida("total es negativo")
	case limit < 1:
		return PaginaRutas{}, paginaInvalida("limit debe ser positivo")
	case offset < 0:
		return PaginaRutas{}, paginaInvalida("offset es negativo")
	case total < len(items):
		return PaginaRutas{}, paginaInvalida("total es menor que la cantidad de items")
	case len(items) > limit:
		return PaginaRutas{}, paginaInvalida("hay más items que el limit de la página")
	case offset != offsetSolicitado:
		return PaginaRutas{}, paginaInvalida("offset no coincide con el solicitado")
	}
	return PaginaRutas{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func paginaInvalida(motivo string) error {
	return fmt.Errorf("%w: rutas-crud devolvió una página inválida (%s)", ErrNoDisponible, motivo)
}

// ListarTodas recorre GET /rutas con páginas de 100 hasta cubrir total. Acumula
// sin duplicados por id y falla si una página vacía deja datos pendientes o si
// se supera el tope de seguridad de 100 páginas.
func (c *ClienteRutas) ListarTodas(ctx context.Context, filtros FiltrosRutas) ([]rutas.Ruta, error) {
	filtros.Limit = LimitePagina
	filtros.Offset = 0

	todas := make([]rutas.Ruta, 0, LimitePagina)
	vistos := make(map[string]struct{}, LimitePagina)
	for pagina := 1; pagina <= MaximoPaginas; pagina++ {
		resultado, err := c.ListarRutas(ctx, filtros)
		if err != nil {
			return nil, err
		}
		if len(resultado.Items) == 0 {
			if resultado.Total > filtros.Offset {
				return nil, fmt.Errorf("%w: rutas-crud devolvió una página vacía con total %d y offset %d",
					ErrNoDisponible, resultado.Total, filtros.Offset)
			}
			return todas, nil
		}
		for _, ruta := range resultado.Items {
			if _, repetida := vistos[ruta.ID]; repetida {
				continue
			}
			vistos[ruta.ID] = struct{}{}
			todas = append(todas, ruta)
		}
		filtros.Offset += len(resultado.Items)
		if resultado.Total <= filtros.Offset {
			return todas, nil
		}
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
