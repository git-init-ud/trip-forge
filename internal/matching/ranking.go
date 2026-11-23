package matching

import "sort"

// Insignia es la etiqueta de confianza que publica Pasajeros y reputación.
type Insignia string

const (
	// InsigniaNuevo identifica a un conductor con pocos viajes completados.
	InsigniaNuevo Insignia = "nuevo"
	// InsigniaConfiable identifica al conductor con mejor reputación.
	InsigniaConfiable Insignia = "confiable"
	// InsigniaRegular es la etiqueta intermedia.
	InsigniaRegular Insignia = "regular"
	// InsigniaEnObservacion bloquea al conductor en la búsqueda y en Reservas.
	InsigniaEnObservacion Insignia = "en_observacion"
)

// Valida comprueba que la insignia pertenezca al enum acordado.
func (i Insignia) Valida() bool {
	switch i {
	case InsigniaNuevo, InsigniaConfiable, InsigniaRegular, InsigniaEnObservacion:
		return true
	default:
		return false
	}
}

// Candidato es una ruta que ya pasó los filtros geográficos y tiene la
// reputación de su conductor. Es independiente del documento HTTP de la ruta:
// así el ranking se prueba sin transportes ni base de datos.
type Candidato struct {
	RutaID            string
	DistanciaOrigenM  float64
	DistanciaDestinoM float64
	PuntajeConductor  float64
	Puntaje           float64
	Insignia          Insignia
}

// NuevoCandidato calcula el valor de ordenamiento del contrato:
// (distanciaOrigenM + distanciaDestinoM) / 100 - puntajeConductor * 2.
func NuevoCandidato(rutaID string, distanciaOrigenM, distanciaDestinoM, puntajeConductor float64, insignia Insignia) Candidato {
	return Candidato{
		RutaID:            rutaID,
		DistanciaOrigenM:  distanciaOrigenM,
		DistanciaDestinoM: distanciaDestinoM,
		PuntajeConductor:  puntajeConductor,
		Puntaje:           Puntaje(distanciaOrigenM, distanciaDestinoM, puntajeConductor),
		Insignia:          insignia,
	}
}

// Puntaje aplica la fórmula del enunciado. Menor es mejor.
func Puntaje(distanciaOrigenM, distanciaDestinoM, puntajeConductor float64) float64 {
	return (distanciaOrigenM+distanciaDestinoM)/100 - puntajeConductor*2
}

// Elegible indica si el candidato puede mostrarse al pasajero.
func (c Candidato) Elegible() bool {
	return c.Insignia != InsigniaEnObservacion
}

// OrdenarCandidatos ordena de menor a mayor puntaje y desempata por ruta.id
// ascendente. El desempate por identificador mantiene el resultado estable
// entre búsquedas y entre procesos.
func OrdenarCandidatos(candidatos []Candidato) {
	sort.SliceStable(candidatos, func(i, j int) bool {
		if candidatos[i].Puntaje != candidatos[j].Puntaje {
			return candidatos[i].Puntaje < candidatos[j].Puntaje
		}
		return candidatos[i].RutaID < candidatos[j].RutaID
	})
}
