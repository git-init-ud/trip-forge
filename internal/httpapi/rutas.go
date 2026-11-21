package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
	"github.com/go-chi/chi/v5"
)

// AlmacenRutas es la persistencia que necesita el CRUD de rutas. El paquete
// mongorutas la implementa; las pruebas usan un doble en memoria.
type AlmacenRutas interface {
	Crear(ctx context.Context, ruta rutas.Ruta) error
	Obtener(ctx context.Context, id string) (rutas.Ruta, error)
	AjustarCupos(ctx context.Context, id string, delta int) (rutas.Ruta, error)
}

// RouterRutas arma el API HTTP del CRUD: salud, publicación, detalle y ajuste
// de cupos.
func RouterRutas(almacen AlmacenRutas) http.Handler {
	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(registrarSolicitudes)
	r.Use(recuperar)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		responderJSON(w, http.StatusNotFound, errorRespuesta{
			Codigo:  codigoNoEncontrada,
			Mensaje: "El recurso no existe",
			Detalle: map[string]any{},
		})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		responderJSON(w, http.StatusMethodNotAllowed, errorRespuesta{
			Codigo:  codigoSolicitudInvalida,
			Mensaje: "Método no permitido para este recurso",
			Detalle: map[string]any{"metodo": r.Method},
		})
	})

	r.Get("/health", salud("rutas-crud"))
	r.Route("/rutas", func(r chi.Router) {
		r.Post("/", crearRuta(almacen))
		r.Get("/{id}", obtenerRuta(almacen))
		r.Patch("/{id}/cupos", ajustarCupos(almacen))
	})
	return r
}

func crearRuta(almacen AlmacenRutas) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var solicitud crearRutaSolicitud
		if err := decodificar(w, r, &solicitud); err != nil {
			responderError(w, r, err, "")
			return
		}
		entrada, err := solicitud.aDominio()
		if err != nil {
			responderError(w, r, err, "")
			return
		}
		ruta, err := rutas.Nueva(entrada, time.Now())
		if err != nil {
			responderError(w, r, err, "")
			return
		}
		if err := almacen.Crear(r.Context(), ruta); err != nil {
			responderError(w, r, err, ruta.ID)
			return
		}
		w.Header().Set("Location", "/rutas/"+ruta.ID)
		responderJSON(w, http.StatusCreated, ruta)
	}
}

func obtenerRuta(almacen AlmacenRutas) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ruta, err := almacen.Obtener(r.Context(), id)
		if err != nil {
			responderError(w, r, err, id)
			return
		}
		responderJSON(w, http.StatusOK, ruta)
	}
}

func ajustarCupos(almacen AlmacenRutas) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var solicitud ajusteCuposSolicitud
		if err := decodificar(w, r, &solicitud); err != nil {
			responderError(w, r, err, id)
			return
		}
		if solicitud.Delta == nil {
			responderError(w, r, &rutas.ErrorValidacion{Campo: "delta", Motivo: "es obligatorio"}, id)
			return
		}
		if err := rutas.ValidarDelta(*solicitud.Delta); err != nil {
			responderError(w, r, err, id)
			return
		}
		ruta, err := almacen.AjustarCupos(r.Context(), id, *solicitud.Delta)
		if err != nil {
			responderError(w, r, err, id)
			return
		}
		responderJSON(w, http.StatusOK, ruta)
	}
}
