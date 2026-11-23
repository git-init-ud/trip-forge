package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/matching"
	"github.com/git-init-ud/trip-forge/internal/mid"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

type midRutasFalsas struct {
	encontradas []rutas.Ruta
	err         error
	filtros     []clients.FiltrosRutas
	llamadas    int
}

func (f *midRutasFalsas) ListarTodas(_ context.Context, filtros clients.FiltrosRutas) ([]rutas.Ruta, error) {
	f.llamadas++
	f.filtros = append(f.filtros, filtros)
	if f.err != nil {
		return nil, f.err
	}
	return f.encontradas, nil
}

type midReputacionFalsa struct {
	mutex        sync.Mutex
	reputaciones map[string]clients.Reputacion
	errores      map[string]error
	llamadas     []string
	requestIDs   []string
}

func (f *midReputacionFalsa) Puntaje(ctx context.Context, conductorID string) (clients.Reputacion, error) {
	f.mutex.Lock()
	f.llamadas = append(f.llamadas, conductorID)
	f.requestIDs = append(f.requestIDs, clients.RequestIDDe(ctx))
	f.mutex.Unlock()

	if err := f.errores[conductorID]; err != nil {
		return clients.Reputacion{}, err
	}
	reputacion, existe := f.reputaciones[conductorID]
	if !existe {
		return clients.Reputacion{}, clients.ErrPerfilNoEncontrado
	}
	return reputacion, nil
}

func (f *midReputacionFalsa) contarLlamadas() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return len(f.llamadas)
}

func (f *midReputacionFalsa) ultimosRequestIDs() []string {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	copia := make([]string, len(f.requestIDs))
	copy(copia, f.requestIDs)
	return copia
}

func midRutaDePrueba(id, conductor string) rutas.Ruta {
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
		Salida:           time.Date(2030, 1, 15, 7, 0, 0, 0, time.UTC),
		CuposTotales:     3,
		CuposDisponibles: 2,
		PrecioTotal:      12000,
		Estado:           rutas.Abierta,
	}
}

func midServicioDePrueba(t *testing.T, rutasFake *midRutasFalsas, reputacionFake *midReputacionFalsa) *mid.Servicio {
	t.Helper()
	servicio, err := mid.NuevoServicio(rutasFake, reputacionFake, mid.Opciones{})
	if err != nil {
		t.Fatalf("construir el servicio: %v", err)
	}
	return servicio
}

func midConsultaValida() string {
	valores := []string{
		"origenLat=4.6500",
		"origenLng=-74.0550",
		"destinoLat=4.6500",
		"destinoLng=-74.0450",
		"desde=2030-01-15T06:00:00Z",
		"hasta=2030-01-15T09:00:00Z",
	}
	return "/rutas/compatibles?" + strings.Join(valores, "&")
}

func TestBusquedaRechazaParametrosInvalidos(t *testing.T) {
	casos := []struct {
		nombre   string
		consulta string
		campo    string
	}{
		{"origenLat ausente", "/rutas/compatibles?origenLng=-74.0550&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "origenLat"},
		{"origenLng ausente", "/rutas/compatibles?origenLat=4.65&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "origenLng"},
		{"destinoLat ausente", "/rutas/compatibles?origenLat=4.65&origenLng=-74.055&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "destinoLat"},
		{"destinoLng ausente", "/rutas/compatibles?origenLat=4.65&origenLng=-74.055&destinoLat=4.65&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "destinoLng"},
		{"desde ausente", "/rutas/compatibles?origenLat=4.65&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&hasta=2030-01-15T09:00:00Z", "desde"},
		{"hasta ausente", "/rutas/compatibles?origenLat=4.65&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z", "hasta"},
		{"latitud fuera de rango", "/rutas/compatibles?origenLat=91&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "origenLat"},
		{"longitud fuera de rango", "/rutas/compatibles?origenLat=4.65&origenLng=-181&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "origenLng"},
		{"latitud no numérica", "/rutas/compatibles?origenLat=abc&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "origenLat"},
		{"latitud NaN", "/rutas/compatibles?origenLat=NaN&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "origenLat"},
		{"latitud infinita", "/rutas/compatibles?origenLat=Inf&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z", "origenLat"},
		{"desde con formato inválido", "/rutas/compatibles?origenLat=4.65&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=ayer&hasta=2030-01-15T09:00:00Z", "desde"},
		{"hasta con formato inválido", "/rutas/compatibles?origenLat=4.65&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T06:00:00Z&hasta=mañana", "hasta"},
		{"ventana con hasta igual a desde", "/rutas/compatibles?origenLat=4.65&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T09:00:00Z&hasta=2030-01-15T09:00:00Z", "hasta"},
		{"ventana invertida", "/rutas/compatibles?origenLat=4.65&origenLng=-74.055&destinoLat=4.65&destinoLng=-74.045&desde=2030-01-15T10:00:00Z&hasta=2030-01-15T09:00:00Z", "hasta"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			repositorio := &midRutasFalsas{}
			reputacion := &midReputacionFalsa{}
			manejador := RouterMID(midServicioDePrueba(t, repositorio, reputacion))

			grabador := solicitar(manejador, http.MethodGet, caso.consulta, "", "", "")
			if grabador.Code != http.StatusBadRequest {
				t.Fatalf("estado = %d, se esperaba 400: %s", grabador.Code, grabador.Body.String())
			}
			respuesta, detalle := decodificarError(t, grabador)
			if respuesta.Codigo != codigoSolicitudInvalida {
				t.Fatalf("codigo = %q", respuesta.Codigo)
			}
			if campo, _ := detalle["campo"].(string); campo != caso.campo {
				t.Fatalf("detalle.campo = %q, se esperaba %q (%v)", campo, caso.campo, detalle)
			}
			if repositorio.llamadas != 0 || reputacion.contarLlamadas() != 0 {
				t.Fatal("no se debe consultar dependencias con una entrada inválida")
			}
		})
	}
}

func TestBusquedaRespondeContrato(t *testing.T) {
	repositorio := &midRutasFalsas{encontradas: []rutas.Ruta{
		midRutaDePrueba("ruta-uno", "usr-12"),
	}}
	reputacion := &midReputacionFalsa{reputaciones: map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4.7, Insignia: matching.InsigniaConfiable},
	}}
	manejador := RouterMID(midServicioDePrueba(t, repositorio, reputacion))

	grabador := solicitar(manejador, http.MethodGet, midConsultaValida(), "", "", "req-busqueda")
	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if grabador.Header().Get(cabeceraRequestID) != "req-busqueda" {
		t.Fatal("la respuesta debe conservar el X-Request-Id")
	}

	var resultado mid.ResultadoBusqueda
	if err := json.Unmarshal(grabador.Body.Bytes(), &resultado); err != nil {
		t.Fatalf("decodificar: %v (%s)", err, grabador.Body.String())
	}
	if resultado.RadioM != mid.RadioDefectoM || len(resultado.Items) != 1 {
		t.Fatalf("resultado = %+v", resultado)
	}
	item := resultado.Items[0]
	if item.Ruta.ID != "ruta-uno" || item.Insignia != matching.InsigniaConfiable || item.PuntajeConductor != 4.7 {
		t.Fatalf("item = %+v", item)
	}
	if item.DistanciaOrigenM <= 0 || item.DistanciaDestinoM <= 0 {
		t.Fatalf("distancias = %v %v", item.DistanciaOrigenM, item.DistanciaDestinoM)
	}

	if len(repositorio.filtros) != 1 {
		t.Fatalf("filtros = %+v", repositorio.filtros)
	}
	filtro := repositorio.filtros[0]
	if filtro.Estado != string(rutas.Abierta) || filtro.Desde == nil || filtro.Hasta == nil {
		t.Fatalf("filtros = %+v", filtro)
	}
	if ids := reputacion.ultimosRequestIDs(); len(ids) != 1 || ids[0] != "req-busqueda" {
		t.Fatalf("el request id no se propagó a reputación: %v", ids)
	}
}

func TestBusquedaSinCandidatosDevuelveListaVacia(t *testing.T) {
	repositorio := &midRutasFalsas{}
	reputacion := &midReputacionFalsa{errores: map[string]error{"usr-12": clients.ErrNoDisponible}}
	manejador := RouterMID(midServicioDePrueba(t, repositorio, reputacion))

	grabador := solicitar(manejador, http.MethodGet, midConsultaValida(), "", "", "")
	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if !strings.Contains(grabador.Body.String(), `"items":[]`) {
		t.Fatalf("items debe ser un arreglo vacío: %s", grabador.Body.String())
	}
	if reputacion.contarLlamadas() != 0 {
		t.Fatal("sin candidatos no se consulta reputación")
	}
}

func TestBusquedaDependenciaNoDisponible(t *testing.T) {
	casos := []struct {
		nombre   string
		rutas    *midRutasFalsas
		rep      *midReputacionFalsa
		servicio string
	}{
		{
			nombre:   "reputación caída",
			rutas:    &midRutasFalsas{encontradas: []rutas.Ruta{midRutaDePrueba("ruta-uno", "usr-12")}},
			rep:      &midReputacionFalsa{errores: map[string]error{"usr-12": clients.ErrNoDisponible}},
			servicio: mid.ServicioReputacion,
		},
		{
			nombre:   "CRUD caído",
			rutas:    &midRutasFalsas{err: clients.ErrNoDisponible},
			rep:      &midReputacionFalsa{},
			servicio: mid.ServicioRutas,
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			manejador := RouterMID(midServicioDePrueba(t, caso.rutas, caso.rep))
			grabador := solicitar(manejador, http.MethodGet, midConsultaValida(), "", "", "")
			if grabador.Code != http.StatusServiceUnavailable {
				t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
			}
			respuesta, detalle := decodificarError(t, grabador)
			if respuesta.Codigo != codigoDependenciaNoDisponible {
				t.Fatalf("codigo = %q", respuesta.Codigo)
			}
			if detalle["servicio"] != caso.servicio {
				t.Fatalf("detalle = %v", detalle)
			}
		})
	}
}

func TestPanelRespondeOcupacion(t *testing.T) {
	primera := midRutaDePrueba("ruta-primera", "usr-12")
	segunda := midRutaDePrueba("ruta-segunda", "usr-12")
	segunda.CuposDisponibles = 3
	repositorio := &midRutasFalsas{encontradas: []rutas.Ruta{segunda, primera}}
	reputacion := &midReputacionFalsa{reputaciones: map[string]clients.Reputacion{
		"usr-12": {
			Puntaje:             4.7,
			Insignia:            matching.InsigniaConfiable,
			ViajesCompletados:   punteroEntero(12),
			TasaCancelacion:     punteroFlotante(0.05),
			TotalCalificaciones: punteroEntero(9),
		},
	}}
	manejador := RouterMID(midServicioDePrueba(t, repositorio, reputacion))

	grabador := solicitar(manejador, http.MethodGet, "/conductores/usr-12/panel", "", "", "req-panel")
	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if grabador.Header().Get(cabeceraRequestID) != "req-panel" {
		t.Fatal("la respuesta debe conservar el X-Request-Id")
	}
	var panel mid.Panel
	if err := json.Unmarshal(grabador.Body.Bytes(), &panel); err != nil {
		t.Fatalf("decodificar: %v (%s)", err, grabador.Body.String())
	}
	if panel.ConductorID != "usr-12" || panel.Puntaje != 4.7 || panel.Insignia != matching.InsigniaConfiable {
		t.Fatalf("panel = %+v", panel)
	}
	if panel.ViajesCompletados == nil || *panel.ViajesCompletados != 12 {
		t.Fatalf("viajesCompletados = %v", panel.ViajesCompletados)
	}
	if len(panel.Rutas) != 2 {
		t.Fatalf("rutas = %+v", panel.Rutas)
	}
	for _, ruta := range panel.Rutas {
		if ruta.Ruta.ID == "ruta-primera" && ruta.Ocupados != 1 {
			t.Fatalf("ocupados de la primera = %d", ruta.Ocupados)
		}
		if ruta.Ruta.ID == "ruta-segunda" && ruta.Ocupados != 0 {
			t.Fatalf("ocupados de la segunda = %d", ruta.Ocupados)
		}
	}
}

func TestPanelPerfilInexistente(t *testing.T) {
	repositorio := &midRutasFalsas{}
	reputacion := &midReputacionFalsa{}
	manejador := RouterMID(midServicioDePrueba(t, repositorio, reputacion))

	grabador := solicitar(manejador, http.MethodGet, "/conductores/usr-99/panel", "", "", "")
	if grabador.Code != http.StatusNotFound {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	respuesta, detalle := decodificarError(t, grabador)
	if respuesta.Codigo != codigoNoEncontrada {
		t.Fatalf("codigo = %q", respuesta.Codigo)
	}
	if detalle["conductorId"] != "usr-99" {
		t.Fatalf("detalle = %v", detalle)
	}
	if repositorio.llamadas != 0 {
		t.Fatal("no se deben listar rutas de un perfil inexistente")
	}
}

func TestPanelDependenciaNoDisponible(t *testing.T) {
	repositorio := &midRutasFalsas{err: clients.ErrNoDisponible}
	reputacion := &midReputacionFalsa{reputaciones: map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4, Insignia: matching.InsigniaConfiable},
	}}
	manejador := RouterMID(midServicioDePrueba(t, repositorio, reputacion))

	grabador := solicitar(manejador, http.MethodGet, "/conductores/usr-12/panel", "", "", "")
	if grabador.Code != http.StatusServiceUnavailable {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	_, detalle := decodificarError(t, grabador)
	if detalle["servicio"] != mid.ServicioRutas {
		t.Fatalf("detalle = %v", detalle)
	}
}

func TestPanelSinRutasDevuelveListaVacia(t *testing.T) {
	repositorio := &midRutasFalsas{}
	reputacion := &midReputacionFalsa{reputaciones: map[string]clients.Reputacion{
		"usr-50": {Puntaje: 5, Insignia: matching.InsigniaConfiable},
	}}
	manejador := RouterMID(midServicioDePrueba(t, repositorio, reputacion))

	grabador := solicitar(manejador, http.MethodGet, "/conductores/usr-50/panel", "", "", "")
	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if !strings.Contains(grabador.Body.String(), `"rutas":[]`) {
		t.Fatalf("rutas debe ser un arreglo vacío: %s", grabador.Body.String())
	}
}

func TestMIDHealth(t *testing.T) {
	manejador := RouterMID(midServicioDePrueba(t, &midRutasFalsas{}, &midReputacionFalsa{}))
	grabador := solicitar(manejador, http.MethodGet, "/health", "", "", "")
	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d", grabador.Code)
	}
	if !strings.Contains(grabador.Body.String(), `"service":"rutas-mid"`) {
		t.Fatalf("cuerpo = %s", grabador.Body.String())
	}
}

func TestRouterMIDPermiteElPreflightDelVisor(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "http://127.0.0.1:8090")
	manejador := RouterMID(midServicioDePrueba(t, &midRutasFalsas{}, &midReputacionFalsa{}))

	peticion := httptest.NewRequest(http.MethodOptions, "/rutas/compatibles", nil)
	peticion.Header.Set("Origin", "http://127.0.0.1:8090")
	peticion.Header.Set("Access-Control-Request-Method", http.MethodGet)
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)

	if grabador.Code != http.StatusNoContent {
		t.Fatalf("estado = %d", grabador.Code)
	}
	if origen := grabador.Header().Get("Access-Control-Allow-Origin"); origen != "http://127.0.0.1:8090" {
		t.Fatalf("origen = %q", origen)
	}
	if grabador.Header().Get(cabeceraRequestID) == "" {
		t.Fatal("el preflight debe llevar X-Request-Id")
	}
}

func TestRouterMIDExigeElSecretoDelGateway(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "secreto-del-cluster")
	reputacion := &midReputacionFalsa{reputaciones: map[string]clients.Reputacion{
		"usr-12": {Puntaje: 4.7, Insignia: matching.InsigniaConfiable},
	}}
	manejador := RouterMID(midServicioDePrueba(t, &midRutasFalsas{}, reputacion))

	sinSecreto := solicitar(manejador, http.MethodGet, midConsultaValida(), "", "", "")
	if sinSecreto.Code != http.StatusUnauthorized {
		t.Fatalf("estado sin secreto = %d: %s", sinSecreto.Code, sinSecreto.Body.String())
	}
	if respuesta, _ := decodificarError(t, sinSecreto); respuesta.Codigo != codigoNoAutorizado {
		t.Fatalf("codigo = %q", respuesta.Codigo)
	}

	peticion := httptest.NewRequest(http.MethodGet, midConsultaValida(), nil)
	peticion.Header.Set(cabeceraSecreto, "secreto-del-cluster")
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)
	if grabador.Code != http.StatusOK {
		t.Fatalf("estado con secreto = %d: %s", grabador.Code, grabador.Body.String())
	}
}

func TestMIDRecursoDesconocido(t *testing.T) {
	manejador := RouterMID(midServicioDePrueba(t, &midRutasFalsas{}, &midReputacionFalsa{}))
	grabador := solicitar(manejador, http.MethodGet, "/desconocida", "", "", "")
	if grabador.Code != http.StatusNotFound {
		t.Fatalf("estado = %d", grabador.Code)
	}
	respuesta, detalle := decodificarError(t, grabador)
	if respuesta.Codigo != codigoNoEncontrada || len(detalle) != 0 {
		t.Fatalf("respuesta = %+v detalle = %v", respuesta, detalle)
	}
}

func punteroEntero(valor int) *int {
	return &valor
}

func punteroFlotante(valor float64) *float64 {
	return &valor
}
