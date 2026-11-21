// Package httpapi contains the HTTP bootstrap shared by both services.
package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

func router(service string) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"service": service, "status": "ok"})
	})
	return r
}

func Serve(service, defaultAddr string) error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           router(service),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("%s listening on %s", service, addr)
	return server.ListenAndServe()
}
