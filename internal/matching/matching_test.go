package matching

import (
	"errors"
	"math"
	"testing"
)

// trazadoHorizontal recorre una línea este-oeste cerca de Bogotá. El punto
// medio usado en las pruebas cae entre los dos vértices, no sobre ellos.
func trazadoHorizontal() []Punto {
	return []Punto{
		{Latitud: 4.6500, Longitud: -74.0600},
		{Latitud: 4.6500, Longitud: -74.0400},
	}
}

func casiIgual(obtenido, esperado, tolerancia float64) bool {
	return math.Abs(obtenido-esperado) <= tolerancia
}

func TestProyectarPuntoEntreVertices(t *testing.T) {
	trazado := trazadoHorizontal()
	// 0.001 grados de latitud al norte del punto medio del segmento.
	punto := Punto{Latitud: 4.6510, Longitud: -74.0500}

	proyeccion, err := ProyectarSobreTrazado(punto, trazado)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}

	// 0.001 grados de latitud equivalen aproximadamente a 111.2 m.
	if !casiIgual(proyeccion.DistanciaM, 111.2, 1.5) {
		t.Fatalf("distancia = %.2f m, se esperaba ~111.2 m", proyeccion.DistanciaM)
	}

	largoTotal := DistanciaMetros(trazado[0], trazado[1])
	if proyeccion.AvanceM <= 0 || proyeccion.AvanceM >= largoTotal {
		t.Fatalf("el avance debe caer estrictamente entre los vértices: %.2f de %.2f", proyeccion.AvanceM, largoTotal)
	}
	if !casiIgual(proyeccion.AvanceM, largoTotal/2, 2) {
		t.Fatalf("avance = %.2f m, se esperaba ~%.2f m", proyeccion.AvanceM, largoTotal/2)
	}
}

func TestProyectarSobreVertice(t *testing.T) {
	trazado := trazadoHorizontal()
	punto := Punto{Latitud: 4.6510, Longitud: -74.0400}

	proyeccion, err := ProyectarSobreTrazado(punto, trazado)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	largoTotal := DistanciaMetros(trazado[0], trazado[1])
	if !casiIgual(proyeccion.AvanceM, largoTotal, 0.01) {
		t.Fatalf("avance = %.2f, se esperaba el extremo %.2f", proyeccion.AvanceM, largoTotal)
	}
	if !casiIgual(proyeccion.DistanciaM, 111.2, 1.5) {
		t.Fatalf("distancia = %.2f m", proyeccion.DistanciaM)
	}
}

func TestProyectarSegmentoDeLongitudCero(t *testing.T) {
	punto := Punto{Latitud: 4.6501, Longitud: -74.0600}
	trazado := []Punto{
		{Latitud: 4.6500, Longitud: -74.0600},
		{Latitud: 4.6500, Longitud: -74.0600},
		{Latitud: 4.6500, Longitud: -74.0400},
	}

	proyeccion, err := ProyectarSobreTrazado(punto, trazado)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	if math.IsNaN(proyeccion.DistanciaM) || math.IsInf(proyeccion.DistanciaM, 0) {
		t.Fatalf("distancia no finita: %v", proyeccion.DistanciaM)
	}
	if !casiIgual(proyeccion.DistanciaM, 11.1, 1.5) {
		t.Fatalf("distancia = %.2f m, se esperaba ~11.1 m", proyeccion.DistanciaM)
	}
	if !casiIgual(proyeccion.AvanceM, 0, 0.001) {
		t.Fatalf("avance = %.4f, se esperaba 0", proyeccion.AvanceM)
	}
}

func TestEvaluarSentidoDelTrayecto(t *testing.T) {
	trazado := trazadoHorizontal()
	origen := Punto{Latitud: 4.6500, Longitud: -74.0550}
	destino := Punto{Latitud: 4.6500, Longitud: -74.0450}

	evaluacion, compatible, err := Evaluar(origen, destino, trazado, 800)
	if err != nil {
		t.Fatalf("evaluar: %v", err)
	}
	if !compatible {
		t.Fatalf("la ruta en el sentido correcto debe ser compatible: %+v", evaluacion)
	}

	_, compatible, err = Evaluar(destino, origen, trazado, 800)
	if err != nil {
		t.Fatalf("evaluar invertida: %v", err)
	}
	if compatible {
		t.Fatalf("la ruta en sentido contrario no debe ser compatible: %+v", evaluacion)
	}
}

func TestEvaluarRadioLimite(t *testing.T) {
	trazado := trazadoHorizontal()
	// Aproximadamente 111 m al norte de la línea.
	origen := Punto{Latitud: 4.6510, Longitud: -74.0550}
	destino := Punto{Latitud: 4.6510, Longitud: -74.0450}

	if _, compatible, err := Evaluar(origen, destino, trazado, 90); err != nil || compatible {
		t.Fatalf("radio de 90 m no debe aceptar puntos a ~111 m: compatible=%v err=%v", compatible, err)
	}
	if _, compatible, err := Evaluar(origen, destino, trazado, 120); err != nil || !compatible {
		t.Fatalf("radio de 120 m debe aceptar puntos a ~111 m: compatible=%v err=%v", compatible, err)
	}
}

func TestEvaluarRechazaDestinoLejano(t *testing.T) {
	trazado := trazadoHorizontal()
	origen := Punto{Latitud: 4.6500, Longitud: -74.0550}
	destino := Punto{Latitud: 4.6600, Longitud: -74.0450}

	_, compatible, err := Evaluar(origen, destino, trazado, 800)
	if err != nil {
		t.Fatalf("evaluar: %v", err)
	}
	if compatible {
		t.Fatal("un destino a más de un kilómetro no debe ser compatible")
	}
}

func TestCoordenadasInvalidas(t *testing.T) {
	invalidas := []Punto{
		{Latitud: 91, Longitud: 0},
		{Latitud: -91, Longitud: 0},
		{Latitud: 0, Longitud: 181},
		{Latitud: 0, Longitud: -181},
		{Latitud: math.NaN(), Longitud: 0},
		{Latitud: 0, Longitud: math.Inf(1)},
	}
	for _, punto := range invalidas {
		if punto.Valido() {
			t.Fatalf("el punto %+v debería ser inválido", punto)
		}
		if _, err := NuevoPunto(punto.Latitud, punto.Longitud); !errors.Is(err, ErrCoordenadaInvalida) {
			t.Fatalf("NuevoPunto(%v, %v) = %v", punto.Latitud, punto.Longitud, err)
		}
		if _, err := ProyectarSobreTrazado(punto, trazadoHorizontal()); !errors.Is(err, ErrCoordenadaInvalida) {
			t.Fatalf("proyectar un punto inválido devolvió %v", err)
		}
	}
}

func TestTrazadoInvalido(t *testing.T) {
	punto := Punto{Latitud: 4.65, Longitud: -74.05}
	for _, trazado := range [][]Punto{
		nil,
		{{Latitud: 4.65, Longitud: -74.05}},
		{
			{Latitud: 4.65, Longitud: -74.05},
			{Latitud: 95, Longitud: -74.04},
		},
	} {
		if _, err := ProyectarSobreTrazado(punto, trazado); !errors.Is(err, ErrTrazadoInvalido) {
			t.Fatalf("trazado %v devolvió %v", trazado, err)
		}
	}
}

func TestRadioInvalido(t *testing.T) {
	trazado := trazadoHorizontal()
	origen := Punto{Latitud: 4.6500, Longitud: -74.0550}
	destino := Punto{Latitud: 4.6500, Longitud: -74.0450}
	for _, radio := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, _, err := Evaluar(origen, destino, trazado, radio); !errors.Is(err, ErrRadioInvalido) {
			t.Fatalf("radio %v devolvió %v", radio, err)
		}
	}
}

func TestDistanciaMetrosConocida(t *testing.T) {
	// Un grado de latitud sobre un meridiano mide aproximadamente 111.19 km
	// con el radio de 6 371 000 m.
	distancia := DistanciaMetros(
		Punto{Latitud: 4, Longitud: -74},
		Punto{Latitud: 5, Longitud: -74},
	)
	if !casiIgual(distancia, 111194.9, 1) {
		t.Fatalf("distancia = %.2f m", distancia)
	}
}
