package mid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/matching"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

type rutasFalsas struct {
	encontradas []rutas.Ruta
	err         error
	filtros     []clients.FiltrosRutas
}

func (f *rutasFalsas) ListarTodas(_ context.Context, filtros clients.FiltrosRutas) ([]rutas.Ruta, error) {
	f.filtros = append(f.filtros, filtros)
	if f.err != nil {
		return nil, f.err
	}
	return f.encontradas, nil
}

type reputacionFalsa struct {
	mutex    chan struct{}
	puntajes map[string]clients.Reputacion
	errores  map[string]error
	llamadas []string
	enCurso  int
	maximo   int
	demora   time.Duration
}

func nuevaReputacionFalsa(puntajes map[string]clients.Reputacion) *reputacionFalsa {
	return &reputacionFalsa{
		mutex:    make(chan struct{}, 1),
		puntajes: puntajes,
		errores:  map[string]error{},
	}
}

func (f *reputacionFalsa) bloquear() {
	f.mutex <- struct{}{}
}

func (f *reputacionFalsa) liberar() {
	<-f.mutex
}

func (f *reputacionFalsa) Puntaje(_ context.Context, conductorID string) (clients.Reputacion, error) {
	f.bloquear()
	f.llamadas = append(f.llamadas, conductorID)
	f.enCurso++
	if f.enCurso > f.maximo {
		f.maximo = f.enCurso
	}
	f.liberar()

	if f.demora > 0 {
		time.Sleep(f.demora)
	}

	f.bloquear()
	f.enCurso--
	err := f.errores[conductorID]
	reputacion, existe := f.puntajes[conductorID]
	f.liberar()

	if err != nil {
		return clients.Reputacion{}, err
	}
	if !existe {
		return clients.Reputacion{}, clients.ErrPerfilNoEncontrado
	}
	return reputacion, nil
}

func (f *reputacionFalsa) contarLlamadas() []string {
	f.bloquear()
	defer f.liberar()
	copia := make([]string, len(f.llamadas))
	copy(copia, f.llamadas)
	return copia
}

func (f *reputacionFalsa) maximoSimultaneo() int {
	f.bloquear()
	defer f.liberar()
	return f.maximo
}

// rutaEnLinea usa un trazado horizontal de 2.2 km; el origen y el destino de
// busquedaDePrueba caen sobre él en el sentido correcto.
func rutaEnLinea(id, conductor string, salida time.Time) rutas.Ruta {
	return rutas.Ruta{
		ID:          id,
		ConductorID: conductor,
		Origen: rutas.Lugar{
			Nombre: "Chapinero",
			Punto:  rutas.Punto{Type: rutas.TipoPunto, Coordinates: []float64{-74.0600, 4.6500}},
		},
		Destino: rutas.Lugar{
			Nombre: "Universidad",
			Punto:  rutas.Punto{Type: rutas.TipoPunto, Coordinates: []float64{-74.0400, 4.6500}},
		},
		Trazado: rutas.LineString{
			Type: rutas.TipoLineString,
			Coordinates: [][]float64{
				{-74.0600, 4.6500},
				{-74.0400, 4.6500},
			},
		},
		Salida:           salida,
		CuposTotales:     3,
		CuposDisponibles: 3,
		PrecioTotal:      12000,
		Estado:           rutas.Abierta,
	}
}

func busquedaDePrueba() Busqueda {
	return Busqueda{
		Origen:  matching.Punto{Latitud: 4.6500, Longitud: -74.0550},
		Destino: matching.Punto{Latitud: 4.6500, Longitud: -74.0450},
		Desde:   time.Date(2030, 1, 15, 6, 0, 0, 0, time.UTC),
		Hasta:   time.Date(2030, 1, 15, 9, 0, 0, 0, time.UTC),
	}
}

// salidaDePrueba cae dentro de la ventana fija de busquedaDePrueba.
func salidaDePrueba() time.Time {
	return time.Date(2030, 1, 15, 7, 0, 0, 0, time.UTC)
}

func servicioDePrueba(t *testing.T, rutasFake BuscadorRutas, reputacionFake ConsultorReputacion, opciones Opciones) *Servicio {
	t.Helper()
	servicio, err := NuevoServicio(rutasFake, reputacionFake, opciones)
	if err != nil {
		t.Fatalf("construir el servicio: %v", err)
	}
	return servicio
}

func TestBuscarOrdenaPorPuntajeDelConductor(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-bajo": {Puntaje: 3.0, Insignia: matching.InsigniaRegular},
		"usr-alto": {Puntaje: 4.8, Insignia: matching.InsigniaConfiable},
	})
	repositorio := &rutasFalsas{encontradas: []rutas.Ruta{
		rutaEnLinea("ruta-baja", "usr-bajo", time.Date(2030, 1, 15, 7, 0, 0, 0, time.UTC)),
		rutaEnLinea("ruta-alta", "usr-alto", time.Date(2030, 1, 15, 7, 0, 0, 0, time.UTC)),
	}}
	servicio := servicioDePrueba(t, repositorio, reputacion, Opciones{})

	resultado, err := servicio.Buscar(context.Background(), busquedaDePrueba())
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if len(resultado.Items) != 2 {
		t.Fatalf("items = %d", len(resultado.Items))
	}
	if resultado.RadioM != RadioDefectoM {
		t.Fatalf("radioM = %v", resultado.RadioM)
	}
	if resultado.Items[0].Ruta.ID != "ruta-alta" || resultado.Items[1].Ruta.ID != "ruta-baja" {
		t.Fatalf("orden inesperado: %+v", resultado.Items)
	}
	primera := resultado.Items[0]
	if primera.Insignia != matching.InsigniaConfiable || primera.PuntajeConductor != 4.8 {
		t.Fatalf("reputación inesperada: %+v", primera)
	}
	esperado := (primera.DistanciaOrigenM+primera.DistanciaDestinoM)/100 - 4.8*2
	if primera.Puntaje != esperado {
		t.Fatalf("puntaje = %v, se esperaba %v", primera.Puntaje, esperado)
	}
	if len(repositorio.filtros) != 1 {
		t.Fatalf("filtros = %+v", repositorio.filtros)
	}
	filtro := repositorio.filtros[0]
	if filtro.Estado != string(rutas.Abierta) || filtro.Desde == nil || filtro.Hasta == nil {
		t.Fatalf("filtros = %+v", filtro)
	}
}

func TestBuscarExcluyeConductorEnObservacion(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-observado": {Puntaje: 2.0, Insignia: matching.InsigniaEnObservacion},
		"usr-regular":   {Puntaje: 3.5, Insignia: matching.InsigniaRegular},
	})
	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{
		rutaEnLinea("ruta-observada", "usr-observado", salidaDePrueba()),
		rutaEnLinea("ruta-regular", "usr-regular", salidaDePrueba()),
	}}, reputacion, Opciones{})

	resultado, err := servicio.Buscar(context.Background(), busquedaDePrueba())
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if len(resultado.Items) != 1 || resultado.Items[0].Ruta.ID != "ruta-regular" {
		t.Fatalf("items = %+v", resultado.Items)
	}
}

func TestBuscarDescartaRutaFueraDeVentana(t *testing.T) {
	ventana := busquedaDePrueba()
	casos := []struct {
		nombre string
		salida time.Time
	}{
		{"antes de desde", ventana.Desde.Add(-time.Second)},
		{"después de hasta", ventana.Hasta.Add(time.Second)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
				"usr-12": {Puntaje: 4.8, Insignia: matching.InsigniaConfiable},
			})
			servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{
				rutaEnLinea("ruta-fuera", "usr-12", caso.salida),
			}}, reputacion, Opciones{})

			resultado, err := servicio.Buscar(context.Background(), ventana)
			if err != nil {
				t.Fatalf("buscar: %v", err)
			}
			if len(resultado.Items) != 0 {
				t.Fatalf("items = %+v", resultado.Items)
			}
			if llamadas := reputacion.contarLlamadas(); len(llamadas) != 0 {
				t.Fatalf("una ruta fuera de ventana no debe llegar a reputación: %v", llamadas)
			}
		})
	}
}

func TestBuscarAceptaSalidasEnLosBordesDeLaVentana(t *testing.T) {
	ventana := busquedaDePrueba()
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4.8, Insignia: matching.InsigniaConfiable},
	})
	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{
		rutaEnLinea("ruta-desde", "usr-12", ventana.Desde),
		rutaEnLinea("ruta-hasta", "usr-12", ventana.Hasta),
	}}, reputacion, Opciones{})

	resultado, err := servicio.Buscar(context.Background(), ventana)
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if len(resultado.Items) != 2 {
		t.Fatalf("los bordes son inclusivos: %+v", resultado.Items)
	}
}

func TestBuscarOmiteConductorSinPerfil(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-existe": {Puntaje: 4.0, Insignia: matching.InsigniaConfiable},
	})
	reputacion.errores["usr-sin-perfil"] = clients.ErrPerfilNoEncontrado

	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{
		rutaEnLinea("ruta-sin-perfil", "usr-sin-perfil", salidaDePrueba()),
		rutaEnLinea("ruta-existe", "usr-existe", salidaDePrueba()),
	}}, reputacion, Opciones{})

	resultado, err := servicio.Buscar(context.Background(), busquedaDePrueba())
	if err != nil {
		t.Fatalf("un 404 de reputación no debe romper la búsqueda: %v", err)
	}
	if len(resultado.Items) != 1 || resultado.Items[0].Ruta.ID != "ruta-existe" {
		t.Fatalf("items = %+v", resultado.Items)
	}
}

func TestBuscarSinCandidatosNoConsultaReputacion(t *testing.T) {
	reputacion := nuevaReputacionFalsa(nil)
	reputacion.errores["cualquiera"] = clients.ErrNoDisponible

	lejana := rutaEnLinea("ruta-lejana", "usr-12", salidaDePrueba())
	lejana.Trazado.Coordinates = [][]float64{
		{-74.2000, 4.6500},
		{-74.1800, 4.6500},
	}
	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{lejana}}, reputacion, Opciones{})

	resultado, err := servicio.Buscar(context.Background(), busquedaDePrueba())
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if resultado.Items == nil || len(resultado.Items) != 0 {
		t.Fatalf("items = %#v", resultado.Items)
	}
	if llamadas := reputacion.contarLlamadas(); len(llamadas) != 0 {
		t.Fatalf("no se debe consultar reputación sin candidatos: %v", llamadas)
	}
}

func TestBuscarReputacionNoDisponibleSinCandidatosNoFalla(t *testing.T) {
	reputacion := nuevaReputacionFalsa(nil)
	reputacion.errores["usr-12"] = clients.ErrNoDisponible
	servicio := servicioDePrueba(t, &rutasFalsas{}, reputacion, Opciones{})

	if _, err := servicio.Buscar(context.Background(), busquedaDePrueba()); err != nil {
		t.Fatalf("sin candidatos la reputación no importa: %v", err)
	}
}

func TestBuscarReputacionNoDisponibleConCandidatos(t *testing.T) {
	reputacion := nuevaReputacionFalsa(nil)
	reputacion.errores["usr-12"] = clients.ErrNoDisponible
	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{
		rutaEnLinea("ruta-uno", "usr-12", salidaDePrueba()),
	}}, reputacion, Opciones{})

	_, err := servicio.Buscar(context.Background(), busquedaDePrueba())
	if err == nil {
		t.Fatal("se esperaba dependencia no disponible")
	}
	var dependencia *ErrorDependencia
	if !errors.As(err, &dependencia) || dependencia.Servicio != ServicioReputacion {
		t.Fatalf("error = %v", err)
	}
}

func TestBuscarCRUDNoDisponible(t *testing.T) {
	reputacion := nuevaReputacionFalsa(nil)
	servicio := servicioDePrueba(t, &rutasFalsas{err: clients.ErrNoDisponible}, reputacion, Opciones{})

	_, err := servicio.Buscar(context.Background(), busquedaDePrueba())
	var dependencia *ErrorDependencia
	if !errors.As(err, &dependencia) || dependencia.Servicio != ServicioRutas {
		t.Fatalf("error = %v", err)
	}
}

func TestBuscarDeduplicaConductores(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4.0, Insignia: matching.InsigniaConfiable},
	})
	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{
		rutaEnLinea("ruta-uno", "usr-12", salidaDePrueba()),
		rutaEnLinea("ruta-dos", "usr-12", salidaDePrueba()),
	}}, reputacion, Opciones{})

	if _, err := servicio.Buscar(context.Background(), busquedaDePrueba()); err != nil {
		t.Fatalf("buscar: %v", err)
	}
	llamadas := reputacion.contarLlamadas()
	if len(llamadas) != 1 || llamadas[0] != "usr-12" {
		t.Fatalf("se esperaba una consulta por conductor: %v", llamadas)
	}
}

func TestBuscarLimitaLaConcurrenciaDeReputacion(t *testing.T) {
	puntajes := map[string]clients.Reputacion{}
	conductores := []string{"usr-1", "usr-2", "usr-3", "usr-4", "usr-5", "usr-6"}
	rutasEncontradas := make([]rutas.Ruta, 0, len(conductores))
	for _, conductor := range conductores {
		puntajes[conductor] = clients.Reputacion{Puntaje: 4.0, Insignia: matching.InsigniaConfiable}
		rutasEncontradas = append(rutasEncontradas, rutaEnLinea("ruta-"+conductor, conductor, salidaDePrueba()))
	}
	reputacion := nuevaReputacionFalsa(puntajes)
	reputacion.demora = 5 * time.Millisecond

	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: rutasEncontradas}, reputacion, Opciones{Concurrencia: 2})
	if _, err := servicio.Buscar(context.Background(), busquedaDePrueba()); err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if llamadas := reputacion.contarLlamadas(); len(llamadas) != len(conductores) {
		t.Fatalf("llamadas = %v", llamadas)
	}
	if maximo := reputacion.maximoSimultaneo(); maximo > 2 {
		t.Fatalf("la concurrencia máxima fue %d, se esperaba como máximo 2", maximo)
	}
}

func TestBuscarDescartaTrazadoInvalido(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4.0, Insignia: matching.InsigniaConfiable},
	})
	invalida := rutaEnLinea("ruta-invalida", "usr-12", salidaDePrueba())
	invalida.Trazado.Coordinates = [][]float64{{-74.06, 95}, {-74.04, 4.65}}
	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{
		invalida,
		rutaEnLinea("ruta-valida", "usr-12", salidaDePrueba()),
	}}, reputacion, Opciones{})

	resultado, err := servicio.Buscar(context.Background(), busquedaDePrueba())
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if len(resultado.Items) != 1 || resultado.Items[0].Ruta.ID != "ruta-valida" {
		t.Fatalf("items = %+v", resultado.Items)
	}
	if llamadas := reputacion.contarLlamadas(); len(llamadas) != 1 {
		t.Fatalf("la ruta inválida no debe llegar a reputación: %v", llamadas)
	}
}

func TestBuscarDescartaRutaSinCupos(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4.0, Insignia: matching.InsigniaConfiable},
	})
	sinCupos := rutaEnLinea("ruta-llena", "usr-12", salidaDePrueba())
	sinCupos.CuposDisponibles = 0
	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{sinCupos}}, reputacion, Opciones{})

	resultado, err := servicio.Buscar(context.Background(), busquedaDePrueba())
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if len(resultado.Items) != 0 {
		t.Fatalf("items = %+v", resultado.Items)
	}
	if llamadas := reputacion.contarLlamadas(); len(llamadas) != 0 {
		t.Fatalf("llamadas = %v", llamadas)
	}
}

func TestPanelCalculaOcupadosYOrdena(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-12": {
			Puntaje:             4.7,
			Insignia:            matching.InsigniaConfiable,
			ViajesCompletados:   punteroA(12),
			TasaCancelacion:     punteroA(0.05),
			TotalCalificaciones: punteroA(9),
		},
	})
	primera := rutaEnLinea("ruta-primera", "usr-12", time.Date(2030, 1, 15, 6, 30, 0, 0, time.UTC))
	primera.CuposDisponibles = 2
	segunda := rutaEnLinea("ruta-segunda", "usr-12", time.Date(2030, 1, 15, 8, 0, 0, 0, time.UTC))
	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{segunda, primera}}, reputacion, Opciones{})

	panel, err := servicio.Panel(context.Background(), "usr-12")
	if err != nil {
		t.Fatalf("panel: %v", err)
	}
	if panel.ConductorID != "usr-12" || panel.Puntaje != 4.7 || panel.Insignia != matching.InsigniaConfiable {
		t.Fatalf("panel = %+v", panel)
	}
	if panel.ViajesCompletados == nil || *panel.ViajesCompletados != 12 {
		t.Fatalf("viajesCompletados = %v", panel.ViajesCompletados)
	}
	if len(panel.Rutas) != 2 || panel.Rutas[0].Ruta.ID != "ruta-primera" || panel.Rutas[1].Ruta.ID != "ruta-segunda" {
		t.Fatalf("orden de rutas = %+v", panel.Rutas)
	}
	if panel.Rutas[0].Ocupados != 1 || panel.Rutas[1].Ocupados != 0 {
		t.Fatalf("ocupados = %+v", panel.Rutas)
	}
}

func TestPanelOmiteRutasAjenasYDuplicadas(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4.7, Insignia: matching.InsigniaConfiable},
	})
	propia := rutaEnLinea("ruta-propia", "usr-12", time.Date(2030, 1, 15, 6, 30, 0, 0, time.UTC))
	ajena := rutaEnLinea("ruta-ajena", "usr-99", time.Date(2030, 1, 15, 7, 0, 0, 0, time.UTC))

	servicio := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{
		ajena, propia, propia,
	}}, reputacion, Opciones{})

	panel, err := servicio.Panel(context.Background(), "usr-12")
	if err != nil {
		t.Fatalf("panel: %v", err)
	}
	if len(panel.Rutas) != 1 || panel.Rutas[0].Ruta.ID != "ruta-propia" {
		t.Fatalf("rutas = %+v", panel.Rutas)
	}

	soloAjena := servicioDePrueba(t, &rutasFalsas{encontradas: []rutas.Ruta{ajena}}, reputacion, Opciones{})
	panel, err = soloAjena.Panel(context.Background(), "usr-12")
	if err != nil {
		t.Fatalf("panel solo con rutas ajenas: %v", err)
	}
	if panel.Rutas == nil || len(panel.Rutas) != 0 {
		t.Fatalf("rutas = %#v, se esperaba lista vacía", panel.Rutas)
	}
}

func TestPanelSinRutasRespondeListaVacia(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-50": {Puntaje: 5, Insignia: matching.InsigniaConfiable},
	})
	servicio := servicioDePrueba(t, &rutasFalsas{}, reputacion, Opciones{})

	panel, err := servicio.Panel(context.Background(), "usr-50")
	if err != nil {
		t.Fatalf("panel: %v", err)
	}
	if panel.Rutas == nil || len(panel.Rutas) != 0 {
		t.Fatalf("rutas = %#v", panel.Rutas)
	}
}

func TestPanelPerfilInexistente(t *testing.T) {
	reputacion := nuevaReputacionFalsa(nil)
	repositorio := &rutasFalsas{}
	servicio := servicioDePrueba(t, repositorio, reputacion, Opciones{})

	_, err := servicio.Panel(context.Background(), "usr-99")
	if !errors.Is(err, ErrConductorNoEncontrado) {
		t.Fatalf("error = %v", err)
	}
	if len(repositorio.filtros) != 0 {
		t.Fatal("no se debe consultar rutas si el perfil no existe")
	}
}

func TestPanelCRUDNoDisponible(t *testing.T) {
	reputacion := nuevaReputacionFalsa(map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4.0, Insignia: matching.InsigniaConfiable},
	})
	servicio := servicioDePrueba(t, &rutasFalsas{err: clients.ErrNoDisponible}, reputacion, Opciones{})

	_, err := servicio.Panel(context.Background(), "usr-12")
	var dependencia *ErrorDependencia
	if !errors.As(err, &dependencia) || dependencia.Servicio != ServicioRutas {
		t.Fatalf("error = %v", err)
	}
}

func TestPanelReputacionNoDisponible(t *testing.T) {
	reputacion := nuevaReputacionFalsa(nil)
	reputacion.errores["usr-12"] = clients.ErrNoDisponible
	servicio := servicioDePrueba(t, &rutasFalsas{}, reputacion, Opciones{})

	_, err := servicio.Panel(context.Background(), "usr-12")
	var dependencia *ErrorDependencia
	if !errors.As(err, &dependencia) || dependencia.Servicio != ServicioReputacion {
		t.Fatalf("error = %v", err)
	}
}

func TestNuevoServicioValidaOpciones(t *testing.T) {
	reputacion := nuevaReputacionFalsa(nil)
	if _, err := NuevoServicio(nil, reputacion, Opciones{}); err == nil {
		t.Fatal("se esperaba error sin cliente de rutas")
	}
	if _, err := NuevoServicio(&rutasFalsas{}, nil, Opciones{}); err == nil {
		t.Fatal("se esperaba error sin cliente de reputación")
	}
	if _, err := NuevoServicio(&rutasFalsas{}, reputacion, Opciones{RadioM: -1}); err == nil {
		t.Fatal("se esperaba error con radio negativo")
	}
	if _, err := NuevoServicio(&rutasFalsas{}, reputacion, Opciones{Concurrencia: -1}); err == nil {
		t.Fatal("se esperaba error con concurrencia negativa")
	}
}

func punteroA[T any](valor T) *T {
	return &valor
}
