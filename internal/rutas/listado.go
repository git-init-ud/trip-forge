package rutas

import (
	"strings"
	"time"
)

const (
	// LimitPorDefecto es el tamaño de página cuando la solicitud no envía
	// limit.
	LimitPorDefecto = 20
	// LimitMaximo es el mayor tamaño de página que acepta el contrato.
	LimitMaximo = 100
)

// FiltroListado es la consulta validada de GET /rutas. Las fechas son
// inclusivas y se comparan contra la salida de la ruta en UTC.
type FiltroListado struct {
	ConductorID string
	Estado      Estado
	Desde       time.Time
	Hasta       time.Time
	Limit       int
	Offset      int
}

// Normalizar deja la ventana en UTC y recorta la identidad del conductor. No
// aplica valores por defecto: la ausencia de limit la resuelve el transporte y
// un limit cero explícito es inválido.
func (f FiltroListado) Normalizar() FiltroListado {
	f.ConductorID = strings.TrimSpace(f.ConductorID)
	if !f.Desde.IsZero() {
		f.Desde = f.Desde.UTC()
	}
	if !f.Hasta.IsZero() {
		f.Hasta = f.Hasta.UTC()
	}
	return f
}

// Validar comprueba los filtros y los límites de paginación del contrato.
func (f FiltroListado) Validar() error {
	if f.ConductorID != "" && strings.TrimSpace(f.ConductorID) == "" {
		return campoInvalido("conductorId", "no puede estar vacío")
	}
	switch f.Estado {
	case "", Abierta, Llena, Cerrada:
	default:
		return campoInvalido("estado", `debe ser "abierta", "llena" o "cerrada"`)
	}
	if !f.Desde.IsZero() && !f.Hasta.IsZero() && f.Desde.After(f.Hasta) {
		return campoInvalido("desde", "no puede ser posterior a hasta")
	}
	if f.Limit < 1 || f.Limit > LimitMaximo {
		return campoInvalido("limit", "debe estar entre 1 y 100")
	}
	if f.Offset < 0 {
		return campoInvalido("offset", "no puede ser negativo")
	}
	return nil
}

// Listado es una página de rutas junto con el total que cumple el filtro, sin
// paginar.
type Listado struct {
	Items []Ruta
	Total int64
}
