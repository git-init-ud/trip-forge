package rutas

import "errors"

var (
	// ErrNoEncontrada indica que no existe una ruta con el identificador dado.
	ErrNoEncontrada = errors.New("la ruta no existe")
	// ErrSinCupos indica que un descuento no encuentra disponibilidad.
	ErrSinCupos = errors.New("la ruta no tiene cupos disponibles")
	// ErrCuposExcedidos indica que una devolución superaría los cupos totales.
	ErrCuposExcedidos = errors.New("la ruta no puede superar los cupos totales")
	// ErrRutaCerrada indica que se intentó modificar una ruta cerrada.
	ErrRutaCerrada = errors.New("la ruta está cerrada")
	// ErrNoAutorizado indica que la identidad autenticada no es el propietario
	// de la ruta o que la solicitud no trae identidad.
	ErrNoAutorizado = errors.New("la identidad no puede operar esta ruta")
	// ErrReservasActivas indica que una edición o un cierre dejaría cupos
	// comprometidos sin una ruta que los sostenga.
	ErrReservasActivas = errors.New("la ruta tiene cupos comprometidos")
)

// ErrorValidacion describe una entrada inválida y el campo que la origina.
// Es un error de dominio: el transporte decide cómo traducirlo a una respuesta.
type ErrorValidacion struct {
	Campo  string
	Motivo string
}

// Error implementa la interfaz error.
func (e *ErrorValidacion) Error() string {
	if e.Campo == "" {
		return e.Motivo
	}
	return e.Campo + ": " + e.Motivo
}

func campoInvalido(campo, motivo string) error {
	return &ErrorValidacion{Campo: campo, Motivo: motivo}
}
