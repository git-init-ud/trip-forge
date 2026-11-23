package httpapi

import (
	"log"
	"net/http"
	"os"
	"strings"
)

// origenesDocumentacion son los orígenes del visor OpenAPI en desarrollo.
// CORS_ORIGINS puede reemplazarlos por una lista separada por comas.
var origenesDocumentacion = []string{
	"http://127.0.0.1:8090",
	"http://localhost:8090",
}

const (
	cabeceraOrigen = "Origin"
	// metodosCORS incluye los métodos que publicará el contrato completo del
	// CRUD; hoy solo GET, POST y PATCH tienen ruta registrada.
	metodosCORS = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	// X-Usuario-Id y X-Cluster-Secret son provisionales: el visor las envía
	// mientras el gateway no esté integrado. Cuando el clúster acuerde los
	// nombres definitivos, actualiza esta lista y el preflight.
	cabecerasCORS = "Content-Type, X-Request-Id, Authorization, X-Usuario-Id, X-Cluster-Secret"
)

// cors permite que Swagger UI, servido por cmd/docs en otro puerto, ejecute las
// operaciones desde el navegador. Solo agrega cabeceras de autorización CORS a
// los orígenes permitidos: por defecto el visor local y los de CORS_ORIGINS.
// Las solicitudes sin Origin no se bloquean, pero conservan Vary: Origin para
// que una caché compartida no reutilice la respuesta cross-origin.
func cors(siguiente http.Handler) http.Handler {
	permitidos := origenesCORS()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", cabeceraOrigen)
		origen := strings.TrimSpace(r.Header.Get(cabeceraOrigen))
		if origen == "" {
			siguiente.ServeHTTP(w, r)
			return
		}
		autorizado := contieneOrigen(permitidos, origen)

		if esPreflight(r) {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			if !autorizado {
				// El navegador descarta esta respuesta; el 403 solo ayuda a
				// diagnosticar desde curl. No es una operación del contrato.
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origen)
			w.Header().Set("Access-Control-Allow-Methods", metodosCORS)
			w.Header().Set("Access-Control-Allow-Headers", cabecerasCORS)
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if autorizado {
			w.Header().Set("Access-Control-Allow-Origin", origen)
			// Location y X-Request-Id son visibles para el navegador; el resto
			// de cabeceras de la respuesta sigue oculto.
			w.Header().Set("Access-Control-Expose-Headers", "Location, X-Request-Id")
		}
		siguiente.ServeHTTP(w, r)
	})
}

// esPreflight distingue la consulta previa de CORS de una solicitud normal con
// método OPTIONS. El navegador siempre envía Access-Control-Request-Method.
func esPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
}

// origenesCORS lee CORS_ORIGINS. Sin la variable se usa el visor local. Una
// lista vacía o compuesta solo por comas también vuelve al valor por defecto.
// El comodín "*" no se acepta: los orígenes deben ser exactos.
func origenesCORS() []string {
	crudo := strings.TrimSpace(os.Getenv("CORS_ORIGINS"))
	if crudo == "" {
		return origenesDocumentacion
	}

	var permitidos []string
	for _, parte := range strings.Split(crudo, ",") {
		origen := strings.TrimRight(strings.TrimSpace(parte), "/")
		switch {
		case origen == "*":
			log.Printf("CORS_ORIGINS: el comodín * no está permitido; enumera orígenes exactos")
		case origen != "":
			permitidos = append(permitidos, origen)
		}
	}
	if len(permitidos) == 0 {
		return origenesDocumentacion
	}
	return permitidos
}

func contieneOrigen(permitidos []string, origen string) bool {
	for _, permitido := range permitidos {
		if permitido == origen {
			return true
		}
	}
	return false
}
