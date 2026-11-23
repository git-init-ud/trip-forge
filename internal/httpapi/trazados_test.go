package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

func TestPublicacionGeneradaPasaPorMIDYORS(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "secreto-interno")
	var solicitudesORS int
	ors := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		solicitudesORS++
		if r.Header.Get("Authorization") != "clave" || r.Method != http.MethodPost {
			t.Errorf("cabeceras ORS incorrectas")
		}
		var cuerpo struct {
			Coordinates [][]float64 `json:"coordinates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
			t.Error(err)
		}
		if len(cuerpo.Coordinates) != 2 || cuerpo.Coordinates[0][0] != -74.0645 {
			t.Errorf("coordenadas ORS: %+v", cuerpo.Coordinates)
		}
		fmt.Fprint(w, `{"features":[{"geometry":{"type":"LineString","coordinates":[[-74.0645,4.6486],[-74.0600,4.6650],[-74.0415,4.7012]]}}]}`)
	}))
	defer ors.Close()
	proveedor, err := clients.NuevoClienteORS(ors.URL, "clave", "driving-car", time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	midServer := httptest.NewServer(RouterMID(nil, proveedor))
	defer midServer.Close()
	generador, err := clients.NuevoClienteGeneradorMID(midServer.URL, "secreto-interno", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var guardada rutas.Ruta
	crud := RouterRutas(&almacenFalso{crear: func(_ context.Context, ruta rutas.Ruta) error { guardada = ruta; return nil }}, generador)
	cuerpo := cuerpoCreacion()
	delete(cuerpo, "trazado")
	cuerpo["generarTrazado"] = true
	datos, _ := json.Marshal(cuerpo)
	peticion := httptest.NewRequest(http.MethodPost, "/rutas", bytes.NewReader(datos))
	peticion.Header.Set("Content-Type", "application/json")
	peticion.Header.Set("X-Cluster-Secret", "secreto-interno")
	peticion.Header.Set("X-Usuario-Id", "usr-12")
	peticion.Header.Set("X-Request-Id", "publicacion-123")
	respuesta := httptest.NewRecorder()
	crud.ServeHTTP(respuesta, peticion)
	if respuesta.Code != 201 || len(guardada.Trazado.Coordinates) != 3 || solicitudesORS != 1 {
		t.Fatalf("estado=%d, guardada=%+v, ORS=%d, cuerpo=%s", respuesta.Code, guardada, solicitudesORS, respuesta.Body.String())
	}
}

func TestGeneracionInvalidaNoLlamaProveedor(t *testing.T) {
	var llamadas int
	proveedor := generadorFalso{generar: func(context.Context, rutas.Punto, rutas.Punto) (rutas.LineString, error) {
		llamadas++
		return rutas.LineString{}, nil
	}}
	crud := RouterRutas(&almacenFalso{}, proveedor)
	cuerpo := cuerpoCreacion()
	cuerpo["generarTrazado"] = true
	cuerpo["precioTotal"] = -1
	datos, _ := json.Marshal(cuerpo)
	peticion := httptest.NewRequest(http.MethodPost, "/rutas", bytes.NewReader(datos))
	peticion.Header.Set("Content-Type", "application/json")
	respuesta := httptest.NewRecorder()
	crud.ServeHTTP(respuesta, peticion)
	if respuesta.Code != 400 || llamadas != 0 {
		t.Fatalf("estado=%d llamadas=%d", respuesta.Code, llamadas)
	}
}

func TestGeneracionReportaFalloSinGuardar(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		fallo  error
		estado int
	}{
		{"sin trayecto", clients.ErrSinRutaVial, 422},
		{"proveedor caído", clients.ErrNoDisponible, 503},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			var guardadas int
			proveedor := generadorFalso{generar: func(context.Context, rutas.Punto, rutas.Punto) (rutas.LineString, error) {
				return rutas.LineString{}, caso.fallo
			}}
			crud := RouterRutas(&almacenFalso{crear: func(context.Context, rutas.Ruta) error { guardadas++; return nil }}, proveedor)
			cuerpo := cuerpoCreacion()
			delete(cuerpo, "trazado")
			cuerpo["generarTrazado"] = true
			datos, _ := json.Marshal(cuerpo)
			peticion := httptest.NewRequest(http.MethodPost, "/rutas", bytes.NewReader(datos))
			peticion.Header.Set("Content-Type", "application/json")
			respuesta := httptest.NewRecorder()
			crud.ServeHTTP(respuesta, peticion)
			if respuesta.Code != caso.estado || guardadas != 0 {
				t.Fatalf("estado=%d guardadas=%d", respuesta.Code, guardadas)
			}
		})
	}
}

func TestMIDGeneracionValidaEntradaYMapeaProveedor(t *testing.T) {
	var llamadas int
	proveedor := generadorFalso{generar: func(context.Context, rutas.Punto, rutas.Punto) (rutas.LineString, error) {
		llamadas++
		return rutas.LineString{}, errors.New("falló")
	}}
	manejador := RouterMID(nil, proveedor)
	for _, caso := range []struct {
		cuerpo string
		estado int
	}{
		{`{"origen":{"type":"Point","coordinates":[-190,4]},"destino":{"type":"Point","coordinates":[-74,4]}}`, 400},
		{`{"origen":{"type":"Point","coordinates":[-74,4]},"destino":{"type":"Point","coordinates":[-74,5]}}`, 503},
	} {
		peticion := httptest.NewRequest(http.MethodPost, "/interno/trazados", bytes.NewBufferString(caso.cuerpo))
		peticion.Header.Set("Content-Type", "application/json")
		respuesta := httptest.NewRecorder()
		manejador.ServeHTTP(respuesta, peticion)
		if respuesta.Code != caso.estado {
			t.Fatalf("estado=%d, esperado=%d", respuesta.Code, caso.estado)
		}
	}
	if llamadas != 1 {
		t.Fatalf("llamadas al proveedor=%d", llamadas)
	}
}

type generadorFalso struct {
	generar func(context.Context, rutas.Punto, rutas.Punto) (rutas.LineString, error)
}

func (g generadorFalso) Generar(ctx context.Context, o, d rutas.Punto) (rutas.LineString, error) {
	return g.generar(ctx, o, d)
}
