package httpapi

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/git-init-ud/trip-forge/internal/matching"
	"github.com/git-init-ud/trip-forge/internal/mid"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

// leerBusqueda valida los parámetros obligatorios de GET /rutas/compatibles.
// Todos los errores son de validación para que el transporte responda 400 con
// detalle.campo.
func leerBusqueda(r *http.Request) (mid.Busqueda, error) {
	valores := r.URL.Query()

	origenLat, err := numeroObligatorio(valores, "origenLat", -90, 90)
	if err != nil {
		return mid.Busqueda{}, err
	}
	origenLng, err := numeroObligatorio(valores, "origenLng", -180, 180)
	if err != nil {
		return mid.Busqueda{}, err
	}
	destinoLat, err := numeroObligatorio(valores, "destinoLat", -90, 90)
	if err != nil {
		return mid.Busqueda{}, err
	}
	destinoLng, err := numeroObligatorio(valores, "destinoLng", -180, 180)
	if err != nil {
		return mid.Busqueda{}, err
	}
	desde, err := fechaObligatoria(valores, "desde")
	if err != nil {
		return mid.Busqueda{}, err
	}
	hasta, err := fechaObligatoria(valores, "hasta")
	if err != nil {
		return mid.Busqueda{}, err
	}
	if !hasta.After(desde) {
		return mid.Busqueda{}, &rutas.ErrorValidacion{Campo: "hasta", Motivo: "debe ser posterior a desde"}
	}

	return mid.Busqueda{
		Origen:  matching.Punto{Latitud: origenLat, Longitud: origenLng},
		Destino: matching.Punto{Latitud: destinoLat, Longitud: destinoLng},
		Desde:   desde,
		Hasta:   hasta,
	}, nil
}

// numeroObligatorio lee un parámetro numérico, rechaza NaN e infinitos y
// comprueba el rango del contrato.
func numeroObligatorio(valores url.Values, campo string, minimo, maximo float64) (float64, error) {
	crudos, existe := valores[campo]
	if !existe || len(crudos) == 0 || strings.TrimSpace(crudos[0]) == "" {
		return 0, &rutas.ErrorValidacion{Campo: campo, Motivo: "es obligatorio"}
	}
	texto := strings.TrimSpace(crudos[0])
	numero, err := strconv.ParseFloat(texto, 64)
	if err != nil || math.IsNaN(numero) || math.IsInf(numero, 0) {
		return 0, &rutas.ErrorValidacion{Campo: campo, Motivo: "debe ser un número finito"}
	}
	if numero < minimo || numero > maximo {
		return 0, &rutas.ErrorValidacion{
			Campo:  campo,
			Motivo: fmt.Sprintf("debe estar entre %g y %g", minimo, maximo),
		}
	}
	return numero, nil
}

// fechaObligatoria lee una fecha RFC3339. Se usa RFC3339Nano porque acepta
// tanto segundos enteros como fracciones, sin endurecer el contrato.
func fechaObligatoria(valores url.Values, campo string) (time.Time, error) {
	crudos, existe := valores[campo]
	if !existe || len(crudos) == 0 || strings.TrimSpace(crudos[0]) == "" {
		return time.Time{}, &rutas.ErrorValidacion{Campo: campo, Motivo: "es obligatoria"}
	}
	fecha, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(crudos[0]))
	if err != nil {
		return time.Time{}, &rutas.ErrorValidacion{Campo: campo, Motivo: "debe usar el formato RFC3339"}
	}
	return fecha, nil
}
