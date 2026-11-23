package rutas

import (
	"errors"
	"testing"
	"time"
)

func actualizacionValida(ahora time.Time) ActualizarRuta {
	base := entradaValida(ahora)
	return ActualizarRuta{
		Origen:       base.Origen,
		Destino:      base.Destino,
		Trazado:      base.Trazado,
		Salida:       base.Salida,
		CuposTotales: base.CuposTotales,
		PrecioTotal:  base.PrecioTotal,
	}
}

func rutaConComprometidos(t *testing.T, totales, disponibles int) Ruta {
	t.Helper()
	ruta := Ruta{
		ID:               "ruta-0000000000000001",
		ConductorID:      "usr-12",
		Origen:           Lugar{Nombre: "Chapinero", Punto: Punto{Type: TipoPunto, Coordinates: []float64{-74.0645, 4.6486}}},
		Destino:          Lugar{Nombre: "Universidad", Punto: Punto{Type: TipoPunto, Coordinates: []float64{-74.0415, 4.7012}}},
		Trazado:          LineString{Type: TipoLineString, Coordinates: [][]float64{{-74.0645, 4.6486}, {-74.0415, 4.7012}}},
		Salida:           time.Date(2026, 11, 21, 6, 30, 0, 0, time.UTC),
		CuposTotales:     totales,
		CuposDisponibles: disponibles,
		PrecioTotal:      12000,
		Estado:           Abierta,
	}
	if disponibles == 0 {
		ruta.Estado = Llena
	}
	return ruta
}

func TestValidarActualizacion(t *testing.T) {
	ahora := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre    string
		modificar func(*ActualizarRuta)
		campo     string
	}{
		{"válida", func(*ActualizarRuta) {}, ""},
		{"un cupo", func(a *ActualizarRuta) { a.CuposTotales = 1 }, ""},
		{"seis cupos", func(a *ActualizarRuta) { a.CuposTotales = 6 }, ""},
		{"precio cero", func(a *ActualizarRuta) { a.PrecioTotal = 0 }, ""},
		{"cupos cero", func(a *ActualizarRuta) { a.CuposTotales = 0 }, "cuposTotales"},
		{"siete cupos", func(a *ActualizarRuta) { a.CuposTotales = 7 }, "cuposTotales"},
		{"precio negativo", func(a *ActualizarRuta) { a.PrecioTotal = -1 }, "precioTotal"},
		{"salida en el pasado", func(a *ActualizarRuta) { a.Salida = ahora.Add(-time.Minute) }, "salida"},
		{"salida cero", func(a *ActualizarRuta) { a.Salida = time.Time{} }, "salida"},
		{"trazado de un punto", func(a *ActualizarRuta) {
			a.Trazado.Coordinates = a.Trazado.Coordinates[:1]
		}, "trazado.coordinates"},
		{"trazado con tipo equivocado", func(a *ActualizarRuta) { a.Trazado.Type = TipoPunto }, "trazado"},
		{"origen sin nombre", func(a *ActualizarRuta) { a.Origen.Nombre = " " }, "origen.nombre"},
		{"punto de destino inválido", func(a *ActualizarRuta) {
			a.Destino.Punto.Type = TipoLineString
		}, "destino.punto.type"},
		{"latitud fuera de rango", func(a *ActualizarRuta) {
			a.Origen.Punto.Coordinates = []float64{-74.0645, 90.5}
		}, "origen.punto.coordinates"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			entrada := actualizacionValida(ahora)
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
				t.Fatalf("campo = %q, se esperaba %q", validacion.Campo, caso.campo)
			}
		})
	}
}

func TestValidarActualizacionContraRutaActual(t *testing.T) {
	ahora := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	actual := rutaConComprometidos(t, 4, 2) // dos comprometidos

	t.Run("total igual a comprometidos", func(t *testing.T) {
		cambios := actualizacionValida(ahora)
		cambios.CuposTotales = 2
		if err := cambios.ValidarContra(actual, ahora); err != nil {
			t.Fatalf("se esperaba válido: %v", err)
		}
	})

	t.Run("ampliar por encima de comprometidos", func(t *testing.T) {
		cambios := actualizacionValida(ahora)
		cambios.CuposTotales = 6
		if err := cambios.ValidarContra(actual, ahora); err != nil {
			t.Fatalf("se esperaba válido: %v", err)
		}
	})

	t.Run("reducir por debajo de comprometidos", func(t *testing.T) {
		cambios := actualizacionValida(ahora)
		cambios.CuposTotales = 1
		if err := cambios.ValidarContra(actual, ahora); !errors.Is(err, ErrReservasActivas) {
			t.Fatalf("se esperaba ErrReservasActivas, se obtuvo %v", err)
		}
	})

	t.Run("ruta cerrada", func(t *testing.T) {
		cerrada := actual
		cerrada.Estado = Cerrada
		cambios := actualizacionValida(ahora)
		cambios.CuposTotales = 6
		if err := cambios.ValidarContra(cerrada, ahora); !errors.Is(err, ErrRutaCerrada) {
			t.Fatalf("se esperaba ErrRutaCerrada, se obtuvo %v", err)
		}
	})

	t.Run("entrada inválida antes de mirar la ruta", func(t *testing.T) {
		cambios := actualizacionValida(ahora)
		cambios.PrecioTotal = -5
		var validacion *ErrorValidacion
		err := cambios.ValidarContra(actual, ahora)
		if !errors.As(err, &validacion) {
			t.Fatalf("se esperaba *ErrorValidacion, se obtuvo %v", err)
		}
	})
}

func TestAplicarActualizacionConservaComprometidos(t *testing.T) {
	ahora := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	zona := time.FixedZone("COT", -5*60*60)
	actual := rutaConComprometidos(t, 4, 2)

	cambios := actualizacionValida(ahora)
	cambios.CuposTotales = 6
	cambios.PrecioTotal = 15000
	cambios.Salida = time.Date(2026, 11, 22, 7, 0, 0, 0, zona)
	cambios.Origen.Nombre = "Chapinero Alto"

	resultado := cambios.Aplicar(actual)
	if resultado.CuposTotales != 6 || resultado.CuposDisponibles != 4 {
		t.Fatalf("cupos = %d/%d, se esperaba 6/4", resultado.CuposDisponibles, resultado.CuposTotales)
	}
	if resultado.Estado != Abierta {
		t.Fatalf("estado = %q", resultado.Estado)
	}
	if resultado.ConductorID != actual.ConductorID || resultado.ID != actual.ID {
		t.Fatalf("cambió la identidad de la ruta: %+v", resultado)
	}
	if resultado.Origen.Nombre != "Chapinero Alto" || resultado.PrecioTotal != 15000 {
		t.Fatalf("no se aplicaron los campos editables: %+v", resultado)
	}
	if resultado.Salida.Location() != time.UTC || !resultado.Salida.Equal(cambios.Salida) {
		t.Fatalf("salida = %v, se esperaba el mismo instante en UTC", resultado.Salida)
	}

	ajustada := cambios
	ajustada.CuposTotales = 2
	resultado = ajustada.Aplicar(actual)
	if resultado.CuposDisponibles != 0 || resultado.Estado != Llena {
		t.Fatalf("ruta al límite: %+v", resultado)
	}
}

func TestValidarCierre(t *testing.T) {
	casos := []struct {
		nombre string
		ruta   Ruta
		err    error
	}{
		{"abierta sin comprometidos", rutaConComprometidos(t, 3, 3), nil},
		{"abierta con comprometidos", rutaConComprometidos(t, 3, 2), ErrReservasActivas},
		{"llena", rutaConComprometidos(t, 2, 0), ErrReservasActivas},
		{"cerrada", func() Ruta {
			ruta := rutaConComprometidos(t, 3, 3)
			ruta.Estado = Cerrada
			return ruta
		}(), nil},
		{"cerrada con comprometidos", func() Ruta {
			ruta := rutaConComprometidos(t, 3, 1)
			ruta.Estado = Cerrada
			return ruta
		}(), nil},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if err := ValidarCierre(caso.ruta); !errors.Is(err, caso.err) {
				t.Fatalf("ValidarCierre() = %v, se esperaba %v", err, caso.err)
			}
		})
	}
}

func TestValidarFiltroListado(t *testing.T) {
	desde := time.Date(2030, 1, 15, 0, 0, 0, 0, time.UTC)
	hasta := time.Date(2030, 1, 16, 0, 0, 0, 0, time.UTC)
	valido := func() FiltroListado {
		return FiltroListado{Limit: LimitPorDefecto, Desde: desde, Hasta: hasta}
	}

	casos := []struct {
		nombre    string
		modificar func(*FiltroListado)
		campo     string
	}{
		{"válido", func(*FiltroListado) {}, ""},
		{"sin ventana", func(f *FiltroListado) { f.Desde = time.Time{}; f.Hasta = time.Time{} }, ""},
		{"ventana de un instante", func(f *FiltroListado) { f.Hasta = f.Desde }, ""},
		{"estado abierta", func(f *FiltroListado) { f.Estado = Abierta }, ""},
		{"estado llena", func(f *FiltroListado) { f.Estado = Llena }, ""},
		{"estado cerrada", func(f *FiltroListado) { f.Estado = Cerrada }, ""},
		{"estado desconocido", func(f *FiltroListado) { f.Estado = "pausada" }, "estado"},
		{"limit mínimo", func(f *FiltroListado) { f.Limit = 1 }, ""},
		{"limit máximo", func(f *FiltroListado) { f.Limit = LimitMaximo }, ""},
		{"limit cero", func(f *FiltroListado) { f.Limit = 0 }, "limit"},
		{"limit negativo", func(f *FiltroListado) { f.Limit = -1 }, "limit"},
		{"limit excedido", func(f *FiltroListado) { f.Limit = LimitMaximo + 1 }, "limit"},
		{"offset negativo", func(f *FiltroListado) { f.Offset = -1 }, "offset"},
		{"ventana invertida", func(f *FiltroListado) { f.Hasta = f.Desde.Add(-time.Second) }, "desde"},
		{"conductor vacío", func(f *FiltroListado) { f.ConductorID = "   " }, "conductorId"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			filtro := valido()
			caso.modificar(&filtro)
			err := filtro.Validar()
			if caso.campo == "" {
				if err != nil {
					t.Fatalf("se esperaba válido, se obtuvo %v", err)
				}
				return
			}
			var validacion *ErrorValidacion
			if !errors.As(err, &validacion) {
				t.Fatalf("se esperaba *ErrorValidacion, se obtuvo %v", err)
			}
			if validacion.Campo != caso.campo {
				t.Fatalf("campo = %q, se esperaba %q", validacion.Campo, caso.campo)
			}
		})
	}
}

func TestNormalizarFiltroListado(t *testing.T) {
	zona := time.FixedZone("COT", -5*60*60)
	desdeLocal := time.Date(2030, 1, 15, 0, 0, 0, 0, zona)
	hastaLocal := time.Date(2030, 1, 16, 0, 0, 0, 0, zona)

	filtro := FiltroListado{ConductorID: "  usr-12  ", Desde: desdeLocal, Hasta: hastaLocal, Limit: 10}
	normalizado := filtro.Normalizar()
	if normalizado.ConductorID != "usr-12" {
		t.Fatalf("conductorId = %q", normalizado.ConductorID)
	}
	if normalizado.Desde.Location() != time.UTC || normalizado.Hasta.Location() != time.UTC {
		t.Fatalf("la ventana no quedó en UTC: %v - %v", normalizado.Desde, normalizado.Hasta)
	}
	if !normalizado.Desde.Equal(desdeLocal) || !normalizado.Hasta.Equal(hastaLocal) {
		t.Fatal("la normalización cambió el instante")
	}
	if normalizado.Limit != 10 {
		t.Fatalf("limit = %d, no debe aplicar valores por defecto", normalizado.Limit)
	}
}
