package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/matching"
)

func TestPuntajeConsultaRutaYCabeceras(t *testing.T) {
	var ruta string
	var cabeceras http.Header
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta = r.URL.Path
		cabeceras = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"puntaje": 4.7,
			"insignia": "confiable",
			"viajesCompletados": 12,
			"tasaCancelacion": 0.05,
			"totalCalificaciones": 9,
			"otroCampoDelProveedor": true
		}`))
	}))
	defer servidor.Close()

	cliente, err := NuevoClienteReputacion(servidor.URL, time.Second, "secreto", "token-reputacion")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	ctx := ConRequestID(context.Background(), "req-9")
	reputacion, err := cliente.Puntaje(ctx, "usr-12")
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if ruta != "/perfiles/usr-12/puntaje" {
		t.Fatalf("ruta = %q", ruta)
	}
	if cabeceras.Get("X-Request-Id") != "req-9" || cabeceras.Get("X-Cluster-Secret") != "secreto" {
		t.Fatalf("cabeceras = %v", cabeceras)
	}
	if autorizacion := cabeceras.Get("Authorization"); autorizacion != "Bearer token-reputacion" {
		t.Fatalf("Authorization = %q", autorizacion)
	}
	if reputacion.Puntaje != 4.7 || reputacion.Insignia != matching.InsigniaConfiable {
		t.Fatalf("reputación = %+v", reputacion)
	}
	if reputacion.ViajesCompletados == nil || *reputacion.ViajesCompletados != 12 {
		t.Fatalf("viajesCompletados = %v", reputacion.ViajesCompletados)
	}
	if reputacion.TasaCancelacion == nil || *reputacion.TasaCancelacion != 0.05 {
		t.Fatalf("tasaCancelacion = %v", reputacion.TasaCancelacion)
	}
	if reputacion.TotalCalificaciones == nil || *reputacion.TotalCalificaciones != 9 {
		t.Fatalf("totalCalificaciones = %v", reputacion.TotalCalificaciones)
	}
}

func TestPuntajeOmiteCamposOpcionalesAusentes(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"puntaje":0,"insignia":"nuevo"}`))
	}))
	defer servidor.Close()

	cliente, err := NuevoClienteReputacion(servidor.URL, time.Second, "", "")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	reputacion, err := cliente.Puntaje(context.Background(), "usr-1")
	if err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if reputacion.ViajesCompletados != nil || reputacion.TasaCancelacion != nil || reputacion.TotalCalificaciones != nil {
		t.Fatalf("los opcionales deben quedar ausentes: %+v", reputacion)
	}
}

func TestPuntajeErroresTipados(t *testing.T) {
	casos := []struct {
		nombre   string
		estado   int
		cuerpo   string
		esperado error
	}{
		{"perfil inexistente", http.StatusNotFound, `{"codigo":"no_encontrada"}`, ErrPerfilNoEncontrado},
		{"servidor caído", http.StatusInternalServerError, `{"codigo":"error_interno"}`, ErrNoDisponible},
		{"puerta de enlace", http.StatusBadGateway, ``, ErrNoDisponible},
		{"petición inesperada", http.StatusBadRequest, `{}`, ErrRespuestaInesperada},
		{"JSON inválido", http.StatusOK, `{"puntaje":`, ErrNoDisponible},
		{"puntaje fuera de rango", http.StatusOK, `{"puntaje":7,"insignia":"confiable"}`, ErrNoDisponible},
		{"puntaje negativo", http.StatusOK, `{"puntaje":-1,"insignia":"regular"}`, ErrNoDisponible},
		{"puntaje ausente", http.StatusOK, `{"insignia":"regular"}`, ErrNoDisponible},
		{"insignia desconocida", http.StatusOK, `{"puntaje":4,"insignia":"excelente"}`, ErrNoDisponible},
		{"insignia ausente", http.StatusOK, `{"puntaje":4}`, ErrNoDisponible},
		{"tasa de cancelación inválida", http.StatusOK, `{"puntaje":4,"insignia":"regular","tasaCancelacion":2}`, ErrNoDisponible},
		{"viajes negativos", http.StatusOK, `{"puntaje":4,"insignia":"regular","viajesCompletados":-1}`, ErrNoDisponible},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(caso.estado)
				_, _ = w.Write([]byte(caso.cuerpo))
			}))
			defer servidor.Close()

			cliente, err := NuevoClienteReputacion(servidor.URL, time.Second, "", "")
			if err != nil {
				t.Fatalf("construir el cliente: %v", err)
			}
			_, err = cliente.Puntaje(context.Background(), "usr-12")
			if !errors.Is(err, caso.esperado) {
				t.Fatalf("error = %v, se esperaba %v", err, caso.esperado)
			}
		})
	}
}

func TestPuntajeNoDobleCodificaElID(t *testing.T) {
	var rutaRecibida, uriRecibida string
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rutaRecibida = r.URL.Path
		uriRecibida = r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"puntaje":4,"insignia":"regular"}`))
	}))
	defer servidor.Close()

	cliente, err := NuevoClienteReputacion(servidor.URL, time.Second, "", "")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	if _, err := cliente.Puntaje(context.Background(), "usr_12-AB"); err != nil {
		t.Fatalf("consultar: %v", err)
	}
	if rutaRecibida != "/perfiles/usr_12-AB/puntaje" {
		t.Fatalf("ruta = %q", rutaRecibida)
	}
	if uriRecibida != "/perfiles/usr_12-AB/puntaje" {
		t.Fatalf("RequestURI = %q, el id se codificó de más", uriRecibida)
	}
}

func TestPuntajeRechazaIdentificadorInvalido(t *testing.T) {
	var peticiones int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&peticiones, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"puntaje":4,"insignia":"regular"}`))
	}))
	defer servidor.Close()

	cliente, err := NuevoClienteReputacion(servidor.URL, time.Second, "", "")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	for _, id := range []string{"a/b", "usr 12", "../usr-12", ""} {
		if _, err := cliente.Puntaje(context.Background(), id); !errors.Is(err, ErrRespuestaInesperada) {
			t.Fatalf("Puntaje(%q) = %v, se esperaba ErrRespuestaInesperada", id, err)
		}
	}
	if peticiones != 0 {
		t.Fatalf("no se debía consultar ninguna ruta: %d peticiones", peticiones)
	}
}

func TestPuntajeRespetaElPlazo(t *testing.T) {
	liberar := make(chan struct{})
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-liberar
	}))
	defer servidor.Close()
	defer close(liberar)

	cliente, err := NuevoClienteReputacion(servidor.URL, 20*time.Millisecond, "", "")
	if err != nil {
		t.Fatalf("construir el cliente: %v", err)
	}
	if _, err := cliente.Puntaje(context.Background(), "usr-12"); !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("error = %v, se esperaba ErrNoDisponible", err)
	}
}
