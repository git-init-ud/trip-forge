package clients

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

func TestORSReintentoCacheYCabeceras(t *testing.T) {
	var llamadas atomic.Int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/directions/driving-car/geojson" || r.Header.Get("Authorization") != "clave-de-prueba" || r.Method != http.MethodPost {
			t.Errorf("petición vial inesperada: %s %s", r.Method, r.URL.Path)
		}
		if llamadas.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"type":"FeatureCollection","features":[{"geometry":{"type":"LineString","coordinates":[[-74.06,4.64],[-74.05,4.66],[-74.04,4.70]]}}]}`)
	}))
	defer servidor.Close()
	cliente, err := NuevoClienteORS(servidor.URL, "clave-de-prueba", "driving-car", time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	origen := rutas.Punto{Type: "Point", Coordinates: []float64{-74.06, 4.64}}
	destino := rutas.Punto{Type: "Point", Coordinates: []float64{-74.04, 4.70}}
	trazado, err := cliente.Generar(context.Background(), origen, destino)
	if err != nil || len(trazado.Coordinates) != 3 {
		t.Fatalf("trazado=%+v, err=%v", trazado, err)
	}
	trazado.Coordinates[0][0] = 0
	servidor.Close() // La caché debe servir el trazado aunque el proveedor caiga.
	cacheado, err := cliente.Generar(context.Background(), origen, destino)
	if err != nil || cacheado.Coordinates[0][0] != -74.06 || llamadas.Load() != 2 {
		t.Fatalf("caché o copia incorrecta: %+v, llamadas=%d, err=%v", cacheado, llamadas.Load(), err)
	}
}

func TestORSSinTrayectoYDegradacion(t *testing.T) {
	origen := rutas.Punto{Type: "Point", Coordinates: []float64{-74.06, 4.64}}
	destino := rutas.Punto{Type: "Point", Coordinates: []float64{-74.04, 4.70}}
	for _, caso := range []struct {
		nombre   string
		estado   int
		esperado error
		llamadas int
	}{
		{"sin trayecto", 422, ErrSinRutaVial, 1},
		{"solicitud rechazada", 400, ErrNoDisponible, 1},
		{"clave inválida", 401, ErrNoDisponible, 1},
		{"proveedor caído", 503, ErrNoDisponible, 2},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			var llamadas atomic.Int32
			servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { llamadas.Add(1); w.WriteHeader(caso.estado) }))
			defer servidor.Close()
			cliente, err := NuevoClienteORS(servidor.URL, "clave", "driving-car", time.Second, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			_, err = cliente.Generar(context.Background(), origen, destino)
			if !errors.Is(err, caso.esperado) || int(llamadas.Load()) != caso.llamadas {
				t.Fatalf("err=%v, llamadas=%d", err, llamadas.Load())
			}
		})
	}
}

func TestORSRespetaContextoYClaveAusente(t *testing.T) {
	origen := rutas.Punto{Type: "Point", Coordinates: []float64{-74.06, 4.64}}
	destino := rutas.Punto{Type: "Point", Coordinates: []float64{-74.04, 4.70}}
	cliente, err := NuevoClienteORS("https://api.openrouteservice.org", "", "driving-car", time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cliente.Generar(context.Background(), origen, destino)
	if !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("sin clave: %v", err)
	}
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer servidor.Close()
	cliente, err = NuevoClienteORS(servidor.URL, "clave", "driving-car", 30*time.Millisecond, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cliente.Generar(context.Background(), origen, destino)
	if !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("plazo: %v", err)
	}
}
