// Package integration prueba el CRUD contra una instancia real de MongoDB y el
// servidor HTTP completo. Las pruebas se omiten si MONGODB_TEST_URI está vacío.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/httpapi"
	"github.com/git-init-ud/trip-forge/internal/mongorutas"
	"github.com/git-init-ud/trip-forge/internal/rutas"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type ambiente struct {
	servidor *httptest.Server
	cliente  *http.Client
	repo     *mongorutas.Repositorio
	mongo    *mongo.Client
	base     string
}

type respuestaHTTP struct {
	estado   int
	cuerpo   []byte
	cabecera http.Header
}

type errorHTTP struct {
	Codigo  string         `json:"codigo"`
	Mensaje string         `json:"mensaje"`
	Detalle map[string]any `json:"detalle"`
}

func nuevoAmbiente(t *testing.T) *ambiente {
	t.Helper()
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("MONGODB_TEST_URI no está definido; se omite la prueba con MongoDB real")
	}

	base := fmt.Sprintf("trip_forge_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	arranque, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	clienteMongo, repositorio, err := mongorutas.Abrir(arranque, uri, base)
	if err != nil {
		cancelar()
		t.Fatalf("abrir mongodb: %v", err)
	}
	if err := repositorio.AsegurarIndices(arranque); err != nil {
		cancelar()
		_ = clienteMongo.Disconnect(context.Background())
		t.Fatalf("crear índices: %v", err)
	}
	cancelar()

	servidor := httptest.NewServer(httpapi.RouterRutas(repositorio))
	t.Cleanup(func() {
		cierre, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		if err := clienteMongo.Database(base).Drop(cierre); err != nil {
			t.Errorf("borrar la base de prueba: %v", err)
		}
		if err := clienteMongo.Disconnect(cierre); err != nil {
			t.Errorf("cerrar el cliente de mongo: %v", err)
		}
	})
	t.Cleanup(servidor.Close)

	return &ambiente{
		servidor: servidor,
		cliente:  &http.Client{Timeout: 10 * time.Second},
		repo:     repositorio,
		mongo:    clienteMongo,
		base:     base,
	}
}

func (a *ambiente) peticion(t *testing.T, metodo, ruta string, cuerpo any) respuestaHTTP {
	t.Helper()
	var lector io.Reader
	if cuerpo != nil {
		datos, err := json.Marshal(cuerpo)
		if err != nil {
			t.Fatalf("serializar el cuerpo: %v", err)
		}
		lector = bytes.NewReader(datos)
	}
	peticion, err := http.NewRequest(metodo, a.servidor.URL+ruta, lector)
	if err != nil {
		t.Fatalf("crear la petición: %v", err)
	}
	if cuerpo != nil {
		peticion.Header.Set("Content-Type", "application/json")
	}
	respuesta, err := a.cliente.Do(peticion)
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

func cuerpoRuta(cupos int) map[string]any {
	return map[string]any{
		"conductorId": "usr-12",
		"origen": map[string]any{
			"nombre": "Chapinero",
			"punto": map[string]any{
				"type":        "Point",
				"coordinates": []float64{-74.0645, 4.6486},
			},
		},
		"destino": map[string]any{
			"nombre": "Universidad",
			"punto": map[string]any{
				"type":        "Point",
				"coordinates": []float64{-74.0415, 4.7012},
			},
		},
		"trazado": map[string]any{
			"type": "LineString",
			"coordinates": [][]float64{
				{-74.0645, 4.6486},
				{-74.0600, 4.6650},
				{-74.0415, 4.7012},
			},
		},
		"salida":       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		"cuposTotales": cupos,
		"precioTotal":  12000,
	}
}

func (a *ambiente) crearRuta(t *testing.T, cupos int) rutas.Ruta {
	t.Helper()
	respuesta := a.peticion(t, http.MethodPost, "/rutas", cuerpoRuta(cupos))
	if respuesta.estado != http.StatusCreated {
		t.Fatalf("POST /rutas = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	var ruta rutas.Ruta
	if err := json.Unmarshal(respuesta.cuerpo, &ruta); err != nil {
		t.Fatalf("decodificar la ruta creada: %v", err)
	}
	return ruta
}

func (a *ambiente) ajustar(t *testing.T, id string, delta int) respuestaHTTP {
	t.Helper()
	return a.peticion(t, http.MethodPatch, "/rutas/"+id+"/cupos", map[string]any{"delta": delta})
}

type resultadoPeticion struct {
	estado int
	codigo string
	cuerpo []byte
	err    error
}

// peticionSinTesting permite lanzar solicitudes desde goroutines sin usar
// *testing.T, que no es seguro entre goroutines.
func (a *ambiente) peticionSinTesting(metodo, ruta, cuerpo string) resultadoPeticion {
	var lector io.Reader
	if cuerpo != "" {
		lector = strings.NewReader(cuerpo)
	}
	peticion, err := http.NewRequest(metodo, a.servidor.URL+ruta, lector)
	if err != nil {
		return resultadoPeticion{err: err}
	}
	if cuerpo != "" {
		peticion.Header.Set("Content-Type", "application/json")
	}
	respuesta, err := a.cliente.Do(peticion)
	if err != nil {
		return resultadoPeticion{err: err}
	}
	defer respuesta.Body.Close()
	datos, err := io.ReadAll(respuesta.Body)
	if err != nil {
		return resultadoPeticion{err: err}
	}
	resultado := resultadoPeticion{estado: respuesta.StatusCode, cuerpo: datos}
	if respuesta.StatusCode == http.StatusConflict {
		var errorCuerpo errorHTTP
		if err := json.Unmarshal(datos, &errorCuerpo); err != nil {
			resultado.err = err
			return resultado
		}
		resultado.codigo = errorCuerpo.Codigo
	}
	return resultado
}

func decodificarRuta(t *testing.T, respuesta respuestaHTTP) rutas.Ruta {
	t.Helper()
	var ruta rutas.Ruta
	if err := json.Unmarshal(respuesta.cuerpo, &ruta); err != nil {
		t.Fatalf("decodificar la ruta: %v (%s)", err, respuesta.cuerpo)
	}
	return ruta
}

func decodificarError(t *testing.T, respuesta respuestaHTTP) errorHTTP {
	t.Helper()
	var cuerpo errorHTTP
	if err := json.Unmarshal(respuesta.cuerpo, &cuerpo); err != nil {
		t.Fatalf("decodificar el error: %v (%s)", err, respuesta.cuerpo)
	}
	return cuerpo
}

type documentoRuta struct {
	CuposDisponibles int    `bson:"cuposDisponibles"`
	CuposTotales     int    `bson:"cuposTotales"`
	Estado           string `bson:"estado"`
}

// leerDocumento consulta el documento directamente para comprobar que la API y
// la persistencia coinciden, sin pasar por el repositorio.
func (a *ambiente) leerDocumento(t *testing.T, id string) documentoRuta {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	var documento documentoRuta
	err := a.mongo.Database(a.base).Collection(mongorutas.ColeccionRutas).
		FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&documento)
	if err != nil {
		t.Fatalf("leer el documento %s: %v", id, err)
	}
	return documento
}

func TestPublicarYConsultarRuta(t *testing.T) {
	ambiente := nuevoAmbiente(t)

	respuesta := ambiente.peticion(t, http.MethodPost, "/rutas", cuerpoRuta(3))
	if respuesta.estado != http.StatusCreated {
		t.Fatalf("POST /rutas = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	creada := decodificarRuta(t, respuesta)
	if creada.ID == "" || creada.Estado != rutas.Abierta || creada.CuposDisponibles != 3 {
		t.Fatalf("ruta creada fuera del contrato: %+v", creada)
	}
	if esperado := "/rutas/" + creada.ID; respuesta.cabecera.Get("Location") != esperado {
		t.Fatalf("Location = %q, se esperaba %q", respuesta.cabecera.Get("Location"), esperado)
	}
	if respuesta.cabecera.Get("X-Request-Id") == "" {
		t.Fatal("la respuesta no incluye X-Request-Id")
	}

	consulta := ambiente.peticion(t, http.MethodGet, "/rutas/"+creada.ID, nil)
	if consulta.estado != http.StatusOK {
		t.Fatalf("GET /rutas/%s = %d: %s", creada.ID, consulta.estado, consulta.cuerpo)
	}
	obtenida := decodificarRuta(t, consulta)
	if obtenida.ID != creada.ID || obtenida.ConductorID != "usr-12" || obtenida.PrecioTotal != 12000 {
		t.Fatalf("detalle inesperado: %+v", obtenida)
	}
	if obtenida.Salida.Location() != time.UTC {
		t.Fatalf("la salida no está en UTC: %v", obtenida.Salida)
	}

	inexistente := ambiente.peticion(t, http.MethodGet, "/rutas/ruta-0000000000000000", nil)
	if inexistente.estado != http.StatusNotFound {
		t.Fatalf("GET inexistente = %d: %s", inexistente.estado, inexistente.cuerpo)
	}
	if codigo := decodificarError(t, inexistente).Codigo; codigo != "no_encontrada" {
		t.Fatalf("codigo = %q", codigo)
	}

	ajusteInexistente := ambiente.ajustar(t, "ruta-0000000000000000", -1)
	if ajusteInexistente.estado != http.StatusNotFound {
		t.Fatalf("PATCH inexistente = %d: %s", ajusteInexistente.estado, ajusteInexistente.cuerpo)
	}
	if codigo := decodificarError(t, ajusteInexistente).Codigo; codigo != "no_encontrada" {
		t.Fatalf("codigo = %q", codigo)
	}
}

func TestDescontarHastaLlenarYReabrir(t *testing.T) {
	ambiente := nuevoAmbiente(t)
	ruta := ambiente.crearRuta(t, 2)

	primero := ambiente.ajustar(t, ruta.ID, -1)
	if primero.estado != http.StatusOK {
		t.Fatalf("primer descuento = %d: %s", primero.estado, primero.cuerpo)
	}
	if actual := decodificarRuta(t, primero); actual.CuposDisponibles != 1 || actual.Estado != rutas.Abierta {
		t.Fatalf("tras el primer descuento: %+v", actual)
	}

	segundo := ambiente.ajustar(t, ruta.ID, -1)
	if segundo.estado != http.StatusOK {
		t.Fatalf("segundo descuento = %d: %s", segundo.estado, segundo.cuerpo)
	}
	llena := decodificarRuta(t, segundo)
	if llena.CuposDisponibles != 0 || llena.Estado != rutas.Llena {
		t.Fatalf("la ruta no quedó llena: %+v", llena)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.CuposDisponibles != 0 || documento.Estado != "llena" {
		t.Fatalf("documento tras agotar: %+v", documento)
	}

	tercero := ambiente.ajustar(t, ruta.ID, -1)
	if tercero.estado != http.StatusConflict {
		t.Fatalf("descuento sin cupos = %d: %s", tercero.estado, tercero.cuerpo)
	}
	if codigo := decodificarError(t, tercero).Codigo; codigo != "sin_cupos" {
		t.Fatalf("codigo = %q, se esperaba sin_cupos", codigo)
	}

	devolucion := ambiente.ajustar(t, ruta.ID, 1)
	if devolucion.estado != http.StatusOK {
		t.Fatalf("devolución = %d: %s", devolucion.estado, devolucion.cuerpo)
	}
	reabierta := decodificarRuta(t, devolucion)
	if reabierta.CuposDisponibles != 1 || reabierta.Estado != rutas.Abierta {
		t.Fatalf("la ruta no se reabrió: %+v", reabierta)
	}

	completa := ambiente.ajustar(t, ruta.ID, 1)
	if completa.estado != http.StatusOK {
		t.Fatalf("segunda devolución = %d: %s", completa.estado, completa.cuerpo)
	}
	if actual := decodificarRuta(t, completa); actual.CuposDisponibles != 2 || actual.Estado != rutas.Abierta {
		t.Fatalf("la ruta no volvió al total: %+v", actual)
	}

	excedida := ambiente.ajustar(t, ruta.ID, 1)
	if excedida.estado != http.StatusConflict {
		t.Fatalf("devolución por encima del total = %d: %s", excedida.estado, excedida.cuerpo)
	}
	if codigo := decodificarError(t, excedida).Codigo; codigo != "cupos_excedidos" {
		t.Fatalf("codigo = %q, se esperaba cupos_excedidos", codigo)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.CuposDisponibles != 2 || documento.Estado != "abierta" {
		t.Fatalf("el documento cambió con un ajuste rechazado: %+v", documento)
	}
}

func TestDevolverPorEncimaDelTotal(t *testing.T) {
	ambiente := nuevoAmbiente(t)
	ruta := ambiente.crearRuta(t, 1)

	respuesta := ambiente.ajustar(t, ruta.ID, 1)
	if respuesta.estado != http.StatusConflict {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cuerpo := decodificarError(t, respuesta)
	if cuerpo.Codigo != "cupos_excedidos" {
		t.Fatalf("codigo = %q", cuerpo.Codigo)
	}
	if cuerpo.Detalle["rutaId"] != ruta.ID {
		t.Fatalf("detalle = %v", cuerpo.Detalle)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.CuposDisponibles != 1 || documento.Estado != "abierta" {
		t.Fatalf("documento inesperado: %+v", documento)
	}
}

func TestCerrarImpideDevolverCupos(t *testing.T) {
	ambiente := nuevoAmbiente(t)
	ruta := ambiente.crearRuta(t, 2)

	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	_, err := ambiente.mongo.Database(ambiente.base).Collection(mongorutas.ColeccionRutas).
		UpdateOne(ctx, bson.D{{Key: "_id", Value: ruta.ID}}, bson.D{{Key: "$set", Value: bson.D{{Key: "estado", Value: "cerrada"}}}})
	if err != nil {
		t.Fatalf("cerrar la ruta en mongo: %v", err)
	}

	devolucion := ambiente.ajustar(t, ruta.ID, 1)
	if devolucion.estado != http.StatusConflict {
		t.Fatalf("devolución a ruta cerrada = %d: %s", devolucion.estado, devolucion.cuerpo)
	}
	if codigo := decodificarError(t, devolucion).Codigo; codigo != "ruta_cerrada" {
		t.Fatalf("codigo = %q", codigo)
	}

	descuento := ambiente.ajustar(t, ruta.ID, -1)
	if descuento.estado != http.StatusConflict {
		t.Fatalf("descuento en ruta cerrada = %d: %s", descuento.estado, descuento.cuerpo)
	}
	if codigo := decodificarError(t, descuento).Codigo; codigo != "sin_cupos" {
		t.Fatalf("codigo = %q", codigo)
	}
}

func TestDosSolicitudesPorElUltimoCupo(t *testing.T) {
	ambiente := nuevoAmbiente(t)
	ruta := ambiente.crearRuta(t, 1)

	type resultado struct {
		estado int
		codigo string
		cuerpo []byte
		err    error
	}
	const solicitudes = 2
	resultados := make([]resultado, solicitudes)

	inicio := make(chan struct{})
	var espera sync.WaitGroup
	for i := 0; i < solicitudes; i++ {
		espera.Add(1)
		go func(indice int) {
			defer espera.Done()
			peticion, err := http.NewRequest(http.MethodPatch,
				ambiente.servidor.URL+"/rutas/"+ruta.ID+"/cupos",
				bytes.NewBufferString(`{"delta":-1}`))
			if err != nil {
				resultados[indice] = resultado{err: err}
				return
			}
			peticion.Header.Set("Content-Type", "application/json")
			peticion.Header.Set("X-Request-Id", fmt.Sprintf("concurrente-%d", indice))
			<-inicio
			respuesta, err := ambiente.cliente.Do(peticion)
			if err != nil {
				resultados[indice] = resultado{err: err}
				return
			}
			defer respuesta.Body.Close()
			datos, err := io.ReadAll(respuesta.Body)
			if err != nil {
				resultados[indice] = resultado{err: err}
				return
			}
			resultados[indice] = resultado{estado: respuesta.StatusCode, cuerpo: datos}
			if respuesta.StatusCode == http.StatusConflict {
				var cuerpo errorHTTP
				if err := json.Unmarshal(datos, &cuerpo); err != nil {
					resultados[indice].err = err
					return
				}
				resultados[indice].codigo = cuerpo.Codigo
			}
		}(i)
	}
	close(inicio)
	espera.Wait()

	exitos, conflictos, sinCupos := 0, 0, 0
	for i, resultado := range resultados {
		if resultado.err != nil {
			t.Fatalf("solicitud %d: %v", i, resultado.err)
		}
		switch resultado.estado {
		case http.StatusOK:
			exitos++
			actual := decodificarRuta(t, respuestaHTTP{cuerpo: resultado.cuerpo})
			if actual.CuposDisponibles != 0 || actual.Estado != rutas.Llena {
				t.Fatalf("respuesta exitosa inesperada: %+v", actual)
			}
		case http.StatusConflict:
			conflictos++
			if resultado.codigo == "sin_cupos" {
				sinCupos++
			}
		default:
			t.Fatalf("solicitud %d devolvió %d: %s", i, resultado.estado, resultado.cuerpo)
		}
	}
	if exitos != 1 || conflictos != 1 || sinCupos != 1 {
		t.Fatalf("se esperaba una 200 y una 409 sin_cupos; hubo %d exitosas, %d conflictos (%d sin_cupos)",
			exitos, conflictos, sinCupos)
	}

	if documento := ambiente.leerDocumento(t, ruta.ID); documento.CuposDisponibles != 0 || documento.Estado != "llena" {
		t.Fatalf("documento final inesperado: %+v", documento)
	}
	consulta := ambiente.peticion(t, http.MethodGet, "/rutas/"+ruta.ID, nil)
	if consulta.estado != http.StatusOK {
		t.Fatalf("GET final = %d", consulta.estado)
	}
	final := decodificarRuta(t, consulta)
	if final.CuposDisponibles != 0 || final.Estado != rutas.Llena {
		t.Fatalf("ruta final inesperada: %+v", final)
	}
}

func TestAjustesMaximosDelContrato(t *testing.T) {
	ambiente := nuevoAmbiente(t)
	ruta := ambiente.crearRuta(t, 6)

	descuento := ambiente.ajustar(t, ruta.ID, -6)
	if descuento.estado != http.StatusOK {
		t.Fatalf("descuento máximo = %d: %s", descuento.estado, descuento.cuerpo)
	}
	if actual := decodificarRuta(t, descuento); actual.CuposDisponibles != 0 || actual.Estado != rutas.Llena {
		t.Fatalf("tras descontar 6: %+v", actual)
	}

	devolucion := ambiente.ajustar(t, ruta.ID, 6)
	if devolucion.estado != http.StatusOK {
		t.Fatalf("devolución máxima = %d: %s", devolucion.estado, devolucion.cuerpo)
	}
	if actual := decodificarRuta(t, devolucion); actual.CuposDisponibles != 6 || actual.Estado != rutas.Abierta {
		t.Fatalf("tras devolver 6: %+v", actual)
	}
}

func TestVeinteDescuentosPorUnCupo(t *testing.T) {
	ambiente := nuevoAmbiente(t)
	ruta := ambiente.crearRuta(t, 1)

	const solicitudes = 20
	resultados := make([]resultadoPeticion, solicitudes)
	inicio := make(chan struct{})
	var espera sync.WaitGroup
	for i := 0; i < solicitudes; i++ {
		espera.Add(1)
		go func(indice int) {
			defer espera.Done()
			<-inicio
			resultados[indice] = ambiente.peticionSinTesting(http.MethodPatch,
				"/rutas/"+ruta.ID+"/cupos", `{"delta":-1}`)
		}(i)
	}
	close(inicio)
	espera.Wait()

	exitos, conflictos := 0, 0
	for i, resultado := range resultados {
		if resultado.err != nil {
			t.Fatalf("solicitud %d: %v", i, resultado.err)
		}
		switch resultado.estado {
		case http.StatusOK:
			exitos++
		case http.StatusConflict:
			conflictos++
			if resultado.codigo != "sin_cupos" {
				t.Fatalf("solicitud %d: codigo = %q", i, resultado.codigo)
			}
		default:
			t.Fatalf("solicitud %d devolvió %d: %s", i, resultado.estado, resultado.cuerpo)
		}
	}
	if exitos != 1 || conflictos != solicitudes-1 {
		t.Fatalf("se esperaba una 200 y %d conflictos; hubo %d exitosas y %d conflictos",
			solicitudes-1, exitos, conflictos)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.CuposDisponibles != 0 || documento.Estado != "llena" {
		t.Fatalf("documento final inesperado: %+v", documento)
	}
}

func TestConcurrenciaDevolucionYDescuentos(t *testing.T) {
	ambiente := nuevoAmbiente(t)
	ruta := ambiente.crearRuta(t, 1)

	for iteracion := 0; iteracion < 10; iteracion++ {
		consulta := ambiente.peticion(t, http.MethodGet, "/rutas/"+ruta.ID, nil)
		if consulta.estado != http.StatusOK {
			t.Fatalf("iteración %d: GET = %d: %s", iteracion, consulta.estado, consulta.cuerpo)
		}
		actual := decodificarRuta(t, consulta)
		switch actual.CuposDisponibles {
		case 0:
		case 1:
			if relleno := ambiente.ajustar(t, ruta.ID, -1); relleno.estado != http.StatusOK {
				t.Fatalf("iteración %d: preparar llena = %d: %s", iteracion, relleno.estado, relleno.cuerpo)
			}
		default:
			t.Fatalf("iteración %d: cupos inesperados = %d", iteracion, actual.CuposDisponibles)
		}

		cuerpos := []string{`{"delta":1}`, `{"delta":-1}`, `{"delta":-1}`}
		resultados := make([]resultadoPeticion, len(cuerpos))
		inicio := make(chan struct{})
		var espera sync.WaitGroup
		for i, cuerpo := range cuerpos {
			espera.Add(1)
			go func(indice int, cuerpo string) {
				defer espera.Done()
				<-inicio
				resultados[indice] = ambiente.peticionSinTesting(http.MethodPatch,
					"/rutas/"+ruta.ID+"/cupos", cuerpo)
			}(i, cuerpo)
		}
		close(inicio)
		espera.Wait()

		for i, resultado := range resultados {
			if resultado.err != nil {
				t.Fatalf("iteración %d, solicitud %d: %v", iteracion, i, resultado.err)
			}
			if resultado.estado == http.StatusOK {
				continue
			}
			if resultado.estado != http.StatusConflict {
				t.Fatalf("iteración %d, solicitud %d devolvió %d: %s", iteracion, i, resultado.estado, resultado.cuerpo)
			}
			switch resultado.codigo {
			case "sin_cupos", "cupos_excedidos", "ruta_cerrada":
			default:
				t.Fatalf("iteración %d, solicitud %d: codigo = %q", iteracion, i, resultado.codigo)
			}
		}
	}
}

func TestIndicesYDocumentoGeoJSON(t *testing.T) {
	ambiente := nuevoAmbiente(t)
	ruta := ambiente.crearRuta(t, 2)

	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	coleccion := ambiente.mongo.Database(ambiente.base).Collection(mongorutas.ColeccionRutas)

	cursor, err := coleccion.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("listar índices: %v", err)
	}
	var indices []struct {
		Nombre string `bson:"name"`
		Clave  bson.D `bson:"key"`
	}
	if err := cursor.All(ctx, &indices); err != nil {
		t.Fatalf("decodificar índices: %v", err)
	}
	encontrado := false
	for _, indice := range indices {
		if indice.Nombre != "trazado_2dsphere" {
			continue
		}
		for _, campo := range indice.Clave {
			if campo.Key == "trazado" && campo.Value == "2dsphere" {
				encontrado = true
			}
		}
	}
	if !encontrado {
		t.Fatalf("no existe el índice trazado_2dsphere: %+v", indices)
	}

	var documento struct {
		Trazado rutas.LineString `bson:"trazado"`
		Origen  rutas.Lugar      `bson:"origen"`
		Destino rutas.Lugar      `bson:"destino"`
		Salida  time.Time        `bson:"salida"`
	}
	if err := coleccion.FindOne(ctx, bson.D{{Key: "_id", Value: ruta.ID}}).Decode(&documento); err != nil {
		t.Fatalf("leer el documento: %v", err)
	}
	if documento.Trazado.Type != rutas.TipoLineString || len(documento.Trazado.Coordinates) < 2 {
		t.Fatalf("trazado inesperado: %+v", documento.Trazado)
	}
	primero := documento.Trazado.Coordinates[0]
	if len(primero) != 2 || primero[0] != -74.0645 || primero[1] != 4.6486 {
		t.Fatalf("orden de coordenadas inesperado: %v", primero)
	}
	if documento.Origen.Punto.Coordinates[0] != -74.0645 || documento.Salida.Location() != time.UTC {
		t.Fatalf("documento inesperado: %+v", documento)
	}
}
