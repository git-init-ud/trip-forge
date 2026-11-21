package rutas

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

func entradaValida(ahora time.Time) CrearRuta {
	return CrearRuta{
		ConductorID: "usr-12",
		Origen: Lugar{
			Nombre: "Chapinero",
			Punto: Punto{
				Type:        TipoPunto,
				Coordinates: []float64{-74.0645, 4.6486},
			},
		},
		Destino: Lugar{
			Nombre: "Universidad",
			Punto: Punto{
				Type:        TipoPunto,
				Coordinates: []float64{-74.0415, 4.7012},
			},
		},
		Trazado: LineString{
			Type: TipoLineString,
			Coordinates: [][]float64{
				{-74.0645, 4.6486},
				{-74.0600, 4.6650},
				{-74.0415, 4.7012},
			},
		},
		Salida:       ahora.Add(24 * time.Hour),
		CuposTotales: 3,
		PrecioTotal:  12000,
	}
}

func TestValidarCreacion(t *testing.T) {
	ahora := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre    string
		modificar func(*CrearRuta)
		campo     string
	}{
		{"válida", func(*CrearRuta) {}, ""},
		{"un cupo", func(c *CrearRuta) { c.CuposTotales = 1 }, ""},
		{"seis cupos", func(c *CrearRuta) { c.CuposTotales = 6 }, ""},
		{"precio cero", func(c *CrearRuta) { c.PrecioTotal = 0 }, ""},
		{"conductor vacío", func(c *CrearRuta) { c.ConductorID = "   " }, "conductorId"},
		{"cupos cero", func(c *CrearRuta) { c.CuposTotales = 0 }, "cuposTotales"},
		{"siete cupos", func(c *CrearRuta) { c.CuposTotales = 7 }, "cuposTotales"},
		{"precio negativo", func(c *CrearRuta) { c.PrecioTotal = -1 }, "precioTotal"},
		{"salida en el pasado", func(c *CrearRuta) { c.Salida = ahora.Add(-time.Minute) }, "salida"},
		{"salida igual al instante actual", func(c *CrearRuta) { c.Salida = ahora }, "salida"},
		{"salida cero", func(c *CrearRuta) { c.Salida = time.Time{} }, "salida"},
		{"trazado de un punto", func(c *CrearRuta) {
			c.Trazado.Coordinates = c.Trazado.Coordinates[:1]
		}, "trazado.coordinates"},
		{"trazado sin coordenadas", func(c *CrearRuta) { c.Trazado.Coordinates = nil }, "trazado.coordinates"},
		{"trazado con tipo equivocado", func(c *CrearRuta) { c.Trazado.Type = TipoPunto }, "trazado"},
		{"punto de origen con tipo equivocado", func(c *CrearRuta) {
			c.Origen.Punto.Type = TipoLineString
		}, "origen.punto.type"},
		{"origen sin nombre", func(c *CrearRuta) { c.Origen.Nombre = "" }, "origen.nombre"},
		{"destino sin nombre", func(c *CrearRuta) { c.Destino.Nombre = " " }, "destino.nombre"},
		{"longitud fuera de rango", func(c *CrearRuta) {
			c.Origen.Punto.Coordinates = []float64{180.5, 4.6486}
		}, "origen.punto.coordinates"},
		{"latitud fuera de rango", func(c *CrearRuta) {
			c.Destino.Punto.Coordinates = []float64{-74.0415, -90.1}
		}, "destino.punto.coordinates"},
		{"posición del trazado fuera de rango", func(c *CrearRuta) {
			c.Trazado.Coordinates[1] = []float64{-74.06, 91}
		}, "trazado.coordinates[1]"},
		{"coordenadas incompletas", func(c *CrearRuta) {
			c.Origen.Punto.Coordinates = []float64{-74.0645}
		}, "origen.punto.coordinates"},
		{"coordenadas con longitud extra", func(c *CrearRuta) {
			c.Destino.Punto.Coordinates = []float64{-74.0415, 4.7012, 0}
		}, "destino.punto.coordinates"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			entrada := entradaValida(ahora)
			caso.modificar(&entrada)

			err := entrada.Validar(ahora)
			if caso.campo == "" {
				if err != nil {
					t.Fatalf("se esperaba una entrada válida, se obtuvo %v", err)
				}
				return
			}
			var validacion *ErrorValidacion
			if !errors.As(err, &validacion) {
				t.Fatalf("se esperaba *ErrorValidacion, se obtuvo %v", err)
			}
			if validacion.Campo != caso.campo {
				t.Fatalf("campo = %q, se esperaba %q (motivo %q)", validacion.Campo, caso.campo, validacion.Motivo)
			}
			if validacion.Motivo == "" {
				t.Fatal("el error de validación debe explicar el motivo")
			}
		})
	}
}

func TestNuevaAsignaIdentificadorYDisponibilidad(t *testing.T) {
	ahora := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	entrada := entradaValida(ahora)
	entrada.CuposTotales = 6

	ruta, err := Nueva(entrada, ahora)
	if err != nil {
		t.Fatalf("Nueva() devolvió error: %v", err)
	}

	if !regexp.MustCompile(`^ruta-[a-z0-9]+$`).MatchString(ruta.ID) {
		t.Fatalf("identificador fuera del contrato: %q", ruta.ID)
	}
	if ruta.CuposDisponibles != 6 || ruta.CuposTotales != 6 {
		t.Fatalf("cupos = %d/%d, se esperaba 6/6", ruta.CuposDisponibles, ruta.CuposTotales)
	}
	if ruta.Estado != Abierta {
		t.Fatalf("estado = %q, se esperaba %q", ruta.Estado, Abierta)
	}

	otra, err := Nueva(entrada, ahora)
	if err != nil {
		t.Fatalf("Nueva() devolvió error: %v", err)
	}
	if otra.ID == ruta.ID {
		t.Fatal("dos rutas no pueden compartir identificador")
	}
}

func TestNuevaNormalizaSalidaUTC(t *testing.T) {
	ahora := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	zona := time.FixedZone("COT", -5*60*60)
	salidaLocal := time.Date(2026, 11, 21, 6, 30, 0, 0, zona)

	entrada := entradaValida(ahora)
	entrada.Salida = salidaLocal
	entrada.ConductorID = "  usr-12  "

	ruta, err := Nueva(entrada, ahora)
	if err != nil {
		t.Fatalf("Nueva() devolvió error: %v", err)
	}
	if ruta.Salida.Location() != time.UTC {
		t.Fatalf("la salida quedó en %v, se esperaba UTC", ruta.Salida.Location())
	}
	if !ruta.Salida.Equal(salidaLocal) {
		t.Fatalf("la salida cambió de instante: %v != %v", ruta.Salida, salidaLocal)
	}
	if ruta.ConductorID != "usr-12" {
		t.Fatalf("conductorId = %q, se esperaba normalizado", ruta.ConductorID)
	}
}

func TestValidarDelta(t *testing.T) {
	for _, delta := range []int{-6, -1, 1, 6} {
		if err := ValidarDelta(delta); err != nil {
			t.Errorf("delta %d debería ser válido: %v", delta, err)
		}
	}
	for _, delta := range []int{0, -7, 7, 100} {
		err := ValidarDelta(delta)
		var validacion *ErrorValidacion
		if !errors.As(err, &validacion) {
			t.Errorf("delta %d debería ser inválido, se obtuvo %v", delta, err)
			continue
		}
		if validacion.Campo != "delta" {
			t.Errorf("delta %d reportó el campo %q", delta, validacion.Campo)
		}
	}
}
