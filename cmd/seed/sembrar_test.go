package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

func fechaSemilla() time.Time {
	return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
}

type almacenSemillaFalso struct {
	documentos    map[string]rutas.Ruta
	reemplazados  []string
	errObtener    error
	errReemplazar error
}

func (a *almacenSemillaFalso) Obtener(_ context.Context, id string) (rutas.Ruta, error) {
	if a.errObtener != nil {
		return rutas.Ruta{}, a.errObtener
	}
	ruta, existe := a.documentos[id]
	if !existe {
		return rutas.Ruta{}, rutas.ErrNoEncontrada
	}
	return ruta, nil
}

func (a *almacenSemillaFalso) Reemplazar(_ context.Context, ruta rutas.Ruta) error {
	if a.errReemplazar != nil {
		return a.errReemplazar
	}
	if a.documentos == nil {
		a.documentos = map[string]rutas.Ruta{}
	}
	a.documentos[ruta.ID] = ruta
	a.reemplazados = append(a.reemplazados, ruta.ID)
	return nil
}

func TestSembrarCreaRutasNuevas(t *testing.T) {
	semilla := construirSemilla(fechaSemilla())
	almacen := &almacenSemillaFalso{documentos: map[string]rutas.Ruta{}}

	if err := sembrar(context.Background(), almacen, semilla, false); err != nil {
		t.Fatalf("sembrar() devolvió error: %v", err)
	}
	if len(almacen.reemplazados) != len(semilla) {
		t.Fatalf("reemplazos = %d, se esperaban %d", len(almacen.reemplazados), len(semilla))
	}
	if len(almacen.documentos) != len(semilla) {
		t.Fatalf("documentos = %d", len(almacen.documentos))
	}
}

func TestSembrarDosVecesRefrescaFechas(t *testing.T) {
	primera := construirSemilla(fechaSemilla())
	segunda := construirSemilla(fechaSemilla().Add(24 * time.Hour))
	almacen := &almacenSemillaFalso{documentos: map[string]rutas.Ruta{}}

	if err := sembrar(context.Background(), almacen, primera, false); err != nil {
		t.Fatalf("primera ejecución: %v", err)
	}
	almacen.reemplazados = nil
	if err := sembrar(context.Background(), almacen, segunda, false); err != nil {
		t.Fatalf("segunda ejecución con el mismo estado: %v", err)
	}
	if len(almacen.reemplazados) != len(segunda) {
		t.Fatalf("reemplazos = %d, se esperaban %d", len(almacen.reemplazados), len(segunda))
	}
	actual := almacen.documentos[segunda[0].ID]
	if !actual.Salida.Equal(segunda[0].Salida) {
		t.Fatalf("salida = %v, se esperaba la refrescada %v", actual.Salida, segunda[0].Salida)
	}
	if actual.Salida.Equal(primera[0].Salida) {
		t.Fatal("la segunda ejecución no refrescó la salida")
	}
}

func TestSembrarAbortaConEstadoDistintoSinForzar(t *testing.T) {
	semilla := construirSemilla(fechaSemilla())
	// Simula un PATCH -1: la ruta existente ya no coincide con la semilla.
	existente := semilla[10]
	existente.CuposDisponibles--
	almacen := &almacenSemillaFalso{documentos: map[string]rutas.Ruta{
		existente.ID: existente,
	}}

	err := sembrar(context.Background(), almacen, semilla, false)
	if err == nil {
		t.Fatal("se esperaba un error al encontrar un estado distinto del esperado")
	}
	if !strings.Contains(err.Error(), existente.ID) {
		t.Fatalf("el error debe identificar la ruta: %v", err)
	}
	if !strings.Contains(err.Error(), "estado distinto del esperado (reserva o edición)") {
		t.Fatalf("el error debe explicar la causa: %v", err)
	}
	if !strings.Contains(err.Error(), "SEED_FORCE=1") {
		t.Fatalf("el error debe indicar la salida SEED_FORCE=1: %v", err)
	}
	if len(almacen.reemplazados) != 0 {
		t.Fatalf("no se debía reemplazar ninguna ruta: %v", almacen.reemplazados)
	}
	if actual := almacen.documentos[existente.ID]; actual.CuposDisponibles != existente.CuposDisponibles {
		t.Fatalf("el documento existente cambió: %+v", actual)
	}
}

func TestSembrarFuerzaConEstadoDistinto(t *testing.T) {
	semilla := construirSemilla(fechaSemilla())
	existente := semilla[10]
	existente.CuposDisponibles--
	almacen := &almacenSemillaFalso{documentos: map[string]rutas.Ruta{
		existente.ID: existente,
	}}

	if err := sembrar(context.Background(), almacen, semilla, true); err != nil {
		t.Fatalf("sembrar() con SEED_FORCE devolvió error: %v", err)
	}
	if len(almacen.reemplazados) != len(semilla) {
		t.Fatalf("reemplazos = %d, se esperaban %d", len(almacen.reemplazados), len(semilla))
	}
	actual := almacen.documentos[existente.ID]
	esperada := semilla[10]
	if actual.CuposDisponibles != esperada.CuposDisponibles || actual.CuposTotales != esperada.CuposTotales {
		t.Fatalf("el documento no se reemplazó: %+v, se esperaba %+v", actual, esperada)
	}
}

func TestSembrarPropagaErrorDeLectura(t *testing.T) {
	semilla := construirSemilla(fechaSemilla())
	almacen := &almacenSemillaFalso{
		documentos: map[string]rutas.Ruta{},
		errObtener: errors.New("mongo caído"),
	}

	if err := sembrar(context.Background(), almacen, semilla, false); err == nil {
		t.Fatal("se esperaba el error de lectura")
	}
	if len(almacen.reemplazados) != 0 {
		t.Fatalf("no se debía escribir tras un error de lectura: %v", almacen.reemplazados)
	}
}
