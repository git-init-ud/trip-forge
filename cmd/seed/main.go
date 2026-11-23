// Command seed carga las rutas de demostración en MongoDB.
//
// Es idempotente por identificador: reemplaza ruta-0001..ruta-0015 y no toca
// otras rutas de la base. Antes de escribir compara cada ruta existente con el
// estado esperado; si alguna difiere por una reserva, edición o cierre, aborta
// sin cambiar nada salvo que SEED_FORCE=1. Las rutas con el estado intacto se
// refrescan sin exigir la variable, porque solo se recalcula su salida. Lee
// MONGODB_URI, MONGODB_DB y SEED_FORCE, abre la conexión con mongorutas.Abrir y
// deja los índices listos antes de escribir.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/git-init-ud/trip-forge/internal/mongorutas"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

func main() {
	if err := ejecutar(); err != nil {
		log.Fatalf("seed: %v", err)
	}
}

func ejecutar() error {
	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	uri := variable("MONGODB_URI", "mongodb://127.0.0.1:27017")
	base := variable("MONGODB_DB", "trip_forge")

	cliente, repositorio, err := mongorutas.Abrir(ctx, uri, base)
	if err != nil {
		return err
	}
	defer func() {
		cierre, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelar()
		if err := cliente.Disconnect(cierre); err != nil {
			log.Printf("seed: cerrar mongodb: %v", err)
		}
	}()

	if err := repositorio.AsegurarIndices(ctx); err != nil {
		return err
	}

	semilla := construirSemilla(time.Now())
	forzar := strings.TrimSpace(os.Getenv("SEED_FORCE")) == "1"
	if err := sembrar(ctx, repositorio, semilla, forzar); err != nil {
		return err
	}
	imprimirResumen(base, semilla)
	fmt.Printf("seed: consulta de demostración:\n  curl '%s%s'\n",
		variable("MID_URL", "http://127.0.0.1:8082"), consultaDemo(semilla))
	return nil
}

func imprimirResumen(base string, semilla []rutas.Ruta) {
	porEstado := map[rutas.Estado]int{}
	for _, ruta := range semilla {
		porEstado[ruta.Estado]++
	}
	fmt.Printf("seed: %d rutas cargadas en la base %q\n", len(semilla), base)
	fmt.Printf("seed: abiertas=%d llenas=%d cerradas=%d\n",
		porEstado[rutas.Abierta], porEstado[rutas.Llena], porEstado[rutas.Cerrada])
	for _, ruta := range semilla {
		fmt.Printf("  %s %s salida=%s estado=%s cupos=%d/%d\n",
			ruta.ID, ruta.ConductorID, ruta.Salida.Format(time.RFC3339), ruta.Estado,
			ruta.CuposDisponibles, ruta.CuposTotales)
	}
}

func variable(nombre, defecto string) string {
	if valor := strings.TrimSpace(os.Getenv(nombre)); valor != "" {
		return valor
	}
	return defecto
}
