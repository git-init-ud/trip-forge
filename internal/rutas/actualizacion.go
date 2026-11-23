package rutas

import "time"

// ActualizarRuta es la entrada validada de PUT /rutas/{id}. No incluye
// conductorId: el propietario se comprueba con la identidad autenticada y el
// conductor original nunca cambia.
type ActualizarRuta struct {
	Origen       Lugar
	Destino      Lugar
	Trazado      LineString
	Salida       time.Time
	CuposTotales int
	PrecioTotal  int64
}

// Validar aplica las mismas reglas de publicación de R-HU1 sobre los datos
// editables: lugares y trazado GeoJSON válidos, salida futura, entre 1 y 6
// cupos y precio no negativo.
func (a ActualizarRuta) Validar(ahora time.Time) error {
	if err := validarLugar("origen", a.Origen); err != nil {
		return err
	}
	if err := validarLugar("destino", a.Destino); err != nil {
		return err
	}
	if err := validarTrazado(a.Trazado); err != nil {
		return err
	}
	if a.Salida.IsZero() {
		return campoInvalido("salida", "es obligatoria")
	}
	if !a.Salida.After(ahora) {
		return campoInvalido("salida", "debe ser una fecha futura")
	}
	if a.CuposTotales < CuposMinimos || a.CuposTotales > CuposMaximos {
		return campoInvalido("cuposTotales", "debe estar entre 1 y 6")
	}
	if a.PrecioTotal < 0 {
		return campoInvalido("precioTotal", "no puede ser negativo")
	}
	return nil
}

// ValidarContra añade las reglas de R-HU3 sobre el documento actual: una ruta
// cerrada no se edita y el nuevo total no puede quedar por debajo de los cupos
// ya comprometidos por reservas.
func (a ActualizarRuta) ValidarContra(actual Ruta, ahora time.Time) error {
	if err := a.Validar(ahora); err != nil {
		return err
	}
	if actual.Estado == Cerrada {
		return ErrRutaCerrada
	}
	if a.CuposTotales < actual.Ocupados() {
		return ErrReservasActivas
	}
	return nil
}

// Aplicar construye la ruta resultante conservando los cupos comprometidos. Los
// disponibles se recalculan como nuevo total menos comprometidos y el estado
// depende de esa disponibilidad: llena al llegar a cero, abierta en otro caso.
func (a ActualizarRuta) Aplicar(actual Ruta) Ruta {
	comprometidos := actual.Ocupados()
	actual.Origen = a.Origen
	actual.Destino = a.Destino
	actual.Trazado = a.Trazado
	actual.Salida = a.Salida.UTC()
	actual.CuposTotales = a.CuposTotales
	actual.CuposDisponibles = a.CuposTotales - comprometidos
	actual.PrecioTotal = a.PrecioTotal
	actual.Estado = estadoSegunDisponibles(actual.CuposDisponibles)
	return actual
}

// estadoSegunDisponibles deriva el estado publicado de la disponibilidad. Una
// ruta agotada queda llena; cualquier disponibilidad positiva, abierta.
func estadoSegunDisponibles(disponibles int) Estado {
	if disponibles <= 0 {
		return Llena
	}
	return Abierta
}
