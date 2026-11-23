package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

// crudAmbiente crea el ambiente de pruebas con el secreto de gateway despejado
// para que las llamadas del CRUD no dependan del entorno del desarrollador.
func crudAmbiente(t *testing.T) *ambiente {
	t.Helper()
	t.Setenv("GATEWAY_SECRET", "")
	return nuevoAmbiente(t)
}

// crudPeticion envía una solicitud con la identidad que inyectaría el gateway.
func crudPeticion(t *testing.T, ambiente *ambiente, metodo, ruta string, cuerpo any, usuario string) respuestaHTTP {
	t.Helper()
	var lector io.Reader
	if cuerpo != nil {
		datos, err := json.Marshal(cuerpo)
		if err != nil {
			t.Fatalf("serializar el cuerpo: %v", err)
		}
		lector = bytes.NewReader(datos)
	}
	peticion, err := http.NewRequest(metodo, ambiente.servidor.URL+ruta, lector)
	if err != nil {
		t.Fatalf("crear la petición: %v", err)
	}
	if cuerpo != nil {
		peticion.Header.Set("Content-Type", "application/json")
	}
	if usuario != "" {
		peticion.Header.Set("X-Usuario-Id", usuario)
	}
	respuesta, err := ambiente.cliente.Do(peticion)
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

// crudPeticionSinTesting permite lanzar solicitudes con identidad desde
// goroutines, donde *testing.T no es seguro.
func crudPeticionSinTesting(ambiente *ambiente, metodo, ruta, cuerpo, usuario string) resultadoPeticion {
	var lector io.Reader
	if cuerpo != "" {
		lector = bytes.NewBufferString(cuerpo)
	}
	peticion, err := http.NewRequest(metodo, ambiente.servidor.URL+ruta, lector)
	if err != nil {
		return resultadoPeticion{err: err}
	}
	if cuerpo != "" {
		peticion.Header.Set("Content-Type", "application/json")
	}
	if usuario != "" {
		peticion.Header.Set("X-Usuario-Id", usuario)
	}
	respuesta, err := ambiente.cliente.Do(peticion)
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
		var cuerpoError errorHTTP
		if err := json.Unmarshal(datos, &cuerpoError); err != nil {
			resultado.err = err
			return resultado
		}
		resultado.codigo = cuerpoError.Codigo
	}
	return resultado
}

func crudCuerpoCreacion(conductor string, cupos int, salida time.Time) map[string]any {
	cuerpo := cuerpoRuta(cupos)
	cuerpo["conductorId"] = conductor
	cuerpo["salida"] = salida.UTC().Format(time.RFC3339)
	return cuerpo
}

func crudCuerpoActualizacion(cupos int, salida time.Time) map[string]any {
	cuerpo := cuerpoRuta(cupos)
	delete(cuerpo, "conductorId")
	cuerpo["salida"] = salida.UTC().Format(time.RFC3339)
	cuerpo["precioTotal"] = 14000
	return cuerpo
}

func crudCrear(t *testing.T, ambiente *ambiente, conductor string, cupos int, salida time.Time) rutas.Ruta {
	t.Helper()
	respuesta := crudPeticion(t, ambiente, http.MethodPost, "/rutas", crudCuerpoCreacion(conductor, cupos, salida), "")
	if respuesta.estado != http.StatusCreated {
		t.Fatalf("POST /rutas = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	return decodificarRuta(t, respuesta)
}

func crudEditar(t *testing.T, ambiente *ambiente, id, usuario string, cupos int, salida time.Time) respuestaHTTP {
	t.Helper()
	return crudPeticion(t, ambiente, http.MethodPut, "/rutas/"+id, crudCuerpoActualizacion(cupos, salida), usuario)
}

func crudCerrar(t *testing.T, ambiente *ambiente, id, usuario string) respuestaHTTP {
	t.Helper()
	return crudPeticion(t, ambiente, http.MethodDelete, "/rutas/"+id, nil, usuario)
}

type crudListado struct {
	Items  []rutas.Ruta `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

func crudDecodificarListado(t *testing.T, respuesta respuestaHTTP) crudListado {
	t.Helper()
	var listado crudListado
	if err := json.Unmarshal(respuesta.cuerpo, &listado); err != nil {
		t.Fatalf("decodificar el listado: %v (%s)", err, respuesta.cuerpo)
	}
	return listado
}

func crudRuta(t *testing.T, ambiente *ambiente, id string) rutas.Ruta {
	t.Helper()
	respuesta := ambiente.peticion(t, http.MethodGet, "/rutas/"+id, nil)
	if respuesta.estado != http.StatusOK {
		t.Fatalf("GET /rutas/%s = %d: %s", id, respuesta.estado, respuesta.cuerpo)
	}
	return decodificarRuta(t, respuesta)
}

func crudURL(query string) string {
	return "/rutas?" + query
}

func crudVentana(desde, hasta time.Time) string {
	return fmt.Sprintf("desde=%s&hasta=%s",
		desde.UTC().Format(time.RFC3339), hasta.UTC().Format(time.RFC3339))
}
