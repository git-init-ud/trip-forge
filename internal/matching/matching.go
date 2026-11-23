// Package matching calcula la geometría y el ranking del MID de rutas.
//
// El paquete no depende de HTTP ni de MongoDB: recibe puntos GeoJSON ya
// decodificados y devuelve distancias en metros, el avance sobre el trazado y
// el orden de las coincidencias. Esa separación permite probar las reglas de
// R-HU4 y R-HU5 sin levantar servicios.
//
// Las coordenadas conservan el orden de GeoJSON: [longitud, latitud]. En este
// paquete Punto expone ambos valores con nombre para evitar el error de
// integración más frecuente del clúster.
package matching

import (
	"errors"
	"math"
)

const (
	// RadioTierraM es el radio medio terrestre, en metros, que usa haversine.
	RadioTierraM = 6371000.0

	gradosARadianes = math.Pi / 180
)

var (
	// ErrCoordenadaInvalida indica una latitud o longitud fuera de rango, o
	// un valor no finito.
	ErrCoordenadaInvalida = errors.New("coordenada fuera de rango")
	// ErrTrazadoInvalido indica un trazado con menos de dos puntos válidos.
	ErrTrazadoInvalido = errors.New("el trazado no es un LineString válido")
	// ErrRadioInvalido indica un radio de búsqueda que no es positivo.
	ErrRadioInvalido = errors.New("el radio debe ser positivo")
)

// Punto es una posición geográfica con el orden de GeoJSON separado en campos.
type Punto struct {
	Latitud  float64
	Longitud float64
}

// NuevoPunto valida y construye un punto geográfico.
func NuevoPunto(latitud, longitud float64) (Punto, error) {
	punto := Punto{Latitud: latitud, Longitud: longitud}
	if !punto.Valido() {
		return Punto{}, ErrCoordenadaInvalida
	}
	return punto, nil
}

// Valido comprueba que la latitud y la longitud estén en rango y sean finitas.
func (p Punto) Valido() bool {
	return esFinito(p.Latitud) && esFinito(p.Longitud) &&
		p.Latitud >= -90 && p.Latitud <= 90 &&
		p.Longitud >= -180 && p.Longitud <= 180
}

// Proyeccion es el resultado de proyectar un punto sobre un trazado. La
// distancia es la del punto al segmento más cercano, no solo a sus vértices; el
// avance se mide desde el inicio del LineString.
type Proyeccion struct {
	DistanciaM float64
	AvanceM    float64
}

// Evaluacion reúne las proyecciones del origen y del destino del pasajero.
type Evaluacion struct {
	Origen  Proyeccion
	Destino Proyeccion
}

// Evaluar proyecta origen y destino sobre el trazado y decide si la ruta sirve.
//
// Es compatible cuando ambas proyecciones quedan dentro del radio y el origen
// aparece estrictamente antes que el destino sobre el recorrido. El orden
// estricto descarta los viajes en sentido contrario.
func Evaluar(origen, destino Punto, trazado []Punto, radioM float64) (Evaluacion, bool, error) {
	if !esFinito(radioM) || radioM <= 0 {
		return Evaluacion{}, false, ErrRadioInvalido
	}
	proyeccionOrigen, err := ProyectarSobreTrazado(origen, trazado)
	if err != nil {
		return Evaluacion{}, false, err
	}
	proyeccionDestino, err := ProyectarSobreTrazado(destino, trazado)
	if err != nil {
		return Evaluacion{}, false, err
	}
	evaluacion := Evaluacion{Origen: proyeccionOrigen, Destino: proyeccionDestino}
	return evaluacion, evaluacion.Compatible(radioM), nil
}

// Compatible aplica el radio y el sentido del trayecto.
func (e Evaluacion) Compatible(radioM float64) bool {
	return e.Origen.DistanciaM <= radioM &&
		e.Destino.DistanciaM <= radioM &&
		e.Origen.AvanceM < e.Destino.AvanceM
}

// ProyectarSobreTrazado busca el segmento del trazado más cercano al punto.
//
// Para cada segmento proyecta el punto sobre la recta en un plano local
// equirectangular, acota la fracción al intervalo [0, 1] y mide la distancia
// con haversine sobre la posición interpolada. Así el punto más cercano puede
// caer entre dos vértices y no solo sobre ellos.
func ProyectarSobreTrazado(punto Punto, trazado []Punto) (Proyeccion, error) {
	if !punto.Valido() {
		return Proyeccion{}, ErrCoordenadaInvalida
	}
	if len(trazado) < 2 {
		return Proyeccion{}, ErrTrazadoInvalido
	}
	for _, vertice := range trazado {
		if !vertice.Valido() {
			return Proyeccion{}, ErrTrazadoInvalido
		}
	}

	mejor := Proyeccion{DistanciaM: math.Inf(1)}
	avanceAcumulado := 0.0
	for indice := 0; indice+1 < len(trazado); indice++ {
		inicio, fin := trazado[indice], trazado[indice+1]
		largoSegmento := DistanciaMetros(inicio, fin)
		fraccion := fraccionProyeccion(punto, inicio, fin)
		cercano := interpolar(inicio, fin, fraccion)
		distancia := DistanciaMetros(punto, cercano)
		avance := avanceAcumulado + fraccion*largoSegmento

		if distancia < mejor.DistanciaM ||
			(distancia == mejor.DistanciaM && avance < mejor.AvanceM) {
			mejor = Proyeccion{DistanciaM: distancia, AvanceM: avance}
		}
		avanceAcumulado += largoSegmento
	}
	return mejor, nil
}

// DistanciaMetros calcula la distancia haversine entre dos puntos.
func DistanciaMetros(a, b Punto) float64 {
	latitudA := a.Latitud * gradosARadianes
	latitudB := b.Latitud * gradosARadianes
	deltaLatitud := latitudB - latitudA
	deltaLongitud := (b.Longitud - a.Longitud) * gradosARadianes

	senoLatitud := math.Sin(deltaLatitud / 2)
	senoLongitud := math.Sin(deltaLongitud / 2)
	haversine := senoLatitud*senoLatitud + math.Cos(latitudA)*math.Cos(latitudB)*senoLongitud*senoLongitud
	return 2 * RadioTierraM * math.Asin(math.Min(1, math.Sqrt(haversine)))
}

// fraccionProyeccion devuelve dónde cae la proyección del punto sobre el
// segmento, entre 0 y 1. Trabaja en grados escalados: el factor común de
// longitud se cancela en la razón y el resultado es independiente del radio
// terrestre.
func fraccionProyeccion(punto, inicio, fin Punto) float64 {
	cosenoLatitud := math.Cos(punto.Latitud * gradosARadianes)
	ax := (inicio.Longitud - punto.Longitud) * cosenoLatitud
	ay := inicio.Latitud - punto.Latitud
	bx := (fin.Longitud - punto.Longitud) * cosenoLatitud
	by := fin.Latitud - punto.Latitud

	dx := bx - ax
	dy := by - ay
	denominador := dx*dx + dy*dy
	if denominador == 0 {
		// Segmento de longitud cero: su único punto es el inicio.
		return 0
	}
	fraccion := -(ax*dx + ay*dy) / denominador
	if fraccion < 0 {
		return 0
	}
	if fraccion > 1 {
		return 1
	}
	return fraccion
}

func interpolar(inicio, fin Punto, fraccion float64) Punto {
	return Punto{
		Latitud:  inicio.Latitud + fraccion*(fin.Latitud-inicio.Latitud),
		Longitud: inicio.Longitud + fraccion*(fin.Longitud-inicio.Longitud),
	}
}

func esFinito(valor float64) bool {
	return !math.IsNaN(valor) && !math.IsInf(valor, 0)
}
