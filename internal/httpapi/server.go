// Package httpapi contains the HTTP bootstrap shared by both services.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/git-init-ud/trip-forge/internal/clients"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

func salud(service string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"service": service, "status": "ok"})
	}
}

// router arma un router mínimo de salud para las pruebas del paquete. Los
// procesos usan RouterRutas y RouterMID.
func router(service string) http.Handler {
	r := chi.NewRouter()
	// requestID va antes de CORS para que las respuestas reales y el preflight
	// lleven X-Request-Id; CORS responde el preflight antes de chi.
	r.Use(requestID)
	r.Use(cors)
	r.Get("/health", salud(service))
	return r
}

func direccion(defecto string) string {
	if addr := os.Getenv("HTTP_ADDR"); addr != "" {
		return addr
	}
	return defecto
}

// ServirRutas arranca el CRUD de rutas con su almacén y se apaga de forma
// ordenada cuando el contexto se cancela.
func ServirRutas(ctx context.Context, almacen AlmacenRutas, generadores ...clients.GeneradorVial) error {
	addr := direccion("127.0.0.1:8081")
	server := &http.Server{
		Addr:              addr,
		Handler:           RouterRutas(almacen, generadores...),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errores := make(chan error, 1)
	go func() {
		errores <- server.ListenAndServe()
	}()
	log.Printf("rutas-crud listening on %s", addr)

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
