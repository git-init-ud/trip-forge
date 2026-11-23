package httpapi

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	// cabeceraUsuario es la identidad que el gateway inyecta tras validar el
	// token. El nombre es provisional: se debe confirmar con el líder del
	// gateway antes de la entrega al clúster.
	cabeceraUsuario = "X-Usuario-Id"
	// cabeceraSecreto es la cabecera compartida que el gateway agrega a toda
	// petición interna. También es provisional.
	cabeceraSecreto = "X-Cluster-Secret"
)

// usuarioAutenticado devuelve la identidad que dejó el gateway, sin espacios.
// La autorización de propietario vive en las operaciones de negocio; este
// ayudante solo lee la cabecera.
func usuarioAutenticado(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get(cabeceraUsuario))
}

// secretoGateway exige la cabecera secreta del gateway cuando GATEWAY_SECRET
// está definida. Sin la variable la comprobación queda desactivada para no
// romper el desarrollo local mientras el clúster acuerda el valor. /health y
// el preflight CORS quedan fuera: el balanceador sondea la salud sin secreto y
// el navegador no envía cabeceras personalizadas en el preflight.
//
// La comparación es de tiempo constante para no filtrar el secreto por la
// duración de la respuesta.
func secretoGateway(siguiente http.Handler) http.Handler {
	secreto := strings.TrimSpace(os.Getenv("GATEWAY_SECRET"))
	if secreto == "" {
		return siguiente
	}
	log.Printf("gateway: se exige la cabecera %s", cabeceraSecreto)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || esPreflight(r) {
			siguiente.ServeHTTP(w, r)
			return
		}
		recibido := r.Header.Get(cabeceraSecreto)
		if subtle.ConstantTimeCompare([]byte(recibido), []byte(secreto)) != 1 {
			responderJSON(w, http.StatusUnauthorized, errorRespuesta{
				Codigo:  codigoNoAutorizado,
				Mensaje: "Falta la cabecera secreta del gateway o no coincide",
				Detalle: map[string]any{},
			})
			return
		}
		siguiente.ServeHTTP(w, r)
	})
}
