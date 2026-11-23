package clients

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

func rutaDePrueba(id, conductor string) rutas.Ruta {
	return rutas.Ruta{
		ID:          id,
		ConductorID: conductor,
		Origen: rutas.Lugar{
			Nombre: "Chapinero",
			Punto:  rutas.Punto{Type: rutas.TipoPunto, Coordinates: []float64{-74.0645, 4.6486}},
		},
		Destino: rutas.Lugar{
			Nombre: "Universidad",
			Punto:  rutas.Punto{Type: rutas.TipoPunto, Coordinates: []float64{-74.0415, 4.7012}},
		},
		Trazado: rutas.LineString{
			Type: rutas.TipoLineString,
			Coordinates: [][]float64{
				{-74.0645, 4.6486},
				{-74.0415, 4.7012},
			},
		},
		Salida:           time.Date(2030, 1, 15, 6, 30, 0, 0, time.UTC),
		CuposTotales:     3,
		CuposDisponibles: 2,
		PrecioTotal:      12000,
		Estado:           rutas.Abierta,
	}
}

func escribirJSON(t *testing.T, w http.ResponseWriter, estado int, cuerpo any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(estado)
	if err := json.NewEncoder(w).Encode(cuerpo); err != nil {
		t.Errorf("escribir la respuesta: %v", err)
	}
}

func TestListarRutasPropagaFiltrosYCabeceras(t *testing.T) {
	var rutaRecibida string
	var consultaRecibida map[string]string
	var cabeceras http.Header

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rutaRecibida = r.URL.Path
		consultaRecibida = map[string]string{}
		for clave := range r.URL.Query() {
			consultaRecibida[clave] = r.URL.Query().Get(clave)
		}
		cabeceras = r.Header.Clone()
		if acepta := r.Header.Get("Accept"); acepta != "application/json" {
			t.Errorf("Accept = %q", acepta)
		}
		escribirJSON(t, w, http.StatusOK, PaginaRutas{
			Items:  []rutas.Ruta{rutaDePrueba("ruta-uno", "usr-12")},
			Total:  1,
			Limit:  100,
			Offset: 0,
		})
	}))
	defer servidor.Close()

	cliente, err := NuevoClienteRutas(servidor.URL, 2*time.Second, "secreto-cluster")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}

	desde := time.Date(2030, 1, 15, 6, 0, 0, 0, time.UTC)
	hasta := time.Date(2030, 1, 15, 9, 0, 0, 0, time.UTC)
	ctx := ConRequestID(context.Background(), "req-7")
	pagina, err := cliente.ListarRutas(ctx, FiltrosRutas{
		ConductorID: "usr-12",
		Estado:      string(rutas.Abierta),
		Desde:       &desde,
		Hasta:       &hasta,
		Limit:       100,
		Offset:      20,
	})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if rutaRecibida != "/rutas" {
		t.Fatalf("ruta = %q", rutaRecibida)
	}
	esperada := map[string]string{
		"conductorId": "usr-12",
		"estado":      "abierta",
		"desde":       "2030-01-15T06:00:00Z",
		"hasta":       "2030-01-15T09:00:00Z",
		"limit":       "100",
		"offset":      "20",
	}
	for clave, valor := range esperada {
		if consultaRecibida[clave] != valor {
			t.Fatalf("query %s = %q, se esperaba %q (%v)", clave, consultaRecibida[clave], valor, consultaRecibida)
		}
	}
	if cabeceras.Get("X-Request-Id") != "req-7" {
		t.Fatalf("X-Request-Id = %q", cabeceras.Get("X-Request-Id"))
	}
	if cabeceras.Get("X-Cluster-Secret") != "secreto-cluster" {
		t.Fatalf("X-Cluster-Secret = %q", cabeceras.Get("X-Cluster-Secret"))
	}
	if len(pagina.Items) != 1 || pagina.Items[0].ID != "ruta-uno" || pagina.Total != 1 {
		t.Fatalf("página inesperada: %+v", pagina)
	}
}

func TestListarRutasSinSecretoNoEnviaCabecera(t *testing.T) {
	var secreto string
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secreto = r.Header.Get("X-Cluster-Secret")
		escribirJSON(t, w, http.StatusOK, PaginaRutas{Items: []rutas.Ruta{}, Total: 0, Limit: 20, Offset: 0})
	}))
	defer servidor.Close()

	cliente, err := NuevoClienteRutas(servidor.URL, time.Second, "")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	if _, err := cliente.ListarRutas(context.Background(), FiltrosRutas{}); err != nil {
		t.Fatalf("listar: %v", err)
	}
	if secreto != "" {
		t.Fatalf("no se esperaba X-Cluster-Secret: %q", secreto)
	}
}

func TestListarRutasErroresTipados(t *testing.T) {
	casos := []struct {
		nombre   string
		estado   int
		cuerpo   string
		esperado error
	}{
		{"servidor caído", http.StatusInternalServerError, `{"codigo":"error_interno"}`, ErrNoDisponible},
		{"servicio no disponible", http.StatusServiceUnavailable, ``, ErrNoDisponible},
		{"petición inválida inesperada", http.StatusBadRequest, `{"codigo":"solicitud_invalida"}`, ErrRespuestaInesperada},
		{"recurso inesperado", http.StatusNotFound, `{"codigo":"no_encontrada"}`, ErrRespuestaInesperada},
		{"JSON inválido", http.StatusOK, `{"items":`, ErrNoDisponible},
		{"página negativa", http.StatusOK, `{"items":[],"total":-1,"limit":20,"offset":0}`, ErrNoDisponible},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(caso.estado)
				_, _ = w.Write([]byte(caso.cuerpo))
			}))
			defer servidor.Close()

			cliente, err := NuevoClienteRutas(servidor.URL, time.Second, "")
			if err != nil {
				t.Fatalf("construir el cliente: %v", err)
			}
			_, err = cliente.ListarRutas(context.Background(), FiltrosRutas{})
			if !errors.Is(err, caso.esperado) {
				t.Fatalf("error = %v, se esperaba %v", err, caso.esperado)
			}
		})
	}
}

func TestListarRutasRespetaElPlazo(t *testing.T) {
	liberar := make(chan struct{})
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-liberar
	}))
	defer servidor.Close()
	defer close(liberar)

	cliente, err := NuevoClienteRutas(servidor.URL, 20*time.Millisecond, "")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	if _, err := cliente.ListarRutas(context.Background(), FiltrosRutas{}); !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("error = %v, se esperaba ErrNoDisponible", err)
	}
}

func TestListarTodasPaginaHastaCubrirTotal(t *testing.T) {
	var peticiones int32
	var mutex sync.Mutex
	var limites, offsets []string

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&peticiones, 1)
		limite := r.URL.Query().Get("limit")
		desplazamiento := r.URL.Query().Get("offset")
		mutex.Lock()
		limites = append(limites, limite)
		offsets = append(offsets, desplazamiento)
		mutex.Unlock()

		items := []rutas.Ruta{}
		switch desplazamiento {
		case "0", "":
			for indice := 0; indice < 100; indice++ {
				items = append(items, rutaDePrueba("ruta-"+string(rune('a'+indice%26)), "usr-12"))
			}
		case "100":
			for indice := 0; indice < 50; indice++ {
				items = append(items, rutaDePrueba("ruta-resto-"+string(rune('a'+indice%26)), "usr-12"))
			}
		}
		escribirJSON(t, w, http.StatusOK, PaginaRutas{Items: items, Total: 150, Limit: 100, Offset: 0})
	}))
	defer servidor.Close()

	cliente, err := NuevoClienteRutas(servidor.URL, 2*time.Second, "")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	todas, err := cliente.ListarTodas(context.Background(), FiltrosRutas{Estado: "abierta"})
	if err != nil {
		t.Fatalf("listar todas: %v", err)
	}
	if len(todas) != 150 {
		t.Fatalf("rutas = %d, se esperaban 150", len(todas))
	}
	if peticiones != 2 {
		t.Fatalf("peticiones = %d, se esperaban 2", peticiones)
	}
	mutex.Lock()
	defer mutex.Unlock()
	for indice, limite := range limites {
		if limite != "100" {
			t.Fatalf("petición %d: limit = %q", indice, limite)
		}
	}
	if offsets[0] != "" || offsets[1] != "100" {
		t.Fatalf("offsets = %v", offsets)
	}
}

func TestListarTodasFallaAlSuperarElTopeDePaginas(t *testing.T) {
	var peticiones int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&peticiones, 1)
		items := make([]rutas.Ruta, 100)
		for indice := range items {
			items[indice] = rutaDePrueba("ruta-"+string(rune('a'+indice%26)), "usr-12")
		}
		escribirJSON(t, w, http.StatusOK, PaginaRutas{Items: items, Total: 1000000, Limit: 100, Offset: 0})
	}))
	defer servidor.Close()

	cliente, err := NuevoClienteRutas(servidor.URL, 2*time.Second, "")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	if _, err := cliente.ListarTodas(context.Background(), FiltrosRutas{}); !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("error = %v, se esperaba ErrNoDisponible", err)
	}
	if peticiones != MaximoPaginas {
		t.Fatalf("peticiones = %d, se esperaban %d", peticiones, MaximoPaginas)
	}
}

func TestNuevoClienteRutasValidaConfiguracion(t *testing.T) {
	casos := []struct {
		nombre string
		base   string
		plazo  time.Duration
	}{
		{"URL vacía", "", time.Second},
		{"URL relativa", "/rutas", time.Second},
		{"esquema no HTTP", "ftp://127.0.0.1:8081", time.Second},
		{"plazo cero", "http://127.0.0.1:8081", 0},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if _, err := NuevoClienteRutas(caso.base, caso.plazo, ""); err == nil {
				t.Fatal("se esperaba un error de configuración")
			}
		})
	}
}
