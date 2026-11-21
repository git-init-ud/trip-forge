// Package rutas contiene el modelo y las reglas de negocio del CRUD de rutas.
//
// El paquete no depende de HTTP ni de MongoDB: el transporte vive en
// internal/httpapi y la persistencia en internal/mongorutas. Las coordenadas
// siguen el orden de GeoJSON, [longitud, latitud], y las salidas se manejan en
// UTC.
package rutas

import "time"

// Estado es la disponibilidad publicada de una ruta.
type Estado string

const (
	// Abierta indica que la ruta acepta reservas.
	Abierta Estado = "abierta"
	// Llena indica que no quedan cupos disponibles.
	Llena Estado = "llena"
	// Cerrada indica que el conductor dejó de ofrecer la ruta.
	Cerrada Estado = "cerrada"
)

const (
	// CuposMaximos es el límite de cupos por ruta fijado por el enunciado.
	CuposMaximos = 6
	// CuposMinimos es el mínimo de cupos que puede ofrecer una ruta.
	CuposMinimos = 1
	// DeltaMaximo es la magnitud máxima de un ajuste de cupos.
	DeltaMaximo = CuposMaximos

	// TipoPunto es el valor obligatorio de "type" para un punto GeoJSON.
	TipoPunto = "Point"
	// TipoLineString es el valor obligatorio de "type" para un trazado GeoJSON.
	TipoLineString = "LineString"
)

// Punto es un punto GeoJSON con coordenadas [longitud, latitud].
type Punto struct {
	Type        string    `bson:"type" json:"type"`
	Coordinates []float64 `bson:"coordinates" json:"coordinates"`
}

// LineString es el trazado GeoJSON de la ruta. Cada posición es
// [longitud, latitud].
type LineString struct {
	Type        string      `bson:"type" json:"type"`
	Coordinates [][]float64 `bson:"coordinates" json:"coordinates"`
}

// Lugar es un extremo de la ruta con un nombre legible y su punto GeoJSON.
type Lugar struct {
	Nombre string `bson:"nombre" json:"nombre"`
	Punto  Punto  `bson:"punto" json:"punto"`
}

// Ruta es el documento que persiste el CRUD. El identificador se guarda en
// "_id" y se expone como "id" en JSON.
type Ruta struct {
	ID               string     `bson:"_id" json:"id"`
	ConductorID      string     `bson:"conductorId" json:"conductorId"`
	Origen           Lugar      `bson:"origen" json:"origen"`
	Destino          Lugar      `bson:"destino" json:"destino"`
	Trazado          LineString `bson:"trazado" json:"trazado"`
	Salida           time.Time  `bson:"salida" json:"salida"`
	CuposTotales     int        `bson:"cuposTotales" json:"cuposTotales"`
	CuposDisponibles int        `bson:"cuposDisponibles" json:"cuposDisponibles"`
	PrecioTotal      int64      `bson:"precioTotal" json:"precioTotal"`
	Estado           Estado     `bson:"estado" json:"estado"`
}

// Ocupados devuelve los cupos comprometidos de la ruta.
func (r Ruta) Ocupados() int {
	return r.CuposTotales - r.CuposDisponibles
}
