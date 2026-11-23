package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/mid"
	"github.com/git-init-ud/trip-forge/internal/rutas"
	"github.com/go-chi/chi/v5"
)

// codigoDependenciaNoDisponible es local del MID: el CRUD no tiene una
// dependencia externa que pueda caerse.
const codigoDependenciaNoDisponible = "dependencia_no_disponible"

func buscarRutasCompatibles(servicio *mid.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busqueda, err := leerBusqueda(r)
		if err != nil {
			responderErrorMID(w, r, err, "")
			return
		}
		resultado, err := servicio.Buscar(contextoDependencias(r), busqueda)
		if err != nil {
			responderErrorMID(w, r, err, "")
			return
		}
		responderJSON(w, http.StatusOK, resultado)
	}
}

func panelConductor(servicio *mid.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conductorID := strings.TrimSpace(chi.URLParam(r, "id"))
		if conductorID == "" {
			responderErrorMID(w, r, &rutas.ErrorValidacion{Campo: "id", Motivo: "es obligatorio"}, "")
			return
		}
		panel, err := servicio.Panel(contextoDependencias(r), conductorID)
		if err != nil {
			responderErrorMID(w, r, err, conductorID)
			return
		}
		responderJSON(w, http.StatusOK, panel)
	}
}

// contextoDependencias propaga el X-Request-Id entrante a los clientes HTTP.
// El middleware guarda el identificador en una clave privada de httpapi, así
// que se copia a la clave pública de clients.
func contextoDependencias(r *http.Request) context.Context {
	ctx := r.Context()
	if id := requestIDDe(ctx); id != "" {
		return clients.ConRequestID(ctx, id)
	}
	return ctx
}

// responderErrorMID traduce los errores del MID al formato común del clúster y
// registra los fallos internos con el request id, sin volcar cuerpos.
func responderErrorMID(w http.ResponseWriter, r *http.Request, err error, conductorID string) {
	estado, cuerpo := mapearErrorMID(err, conductorID)
	switch {
	case estado == http.StatusInternalServerError:
		log.Printf("error interno metodo=%s ruta=%s request_id=%s err=%s",
			r.Method, sanitizarParaLog(r.URL.Path), requestIDDe(r.Context()),
			sanitizarParaLog(err.Error()))
	case estado == http.StatusServiceUnavailable:
		log.Printf("dependencia no disponible servicio=%v metodo=%s ruta=%s request_id=%s",
			cuerpo.Detalle["servicio"], r.Method, sanitizarParaLog(r.URL.Path),
			requestIDDe(r.Context()))
	}
	responderJSON(w, estado, cuerpo)
}

func mapearErrorMID(err error, conductorID string) (int, errorRespuesta) {
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

	var dependencia *mid.ErrorDependencia
	if errors.As(err, &dependencia) {
		mensaje := "Una dependencia del MID no está disponible"
		switch dependencia.Servicio {
		case mid.ServicioReputacion:
			mensaje = "El servicio de reputación no está disponible"
		case mid.ServicioRutas:
			mensaje = "El servicio de rutas no está disponible"
		}
		return http.StatusServiceUnavailable, errorRespuesta{
			Codigo:  codigoDependenciaNoDisponible,
			Mensaje: mensaje,
			Detalle: map[string]any{"servicio": dependencia.Servicio},
		}
	}

	if errors.Is(err, mid.ErrConductorNoEncontrado) {
		detalle := map[string]any{}
		if conductorID != "" {
			detalle["conductorId"] = conductorID
		}
		return http.StatusNotFound, errorRespuesta{
			Codigo:  codigoNoEncontrada,
			Mensaje: "El perfil del conductor no existe",
			Detalle: detalle,
		}
	}

	return http.StatusInternalServerError, errorRespuesta{
		Codigo:  codigoErrorInterno,
		Mensaje: "No fue posible completar la solicitud",
		Detalle: map[string]any{},
	}
}
