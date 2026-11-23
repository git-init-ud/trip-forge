package clients

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"time"

	"github.com/git-init-ud/trip-forge/internal/matching"
)

// Reputacion es el estado de confianza de un conductor según Pasajeros y
// reputación. Los campos opcionales se copian al panel solo si el proveedor los
// devuelve.
type Reputacion struct {
	Puntaje             float64
	Insignia            matching.Insignia
	ViajesCompletados   *int
	TasaCancelacion     *float64
	TotalCalificaciones *int
}

// ClienteReputacion consulta GET /perfiles/{id}/puntaje.
type ClienteReputacion struct {
	base *clienteBase
}

// NuevoClienteReputacion construye el cliente de reputación. secreto es el
// valor de GATEWAY_SECRET y token el token de servicio opcional.
func NuevoClienteReputacion(baseURL string, plazo time.Duration, secreto, token string) (*ClienteReputacion, error) {
	base, err := nuevoClienteBase("reputación", baseURL, plazo, secreto, token)
	if err != nil {
		return nil, err
	}
	return &ClienteReputacion{base: base}, nil
}

// respuestaPuntaje usa punteros para distinguir un campo ausente de un cero
// legítimo: puntaje 0 e insignia vacía no son respuestas válidas.
type respuestaPuntaje struct {
	Puntaje             *float64 `json:"puntaje"`
	Insignia            *string  `json:"insignia"`
	ViajesCompletados   *int     `json:"viajesCompletados"`
	TasaCancelacion     *float64 `json:"tasaCancelacion"`
	TotalCalificaciones *int     `json:"totalCalificaciones"`
}

// Puntaje consulta la reputación de un conductor.
//
// Un 404 se traduce en ErrPerfilNoEncontrado para que el MID omita sus rutas
// (búsqueda) o responda 404 (panel). Cualquier otro fallo, un JSON inválido o
// campos fuera del contrato se traducen en ErrNoDisponible.
func (c *ClienteReputacion) Puntaje(ctx context.Context, conductorID string) (Reputacion, error) {
	// El identificador se inserta como un segmento literal: se valida antes
	// para no depender de url.PathEscape y evitar la doble codificación de la
	// URL. Un valor fuera del patrón no se consulta.
	if !IDConductorValido(conductorID) {
		return Reputacion{}, fmt.Errorf("%w: identificador de conductor inválido", ErrRespuestaInesperada)
	}
	var respuesta respuestaPuntaje
	consulta := url.Values{}
	estado, err := c.base.obtener(ctx, "/perfiles/"+conductorID+"/puntaje", consulta, &respuesta)
	if err != nil {
		return Reputacion{}, err
	}
	switch {
	case estado == 404:
		return Reputacion{}, fmt.Errorf("%w: %s", ErrPerfilNoEncontrado, conductorID)
	case estado != 200:
		return Reputacion{}, errorDeEstado("reputación", estado)
	}
	return validarPuntaje(respuesta)
}

func validarPuntaje(respuesta respuestaPuntaje) (Reputacion, error) {
	if respuesta.Puntaje == nil || !enRango(*respuesta.Puntaje, 0, 5) {
		return Reputacion{}, fmt.Errorf("%w: reputación devolvió un puntaje inválido", ErrNoDisponible)
	}
	if respuesta.Insignia == nil || !matching.Insignia(*respuesta.Insignia).Valida() {
		return Reputacion{}, fmt.Errorf("%w: reputación devolvió una insignia inválida", ErrNoDisponible)
	}
	if respuesta.ViajesCompletados != nil && *respuesta.ViajesCompletados < 0 {
		return Reputacion{}, fmt.Errorf("%w: reputación devolvió viajesCompletados inválido", ErrNoDisponible)
	}
	if respuesta.TotalCalificaciones != nil && *respuesta.TotalCalificaciones < 0 {
		return Reputacion{}, fmt.Errorf("%w: reputación devolvió totalCalificaciones inválido", ErrNoDisponible)
	}
	if respuesta.TasaCancelacion != nil &&
		(!esFinito(*respuesta.TasaCancelacion) || *respuesta.TasaCancelacion < 0 || *respuesta.TasaCancelacion > 1) {
		return Reputacion{}, fmt.Errorf("%w: reputación devolvió tasaCancelacion inválida", ErrNoDisponible)
	}
	return Reputacion{
		Puntaje:             *respuesta.Puntaje,
		Insignia:            matching.Insignia(*respuesta.Insignia),
		ViajesCompletados:   respuesta.ViajesCompletados,
		TasaCancelacion:     respuesta.TasaCancelacion,
		TotalCalificaciones: respuesta.TotalCalificaciones,
	}, nil
}

func enRango(valor, minimo, maximo float64) bool {
	return esFinito(valor) && valor >= minimo && valor <= maximo
}

func esFinito(valor float64) bool {
	return !math.IsNaN(valor) && !math.IsInf(valor, 0)
}
