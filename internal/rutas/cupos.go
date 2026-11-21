package rutas

// ValidarDelta comprueba el ajuste de cupos que acepta PATCH
// /rutas/{id}/cupos: un entero distinto de cero con magnitud máxima de 6, tal
// como declara el contrato.
func ValidarDelta(delta int) error {
	if delta == 0 {
		return campoInvalido("delta", "no puede ser cero")
	}
	if delta < -DeltaMaximo || delta > DeltaMaximo {
		return campoInvalido("delta", "debe estar entre -6 y 6 y ser distinto de cero")
	}
	return nil
}
