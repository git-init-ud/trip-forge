package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/httpapi"
	"github.com/git-init-ud/trip-forge/internal/mongorutas"
)

var _ httpapi.AlmacenRutas = (*mongorutas.Repositorio)(nil)

func main() {
	if err := ejecutar(); err != nil {
		log.Fatalf("rutas-crud: %v", err)
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
			log.Printf("rutas-crud: cerrar mongodb: %v", err)
		}
	}()

	if err := repositorio.AsegurarIndices(ctx); err != nil {
		return err
	}
	generador, err := clients.NuevoClienteGeneradorMID(variable("MID_URL", "http://127.0.0.1:8082"), os.Getenv("GATEWAY_SECRET"), plazoMID())
	if err != nil {
		return err
	}
	return httpapi.ServirRutas(ctx, repositorio, generador)
}

func plazoMID() time.Duration {
	crudo := strings.TrimSpace(os.Getenv("MID_TIMEOUT_MS"))
	if crudo == "" {
		return 5 * time.Second
	}
	valor, err := strconv.Atoi(crudo)
	if err != nil || valor <= 0 {
		log.Printf("rutas-crud: MID_TIMEOUT_MS inválido; se usa 5000 ms")
		return 5 * time.Second
	}
	return time.Duration(valor) * time.Millisecond
}

func variable(nombre, defecto string) string {
	if valor := strings.TrimSpace(os.Getenv(nombre)); valor != "" {
		return valor
	}
	return defecto
}
