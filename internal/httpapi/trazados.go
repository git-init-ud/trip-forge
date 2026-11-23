package httpapi

import (
	"errors"
	"log"
	"net/http"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

type solicitudTrazado struct {
	Origen  rutas.Punto `json:"origen"`
	Destino rutas.Punto `json:"destino"`
}

func generarTrazado(generador clients.GeneradorVial) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var solicitud solicitudTrazado
		if err := decodificar(w, r, &solicitud); err != nil {
			responderErrorMID(w, r, err, "")
			return
		}
		if err := rutas.ValidarPunto("origen", solicitud.Origen); err != nil {
			responderErrorMID(w, r, err, "")
			return
		}
		if err := rutas.ValidarPunto("destino", solicitud.Destino); err != nil {
			responderErrorMID(w, r, err, "")
			return
		}
		if generador == nil {
			responderVialNoDisponible(w, r)
			return
		}
		trazado, err := generador.Generar(contextoDependencias(r), solicitud.Origen, solicitud.Destino)
		if errors.Is(err, clients.ErrSinRutaVial) {
			responderJSON(w, http.StatusUnprocessableEntity, errorRespuesta{Codigo: "ruta_vial_no_encontrada", Mensaje: "No se encontró un trayecto vial entre origen y destino", Detalle: map[string]any{}})
			return
		}
		if err != nil {
			responderVialNoDisponible(w, r)
			return
		}
		if err := rutas.ValidarTrazado(trazado); err != nil {
			responderVialNoDisponible(w, r)
			return
		}
		responderJSON(w, http.StatusOK, trazado)
	}
}

func responderVialNoDisponible(w http.ResponseWriter, r *http.Request) {
	log.Printf("dependencia no disponible servicio=openrouteservice metodo=%s ruta=%s request_id=%s", r.Method, r.URL.Path, requestIDDe(r.Context()))
	responderJSON(w, http.StatusServiceUnavailable, errorRespuesta{Codigo: codigoDependenciaNoDisponible, Mensaje: "El proveedor vial no está disponible", Detalle: map[string]any{"servicio": "openrouteservice"}})
}
