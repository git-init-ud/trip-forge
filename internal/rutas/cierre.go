package rutas

// ValidarCierre comprueba si el documento puede cerrarse. Cerrar una ruta ya
// cerrada es idempotente, pero solo si no tiene cupos comprometidos: un
// documento inconsistente con reservas activas se rechaza para no ocultarlas.
func ValidarCierre(actual Ruta) error {
	if actual.Ocupados() > 0 {
		return ErrReservasActivas
	}
	// Sin comprometidos, tanto una ruta abierta como una ya cerrada son un
	// cierre válido; el llamador informa el éxito y no reescribe si ya estaba
	// cerrada.
	return nil
}
