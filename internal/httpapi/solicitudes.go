package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
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

// actualizarRutaSolicitud es el cuerpo de PUT /rutas/{id}. No declara
// conductorId: el propietario se comprueba con X-Usuario-Id y no se puede
// cambiar.
type actualizarRutaSolicitud struct {
	Origen       lugarSolicitud      `json:"origen"`
	Destino      lugarSolicitud      `json:"destino"`
	Trazado      lineStringSolicitud `json:"trazado"`
	Salida       string              `json:"salida"`
	CuposTotales *int                `json:"cuposTotales"`
	PrecioTotal  *int64              `json:"precioTotal"`
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
		ConductorID:  s.ConductorID,
		Origen:       s.Origen.aDominio(),
		Destino:      s.Destino.aDominio(),
		Trazado:      s.Trazado.aDominio(),
		Salida:       salida,
		CuposTotales: *s.CuposTotales,
		PrecioTotal:  *s.PrecioTotal,
	}, nil
}

func (s actualizarRutaSolicitud) aDominio() (rutas.ActualizarRuta, error) {
	if s.CuposTotales == nil {
		return rutas.ActualizarRuta{}, &rutas.ErrorValidacion{Campo: "cuposTotales", Motivo: "es obligatorio"}
	}
	if s.PrecioTotal == nil {
		return rutas.ActualizarRuta{}, &rutas.ErrorValidacion{Campo: "precioTotal", Motivo: "es obligatorio"}
	}
	salida, err := leerSalida(s.Salida)
	if err != nil {
		return rutas.ActualizarRuta{}, err
	}
	return rutas.ActualizarRuta{
		Origen:       s.Origen.aDominio(),
		Destino:      s.Destino.aDominio(),
		Trazado:      s.Trazado.aDominio(),
		Salida:       salida,
		CuposTotales: *s.CuposTotales,
		PrecioTotal:  *s.PrecioTotal,
	}, nil
}

func (s lugarSolicitud) aDominio() rutas.Lugar {
	return rutas.Lugar{
		Nombre: s.Nombre,
		Punto: rutas.Punto{
			Type:        s.Punto.Type,
			Coordinates: s.Punto.Coordinates,
		},
	}
}

func (s lineStringSolicitud) aDominio() rutas.LineString {
	return rutas.LineString{
		Type:        s.Type,
		Coordinates: s.Coordinates,
	}
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

// leerFiltroListado traduce y valida la consulta de GET /rutas. Los parámetros
// ausentes quedan sin filtro; limit usa 20 por defecto y offset 0. Cualquier
// valor fuera del contrato devuelve un *rutas.ErrorValidacion con el campo.
func leerFiltroListado(r *http.Request) (rutas.FiltroListado, error) {
	consulta := r.URL.Query()
	filtro := rutas.FiltroListado{
		ConductorID: strings.TrimSpace(consulta.Get("conductorId")),
		Estado:      rutas.Estado(strings.TrimSpace(consulta.Get("estado"))),
		Limit:       rutas.LimitPorDefecto,
	}
	if consulta.Has("conductorId") && filtro.ConductorID == "" {
		return rutas.FiltroListado{}, &rutas.ErrorValidacion{Campo: "conductorId", Motivo: "no puede estar vacío"}
	}

	desde, err := leerInstanteDeConsulta(consulta, "desde")
	if err != nil {
		return rutas.FiltroListado{}, err
	}
	hasta, err := leerInstanteDeConsulta(consulta, "hasta")
	if err != nil {
		return rutas.FiltroListado{}, err
	}
	if consulta.Has("estado") && filtro.Estado == "" {
		return rutas.FiltroListado{}, &rutas.ErrorValidacion{Campo: "estado", Motivo: "no puede estar vacío"}
	}
	filtro.Desde = desde
	filtro.Hasta = hasta

	limit, err := leerEnteroDeConsulta(consulta, "limit", rutas.LimitPorDefecto)
	if err != nil {
		return rutas.FiltroListado{}, err
	}
	offset, err := leerEnteroDeConsulta(consulta, "offset", 0)
	if err != nil {
		return rutas.FiltroListado{}, err
	}
	filtro.Limit = limit
	filtro.Offset = offset
	filtro = filtro.Normalizar()

	if err := filtro.Validar(); err != nil {
		return rutas.FiltroListado{}, err
	}
	return filtro, nil
}

func leerInstanteDeConsulta(consulta url.Values, campo string) (time.Time, error) {
	if consulta.Has(campo) && strings.TrimSpace(consulta.Get(campo)) == "" {
		return time.Time{}, &rutas.ErrorValidacion{Campo: campo, Motivo: "no puede estar vacío"}
	}
	crudo := strings.TrimSpace(consulta.Get(campo))
	if crudo == "" {
		return time.Time{}, nil
	}
	instante, err := time.Parse(time.RFC3339Nano, crudo)
	if err != nil {
		return time.Time{}, &rutas.ErrorValidacion{Campo: campo, Motivo: "debe usar el formato RFC3339"}
	}
	return instante.UTC(), nil
}

func leerEnteroDeConsulta(consulta url.Values, campo string, defecto int) (int, error) {
	if !consulta.Has(campo) {
		return defecto, nil
	}
	valor, err := strconv.Atoi(strings.TrimSpace(consulta.Get(campo)))
	if err != nil {
		return 0, &rutas.ErrorValidacion{Campo: campo, Motivo: "debe ser un entero"}
	}
	return valor, nil
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
		if err == nil {
			// Un segundo valor JSON válido (por ejemplo `{...}{}` o
			// `{...} null`) decodifica sin error: el cuerpo debe contener un
			// único objeto.
			return &rutas.ErrorValidacion{Campo: "cuerpo", Motivo: "contiene datos después del objeto"}
		}
		return errorDeDecodificacion(err)
	}
	return nil
}

func errorDeDecodificacion(err error) error {
	if err == nil {
		// Defensa: un fallo sin detalle no debe convertirse en pánico.
		return &rutas.ErrorValidacion{Campo: "cuerpo", Motivo: "no es un JSON válido"}
	}
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
