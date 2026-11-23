package rutas

// ValidarCierre comprueba si el documento puede cerrarse. Cerrar una ruta ya
// cerrada es idempotente: no cambia nada y se informa como éxito. Si hay cupos
// comprometidos el cierre se rechaza para no dejar reservas activas sin ruta.
func ValidarCierre(actual Ruta) error {
	if actual.Estado == Cerrada {
		return nil
	}
	if actual.Ocupados() > 0 {
		return ErrReservasActivas
	}
	return nil
}
