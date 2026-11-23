package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func prepararRaiz(t *testing.T) string {
	t.Helper()
	raiz := t.TempDir()
	if err := os.MkdirAll(filepath.Join(raiz, "docs", "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(raiz, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(raiz, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	archivos := map[string]string{
		"docs/site/index.html":        "<h1>inicio</h1>",
		"docs/site/swagger.html":      "<h1>swagger</h1>",
		"api/rutas-crud.openapi.yaml": "openapi: 3.0.3",
		"api/rutas-mid.openapi.yaml":  "openapi: 3.0.3",
		"README.md":                   "# Hola",
		"AGENTS.md":                   "# Convenciones",
		"X1.cluster-subete.md":        "# Clúster",
		"PROPUESTA-GRUPO-A.md":        "# Propuesta",
	}
	for nombre, contenido := range archivos {
		if err := os.WriteFile(filepath.Join(raiz, filepath.FromSlash(nombre)), []byte(contenido), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return raiz
}

func TestManejador(t *testing.T) {
	servidor := httptest.NewServer(manejador(prepararRaiz(t)))
	t.Cleanup(servidor.Close)

	casos := []struct {
		nombre   string
		ruta     string
		estado   int
		tipo     string
		contiene string
	}{
		{"inicio", "/", http.StatusOK, "text/html", "inicio"},
		{"swagger", "/swagger.html", http.StatusOK, "text/html", "swagger"},
		{"salud", "/health", http.StatusOK, "application/json", `"service":"docs"`},
		{"contrato", "/api/rutas-crud.openapi.yaml", http.StatusOK, "application/yaml", "openapi: 3.0.3"},
		{"contrato MID", "/api/rutas-mid.openapi.yaml", http.StatusOK, "application/yaml", "openapi: 3.0.3"},
		{"markdown", "/md/README.md", http.StatusOK, "text/markdown", "# Hola"},
		{"contrato desconocido", "/api/otro.yaml", http.StatusNotFound, "", ""},
		{"markdown desconocido", "/md/otro.md", http.StatusNotFound, "", ""},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			respuesta, err := http.Get(servidor.URL + caso.ruta)
			if err != nil {
				t.Fatalf("GET %s: %v", caso.ruta, err)
			}
			defer respuesta.Body.Close()
			cuerpo, err := io.ReadAll(respuesta.Body)
			if err != nil {
				t.Fatal(err)
			}
			if respuesta.StatusCode != caso.estado {
				t.Fatalf("estado = %d, se esperaba %d: %s", respuesta.StatusCode, caso.estado, cuerpo)
			}
			if caso.tipo != "" && !strings.HasPrefix(respuesta.Header.Get("Content-Type"), caso.tipo) {
				t.Fatalf("Content-Type = %q, se esperaba %q", respuesta.Header.Get("Content-Type"), caso.tipo)
			}
			if caso.contiene != "" && !strings.Contains(string(cuerpo), caso.contiene) {
				t.Fatalf("cuerpo = %q, se esperaba %q", cuerpo, caso.contiene)
			}
		})
	}
}

// TestSwaggerHabilitaEjecucion protege la página real: el visor debe permitir
// Try it out y apuntar a los dos procesos locales.
func TestSwaggerHabilitaEjecucion(t *testing.T) {
	contenido, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "swagger.html"))
	if err != nil {
		t.Fatal(err)
	}
	pagina := string(contenido)
	if strings.Contains(pagina, "supportedSubmitMethods: []") {
		t.Fatal("la página sigue desactivando la ejecución de operaciones")
	}
	for _, esperado := range []string{
		"tryItOutEnabled: true",
		"http://127.0.0.1:8081",
		"http://127.0.0.1:8082",
		"comprobarServicio",
		"AbortController",
		"cuerpo.status",
		"cuerpo.service",
		"specSelectors.servers()",
	} {
		if !strings.Contains(pagina, esperado) {
			t.Fatalf("falta %q en la página del visor", esperado)
		}
	}
}

// TestEjemploDeSalidaSigueSiendoFuturo evita que el ejemplo ejecutable del
// contrato caduque sin que las pruebas lo detecten.
func TestEjemploDeSalidaSigueSiendoFuturo(t *testing.T) {
	contenido, err := os.ReadFile(filepath.Join("..", "..", "api", "rutas-crud.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	patron := regexp.MustCompile(`salida: '([^']+)'`)
	coincidencia := patron.FindStringSubmatch(string(contenido))
	if len(coincidencia) != 2 {
		t.Fatal("no se encontró el ejemplo de salida en el contrato")
	}
	salida, err := time.Parse(time.RFC3339, coincidencia[1])
	if err != nil {
		t.Fatalf("el ejemplo de salida no es RFC3339: %v", err)
	}
	if !salida.After(time.Now().UTC()) {
		t.Fatalf("el ejemplo de salida %s ya no es futuro; actualízalo para que POST /rutas funcione", salida)
	}
}
