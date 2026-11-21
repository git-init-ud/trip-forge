// Comando docs sirve la documentación del repositorio en un navegador.
//
// Expone el sitio estático de docs/site, los dos contratos OpenAPI de api/ y
// una lista cerrada de documentos Markdown. Está pensado para desarrollo
// local: escucha en loopback y no requiere dependencias externas en Go.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var documentos = map[string]string{
	"README.md":            "README.md",
	"PROPUESTA-GRUPO-A.md": "PROPUESTA-GRUPO-A.md",
	"AGENTS.md":            "AGENTS.md",
	"X1.cluster-subete.md": "X1.cluster-subete.md",
}

var especificaciones = map[string]bool{
	"rutas-crud.openapi.yaml": true,
	"rutas-mid.openapi.yaml":  true,
}

func main() {
	if err := ejecutar(); err != nil {
		log.Fatalf("docs: %v", err)
	}
}

func ejecutar() error {
	raiz, err := raizProyecto()
	if err != nil {
		return err
	}
	direccion := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if direccion == "" {
		direccion = "127.0.0.1:8090"
	}
	servidor := &http.Server{
		Addr:              direccion,
		Handler:           manejador(raiz),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("documentación en http://%s", direccion)
	return servidor.ListenAndServe()
}

// raizProyecto busca hacia arriba la carpeta que contiene docs/site/index.html.
// DOCS_ROOT permite fijarla cuando el comando se ejecuta desde otro directorio.
func raizProyecto() (string, error) {
	if fijada := strings.TrimSpace(os.Getenv("DOCS_ROOT")); fijada != "" {
		return filepath.Abs(fijada)
	}
	directorio, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("obtener el directorio actual: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directorio, "docs", "site", "index.html")); err == nil {
			return directorio, nil
		}
		padre := filepath.Dir(directorio)
		if padre == directorio {
			return "", errors.New("no se encontró docs/site/index.html; define DOCS_ROOT")
		}
		directorio = padre
	}
}

func manejador(raiz string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"service": "docs", "status": "ok"})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		nombre := strings.TrimPrefix(r.URL.Path, "/api/")
		if !especificaciones[nombre] {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		http.ServeFile(w, r, filepath.Join(raiz, "api", nombre))
	})
	mux.HandleFunc("/md/", func(w http.ResponseWriter, r *http.Request) {
		nombre := strings.TrimPrefix(r.URL.Path, "/md/")
		archivo, existe := documentos[nombre]
		if !existe {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		http.ServeFile(w, r, filepath.Join(raiz, archivo))
	})
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(raiz, "docs", "site"))))
	return mux
}
