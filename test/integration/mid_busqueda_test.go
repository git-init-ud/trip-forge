package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/clients"
	"github.com/git-init-ud/trip-forge/internal/httpapi"
	"github.com/git-init-ud/trip-forge/internal/mid"
	"github.com/git-init-ud/trip-forge/internal/rutas"
)

// midPerfil es la respuesta configurable del doble de reputación.
type midPerfil struct {
	puntaje             float64
	insignia            string
	viajesCompletados   *int
	tasaCancelacion     *float64
	totalCalificaciones *int
}

// midReputacionDoble simula Pasajeros y reputación por HTTP. Permite apagarlo
// para comprobar el 503 del MID.
type midReputacionDoble struct {
	mutex     sync.Mutex
	perfiles  map[string]midPerfil
	caido     bool
	consultas map[string]int
}

func midNuevaReputacion() *midReputacionDoble {
	return &midReputacionDoble{
		perfiles:  map[string]midPerfil{},
		consultas: map[string]int{},
	}
}

func (d *midReputacionDoble) configurar(conductor string, perfil midPerfil) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.perfiles[conductor] = perfil
}

func (d *midReputacionDoble) marcarCaido(caido bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.caido = caido
}

func (d *midReputacionDoble) consultasDe(conductor string) int {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.consultas[conductor]
}

func (d *midReputacionDoble) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	partes := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if r.Method != http.MethodGet || len(partes) != 3 || partes[0] != "perfiles" || partes[2] != "puntaje" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	conductor := partes[1]

	d.mutex.Lock()
	caido := d.caido
	perfil, existe := d.perfiles[conductor]
	d.consultas[conductor]++
	d.mutex.Unlock()

	if caido {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !existe {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"codigo":"no_encontrada","mensaje":"El perfil no existe","detalle":{}}`))
		return
	}
	respuesta := map[string]any{
		"puntaje":  perfil.puntaje,
		"insignia": perfil.insignia,
	}
	if perfil.viajesCompletados != nil {
		respuesta["viajesCompletados"] = *perfil.viajesCompletados
	}
	if perfil.tasaCancelacion != nil {
		respuesta["tasaCancelacion"] = *perfil.tasaCancelacion
	}
	if perfil.totalCalificaciones != nil {
		respuesta["totalCalificaciones"] = *perfil.totalCalificaciones
	}
	if err := json.NewEncoder(w).Encode(respuesta); err != nil {
		http.Error(w, "respuesta", http.StatusInternalServerError)
	}
}

// midEntorno reúne el CRUD con Mongo real, el doble de reputación y el MID.
type midEntorno struct {
	crud       *ambiente
	servidor   *httptest.Server
	reputacion *midReputacionDoble
}

func midNuevoEntorno(t *testing.T) *midEntorno {
	t.Helper()
	crud := nuevoAmbiente(t)

	reputacion := midNuevaReputacion()
	servidorReputacion := httptest.NewServer(reputacion)
	t.Cleanup(servidorReputacion.Close)

	return &midEntorno{
		crud:       crud,
		servidor:   midServidorHacia(t, crud.servidor.URL, servidorReputacion.URL),
		reputacion: reputacion,
	}
}

// midServidorHacia arma un MID contra las URLs indicadas. Se usa también para
// simular un CRUD caído apuntando a un servidor ya cerrado.
func midServidorHacia(t *testing.T, crudURL, reputacionURL string) *httptest.Server {
	t.Helper()
	clienteRutas, err := clients.NuevoClienteRutas(crudURL, 3*time.Second, "")
	if err != nil {
		t.Fatalf("cliente de rutas: %v", err)
	}
	clienteReputacion, err := clients.NuevoClienteReputacion(reputacionURL, 2*time.Second, "")
	if err != nil {
		t.Fatalf("cliente de reputación: %v", err)
	}
	servicio, err := mid.NuevoServicio(clienteRutas, clienteReputacion, mid.Opciones{
		RadioM:       800,
		Concurrencia: 4,
	})
	if err != nil {
		t.Fatalf("servicio del MID: %v", err)
	}
	servidor := httptest.NewServer(httpapi.RouterMID(servicio))
	t.Cleanup(servidor.Close)
	return servidor
}

func midPeticion(t *testing.T, servidor *httptest.Server, metodo, ruta string) respuestaHTTP {
	t.Helper()
	peticion, err := http.NewRequest(metodo, servidor.URL+ruta, nil)
	if err != nil {
		t.Fatalf("crear la petición: %v", err)
	}
	cliente := &http.Client{Timeout: 10 * time.Second}
	respuesta, err := cliente.Do(peticion)
	if err != nil {
		t.Fatalf("%s %s: %v", metodo, ruta, err)
	}
	defer respuesta.Body.Close()
	datos, err := io.ReadAll(respuesta.Body)
	if err != nil {
		t.Fatalf("leer la respuesta: %v", err)
	}
	return respuestaHTTP{estado: respuesta.StatusCode, cuerpo: datos, cabecera: respuesta.Header}
}

func midCrearRuta(t *testing.T, crud *ambiente, conductor string, cupos int, salida time.Time, trazado [][]float64) rutas.Ruta {
	t.Helper()
	cuerpo := cuerpoRuta(cupos)
	cuerpo["conductorId"] = conductor
	cuerpo["salida"] = salida.UTC().Format(time.RFC3339)
	cuerpo["trazado"] = map[string]any{
		"type":        "LineString",
		"coordinates": trazado,
	}
	respuesta := crud.peticion(t, http.MethodPost, "/rutas", cuerpo)
	if respuesta.estado != http.StatusCreated {
		t.Fatalf("POST /rutas de %s = %d: %s", conductor, respuesta.estado, respuesta.cuerpo)
	}
	return decodificarRuta(t, respuesta)
}

func midConsulta(desde, hasta time.Time) string {
	return fmt.Sprintf(
		"/rutas/compatibles?origenLat=4.6500&origenLng=-74.0550&destinoLat=4.6500&destinoLng=-74.0450&desde=%s&hasta=%s",
		desde.UTC().Format(time.RFC3339), hasta.UTC().Format(time.RFC3339),
	)
}

func midTrazado(sentido bool) [][]float64 {
	if sentido {
		return [][]float64{{-74.0600, 4.6500}, {-74.0400, 4.6500}}
	}
	return [][]float64{{-74.0400, 4.6500}, {-74.0600, 4.6500}}
}

func TestMIDBusquedaOrdenaYExcluyeConductores(t *testing.T) {
	entorno := midNuevoEntorno(t)
	desde := time.Now().UTC().Add(time.Hour)
	hasta := desde.Add(72 * time.Hour)
	salida := desde.Add(2 * time.Hour)

	revisada := midCrearRuta(t, entorno.crud, "usr-12", 3, salida, midTrazado(true))
	regular := midCrearRuta(t, entorno.crud, "usr-13", 2, salida, midTrazado(true))
	observada := midCrearRuta(t, entorno.crud, "usr-14", 2, salida, midTrazado(true))
	invertida := midCrearRuta(t, entorno.crud, "usr-16", 2, salida, midTrazado(false))
	sinPerfil := midCrearRuta(t, entorno.crud, "usr-15", 2, salida, midTrazado(true))

	entorno.reputacion.configurar("usr-12", midPerfil{puntaje: 4.9, insignia: "confiable"})
	entorno.reputacion.configurar("usr-13", midPerfil{puntaje: 3.2, insignia: "regular"})
	entorno.reputacion.configurar("usr-14", midPerfil{puntaje: 1.0, insignia: "en_observacion"})
	// usr-15 queda sin perfil: reputación responde 404 y el MID omite sus rutas.
	entorno.reputacion.configurar("usr-16", midPerfil{puntaje: 5, insignia: "confiable"})

	respuesta := midPeticion(t, entorno.servidor, http.MethodGet, midConsulta(desde, hasta))
	if respuesta.estado != http.StatusOK {
		t.Fatalf("GET compatibles = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	var resultado mid.ResultadoBusqueda
	if err := json.Unmarshal(respuesta.cuerpo, &resultado); err != nil {
		t.Fatalf("decodificar la búsqueda: %v (%s)", err, respuesta.cuerpo)
	}
	if resultado.RadioM != 800 {
		t.Fatalf("radioM = %v", resultado.RadioM)
	}
	if len(resultado.Items) != 2 {
		t.Fatalf("items = %d: %s", len(resultado.Items), respuesta.cuerpo)
	}
	if resultado.Items[0].Ruta.ID != revisada.ID || resultado.Items[1].Ruta.ID != regular.ID {
		t.Fatalf("orden = %q, %q; se esperaban %q, %q",
			resultado.Items[0].Ruta.ID, resultado.Items[1].Ruta.ID, revisada.ID, regular.ID)
	}
	for _, item := range resultado.Items {
		if item.Ruta.ID == observada.ID || item.Ruta.ID == sinPerfil.ID || item.Ruta.ID == invertida.ID {
			t.Fatalf("la búsqueda incluyó una ruta que debía excluir: %+v", item)
		}
		if item.DistanciaOrigenM <= 0 || item.DistanciaOrigenM > 800 {
			t.Fatalf("distanciaOrigenM = %v", item.DistanciaOrigenM)
		}
		if item.DistanciaDestinoM <= 0 || item.DistanciaDestinoM > 800 {
			t.Fatalf("distanciaDestinoM = %v", item.DistanciaDestinoM)
		}
	}
	if primera := resultado.Items[0]; primera.Insignia != "confiable" || primera.PuntajeConductor != 4.9 {
		t.Fatalf("reputación de la primera = %+v", primera)
	}
	esperado := (resultado.Items[0].DistanciaOrigenM+resultado.Items[0].DistanciaDestinoM)/100 - 4.9*2
	if resultado.Items[0].Puntaje != esperado {
		t.Fatalf("puntaje = %v, se esperaba %v", resultado.Items[0].Puntaje, esperado)
	}

	for _, conductor := range []string{"usr-12", "usr-13", "usr-14", "usr-15"} {
		if consultas := entorno.reputacion.consultasDe(conductor); consultas != 1 {
			t.Fatalf("consultas a %s = %d, se esperaba una", conductor, consultas)
		}
	}
	if consultas := entorno.reputacion.consultasDe("usr-16"); consultas != 0 {
		t.Fatalf("no se debía consultar a la ruta en sentido contrario: %d", consultas)
	}
}

func TestMIDBusquedaSinCandidatosNoConsultaReputacion(t *testing.T) {
	entorno := midNuevoEntorno(t)
	desde := time.Now().UTC().Add(time.Hour)
	hasta := desde.Add(72 * time.Hour)

	midCrearRuta(t, entorno.crud, "usr-12", 3, desde.Add(time.Hour), [][]float64{
		{-74.2000, 4.6500},
		{-74.1800, 4.6500},
	})
	entorno.reputacion.configurar("usr-12", midPerfil{puntaje: 5, insignia: "confiable"})
	entorno.reputacion.marcarCaido(true)

	respuesta := midPeticion(t, entorno.servidor, http.MethodGet, midConsulta(desde, hasta))
	if respuesta.estado != http.StatusOK {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	if !strings.Contains(string(respuesta.cuerpo), `"items":[]`) {
		t.Fatalf("items debe ser vacío: %s", respuesta.cuerpo)
	}
	if consultas := entorno.reputacion.consultasDe("usr-12"); consultas != 0 {
		t.Fatalf("no se debía consultar reputación sin candidatos: %d", consultas)
	}
}

func TestMIDBusquedaConReputacionCaidaResponde503(t *testing.T) {
	entorno := midNuevoEntorno(t)
	desde := time.Now().UTC().Add(time.Hour)
	hasta := desde.Add(72 * time.Hour)

	midCrearRuta(t, entorno.crud, "usr-12", 3, desde.Add(time.Hour), midTrazado(true))
	entorno.reputacion.configurar("usr-12", midPerfil{puntaje: 4.9, insignia: "confiable"})
	entorno.reputacion.marcarCaido(true)

	respuesta := midPeticion(t, entorno.servidor, http.MethodGet, midConsulta(desde, hasta))
	if respuesta.estado != http.StatusServiceUnavailable {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cuerpo := decodificarError(t, respuesta)
	if cuerpo.Codigo != "dependencia_no_disponible" || cuerpo.Detalle["servicio"] != "reputacion" {
		t.Fatalf("error = %+v", cuerpo)
	}
}

func TestMIDBusquedaRechazaVentanaInvalida(t *testing.T) {
	entorno := midNuevoEntorno(t)
	desde := time.Now().UTC().Add(2 * time.Hour)
	respuesta := midPeticion(t, entorno.servidor, http.MethodGet, midConsulta(desde, desde.Add(-time.Hour)))
	if respuesta.estado != http.StatusBadRequest {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cuerpo := decodificarError(t, respuesta)
	if cuerpo.Codigo != "solicitud_invalida" || cuerpo.Detalle["campo"] != "hasta" {
		t.Fatalf("error = %+v", cuerpo)
	}
}
