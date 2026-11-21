package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// recuperar convierte un pánico en la respuesta de error común. El pánico se
// registra con el request id y sin volcar cuerpos de la solicitud.
func recuperar(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if motivo := recover(); motivo != nil {
				log.Printf("panico metodo=%s ruta=%s request_id=%s motivo=%s",
					r.Method, sanitizarParaLog(r.URL.Path), requestIDDe(r.Context()),
					sanitizarParaLog(fmt.Sprint(motivo)))
				responderJSON(w, http.StatusInternalServerError, errorRespuesta{
					Codigo:  codigoErrorInterno,
					Mensaje: "No fue posible completar la solicitud",
					Detalle: map[string]any{},
				})
			}
		}()
		siguiente.ServeHTTP(w, r)
	})
}

const (
	cabeceraRequestID = "X-Request-Id"
	maxLargoRequestID = 128
)

type claveContexto string

const claveRequestID claveContexto = "request-id"

// requestID conserva el identificador entrante o genera uno, lo deja en el
// contexto y lo devuelve en la respuesta. Se descartan valores con caracteres
// de control para no inyectar saltos de línea en los registros.
func requestID(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := normalizarRequestID(r.Header.Get(cabeceraRequestID))
		if id == "" {
			id = generarRequestID()
		}
		w.Header().Set(cabeceraRequestID, id)
		ctx := context.WithValue(r.Context(), claveRequestID, id)
		siguiente.ServeHTTP(w, r.WithContext(ctx))
	})
}

// registrarSolicitudes anota método, ruta, estado, duración e identificador.
// Nunca registra cuerpos ni cabeceras sensibles. /health queda fuera para no
// llenar el registro con el sondeo de disponibilidad.
func registrarSolicitudes(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		envuelto := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		siguiente.ServeHTTP(envuelto, r)
		if r.URL.Path == "/health" {
			return
		}
		patron := chi.RouteContext(r.Context()).RoutePattern()
		if patron == "" {
			patron = r.URL.Path
		}
		log.Printf("metodo=%s ruta=%s estado=%d duracion=%s request_id=%s",
			r.Method, sanitizarParaLog(patron), envuelto.Status(), time.Since(inicio), requestIDDe(r.Context()))
	})
}

func requestIDDe(ctx context.Context) string {
	id, _ := ctx.Value(claveRequestID).(string)
	return id
}

// sanitizarParaLog evita que un camino con caracteres de control forje líneas
// en el registro y limita el largo anotado.
func sanitizarParaLog(valor string) string {
	limpio := strings.Map(func(caracter rune) rune {
		if caracter < 0x20 || caracter == 0x7f {
			return -1
		}
		return caracter
	}, valor)
	if len(limpio) > 256 {
		return limpio[:256] + "..."
	}
	return limpio
}

func normalizarRequestID(valor string) string {
	valor = strings.TrimSpace(valor)
	if valor == "" || len(valor) > maxLargoRequestID {
		return ""
	}
	for _, caracter := range valor {
		if caracter < 0x20 || caracter > 0x7e {
			return ""
		}
	}
	return valor
}

func generarRequestID() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}
