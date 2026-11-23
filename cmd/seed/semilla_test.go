package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

func TestConstruirSemilla(t *testing.T) {
	ahora := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	semilla := construirSemilla(ahora)

	if len(semilla) != 15 {
		t.Fatalf("rutas = %d, se esperaban 15", len(semilla))
	}

	vistos := map[string]bool{}
	conductores := map[string]bool{}
	porEstado := map[rutas.Estado]int{}
	conComprometidos := 0

	for indice, ruta := range semilla {
		identificador := fmt.Sprintf("ruta-%04d", indice+1)
		if ruta.ID != identificador {
			t.Fatalf("posición %d: id = %q, se esperaba %q", indice, ruta.ID, identificador)
		}
		if vistos[ruta.ID] {
			t.Fatalf("id repetido: %s", ruta.ID)
		}
		vistos[ruta.ID] = true
		conductores[ruta.ConductorID] = true
		porEstado[ruta.Estado]++

		if !ruta.Salida.After(ahora) {
			t.Fatalf("%s: salida %v no es futura respecto de %v", ruta.ID, ruta.Salida, ahora)
		}
		if ruta.Salida.Location() != time.UTC {
			t.Fatalf("%s: la salida no está en UTC: %v", ruta.ID, ruta.Salida)
		}
		if ruta.CuposTotales < rutas.CuposMinimos || ruta.CuposTotales > rutas.CuposMaximos {
			t.Fatalf("%s: cuposTotales = %d", ruta.ID, ruta.CuposTotales)
		}
		if ruta.CuposDisponibles < 0 || ruta.CuposDisponibles > ruta.CuposTotales {
			t.Fatalf("%s: cupos = %d/%d", ruta.ID, ruta.CuposDisponibles, ruta.CuposTotales)
		}
		if ruta.CuposDisponibles < ruta.CuposTotales {
			conComprometidos++
		}
		switch ruta.Estado {
		case rutas.Abierta:
			if ruta.CuposDisponibles == 0 {
				t.Fatalf("%s: una ruta abierta no puede estar sin cupos", ruta.ID)
			}
		case rutas.Llena:
			if ruta.CuposDisponibles != 0 {
				t.Fatalf("%s: una ruta llena debe tener 0 disponibles", ruta.ID)
			}
		case rutas.Cerrada:
		default:
			t.Fatalf("%s: estado desconocido %q", ruta.ID, ruta.Estado)
		}
		if ruta.PrecioTotal < 0 {
			t.Fatalf("%s: precio negativo", ruta.ID)
		}

		validarGeometria(t, ruta)
		if primero := ruta.Trazado.Coordinates[0]; !mismaCoordenada(primero, ruta.Origen.Punto.Coordinates) {
			t.Fatalf("%s: el trazado no empieza en el origen: %v != %v", ruta.ID, primero, ruta.Origen.Punto.Coordinates)
		}
		ultimo := ruta.Trazado.Coordinates[len(ruta.Trazado.Coordinates)-1]
		if !mismaCoordenada(ultimo, ruta.Destino.Punto.Coordinates) {
			t.Fatalf("%s: el trazado no termina en el destino: %v != %v", ruta.ID, ultimo, ruta.Destino.Punto.Coordinates)
		}
	}

	if porEstado[rutas.Llena] != 1 {
		t.Fatalf("rutas llenas = %d, se esperaba al menos una", porEstado[rutas.Llena])
	}
	if porEstado[rutas.Cerrada] != 1 {
		t.Fatalf("rutas cerradas = %d, se esperaba al menos una", porEstado[rutas.Cerrada])
	}
	if porEstado[rutas.Abierta] < 10 {
		t.Fatalf("rutas abiertas = %d, se esperaban al menos 10", porEstado[rutas.Abierta])
	}
	if conComprometidos == 0 {
		t.Fatal("la semilla debe incluir rutas con cupos ya comprometidos")
	}

	for i := 1; i <= 12; i++ {
		conductor := fmt.Sprintf("usr-%02d", i)
		if !conductores[conductor] {
			t.Fatalf("falta el conductor %s de los perfiles semilla", conductor)
		}
	}

	assertarRutaInversa(t, semilla)
	assertarRutaLejosDelOrigen(t, semilla)
}

func validarGeometria(t *testing.T, ruta rutas.Ruta) {
	t.Helper()
	if ruta.Origen.Punto.Type != rutas.TipoPunto || ruta.Destino.Punto.Type != rutas.TipoPunto {
		t.Fatalf("%s: origen y destino deben ser Point", ruta.ID)
	}
	if ruta.Trazado.Type != rutas.TipoLineString {
		t.Fatalf("%s: trazado debe ser LineString", ruta.ID)
	}
	if len(ruta.Trazado.Coordinates) < 2 {
		t.Fatalf("%s: trazado con %d puntos", ruta.ID, len(ruta.Trazado.Coordinates))
	}
	for _, punto := range append([][]float64{ruta.Origen.Punto.Coordinates, ruta.Destino.Punto.Coordinates}, ruta.Trazado.Coordinates...) {
		if len(punto) != 2 {
			t.Fatalf("%s: coordenada con %d valores: %v", ruta.ID, len(punto), punto)
		}
		longitud, latitud := punto[0], punto[1]
		if longitud < -74.25 || longitud > -73.95 || latitud < 4.50 || latitud > 4.80 {
			t.Fatalf("%s: coordenada fuera de la zona de Bogotá: %v", ruta.ID, punto)
		}
	}
}

func assertarRutaInversa(t *testing.T, semilla []rutas.Ruta) {
	t.Helper()
	for _, ruta := range semilla {
		if ruta.Origen.Nombre != "Universidad" || ruta.Destino.Nombre != "Chapinero" {
			continue
		}
		primero := ruta.Trazado.Coordinates[0]
		ultimo := ruta.Trazado.Coordinates[len(ruta.Trazado.Coordinates)-1]
		if primero[0] <= ultimo[0] {
			t.Fatalf("%s: la ruta inversa no retrocede hacia el sur: %v -> %v", ruta.ID, primero, ultimo)
		}
		return
	}
	t.Fatal("falta una ruta en sentido inverso al corredor principal")
}

func assertarRutaLejosDelOrigen(t *testing.T, semilla []rutas.Ruta) {
	t.Helper()
	for _, ruta := range semilla {
		if ruta.Destino.Nombre != "Universidad" {
			continue
		}
		if ruta.Origen.Punto.Coordinates[0] > -74.10 {
			continue
		}
		// El trazado debe acercarse al destino aunque el origen quede lejos del
		// corredor Chapinero-Universidad.
		ultimo := ruta.Trazado.Coordinates[len(ruta.Trazado.Coordinates)-1]
		if len(ultimo) == 2 && ultimo[0] == puntoUniversidad[0] && ultimo[1] == puntoUniversidad[1] {
			return
		}
	}
	t.Fatal("falta una ruta que pase cerca del destino pero salga lejos del origen")
}

func mismaCoordenada(a, b []float64) bool {
	return len(a) == 2 && len(b) == 2 && a[0] == b[0] && a[1] == b[1]
}
