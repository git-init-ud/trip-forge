package httpapi

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestCORSPreflightPermitido(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "http://127.0.0.1:8090")
	manejador := cors(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("el preflight no debe llegar al manejador de la API")
	}))

	peticion := httptest.NewRequest(http.MethodOptions, "/rutas", nil)
	peticion.Header.Set("Origin", "http://127.0.0.1:8090")
	peticion.Header.Set("Access-Control-Request-Method", http.MethodPost)
	peticion.Header.Set("Access-Control-Request-Headers", "content-type")
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)

	if grabador.Code != http.StatusNoContent {
		t.Fatalf("estado = %d, se esperaba 204", grabador.Code)
	}
	if origen := grabador.Header().Get("Access-Control-Allow-Origin"); origen != "http://127.0.0.1:8090" {
		t.Fatalf("Access-Control-Allow-Origin = %q", origen)
	}
	if metodos := grabador.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(metodos, http.MethodPost) || !strings.Contains(metodos, http.MethodPatch) {
		t.Fatalf("Access-Control-Allow-Methods = %q", metodos)
	}
	if cabeceras := grabador.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(cabeceras, "Content-Type") || !strings.Contains(cabeceras, "X-Request-Id") {
		t.Fatalf("Access-Control-Allow-Headers = %q", cabeceras)
	}
	if maxAge := grabador.Header().Get("Access-Control-Max-Age"); maxAge != "600" {
		t.Fatalf("Access-Control-Max-Age = %q", maxAge)
	}
	vary := strings.Join(grabador.Header().Values("Vary"), ",")
	for _, esperado := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
		if !strings.Contains(vary, esperado) {
			t.Fatalf("Vary = %q, falta %q", vary, esperado)
		}
	}
}

func TestCORSSolicitudPermitidaExponeCabeceras(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "http://127.0.0.1:8090")
	manejador := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/rutas/ruta-1a2b3c4d5e6f7a8b")
		w.Header().Set("X-Request-Id", "req-desde-swagger")
		w.WriteHeader(http.StatusCreated)
	}))

	peticion := httptest.NewRequest(http.MethodPost, "/rutas", strings.NewReader(`{}`))
	peticion.Header.Set("Origin", "http://127.0.0.1:8090")
	peticion.Header.Set("Content-Type", "application/json")
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)

	if grabador.Code != http.StatusCreated {
		t.Fatalf("estado = %d, se esperaba 201", grabador.Code)
	}
	if origen := grabador.Header().Get("Access-Control-Allow-Origin"); origen != "http://127.0.0.1:8090" {
		t.Fatalf("Access-Control-Allow-Origin = %q", origen)
	}
	expuestas := grabador.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(expuestas, "Location") || !strings.Contains(expuestas, "X-Request-Id") {
		t.Fatalf("Access-Control-Expose-Headers = %q", expuestas)
	}
	if !slices.Contains(grabador.Header().Values("Vary"), "Origin") {
		t.Fatalf("Vary = %v, falta Origin", grabador.Header().Values("Vary"))
	}
}

func TestCORSRechazaPreflightDeOrigenDesconocido(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "http://127.0.0.1:8090")
	manejador := cors(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("un preflight no autorizado no debe llegar al manejador")
	}))

	peticion := httptest.NewRequest(http.MethodOptions, "/rutas", nil)
	peticion.Header.Set("Origin", "https://sitio-no-autorizado.test")
	peticion.Header.Set("Access-Control-Request-Method", http.MethodPost)
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)

	if grabador.Code != http.StatusForbidden {
		t.Fatalf("estado = %d, se esperaba 403", grabador.Code)
	}
	if origen := grabador.Header().Get("Access-Control-Allow-Origin"); origen != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, se esperaba vacío", origen)
	}
	respuesta, _ := decodificarError(t, grabador)
	if respuesta.Codigo != codigoNoAutorizado || respuesta.Mensaje == "" {
		t.Fatalf("el 403 de CORS debe usar el formato común: %+v", respuesta)
	}
}

func TestCORSSolicitudDeOrigenDesconocidoNoSeBloquea(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "http://127.0.0.1:8090")
	llamado := false
	manejador := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		llamado = true
		w.WriteHeader(http.StatusOK)
	}))

	peticion := httptest.NewRequest(http.MethodGet, "/health", nil)
	peticion.Header.Set("Origin", "https://sitio-no-autorizado.test")
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)

	if !llamado || grabador.Code != http.StatusOK {
		t.Fatalf("la solicitud normal debe ejecutarse: llamado=%v estado=%d", llamado, grabador.Code)
	}
	if origen := grabador.Header().Get("Access-Control-Allow-Origin"); origen != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, se esperaba vacío", origen)
	}
}

func TestCORSSinOrigenMantieneVarySinAutorizacion(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "http://127.0.0.1:8090")
	manejador := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, httptest.NewRequest(http.MethodGet, "/health", nil))

	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d", grabador.Code)
	}
	if origen := grabador.Header().Get("Access-Control-Allow-Origin"); origen != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q", origen)
	}
	if !slices.Contains(grabador.Header().Values("Vary"), "Origin") {
		t.Fatalf("Vary = %v, falta Origin", grabador.Header().Values("Vary"))
	}
}

func TestCORSComodinNoSeAcepta(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "*")
	permitidos := origenesCORS()
	if !slices.Equal(permitidos, origenesDocumentacion) {
		t.Fatalf("orígenes = %v, se esperaba el valor por defecto", permitidos)
	}

	manejador := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	peticion := httptest.NewRequest(http.MethodGet, "/health", nil)
	peticion.Header.Set("Origin", "https://visor.example.test")
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)

	if origen := grabador.Header().Get("Access-Control-Allow-Origin"); origen != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, se esperaba vacío", origen)
	}
}

func TestOrigenesCORSPorDefecto(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "")
	permitidos := origenesCORS()

	for _, esperado := range origenesDocumentacion {
		if !slices.Contains(permitidos, esperado) {
			t.Fatalf("orígenes = %v, falta %q", permitidos, esperado)
		}
	}
}

func TestOrigenesCORSNormalizaLista(t *testing.T) {
	t.Setenv("CORS_ORIGINS", " https://docs.example.test/ , http://localhost:9000 ")
	permitidos := origenesCORS()

	if !slices.Equal(permitidos, []string{"https://docs.example.test", "http://localhost:9000"}) {
		t.Fatalf("orígenes = %v", permitidos)
	}
}
