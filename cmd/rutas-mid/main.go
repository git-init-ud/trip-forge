package main

import (
	"context"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/httpapi"
	"github.com/git-init-ud/trip-forge/internal/mid"
)

func main() {
	if err := ejecutar(); err != nil {
		log.Fatalf("rutas-mid: %v", err)
	}
}

func ejecutar() error {
	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	secreto := strings.TrimSpace(os.Getenv("GATEWAY_SECRET"))

	clienteRutas, err := clients.NuevoClienteRutas(
		variable("RUTAS_CRUD_URL", "http://127.0.0.1:8081"),
		milisegundos("RUTAS_TIMEOUT_MS", 3000),
		secreto,
		variable("RUTAS_TOKEN", ""),
	)
	if err != nil {
		return err
	}
	clienteReputacion, err := clients.NuevoClienteReputacion(
		variable("REPUTACION_URL", "http://127.0.0.1:8083"),
		milisegundos("REPUTACION_TIMEOUT_MS", 2000),
		secreto,
		variable("REPUTACION_TOKEN", ""),
	)
	if err != nil {
		return err
	}

	servicio, err := mid.NuevoServicio(clienteRutas, clienteReputacion, mid.Opciones{
		RadioM:       numeroPositivo("RADIO_M", mid.RadioDefectoM),
		Concurrencia: enteroPositivo("REPUTACION_CONCURRENCIA", mid.ConcurrenciaDefecto),
	})
	if err != nil {
		return err
	}

	return httpapi.ServirMID(ctx, servicio)
}

func variable(nombre, defecto string) string {
	if valor := strings.TrimSpace(os.Getenv(nombre)); valor != "" {
		return valor
	}
	return defecto
}

// milisegundos convierte una variable de entorno en plazo. Un valor ausente,
// no numérico o no positivo vuelve al defecto seguro.
func milisegundos(nombre string, defecto int) time.Duration {
	return time.Duration(enteroPositivo(nombre, defecto)) * time.Millisecond
}

func numeroPositivo(nombre string, defecto float64) float64 {
	crudo := strings.TrimSpace(os.Getenv(nombre))
	if crudo == "" {
		return defecto
	}
	valor, err := strconv.ParseFloat(crudo, 64)
	if err != nil || math.IsNaN(valor) || math.IsInf(valor, 0) || valor <= 0 {
		log.Printf("rutas-mid: %s=%q no es un número positivo; se usa %g", nombre, crudo, defecto)
		return defecto
	}
	return valor
}

func enteroPositivo(nombre string, defecto int) int {
	crudo := strings.TrimSpace(os.Getenv(nombre))
	if crudo == "" {
		return defecto
	}
	valor, err := strconv.Atoi(crudo)
	if err != nil || valor <= 0 {
		log.Printf("rutas-mid: %s=%q no es un entero positivo; se usa %d", nombre, crudo, defecto)
		return defecto
	}
	return valor
}
