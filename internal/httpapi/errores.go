package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

const (
	codigoSolicitudInvalida = "solicitud_invalida"
	codigoNoEncontrada      = "no_encontrada"
	codigoSinCupos          = "sin_cupos"
	codigoCuposExcedidos    = "cupos_excedidos"
	codigoRutaCerrada       = "ruta_cerrada"
	codigoErrorInterno      = "error_interno"
)

// errorRespuesta es la forma común de error del clúster. "detalle" siempre se
// serializa, aunque quede vacío.
type errorRespuesta struct {
	Codigo  string         `json:"codigo"`
	Mensaje string         `json:"mensaje"`
	Detalle map[string]any `json:"detalle"`
}

func responderJSON(w http.ResponseWriter, estado int, cuerpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(estado)
	if err := json.NewEncoder(w).Encode(cuerpo); err != nil {
		log.Printf("escribir la respuesta: %v", err)
	}
}

// responderError traduce un error de dominio al contrato HTTP. Los fallos
// internos se registran con el request id y nunca exponen su causa al cliente.
func responderError(w http.ResponseWriter, r *http.Request, err error, rutaID string) {
	estado, cuerpo := mapearError(err, rutaID)
	if estado >= http.StatusInternalServerError {
		log.Printf("error interno metodo=%s ruta=%s request_id=%s err=%s",
			r.Method, sanitizarParaLog(r.URL.Path), requestIDDe(r.Context()),
			sanitizarParaLog(err.Error()))
	}
	responderJSON(w, estado, cuerpo)
}

func mapearError(err error, rutaID string) (int, errorRespuesta) {
	var validacion *rutas.ErrorValidacion
	if errors.As(err, &validacion) {
		detalle := map[string]any{}
		if validacion.Campo != "" {
			detalle["campo"] = validacion.Campo
		}
		if validacion.Motivo != "" {
			detalle["motivo"] = validacion.Motivo
		}
		return http.StatusBadRequest, errorRespuesta{
			Codigo:  codigoSolicitudInvalida,
			Mensaje: mensajeDeValidacion(validacion),
			Detalle: detalle,
		}
	}

	detalleRuta := map[string]any{}
	if rutaID != "" {
		detalleRuta["rutaId"] = rutaID
	}

	switch {
	case errors.Is(err, rutas.ErrNoEncontrada):
		return http.StatusNotFound, errorRespuesta{
			Codigo:  codigoNoEncontrada,
			Mensaje: "La ruta no existe",
			Detalle: detalleRuta,
		}
	case errors.Is(err, rutas.ErrSinCupos):
		return http.StatusConflict, errorRespuesta{
			Codigo:  codigoSinCupos,
			Mensaje: "La ruta no tiene cupos disponibles",
			Detalle: detalleRuta,
		}
	case errors.Is(err, rutas.ErrCuposExcedidos):
		return http.StatusConflict, errorRespuesta{
			Codigo:  codigoCuposExcedidos,
			Mensaje: "La ruta no puede superar los cupos totales",
			Detalle: detalleRuta,
		}
	case errors.Is(err, rutas.ErrRutaCerrada):
		return http.StatusConflict, errorRespuesta{
			Codigo:  codigoRutaCerrada,
			Mensaje: "La ruta está cerrada",
			Detalle: detalleRuta,
		}
	default:
		return http.StatusInternalServerError, errorRespuesta{
			Codigo:  codigoErrorInterno,
			Mensaje: "No fue posible completar la solicitud",
			Detalle: map[string]any{},
		}
	}
}

func mensajeDeValidacion(err *rutas.ErrorValidacion) string {
	if err.Campo == "" {
		return err.Motivo
	}
	return err.Campo + " " + err.Motivo
}
