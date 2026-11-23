package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/mid"
	"github.com/go-chi/chi/v5"
)

// RouterMID arma el API HTTP del MID: salud, búsqueda de rutas compatibles y
// panel del conductor. El orden del middleware es el mismo que RouterRutas:
// request id, registro, CORS, secreto del gateway y recuperación de pánicos.
func RouterMID(servicio *mid.Servicio, generadores ...clients.GeneradorVial) http.Handler {
	var generador clients.GeneradorVial
	if len(generadores) > 0 {
		generador = generadores[0]
	}
	r := chi.NewRouter()
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

	r.Get("/health", salud("rutas-mid"))
	r.Get("/rutas/compatibles", buscarRutasCompatibles(servicio))
	r.Get("/conductores/{id}/panel", panelConductor(servicio))
	r.Post("/interno/trazados", generarTrazado(generador))
	return r
}

// ServirMID arranca el MID de rutas y se apaga de forma ordenada cuando el
// contexto se cancela.
func ServirMID(ctx context.Context, servicio *mid.Servicio, generadores ...clients.GeneradorVial) error {
	addr := direccion("127.0.0.1:8082")
	server := &http.Server{
		Addr:              addr,
		Handler:           RouterMID(servicio, generadores...),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errores := make(chan error, 1)
	go func() {
		errores <- server.ListenAndServe()
	}()
	log.Printf("rutas-mid listening on %s", addr)

	select {
	case err := <-errores:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		cierre, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelar()
		return server.Shutdown(cierre)
	}
}
