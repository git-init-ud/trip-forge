// Package mid orquesta la búsqueda de rutas compatibles y el panel del
// conductor.
//
// No habla con MongoDB: obtiene rutas del CRUD y reputación de Pasajeros y
// reputación mediante las interfaces BuscadorRutas y ConsultorReputacion. La
// geometría y el orden viven en internal/matching, de modo que las reglas se
// prueban con dobles sin HTTP.
package mid

import (
	"context"
	"errors"
	"log"
	"math"
	"sync"
	"time"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/matching"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

const (
	// RadioDefectoM es el radio de caminata acordado para la demo.
	RadioDefectoM = 800
	// ConcurrenciaDefecto limita las consultas simultáneas de reputación.
	ConcurrenciaDefecto = 4
)

const (
	// ServicioRutas identifica al CRUD en detalle.servicio de un 503.
	ServicioRutas = "rutas-crud"
	// ServicioReputacion identifica a Pasajeros y reputación.
	ServicioReputacion = "reputacion"
)

var (
	// ErrConductorNoEncontrado indica que el panel se pidió para un perfil
	// inexistente.
	ErrConductorNoEncontrado = errors.New("el perfil del conductor no existe")
)

// ErrorDependencia indica que una dependencia necesaria para calcular la
// respuesta no está disponible. El transporte lo traduce a 503 con
// detalle.servicio.
type ErrorDependencia struct {
	Servicio string
	Err      error
}

// Error implementa la interfaz error.
func (e *ErrorDependencia) Error() string {
	return "dependencia " + e.Servicio + " no disponible: " + e.Err.Error()
}

// Unwrap permite inspeccionar la causa con errors.Is.
func (e *ErrorDependencia) Unwrap() error {
	return e.Err
}

// BuscadorRutas es la consulta paginada de rutas al CRUD.
type BuscadorRutas interface {
	ListarTodas(ctx context.Context, filtros clients.FiltrosRutas) ([]rutas.Ruta, error)
}

// ConsultorReputacion devuelve el puntaje y la insignia de un conductor.
type ConsultorReputacion interface {
	Puntaje(ctx context.Context, conductorID string) (clients.Reputacion, error)
}

// Opciones configura el radio y la concurrencia del servicio. Los ceros usan
// los valores por defecto.
type Opciones struct {
	RadioM       float64
	Concurrencia int
}

// Busqueda es la entrada ya validada de GET /rutas/compatibles.
type Busqueda struct {
	Origen  matching.Punto
	Destino matching.Punto
	Desde   time.Time
	Hasta   time.Time
}

// Coincidencia es un item de la búsqueda. Reproduce el contrato del MID.
type Coincidencia struct {
	Ruta              rutas.Ruta        `json:"ruta"`
	DistanciaOrigenM  float64           `json:"distanciaOrigenM"`
	DistanciaDestinoM float64           `json:"distanciaDestinoM"`
	Puntaje           float64           `json:"puntaje"`
	PuntajeConductor  float64           `json:"puntajeConductor"`
	Insignia          matching.Insignia `json:"insignia"`
}

// ResultadoBusqueda es la respuesta completa de GET /rutas/compatibles.
type ResultadoBusqueda struct {
	Items  []Coincidencia `json:"items"`
	RadioM float64        `json:"radioM"`
}

// RutaConOcupacion agrega los cupos comprometidos a una ruta del panel.
type RutaConOcupacion struct {
	Ruta     rutas.Ruta `json:"ruta"`
	Ocupados int        `json:"ocupados"`
}

// Panel es la respuesta de GET /conductores/{id}/panel. Los campos opcionales
// se copian del perfil cuando el proveedor los informa.
type Panel struct {
	ConductorID         string             `json:"conductorId"`
	Puntaje             float64            `json:"puntaje"`
	Insignia            matching.Insignia  `json:"insignia"`
	ViajesCompletados   *int               `json:"viajesCompletados,omitempty"`
	TasaCancelacion     *float64           `json:"tasaCancelacion,omitempty"`
	TotalCalificaciones *int               `json:"totalCalificaciones,omitempty"`
	Rutas               []RutaConOcupacion `json:"rutas"`
}

// Servicio implementa las dos operaciones del MID.
type Servicio struct {
	rutas        BuscadorRutas
	reputacion   ConsultorReputacion
	radioM       float64
	concurrencia int
}

// NuevoServicio valida las dependencias y aplica los valores por defecto.
func NuevoServicio(rutas BuscadorRutas, reputacion ConsultorReputacion, opciones Opciones) (*Servicio, error) {
	if rutas == nil {
		return nil, errors.New("mid: falta el cliente de rutas")
	}
	if reputacion == nil {
		return nil, errors.New("mid: falta el cliente de reputación")
	}
	radio := opciones.RadioM
	if radio == 0 {
		radio = RadioDefectoM
	}
	if math.IsNaN(radio) || math.IsInf(radio, 0) || radio < 0 {
		return nil, errors.New("mid: RADIO_M debe ser positivo")
	}
	concurrencia := opciones.Concurrencia
	if concurrencia == 0 {
		concurrencia = ConcurrenciaDefecto
	}
	if concurrencia < 1 {
		return nil, errors.New("mid: REPUTACION_CONCURRENCIA debe ser mayor que cero")
	}
	return &Servicio{
		rutas:        rutas,
		reputacion:   reputacion,
		radioM:       radio,
		concurrencia: concurrencia,
	}, nil
}

// candidato es una ruta que ya pasó los filtros geográficos y todavía no tiene
// reputación.
type candidato struct {
	ruta              rutas.Ruta
	distanciaOrigenM  float64
	distanciaDestinoM float64
}

// Buscar aplica los filtros geográficos y de sentido, consulta la reputación
// una vez por conductor y devuelve las coincidencias ordenadas.
func (s *Servicio) Buscar(ctx context.Context, busqueda Busqueda) (ResultadoBusqueda, error) {
	desde := busqueda.Desde.UTC()
	hasta := busqueda.Hasta.UTC()
	encontradas, err := s.rutas.ListarTodas(ctx, clients.FiltrosRutas{
		Estado: string(rutas.Abierta),
		Desde:  &desde,
		Hasta:  &hasta,
	})
	if err != nil {
		return ResultadoBusqueda{}, &ErrorDependencia{Servicio: ServicioRutas, Err: err}
	}

	candidatas := make([]candidato, 0, len(encontradas))
	porID := make(map[string]rutas.Ruta, len(encontradas))
	vistos := make(map[string]struct{}, len(encontradas))
	for _, ruta := range encontradas {
		if _, repetida := vistos[ruta.ID]; repetida {
			continue
		}
		vistos[ruta.ID] = struct{}{}
		// Preselección defensiva: el CRUD ya filtra por estado, pero el MID no
		// confía en un inventario que llegue incompleto o inconsistente.
		if ruta.Estado != rutas.Abierta || ruta.CuposDisponibles < 1 || ruta.ConductorID == "" {
			continue
		}
		trazado, err := puntosDelTrazado(ruta.Trazado)
		if err != nil {
			log.Printf("mid: se descarta la ruta %q por trazado inválido request_id=%q",
				ruta.ID, clients.RequestIDDe(ctx))
			continue
		}
		evaluacion, compatible, err := matching.Evaluar(busqueda.Origen, busqueda.Destino, trazado, s.radioM)
		if err != nil {
			log.Printf("mid: se descarta la ruta %q en la evaluación geográfica request_id=%q",
				ruta.ID, clients.RequestIDDe(ctx))
			continue
		}
		if !compatible {
			continue
		}
		porID[ruta.ID] = ruta
		candidatas = append(candidatas, candidato{
			ruta:              ruta,
			distanciaOrigenM:  evaluacion.Origen.DistanciaM,
			distanciaDestinoM: evaluacion.Destino.DistanciaM,
		})
	}

	if len(candidatas) == 0 {
		return ResultadoBusqueda{Items: []Coincidencia{}, RadioM: s.radioM}, nil
	}

	conductores := conductoresDistintos(candidatas)
	reputaciones, err := s.consultarReputaciones(ctx, conductores)
	if err != nil {
		return ResultadoBusqueda{}, err
	}

	ordenadas := make([]matching.Candidato, 0, len(candidatas))
	for _, candidata := range candidatas {
		reputacion, existe := reputaciones[candidata.ruta.ConductorID]
		if !existe {
			// El perfil respondió 404; la inconsistencia ya quedó registrada.
			continue
		}
		ordenadas = append(ordenadas,
			matching.NuevoCandidato(candidata.ruta.ID,
				candidata.distanciaOrigenM, candidata.distanciaDestinoM,
				reputacion.Puntaje, reputacion.Insignia))
	}
	matching.OrdenarCandidatos(ordenadas)

	items := make([]Coincidencia, 0, len(ordenadas))
	for _, candidato := range ordenadas {
		if !candidato.Elegible() {
			continue
		}
		items = append(items, Coincidencia{
			Ruta:              porID[candidato.RutaID],
			DistanciaOrigenM:  candidato.DistanciaOrigenM,
			DistanciaDestinoM: candidato.DistanciaDestinoM,
			Puntaje:           candidato.Puntaje,
			PuntajeConductor:  candidato.PuntajeConductor,
			Insignia:          candidato.Insignia,
		})
	}
	return ResultadoBusqueda{Items: items, RadioM: s.radioM}, nil
}

// consultarReputaciones consulta una vez por conductor con concurrencia
// acotada. Un 404 omite al conductor; cualquier otro fallo aborta la búsqueda
// para no devolver un ranking incompleto.
func (s *Servicio) consultarReputaciones(ctx context.Context, conductores []string) (map[string]clients.Reputacion, error) {
	type resultado struct {
		reputacion clients.Reputacion
	}
	resultados := make(map[string]resultado, len(conductores))

	semaforo := make(chan struct{}, s.concurrencia)
	var espera sync.WaitGroup
	var mutex sync.Mutex
	var fallo error

	for _, conductor := range conductores {
		espera.Add(1)
		go func(conductor string) {
			defer espera.Done()
			semaforo <- struct{}{}
			defer func() { <-semaforo }()

			reputacion, err := s.reputacion.Puntaje(ctx, conductor)

			mutex.Lock()
			defer mutex.Unlock()
			switch {
			case err == nil:
				resultados[conductor] = resultado{reputacion: reputacion}
			case errors.Is(err, clients.ErrPerfilNoEncontrado):
				log.Printf("mid: reputación sin perfil conductor=%q request_id=%q",
					conductor, clients.RequestIDDe(ctx))
			default:
				if fallo == nil {
					fallo = err
				}
			}
		}(conductor)
	}
	espera.Wait()

	if fallo != nil {
		return nil, &ErrorDependencia{Servicio: ServicioReputacion, Err: fallo}
	}
	salida := make(map[string]clients.Reputacion, len(resultados))
	for conductor, valor := range resultados {
		salida[conductor] = valor.reputacion
	}
	return salida, nil
}

func conductoresDistintos(candidatas []candidato) []string {
	vistos := make(map[string]struct{}, len(candidatas))
	conductores := make([]string, 0, len(candidatas))
	for _, candidata := range candidatas {
		conductor := candidata.ruta.ConductorID
		if _, repetido := vistos[conductor]; repetido {
			continue
		}
		vistos[conductor] = struct{}{}
		conductores = append(conductores, conductor)
	}
	return conductores
}
