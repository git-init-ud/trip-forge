// Package clients contiene los clientes HTTP del MID de rutas.
//
// Cada cliente impone un plazo propio, propaga X-Request-Id y, si el proceso
// tiene GATEWAY_SECRET, agrega X-Cluster-Secret. Los errores son tipados para
// que la capa HTTP distinga indisponibilidad de una respuesta inesperada sin
// exponer nunca el cuerpo de la dependencia.
package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrNoDisponible indica un fallo de red, un plazo vencido, un 5xx o una
	// respuesta que no cumple el contrato.
	ErrNoDisponible = errors.New("la dependencia no está disponible")
	// ErrRespuestaInesperada indica un 4xx que no describe el estado de un
	// recurso consultado. El llamador la trata como dependencia no disponible.
	ErrRespuestaInesperada = errors.New("la dependencia respondió con un estado inesperado")
	// ErrPerfilNoEncontrado indica que el perfil consultado no existe (404).
	ErrPerfilNoEncontrado = errors.New("el perfil no existe")
)

const (
	// cabeceraSecreto es la cabecera compartida del gateway. Es provisional.
	cabeceraSecreto = "X-Cluster-Secret"
	// cabeceraRequestID propaga el identificador de la solicitud original.
	cabeceraRequestID = "X-Request-Id"

	// maxRespuesta acota cuánto se lee de una dependencia para no consumir
	// memoria con una respuesta defectuosa.
	maxRespuesta = 4 << 20
)

type claveContexto string

const claveRequestID claveContexto = "request-id"

// patronIDConductor acepta identificadores que no necesitan codificación en
// una ruta HTTP. Es deliberadamente estricto: el contrato de perfiles usa
// identificadores opacos como "usr-12".
var patronIDConductor = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// IDConductorValido indica si el identificador se puede insertar sin escapar
// en la ruta de reputación. Evita la doble codificación de la URL y los
// segmentos inesperados.
func IDConductorValido(id string) bool {
	return patronIDConductor.MatchString(id)
}

// ConRequestID devuelve un contexto con el identificador que los clientes
// propagan como X-Request-Id. El MID lo usa para continuar la traza que inició
// el gateway.
func ConRequestID(ctx context.Context, id string) context.Context {
	if strings.TrimSpace(id) == "" {
		return ctx
	}
	return context.WithValue(ctx, claveRequestID, id)
}

// RequestIDDe lee el identificador de la solicitud del contexto.
func RequestIDDe(ctx context.Context) string {
	id, _ := ctx.Value(claveRequestID).(string)
	return id
}

// clienteBase comparte la construcción de URLs, cabeceras y lectura de
// respuestas entre los clientes del MID.
type clienteBase struct {
	base    *url.URL
	plazo   time.Duration
	secreto string
	token   string
	http    *http.Client
}

func nuevoClienteBase(nombre, baseURL string, plazo time.Duration, secreto, token string) (*clienteBase, error) {
	if plazo <= 0 {
		return nil, fmt.Errorf("%s: el plazo debe ser positivo", nombre)
	}
	direccion, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || direccion.Scheme == "" || direccion.Host == "" {
		return nil, fmt.Errorf("%s: la URL base es inválida", nombre)
	}
	if direccion.Scheme != "http" && direccion.Scheme != "https" {
		return nil, fmt.Errorf("%s: la URL base debe usar http o https", nombre)
	}
	return &clienteBase{
		base:    direccion,
		plazo:   plazo,
		secreto: strings.TrimSpace(secreto),
		token:   strings.TrimSpace(token),
		http: &http.Client{
			Timeout: plazo,
			// No se siguen redirecciones: el secreto y el token del gateway no
			// deben viajar a un destino distinto del configurado. Un 3xx se
			// trata como respuesta inesperada.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// obtener hace un GET y, cuando la respuesta es 200, decodifica el JSON en
// destino. Devuelve el estado para que cada cliente decida qué significa un
// 404. Los cuerpos no se incluyen en los errores.
func (c *clienteBase) obtener(ctx context.Context, ruta string, consulta url.Values, destino any) (int, error) {
	direccion := *c.base
	direccion.Path = strings.TrimRight(direccion.Path, "/") + ruta
	direccion.RawQuery = consulta.Encode()

	ctx, cancelar := context.WithTimeout(ctx, c.plazo)
	defer cancelar()

	peticion, err := http.NewRequestWithContext(ctx, http.MethodGet, direccion.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("%w: no se pudo construir la petición", ErrNoDisponible)
	}
	peticion.Header.Set("Accept", "application/json")
	if id := RequestIDDe(ctx); id != "" {
		peticion.Header.Set(cabeceraRequestID, id)
	}
	if c.secreto != "" {
		peticion.Header.Set(cabeceraSecreto, c.secreto)
	}
	if c.token != "" {
		// El token de servicio se conserva solo en memoria: nunca se registra.
		peticion.Header.Set("Authorization", "Bearer "+c.token)
	}

	respuesta, err := c.http.Do(peticion)
	if err != nil {
		return 0, fmt.Errorf("%w: %s no respondió", ErrNoDisponible, nombreDe(ruta))
	}
	defer respuesta.Body.Close()

	if respuesta.StatusCode != http.StatusOK {
		// Se drena un fragmento acotado para permitir reutilizar la conexión,
		// sin registrar ni conservar el cuerpo de la dependencia.
		_, _ = io.Copy(io.Discard, io.LimitReader(respuesta.Body, 4<<10))
		return respuesta.StatusCode, nil
	}

	datos, err := io.ReadAll(io.LimitReader(respuesta.Body, maxRespuesta+1))
	if err != nil {
		return respuesta.StatusCode, fmt.Errorf("%w: no se pudo leer la respuesta", ErrNoDisponible)
	}
	if len(datos) > maxRespuesta {
		return respuesta.StatusCode, fmt.Errorf("%w: la respuesta supera el límite de %d bytes", ErrNoDisponible, maxRespuesta)
	}
	if err := json.Unmarshal(datos, destino); err != nil {
		return respuesta.StatusCode, fmt.Errorf("%w: la respuesta no es JSON válido", ErrNoDisponible)
	}
	return respuesta.StatusCode, nil
}

// errorDeEstado traduce un estado no exitoso. Un 5xx es indisponibilidad; un
// 4xx inesperado se distingue para que el llamador decida, pero nunca se
// convierte en una respuesta parcial.
func errorDeEstado(nombre string, estado int) error {
	if estado >= http.StatusInternalServerError {
		return fmt.Errorf("%w: %s respondió %d", ErrNoDisponible, nombre, estado)
	}
	return fmt.Errorf("%w: %s respondió %d", ErrRespuestaInesperada, nombre, estado)
}

// nombreDe deriva un nombre de dependencia estable a partir de la ruta, para
// los registros y los errores tipados.
func nombreDe(ruta string) string {
	switch {
	case strings.HasPrefix(ruta, "/perfiles/"):
		return "reputación"
	case strings.HasPrefix(ruta, "/rutas"):
		return "rutas"
	default:
		return "la dependencia"
	}
}
