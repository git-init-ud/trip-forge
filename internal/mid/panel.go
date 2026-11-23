package mid

import (
	"context"
	"errors"
	"sort"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/matching"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

// Panel consulta primero el perfil, luego todas las rutas del conductor y
// calcula la ocupación. No devuelve un panel parcial: si una dependencia falla,
// el error se traduce en 503.
func (s *Servicio) Panel(ctx context.Context, conductorID string) (Panel, error) {
	reputacion, err := s.reputacion.Puntaje(ctx, conductorID)
	if err != nil {
		if errors.Is(err, clients.ErrPerfilNoEncontrado) {
			return Panel{}, ErrConductorNoEncontrado
		}
		return Panel{}, &ErrorDependencia{Servicio: ServicioReputacion, Err: err}
	}

	encontradas, err := s.rutas.ListarTodas(ctx, clients.FiltrosRutas{ConductorID: conductorID})
	if err != nil {
		return Panel{}, &ErrorDependencia{Servicio: ServicioRutas, Err: err}
	}

	// El CRUD ordena por salida ascendente; se repite el orden de forma
	// defensiva para que el contrato no dependa del transporte.
	sort.SliceStable(encontradas, func(i, j int) bool {
		if encontradas[i].Salida.Equal(encontradas[j].Salida) {
			return encontradas[i].ID < encontradas[j].ID
		}
		return encontradas[i].Salida.Before(encontradas[j].Salida)
	})

	rutasConOcupacion := make([]RutaConOcupacion, 0, len(encontradas))
	for _, ruta := range encontradas {
		ocupados := ruta.CuposTotales - ruta.CuposDisponibles
		if ocupados < 0 {
			ocupados = 0
		}
		rutasConOcupacion = append(rutasConOcupacion, RutaConOcupacion{
			Ruta:     ruta,
			Ocupados: ocupados,
		})
	}

	return Panel{
		ConductorID:         conductorID,
		Puntaje:             reputacion.Puntaje,
		Insignia:            reputacion.Insignia,
		ViajesCompletados:   reputacion.ViajesCompletados,
		TasaCancelacion:     reputacion.TasaCancelacion,
		TotalCalificaciones: reputacion.TotalCalificaciones,
		Rutas:               rutasConOcupacion,
	}, nil
}

// puntosDelTrazado convierte las coordenadas GeoJSON [longitud, latitud] del
// contrato en puntos validados. Una ruta con un trazado inválido se descarta,
// nunca se proyecta con datos parciales.
func puntosDelTrazado(trazado rutas.LineString) ([]matching.Punto, error) {
	if trazado.Type != rutas.TipoLineString || len(trazado.Coordinates) < 2 {
		return nil, matching.ErrTrazadoInvalido
	}
	puntos := make([]matching.Punto, 0, len(trazado.Coordinates))
	for _, posicion := range trazado.Coordinates {
		if len(posicion) != 2 {
			return nil, matching.ErrTrazadoInvalido
		}
		punto, err := matching.NuevoPunto(posicion[1], posicion[0])
		if err != nil {
			return nil, matching.ErrTrazadoInvalido
		}
		puntos = append(puntos, punto)
	}
	return puntos, nil
}
