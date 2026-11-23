package main

import (
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

// Coordenadas de la zona de demostración, en orden GeoJSON [longitud, latitud].
// El corredor principal sube de Chapinero a la Universidad pasando por la
// Calle 72, el Parque 93 y la Calle 100.
var (
	puntoChapinero   = [2]float64{-74.0645, 4.6486}
	puntoCalle72     = [2]float64{-74.0600, 4.6650}
	puntoParque93    = [2]float64{-74.0480, 4.6770}
	puntoCalle100    = [2]float64{-74.0480, 4.6900}
	puntoUniversidad = [2]float64{-74.0415, 4.7012}
	puntoSuba        = [2]float64{-74.0830, 4.7410}
	puntoUsaquen     = [2]float64{-74.0300, 4.7200}
	puntoKennedy     = [2]float64{-74.1500, 4.6280}
	puntoRestrepo    = [2]float64{-74.0900, 4.6200}
	puntoCandelaria  = [2]float64{-74.0720, 4.5960}
	puntoTeusaquillo = [2]float64{-74.0700, 4.6500}
	puntoSalitre     = [2]float64{-74.1000, 4.6600}
	puntoAeropuerto  = [2]float64{-74.1460, 4.7010}
)

// construirSemilla arma las 15 rutas de demostración relativas al instante
// recibido. Es una función pura para poder validarla sin MongoDB; la salida se
// expresa en UTC y los horarios se calculan en la zona de Bogotá.
func construirSemilla(ahora time.Time) []rutas.Ruta {
	zona := zonaBogota()
	base := ahora.In(zona)
	salida := func(dias, hora, minuto int) time.Time {
		return time.Date(base.Year(), base.Month(), base.Day()+dias, hora, minuto, 0, 0, zona).UTC()
	}

	corredor := [][2]float64{puntoChapinero, puntoCalle72, puntoParque93, puntoCalle100, puntoUniversidad}
	corredorInverso := [][2]float64{puntoUniversidad, puntoCalle100, puntoParque93, puntoCalle72, puntoChapinero}

	return []rutas.Ruta{
		rutaSemilla("ruta-0001", "usr-01",
			lugar("Chapinero", puntoChapinero), lugar("Universidad", puntoUniversidad),
			corredor, salida(1, 6, 30), 3, 3, 12000, rutas.Abierta),
		rutaSemilla("ruta-0002", "usr-02",
			lugar("Calle 72", puntoCalle72), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoCalle72, puntoParque93, puntoCalle100, puntoUniversidad},
			salida(1, 7, 0), 4, 4, 14000, rutas.Abierta),
		rutaSemilla("ruta-0003", "usr-03",
			lugar("Suba", puntoSuba), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoSuba, puntoCalle100, puntoUniversidad},
			salida(1, 6, 45), 2, 2, 10000, rutas.Abierta),
		rutaSemilla("ruta-0004", "usr-04",
			lugar("Chapinero", puntoChapinero), lugar("Calle 100", puntoCalle100),
			[][2]float64{puntoChapinero, puntoCalle72, puntoParque93, puntoCalle100},
			salida(2, 7, 15), 3, 1, 9000, rutas.Abierta),
		// Sentido inverso del corredor principal: la salida está al norte y el
		// trazado retrocede hacia el sur. La búsqueda debe descartarla para un
		// pasajero que va de Chapinero a la Universidad.
		rutaSemilla("ruta-0005", "usr-05",
			lugar("Universidad", puntoUniversidad), lugar("Chapinero", puntoChapinero),
			corredorInverso, salida(2, 6, 30), 4, 4, 12000, rutas.Abierta),
		// Pasa cerca del destino pero arranca lejos del origen del corredor.
		rutaSemilla("ruta-0006", "usr-06",
			lugar("Kennedy", puntoKennedy), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoKennedy, puntoSalitre, puntoCalle100, puntoUniversidad},
			salida(2, 6, 0), 5, 5, 16000, rutas.Abierta),
		// Ruta llena: sin disponibilidad, el estado publicado es "llena".
		rutaSemilla("ruta-0007", "usr-07",
			lugar("Restrepo", puntoRestrepo), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoRestrepo, puntoTeusaquillo, puntoCalle72, puntoUniversidad},
			salida(1, 18, 0), 3, 0, 11000, rutas.Llena),
		rutaSemilla("ruta-0008", "usr-08",
			lugar("Usaquén", puntoUsaquen), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoUsaquen, puntoCalle100, puntoUniversidad},
			salida(3, 7, 30), 2, 2, 13000, rutas.Abierta),
		rutaSemilla("ruta-0009", "usr-09",
			lugar("Teusaquillo", puntoTeusaquillo), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoTeusaquillo, puntoCalle72, puntoParque93, puntoUniversidad},
			salida(3, 6, 40), 3, 2, 11000, rutas.Abierta),
		rutaSemilla("ruta-0010", "usr-10",
			lugar("La Candelaria", puntoCandelaria), lugar("Calle 100", puntoCalle100),
			[][2]float64{puntoCandelaria, puntoChapinero, puntoCalle72, puntoCalle100},
			salida(4, 6, 20), 6, 6, 18000, rutas.Abierta),
		rutaSemilla("ruta-0011", "usr-11",
			lugar("Aeropuerto", puntoAeropuerto), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoAeropuerto, puntoSalitre, puntoCalle100, puntoUniversidad},
			salida(4, 5, 50), 4, 3, 17000, rutas.Abierta),
		rutaSemilla("ruta-0012", "usr-12",
			lugar("Chapinero", puntoChapinero), lugar("Universidad", puntoUniversidad),
			corredor, salida(5, 6, 30), 3, 3, 12000, rutas.Abierta),
		// Ruta cerrada: se conserva el documento para que el listado y el
		// detalle muestren una ruta que el conductor dejó de ofrecer.
		rutaSemilla("ruta-0013", "usr-01",
			lugar("Chapinero", puntoChapinero), lugar("Universidad", puntoUniversidad),
			corredor, salida(2, 7, 0), 3, 3, 12000, rutas.Cerrada),
		rutaSemilla("ruta-0014", "usr-02",
			lugar("Parque 93", puntoParque93), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoParque93, puntoCalle100, puntoUniversidad},
			salida(5, 7, 10), 2, 1, 9000, rutas.Abierta),
		rutaSemilla("ruta-0015", "usr-03",
			lugar("Chapinero", puntoChapinero), lugar("Universidad", puntoUniversidad),
			[][2]float64{puntoChapinero, puntoCalle72, puntoParque93, {-74.0500, 4.6950}, puntoUniversidad},
			salida(5, 18, 30), 6, 5, 15000, rutas.Abierta),
	}
}

func rutaSemilla(id, conductor string, origen, destino rutas.Lugar, coordenadas [][2]float64, salida time.Time, totales, disponibles int, precio int64, estado rutas.Estado) rutas.Ruta {
	return rutas.Ruta{
		ID:               id,
		ConductorID:      conductor,
		Origen:           origen,
		Destino:          destino,
		Trazado:          lineString(coordenadas),
		Salida:           salida.UTC(),
		CuposTotales:     totales,
		CuposDisponibles: disponibles,
		PrecioTotal:      precio,
		Estado:           estado,
	}
}

func lugar(nombre string, coordenada [2]float64) rutas.Lugar {
	return rutas.Lugar{
		Nombre: nombre,
		Punto: rutas.Punto{
			Type:        rutas.TipoPunto,
			Coordinates: []float64{coordenada[0], coordenada[1]},
		},
	}
}

func lineString(coordenadas [][2]float64) rutas.LineString {
	trazado := rutas.LineString{
		Type:        rutas.TipoLineString,
		Coordinates: make([][]float64, 0, len(coordenadas)),
	}
	for _, coordenada := range coordenadas {
		trazado.Coordinates = append(trazado.Coordinates, []float64{coordenada[0], coordenada[1]})
	}
	return trazado
}

// zonaBogota usa la base horaria de Bogotá con una reserva fija si el sistema
// no trae la base de zonas horarias.
func zonaBogota() *time.Location {
	zona, err := time.LoadLocation("America/Bogota")
	if err != nil {
		return time.FixedZone("COT", -5*60*60)
	}
	return zona
}
