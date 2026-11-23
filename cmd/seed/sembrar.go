package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

// almacenSemilla es la parte del repositorio que necesita la carga: leer una
// ruta por id y reemplazarla o crearla. Permite probar el guard sin MongoDB.
type almacenSemilla interface {
	Obtener(ctx context.Context, id string) (rutas.Ruta, error)
	Reemplazar(ctx context.Context, ruta rutas.Ruta) error
}

// sembrar valida todas las rutas antes de escribir. Una ruta existente que
// coincide con la semilla se refresca sin exigir SEED_FORCE: solo cambia su
// salida, que se recalcula relativa a ahora. Si difiere en algún dato de
// dominio —por una reserva, una edición o un cierre— aborta sin reemplazar
// ninguna, salvo forzar=true, para no resetear inventario ajeno. Las rutas
// nuevas se crean.
func sembrar(ctx context.Context, almacen almacenSemilla, semilla []rutas.Ruta, forzar bool) error {
	var distintas []rutas.Ruta
	for _, ruta := range semilla {
		actual, err := almacen.Obtener(ctx, ruta.ID)
		if errors.Is(err, rutas.ErrNoEncontrada) {
			continue
		}
		if err != nil {
			return fmt.Errorf("comprobar la ruta %s antes de sembrar: %w", ruta.ID, err)
		}
		if mismaSemilla(ruta, actual) {
			continue
		}
		if !forzar {
			return fmt.Errorf(
				"la ruta %s ya existe con un estado distinto del esperado (reserva o edición); "+
					"no ejecutes la semilla sobre datos de demostración modificados. "+
					"Conserva el inventario o usa SEED_FORCE=1 solo en una base de demostración",
				ruta.ID)
		}
		distintas = append(distintas, actual)
	}

	for _, actual := range distintas {
		log.Printf("seed: se reemplaza %s con un estado distinto del esperado (reserva o edición) porque SEED_FORCE=1",
			actual.ID)
	}
	for _, ruta := range semilla {
		if err := almacen.Reemplazar(ctx, ruta); err != nil {
			return fmt.Errorf("guardar la ruta %s: %w", ruta.ID, err)
		}
	}
	return nil
}

// mismaSemilla compara los datos de dominio de la ruta existente con la
// esperada. Ignora Salida porque la semilla la recalcula en cada ejecución
// relativa a time.Now; el resto de campos identifica si alguien reservó, editó
// o cerró la ruta.
func mismaSemilla(esperada, actual rutas.Ruta) bool {
	return esperada.ConductorID == actual.ConductorID &&
		reflect.DeepEqual(esperada.Origen, actual.Origen) &&
		reflect.DeepEqual(esperada.Destino, actual.Destino) &&
		reflect.DeepEqual(esperada.Trazado, actual.Trazado) &&
		esperada.CuposTotales == actual.CuposTotales &&
		esperada.CuposDisponibles == actual.CuposDisponibles &&
		esperada.PrecioTotal == actual.PrecioTotal &&
		esperada.Estado == actual.Estado
}
