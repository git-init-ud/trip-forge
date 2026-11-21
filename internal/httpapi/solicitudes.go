package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

const maxCuerpo = 1 << 20

type puntoSolicitud struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

type lineStringSolicitud struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

type lugarSolicitud struct {
	Nombre string         `json:"nombre"`
	Punto  puntoSolicitud `json:"punto"`
}

type crearRutaSolicitud struct {
	ConductorID  string              `json:"conductorId"`
	Origen       lugarSolicitud      `json:"origen"`
	Destino      lugarSolicitud      `json:"destino"`
	Trazado      lineStringSolicitud `json:"trazado"`
	Salida       string              `json:"salida"`
	CuposTotales *int                `json:"cuposTotales"`
	PrecioTotal  *int64              `json:"precioTotal"`
}

type ajusteCuposSolicitud struct {
	Delta *int `json:"delta"`
}

func (s crearRutaSolicitud) aDominio() (rutas.CrearRuta, error) {
	if s.CuposTotales == nil {
		return rutas.CrearRuta{}, &rutas.ErrorValidacion{Campo: "cuposTotales", Motivo: "es obligatorio"}
	}
	if s.PrecioTotal == nil {
		return rutas.CrearRuta{}, &rutas.ErrorValidacion{Campo: "precioTotal", Motivo: "es obligatorio"}
	}
	salida, err := leerSalida(s.Salida)
	if err != nil {
		return rutas.CrearRuta{}, err
	}
	return rutas.CrearRuta{
		ConductorID: s.ConductorID,
		Origen: rutas.Lugar{
			Nombre: s.Origen.Nombre,
			Punto: rutas.Punto{
				Type:        s.Origen.Punto.Type,
				Coordinates: s.Origen.Punto.Coordinates,
			},
		},
		Destino: rutas.Lugar{
			Nombre: s.Destino.Nombre,
			Punto: rutas.Punto{
				Type:        s.Destino.Punto.Type,
				Coordinates: s.Destino.Punto.Coordinates,
			},
		},
		Trazado: rutas.LineString{
			Type:        s.Trazado.Type,
			Coordinates: s.Trazado.Coordinates,
		},
		Salida:       salida,
		CuposTotales: *s.CuposTotales,
		PrecioTotal:  *s.PrecioTotal,
	}, nil
}

func leerSalida(valor string) (time.Time, error) {
	if strings.TrimSpace(valor) == "" {
		return time.Time{}, &rutas.ErrorValidacion{Campo: "salida", Motivo: "es obligatoria"}
	}
	salida, err := time.Parse(time.RFC3339Nano, valor)
	if err != nil {
		return time.Time{}, &rutas.ErrorValidacion{Campo: "salida", Motivo: "debe usar el formato RFC3339"}
	}
	return salida, nil
}

// decodificar exige JSON, limita el cuerpo a 1 MB y rechaza campos
// desconocidos o datos después del objeto. Las respuestas de error conservan
// el campo cuando encoding/json puede identificarlo.
func decodificar(w http.ResponseWriter, r *http.Request, destino any) error {
	tipo, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || tipo != "application/json" {
		return &rutas.ErrorValidacion{Campo: "Content-Type", Motivo: "debe ser application/json"}
	}

	decodificador := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCuerpo))
	decodificador.DisallowUnknownFields()
	if err := decodificador.Decode(destino); err != nil {
		return errorDeDecodificacion(err)
	}
	if err := decodificador.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return errorDeDecodificacion(err)
	}
	return nil
}

func errorDeDecodificacion(err error) error {
	var excedido *http.MaxBytesError
	if errors.As(err, &excedido) {
		return &rutas.ErrorValidacion{Campo: "cuerpo", Motivo: "supera el límite de 1 MB"}
	}
	if errors.Is(err, io.EOF) {
		return &rutas.ErrorValidacion{Campo: "cuerpo", Motivo: "es obligatorio"}
	}
	var tipo *json.UnmarshalTypeError
	if errors.As(err, &tipo) && tipo.Field != "" {
		return &rutas.ErrorValidacion{Campo: tipo.Field, Motivo: "tiene un tipo de dato inválido"}
	}
	if strings.Contains(err.Error(), "unknown field") {
		return &rutas.ErrorValidacion{Campo: "cuerpo", Motivo: "contiene campos desconocidos"}
	}
	return &rutas.ErrorValidacion{Campo: "cuerpo", Motivo: "no es un JSON válido"}
}
