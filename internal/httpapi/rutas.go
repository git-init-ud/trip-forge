package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
	"github.com/go-chi/chi/v5"
)

// AlmacenRutas es la persistencia que necesita el CRUD de rutas. El paquete
// mongorutas la implementa; las pruebas usan un doble en memoria.
type AlmacenRutas interface {
	Listar(ctx context.Context, filtro rutas.FiltroListado) (rutas.Listado, error)
	Crear(ctx context.Context, ruta rutas.Ruta) error
	Obtener(ctx context.Context, id string) (rutas.Ruta, error)
	Actualizar(ctx context.Context, id string, cambios rutas.ActualizarRuta, identidad string) (rutas.Ruta, error)
	Cerrar(ctx context.Context, id, identidad string) error
	AjustarCupos(ctx context.Context, id string, delta int) (rutas.Ruta, error)
}

// RouterRutas arma el API HTTP del CRUD: salud, listado, publicación, detalle,
// edición, cierre y ajuste de cupos.
func RouterRutas(almacen AlmacenRutas) http.Handler {
	r := chi.NewRouter()
	// CORS va antes del recuperador y de chi para responder el preflight del
	// visor OpenAPI; requestID y el registro también cubren esa respuesta.
	r.Use(requestID)
	r.Use(registrarSolicitudes)
	r.Use(cors)
	r.Use(secretoGateway)
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
		r.Get("/", listarRutas(almacen))
		r.Post("/", crearRuta(almacen))
		r.Get("/{id}", obtenerRuta(almacen))
		r.Put("/{id}", actualizarRuta(almacen))
		r.Delete("/{id}", cerrarRuta(almacen))
		r.Patch("/{id}/cupos", ajustarCupos(almacen))
	})
	return r
}

// listadoRespuesta es la forma paginada que declara el contrato.
type listadoRespuesta struct {
	Items  []rutas.Ruta `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

func listarRutas(almacen AlmacenRutas) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filtro, err := leerFiltroListado(r)
		if err != nil {
			responderError(w, r, err, "")
			return
		}
		listado, err := almacen.Listar(r.Context(), filtro)
		if err != nil {
			responderError(w, r, err, "")
			return
		}
		items := listado.Items
		if items == nil {
			items = []rutas.Ruta{}
		}
		responderJSON(w, http.StatusOK, listadoRespuesta{
			Items:  items,
			Total:  listado.Total,
			Limit:  filtro.Limit,
			Offset: filtro.Offset,
		})
	}
}

func crearRuta(almacen AlmacenRutas) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identidad := usuarioAutenticado(r)
		// Con el gateway activo la identidad es obligatoria antes de leer el
		// cuerpo. Sin GATEWAY_SECRET se conserva la compatibilidad local: la
		// cabecera es opcional, pero si llega debe ser la del conductor.
		if gatewayActivo() && identidad == "" {
			responderError(w, r, rutas.ErrNoAutorizado, "")
			return
		}
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
		if identidad != "" && identidad != strings.TrimSpace(entrada.ConductorID) {
			responderError(w, r, rutas.ErrNoAutorizado, "")
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

func actualizarRuta(almacen AlmacenRutas) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		identidad := usuarioAutenticado(r)
		if identidad == "" {
			responderError(w, r, rutas.ErrNoAutorizado, id)
			return
		}
		var solicitud actualizarRutaSolicitud
		if err := decodificar(w, r, &solicitud); err != nil {
			responderError(w, r, err, id)
			return
		}
		cambios, err := solicitud.aDominio()
		if err != nil {
			responderError(w, r, err, id)
			return
		}
		if err := cambios.Validar(time.Now()); err != nil {
			responderError(w, r, err, id)
			return
		}
		ruta, err := almacen.Actualizar(r.Context(), id, cambios, identidad)
		if err != nil {
			responderError(w, r, err, id)
			return
		}
		responderJSON(w, http.StatusOK, ruta)
	}
}

func cerrarRuta(almacen AlmacenRutas) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		identidad := usuarioAutenticado(r)
		if identidad == "" {
			responderError(w, r, rutas.ErrNoAutorizado, id)
			return
		}
		if err := almacen.Cerrar(r.Context(), id, identidad); err != nil {
			responderError(w, r, err, id)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
