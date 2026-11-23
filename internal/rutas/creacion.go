package rutas

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

// CrearRuta es la entrada validada de POST /rutas. El identificador, la
// disponibilidad inicial y el estado los asigna el servidor.
type CrearRuta struct {
	ConductorID  string
	Origen       Lugar
	Destino      Lugar
	Trazado      LineString
	Salida       time.Time
	CuposTotales int
	PrecioTotal  int64
}

// Validar comprueba las reglas de publicación de R-HU1 contra el instante
// ahora. Devuelve un *ErrorValidacion con el campo que incumple la regla.
func (c CrearRuta) Validar(ahora time.Time) error {
	if err := c.ValidarSinTrazado(ahora); err != nil {
		return err
	}
	return validarTrazado(c.Trazado)
}

// ValidarSinTrazado valida la publicación antes de solicitar un trazado vial.
func (c CrearRuta) ValidarSinTrazado(ahora time.Time) error {
	if strings.TrimSpace(c.ConductorID) == "" {
		return campoInvalido("conductorId", "es obligatorio")
	}
	if err := validarLugar("origen", c.Origen); err != nil {
		return err
	}
	if err := validarLugar("destino", c.Destino); err != nil {
		return err
	}
	if c.Salida.IsZero() {
		return campoInvalido("salida", "es obligatoria")
	}
	if !c.Salida.After(ahora) {
		return campoInvalido("salida", "debe ser una fecha futura")
	}
	if c.CuposTotales < CuposMinimos || c.CuposTotales > CuposMaximos {
		return campoInvalido("cuposTotales", "debe estar entre 1 y 6")
	}
	if c.PrecioTotal < 0 {
		return campoInvalido("precioTotal", "no puede ser negativo")
	}
	return nil
}

// ValidarTrazado comprueba el LineString devuelto por un proveedor vial.
func ValidarTrazado(trazado LineString) error { return validarTrazado(trazado) }

// ValidarPunto comprueba un extremo GeoJSON de una solicitud vial.
func ValidarPunto(campo string, punto Punto) error {
	if punto.Type != TipoPunto {
		return campoInvalido(campo+".type", `debe ser "Point"`)
	}
	return validarCoordenadas(campo+".coordinates", punto.Coordinates)
}

// Nueva valida la entrada, normaliza la salida a UTC y construye la ruta que se
// persiste. El identificador tiene el prefijo "ruta-" que exige el contrato.
func Nueva(c CrearRuta, ahora time.Time) (Ruta, error) {
	if err := c.Validar(ahora); err != nil {
		return Ruta{}, err
	}
	id, err := nuevoID()
	if err != nil {
		return Ruta{}, err
	}
	return Ruta{
		ID:               id,
		ConductorID:      strings.TrimSpace(c.ConductorID),
		Origen:           c.Origen,
		Destino:          c.Destino,
		Trazado:          c.Trazado,
		Salida:           c.Salida.UTC(),
		CuposTotales:     c.CuposTotales,
		CuposDisponibles: c.CuposTotales,
		PrecioTotal:      c.PrecioTotal,
		Estado:           Abierta,
	}, nil
}

func validarLugar(campo string, lugar Lugar) error {
	if strings.TrimSpace(lugar.Nombre) == "" {
		return campoInvalido(campo+".nombre", "es obligatorio")
	}
	if lugar.Punto.Type != TipoPunto {
		return campoInvalido(campo+".punto.type", `debe ser "Point"`)
	}
	return validarCoordenadas(campo+".punto.coordinates", lugar.Punto.Coordinates)
}

func validarTrazado(trazado LineString) error {
	if trazado.Type != TipoLineString {
		return campoInvalido("trazado", `debe ser un "LineString"`)
	}
	if len(trazado.Coordinates) < 2 {
		return campoInvalido("trazado.coordinates", "debe tener al menos 2 posiciones")
	}
	for i, posicion := range trazado.Coordinates {
		if err := validarCoordenadas(fmt.Sprintf("trazado.coordinates[%d]", i), posicion); err != nil {
			return err
		}
	}
	return nil
}

func validarCoordenadas(campo string, coordenadas []float64) error {
	if len(coordenadas) != 2 {
		return campoInvalido(campo, "debe ser [longitud, latitud]")
	}
	longitud, latitud := coordenadas[0], coordenadas[1]
	if math.IsNaN(longitud) || math.IsInf(longitud, 0) || longitud < -180 || longitud > 180 {
		return campoInvalido(campo, "la longitud debe estar entre -180 y 180")
	}
	if math.IsNaN(latitud) || math.IsInf(latitud, 0) || latitud < -90 || latitud > 90 {
		return campoInvalido(campo, "la latitud debe estar entre -90 y 90")
	}
	return nil
}

func nuevoID() (string, error) {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generar el identificador de la ruta: %w", err)
	}
	return "ruta-" + hex.EncodeToString(bytes[:]), nil
}
