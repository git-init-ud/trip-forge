package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Estas pruebas comprueban que cada router conecta el middleware CORS.
// Las reglas del middleware se prueban por separado en cors_test.go.
func TestRouterRutasPermiteSwagger(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "http://127.0.0.1:8090")
	manejador := RouterRutas(&almacenFalso{})

	preflight := httptest.NewRequest(http.MethodOptions, "/rutas", nil)
	preflight.Header.Set("Origin", "http://127.0.0.1:8090")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "content-type")
	respuesta := httptest.NewRecorder()
	manejador.ServeHTTP(respuesta, preflight)
	if respuesta.Code != http.StatusNoContent || respuesta.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:8090" {
		t.Fatalf("preflight: estado=%d origen=%q", respuesta.Code, respuesta.Header().Get("Access-Control-Allow-Origin"))
	}
	if respuesta.Header().Get("X-Request-Id") == "" {
		t.Fatal("el preflight debe llevar X-Request-Id")
	}

	publicacion := httptest.NewRequest(http.MethodPost, "/rutas", strings.NewReader(mustJSON(t, cuerpoCreacion())))
	publicacion.Header.Set("Origin", "http://127.0.0.1:8090")
	publicacion.Header.Set("Content-Type", "application/json")
	respuesta = httptest.NewRecorder()
	manejador.ServeHTTP(respuesta, publicacion)
	if respuesta.Code != http.StatusCreated || respuesta.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:8090" {
		t.Fatalf("POST: estado=%d origen=%q cuerpo=%s", respuesta.Code, respuesta.Header().Get("Access-Control-Allow-Origin"), respuesta.Body.String())
	}
	if respuesta.Header().Get("Location") == "" || !strings.Contains(respuesta.Header().Get("Access-Control-Expose-Headers"), "Location") {
		t.Fatal("el navegador debe poder leer Location en la respuesta 201")
	}
}

func TestRouterMIDPermiteSwagger(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "http://127.0.0.1:8090")
	manejador := router("rutas-mid")

	preflight := httptest.NewRequest(http.MethodOptions, "/health", nil)
	preflight.Header.Set("Origin", "http://127.0.0.1:8090")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	respuesta := httptest.NewRecorder()
	manejador.ServeHTTP(respuesta, preflight)
	if respuesta.Code != http.StatusNoContent || respuesta.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:8090" {
		t.Fatalf("preflight: estado=%d origen=%q", respuesta.Code, respuesta.Header().Get("Access-Control-Allow-Origin"))
	}
	if respuesta.Header().Get("X-Request-Id") == "" {
		t.Fatal("el preflight debe llevar X-Request-Id")
	}

	consulta := httptest.NewRequest(http.MethodGet, "/health", nil)
	consulta.Header.Set("Origin", "http://127.0.0.1:8090")
	respuesta = httptest.NewRecorder()
	manejador.ServeHTTP(respuesta, consulta)
	if respuesta.Code != http.StatusOK || respuesta.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:8090" {
		t.Fatalf("GET: estado=%d origen=%q", respuesta.Code, respuesta.Header().Get("Access-Control-Allow-Origin"))
	}
	if respuesta.Header().Get("X-Request-Id") == "" {
		t.Fatal("la respuesta debe llevar X-Request-Id")
	}
}
