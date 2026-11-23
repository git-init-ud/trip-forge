package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

type almacenFalso struct {
	crear      func(context.Context, rutas.Ruta) error
	obtener    func(context.Context, string) (rutas.Ruta, error)
	ajustar    func(context.Context, string, int) (rutas.Ruta, error)
	listar     func(context.Context, rutas.FiltroListado) (rutas.Listado, error)
	actualizar func(context.Context, string, rutas.ActualizarRuta, string) (rutas.Ruta, error)
	cerrar     func(context.Context, string, string) error
}

func (a *almacenFalso) Crear(ctx context.Context, ruta rutas.Ruta) error {
	if a.crear == nil {
		return nil
	}
	return a.crear(ctx, ruta)
}

func (a *almacenFalso) Listar(ctx context.Context, filtro rutas.FiltroListado) (rutas.Listado, error) {
	if a.listar == nil {
		return rutas.Listado{}, nil
	}
	return a.listar(ctx, filtro)
}

func (a *almacenFalso) Obtener(ctx context.Context, id string) (rutas.Ruta, error) {
	if a.obtener == nil {
		return rutas.Ruta{}, rutas.ErrNoEncontrada
	}
	return a.obtener(ctx, id)
}

func (a *almacenFalso) Actualizar(ctx context.Context, id string, cambios rutas.ActualizarRuta, identidad string) (rutas.Ruta, error) {
	if a.actualizar == nil {
		return rutas.Ruta{}, rutas.ErrNoEncontrada
	}
	return a.actualizar(ctx, id, cambios, identidad)
}

func (a *almacenFalso) Cerrar(ctx context.Context, id, identidad string) error {
	if a.cerrar == nil {
		return nil
	}
	return a.cerrar(ctx, id, identidad)
}

func (a *almacenFalso) AjustarCupos(ctx context.Context, id string, delta int) (rutas.Ruta, error) {
	if a.ajustar == nil {
		return rutas.Ruta{}, rutas.ErrSinCupos
	}
	return a.ajustar(ctx, id, delta)
}

func rutaEjemplo() rutas.Ruta {
	salida := time.Date(2026, 11, 21, 6, 30, 0, 0, time.UTC)
	return rutas.Ruta{
		ID:          "ruta-1a2b3c4d5e6f7a8b",
		ConductorID: "usr-12",
		Origen: rutas.Lugar{
			Nombre: "Chapinero",
			Punto:  rutas.Punto{Type: rutas.TipoPunto, Coordinates: []float64{-74.0645, 4.6486}},
		},
		Destino: rutas.Lugar{
			Nombre: "Universidad",
			Punto:  rutas.Punto{Type: rutas.TipoPunto, Coordinates: []float64{-74.0415, 4.7012}},
		},
		Trazado: rutas.LineString{
			Type: rutas.TipoLineString,
			Coordinates: [][]float64{
				{-74.0645, 4.6486},
				{-74.0415, 4.7012},
			},
		},
		Salida:           salida,
		CuposTotales:     3,
		CuposDisponibles: 3,
		PrecioTotal:      12000,
		Estado:           rutas.Abierta,
	}
}

func cuerpoCreacion() map[string]any {
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
		"cuposTotales": 3,
		"precioTotal":  12000,
	}
}

// cuerpoActualizacion reutiliza la forma de creación sin conductorId, que el
// contrato de PUT no admite.
func cuerpoActualizacion() map[string]any {
	cuerpo := cuerpoCreacion()
	delete(cuerpo, "conductorId")
	cuerpo["salida"] = time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	cuerpo["cuposTotales"] = 4
	cuerpo["precioTotal"] = 14000
	return cuerpo
}

func solicitar(manejador http.Handler, metodo, ruta, cuerpo, tipoContenido, requestID string) *httptest.ResponseRecorder {
	peticion := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	if tipoContenido != "" {
		peticion.Header.Set("Content-Type", tipoContenido)
	}
	if requestID != "" {
		peticion.Header.Set("X-Request-Id", requestID)
	}
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)
	return grabador
}

func solicitarJSON(t *testing.T, manejador http.Handler, metodo, ruta string, cuerpo any) *httptest.ResponseRecorder {
	t.Helper()
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		t.Fatalf("serializar el cuerpo: %v", err)
	}
	return solicitar(manejador, metodo, ruta, string(datos), "application/json", "")
}

// solicitarConUsuario envía JSON con la cabecera de identidad que el gateway
// inyectaría. usuario vacío omite la cabecera.
func solicitarConUsuario(t *testing.T, manejador http.Handler, metodo, ruta string, cuerpo any, usuario string) *httptest.ResponseRecorder {
	t.Helper()
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		t.Fatalf("serializar el cuerpo: %v", err)
	}
	peticion := httptest.NewRequest(metodo, ruta, strings.NewReader(string(datos)))
	peticion.Header.Set("Content-Type", "application/json")
	if usuario != "" {
		peticion.Header.Set(cabeceraUsuario, usuario)
	}
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)
	return grabador
}

// solicitarCrudo envía un cuerpo tal cual, sin serializarlo, para probar
// cuerpos con datos después del objeto o cabeceras adicionales.
func solicitarCrudo(t *testing.T, manejador http.Handler, metodo, ruta, cuerpo string, cabeceras map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	peticion := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	peticion.Header.Set("Content-Type", "application/json")
	for nombre, valor := range cabeceras {
		peticion.Header.Set(nombre, valor)
	}
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)
	return grabador
}

func decodificarError(t *testing.T, grabador *httptest.ResponseRecorder) (errorRespuesta, map[string]any) {
	t.Helper()
	var crudo map[string]json.RawMessage
	if err := json.Unmarshal(grabador.Body.Bytes(), &crudo); err != nil {
		t.Fatalf("el error no es JSON: %v (%s)", err, grabador.Body.String())
	}
	for _, clave := range []string{"codigo", "mensaje", "detalle"} {
		if _, existe := crudo[clave]; !existe {
			t.Fatalf("la respuesta de error no incluye %q: %s", clave, grabador.Body.String())
		}
	}
	var respuesta errorRespuesta
	if err := json.Unmarshal(grabador.Body.Bytes(), &respuesta); err != nil {
		t.Fatalf("decodificar la respuesta de error: %v", err)
	}
	var detalle map[string]any
	if err := json.Unmarshal(crudo["detalle"], &detalle); err != nil {
		t.Fatalf("detalle no es un objeto: %v", err)
	}
	return respuesta, detalle
}

func TestCrearRutaResponde201(t *testing.T) {
	var capturada rutas.Ruta
	almacen := &almacenFalso{crear: func(_ context.Context, ruta rutas.Ruta) error {
		capturada = ruta
		return nil
	}}
	grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPost, "/rutas", cuerpoCreacion())

	if grabador.Code != http.StatusCreated {
		t.Fatalf("estado = %d, se esperaba 201: %s", grabador.Code, grabador.Body.String())
	}
	if tipo := grabador.Header().Get("Content-Type"); tipo != "application/json" {
		t.Fatalf("Content-Type = %q", tipo)
	}
	if esperado := "/rutas/" + capturada.ID; grabador.Header().Get("Location") != esperado {
		t.Fatalf("Location = %q, se esperaba %q", grabador.Header().Get("Location"), esperado)
	}
	if grabador.Header().Get(cabeceraRequestID) == "" {
		t.Fatal("falta X-Request-Id en la respuesta")
	}
	if capturada.ID == "" || capturada.CuposDisponibles != capturada.CuposTotales || capturada.Estado != rutas.Abierta {
		t.Fatalf("la ruta guardada no cumple el contrato: %+v", capturada)
	}

	var cuerpo rutas.Ruta
	if err := json.Unmarshal(grabador.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("decodificar la ruta: %v", err)
	}
	if cuerpo.ID != capturada.ID || cuerpo.ConductorID != "usr-12" || cuerpo.CuposDisponibles != 3 {
		t.Fatalf("cuerpo inesperado: %+v", cuerpo)
	}
}

func TestCrearRutaPropagaRequestID(t *testing.T) {
	almacen := &almacenFalso{}
	grabador := solicitar(RouterRutas(almacen), http.MethodPost, "/rutas",
		mustJSON(t, cuerpoCreacion()), "application/json", "req-entrante-42")

	if grabador.Code != http.StatusCreated {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if id := grabador.Header().Get(cabeceraRequestID); id != "req-entrante-42" {
		t.Fatalf("X-Request-Id = %q, se esperaba el entrante", id)
	}
}

func mustJSON(t *testing.T, valor any) string {
	t.Helper()
	datos, err := json.Marshal(valor)
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	return string(datos)
}

func TestCrearRutaRechazaEntradasInvalidas(t *testing.T) {
	origen := func(cuerpo map[string]any) map[string]any {
		return cuerpo["origen"].(map[string]any)
	}
	punto := func(lugar map[string]any) map[string]any {
		return lugar["punto"].(map[string]any)
	}

	casos := []struct {
		nombre    string
		modificar func(map[string]any)
		campo     string
	}{
		{"cupos cero", func(m map[string]any) { m["cuposTotales"] = 0 }, "cuposTotales"},
		{"siete cupos", func(m map[string]any) { m["cuposTotales"] = 7 }, "cuposTotales"},
		{"precio negativo", func(m map[string]any) { m["precioTotal"] = -1 }, "precioTotal"},
		{"salida pasada", func(m map[string]any) {
			m["salida"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
		}, "salida"},
		{"salida con formato inválido", func(m map[string]any) { m["salida"] = "mañana" }, "salida"},
		{"trazado de un punto", func(m map[string]any) {
			m["trazado"].(map[string]any)["coordinates"] = [][]float64{{-74.0645, 4.6486}}
		}, "trazado.coordinates"},
		{"coordenadas fuera de rango", func(m map[string]any) {
			punto(origen(m))["coordinates"] = []float64{181, 4.6486}
		}, "origen.punto.coordinates"},
		{"campos faltantes", func(m map[string]any) { delete(m, "precioTotal") }, "precioTotal"},
		{"conductor vacío", func(m map[string]any) { m["conductorId"] = " " }, "conductorId"},
		{"campo desconocido", func(m map[string]any) { m["otro"] = true }, "cuerpo"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			llamado := false
			almacen := &almacenFalso{crear: func(context.Context, rutas.Ruta) error {
				llamado = true
				return nil
			}}
			cuerpo := cuerpoCreacion()
			caso.modificar(cuerpo)
			grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPost, "/rutas", cuerpo)

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
			if respuesta.Mensaje == "" {
				t.Fatal("el mensaje no puede estar vacío")
			}
			if llamado {
				t.Fatal("no se debe llamar al almacén con una entrada inválida")
			}
		})
	}
}

func TestCrearRutaRechazaJSONBasura(t *testing.T) {
	almacen := &almacenFalso{}
	manejador := RouterRutas(almacen)

	grabador := solicitar(manejador, http.MethodPost, "/rutas", `{"conductorId":`, "application/json", "")
	if grabador.Code != http.StatusBadRequest {
		t.Fatalf("JSON roto: estado = %d", grabador.Code)
	}
	respuesta, _ := decodificarError(t, grabador)
	if respuesta.Codigo != codigoSolicitudInvalida {
		t.Fatalf("codigo = %q", respuesta.Codigo)
	}

	grabador = solicitar(manejador, http.MethodPost, "/rutas", mustJSON(t, cuerpoCreacion()), "text/plain", "")
	if grabador.Code != http.StatusBadRequest {
		t.Fatalf("Content-Type incorrecto: estado = %d", grabador.Code)
	}
	_, detalle := decodificarError(t, grabador)
	if campo, _ := detalle["campo"].(string); campo != "Content-Type" {
		t.Fatalf("detalle.campo = %q", campo)
	}

	enorme := `{"conductorId":"` + strings.Repeat("a", maxCuerpo+1) + `"}`
	grabador = solicitar(manejador, http.MethodPost, "/rutas", enorme, "application/json", "")
	if grabador.Code != http.StatusBadRequest {
		t.Fatalf("cuerpo enorme: estado = %d", grabador.Code)
	}
}

func TestObtenerRuta(t *testing.T) {
	t.Run("encontrada", func(t *testing.T) {
		esperada := rutaEjemplo()
		almacen := &almacenFalso{obtener: func(_ context.Context, id string) (rutas.Ruta, error) {
			if id != esperada.ID {
				return rutas.Ruta{}, rutas.ErrNoEncontrada
			}
			return esperada, nil
		}}
		grabador := solicitar(RouterRutas(almacen), http.MethodGet, "/rutas/"+esperada.ID, "", "", "req-get")

		if grabador.Code != http.StatusOK {
			t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
		}
		if grabador.Header().Get(cabeceraRequestID) != "req-get" {
			t.Fatal("no se propagó X-Request-Id")
		}
		var cuerpo rutas.Ruta
		if err := json.Unmarshal(grabador.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("decodificar la ruta: %v", err)
		}
		if cuerpo.ID != esperada.ID || cuerpo.Estado != rutas.Abierta {
			t.Fatalf("cuerpo inesperado: %+v", cuerpo)
		}
	})

	t.Run("inexistente", func(t *testing.T) {
		almacen := &almacenFalso{obtener: func(context.Context, string) (rutas.Ruta, error) {
			return rutas.Ruta{}, rutas.ErrNoEncontrada
		}}
		grabador := solicitar(RouterRutas(almacen), http.MethodGet, "/rutas/ruta-0000000000000000", "", "", "")
		if grabador.Code != http.StatusNotFound {
			t.Fatalf("estado = %d", grabador.Code)
		}
		respuesta, detalle := decodificarError(t, grabador)
		if respuesta.Codigo != codigoNoEncontrada {
			t.Fatalf("codigo = %q", respuesta.Codigo)
		}
		if detalle["rutaId"] != "ruta-0000000000000000" {
			t.Fatalf("detalle = %v", detalle)
		}
	})

	t.Run("fallo interno", func(t *testing.T) {
		almacen := &almacenFalso{obtener: func(context.Context, string) (rutas.Ruta, error) {
			return rutas.Ruta{}, errors.New("mongo caído")
		}}
		grabador := solicitar(RouterRutas(almacen), http.MethodGet, "/rutas/ruta-0000000000000000", "", "", "")
		if grabador.Code != http.StatusInternalServerError {
			t.Fatalf("estado = %d", grabador.Code)
		}
		respuesta, detalle := decodificarError(t, grabador)
		if respuesta.Codigo != codigoErrorInterno || respuesta.Mensaje == "" {
			t.Fatalf("respuesta = %+v", respuesta)
		}
		if len(detalle) != 0 {
			t.Fatalf("detalle = %v, se esperaba vacío", detalle)
		}
		if strings.Contains(grabador.Body.String(), "mongo caído") {
			t.Fatal("la respuesta interna no debe exponer la causa")
		}
	})
}

func TestAjustarCupos(t *testing.T) {
	t.Run("descuento", func(t *testing.T) {
		esperada := rutaEjemplo()
		esperada.CuposDisponibles = 2
		var deltaRecibido int
		almacen := &almacenFalso{ajustar: func(_ context.Context, id string, delta int) (rutas.Ruta, error) {
			if id != esperada.ID {
				t.Fatalf("id = %q", id)
			}
			deltaRecibido = delta
			return esperada, nil
		}}
		grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
			"/rutas/"+esperada.ID+"/cupos", map[string]any{"delta": -1})

		if grabador.Code != http.StatusOK {
			t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
		}
		if deltaRecibido != -1 {
			t.Fatalf("delta = %d", deltaRecibido)
		}
		var cuerpo rutas.Ruta
		if err := json.Unmarshal(grabador.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("decodificar la ruta: %v", err)
		}
		if cuerpo.CuposDisponibles != 2 {
			t.Fatalf("cupos = %d", cuerpo.CuposDisponibles)
		}
	})

	casos := []struct {
		nombre string
		delta  int
	}{
		{"cero", 0},
		{"positivo fuera de rango", 7},
		{"negativo fuera de rango", -7},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			llamado := false
			almacen := &almacenFalso{ajustar: func(context.Context, string, int) (rutas.Ruta, error) {
				llamado = true
				return rutas.Ruta{}, nil
			}}
			grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
				"/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", map[string]any{"delta": caso.delta})

			if grabador.Code != http.StatusBadRequest {
				t.Fatalf("estado = %d, se esperaba 400: %s", grabador.Code, grabador.Body.String())
			}
			respuesta, detalle := decodificarError(t, grabador)
			if respuesta.Codigo != codigoSolicitudInvalida {
				t.Fatalf("codigo = %q", respuesta.Codigo)
			}
			if detalle["campo"] != "delta" {
				t.Fatalf("detalle = %v", detalle)
			}
			if llamado {
				t.Fatal("no se debe llamar al almacén con un delta inválido")
			}
		})
	}

	t.Run("delta ausente", func(t *testing.T) {
		almacen := &almacenFalso{}
		grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
			"/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", map[string]any{})
		if grabador.Code != http.StatusBadRequest {
			t.Fatalf("estado = %d", grabador.Code)
		}
		_, detalle := decodificarError(t, grabador)
		if detalle["campo"] != "delta" {
			t.Fatalf("detalle = %v", detalle)
		}
	})

	t.Run("delta no entero", func(t *testing.T) {
		almacen := &almacenFalso{}
		grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
			"/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", map[string]any{"delta": 1.5})
		if grabador.Code != http.StatusBadRequest {
			t.Fatalf("estado = %d", grabador.Code)
		}
		respuesta, _ := decodificarError(t, grabador)
		if respuesta.Codigo != codigoSolicitudInvalida {
			t.Fatalf("codigo = %q", respuesta.Codigo)
		}
	})

	conflictos := []struct {
		nombre string
		err    error
		codigo string
	}{
		{"sin cupos", rutas.ErrSinCupos, codigoSinCupos},
		{"cupos excedidos", rutas.ErrCuposExcedidos, codigoCuposExcedidos},
		{"ruta cerrada", rutas.ErrRutaCerrada, codigoRutaCerrada},
	}
	for _, conflicto := range conflictos {
		t.Run(conflicto.nombre, func(t *testing.T) {
			almacen := &almacenFalso{ajustar: func(context.Context, string, int) (rutas.Ruta, error) {
				return rutas.Ruta{}, conflicto.err
			}}
			grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
				"/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", map[string]any{"delta": -1})
			if grabador.Code != http.StatusConflict {
				t.Fatalf("estado = %d", grabador.Code)
			}
			respuesta, detalle := decodificarError(t, grabador)
			if respuesta.Codigo != conflicto.codigo {
				t.Fatalf("codigo = %q, se esperaba %q", respuesta.Codigo, conflicto.codigo)
			}
			if detalle["rutaId"] != "ruta-1a2b3c4d5e6f7a8b" {
				t.Fatalf("detalle = %v", detalle)
			}
		})
	}

	t.Run("ruta inexistente", func(t *testing.T) {
		almacen := &almacenFalso{ajustar: func(context.Context, string, int) (rutas.Ruta, error) {
			return rutas.Ruta{}, rutas.ErrNoEncontrada
		}}
		grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
			"/rutas/ruta-0000000000000000/cupos", map[string]any{"delta": 1})
		if grabador.Code != http.StatusNotFound {
			t.Fatalf("estado = %d", grabador.Code)
		}
		respuesta, _ := decodificarError(t, grabador)
		if respuesta.Codigo != codigoNoEncontrada {
			t.Fatalf("codigo = %q", respuesta.Codigo)
		}
	})

	t.Run("fallo interno", func(t *testing.T) {
		almacen := &almacenFalso{ajustar: func(context.Context, string, int) (rutas.Ruta, error) {
			return rutas.Ruta{}, errors.New("timeout de mongo")
		}}
		grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
			"/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", map[string]any{"delta": -1})
		if grabador.Code != http.StatusInternalServerError {
			t.Fatalf("estado = %d", grabador.Code)
		}
		respuesta, detalle := decodificarError(t, grabador)
		if respuesta.Codigo != codigoErrorInterno || len(detalle) != 0 {
			t.Fatalf("respuesta = %+v detalle = %v", respuesta, detalle)
		}
		if strings.Contains(grabador.Body.String(), "timeout de mongo") {
			t.Fatal("la respuesta interna no debe exponer la causa")
		}
	})
}

func TestAjustarCuposAceptaLosLimitesDelContrato(t *testing.T) {
	casos := []struct {
		nombre string
		delta  int
	}{
		{"descuento máximo", -6},
		{"devolución máxima", 6},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			var recibido int
			almacen := &almacenFalso{ajustar: func(_ context.Context, _ string, delta int) (rutas.Ruta, error) {
				recibido = delta
				return rutaEjemplo(), nil
			}}
			grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
				"/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", map[string]any{"delta": caso.delta})
			if grabador.Code != http.StatusOK {
				t.Fatalf("delta %d: estado = %d: %s", caso.delta, grabador.Code, grabador.Body.String())
			}
			if recibido != caso.delta {
				t.Fatalf("delta recibido = %d, se esperaba %d", recibido, caso.delta)
			}
		})
	}
}

func TestErrorDeValidacionConservaFormato(t *testing.T) {
	almacen := &almacenFalso{}
	grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPatch,
		"/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", map[string]any{"delta": -7})

	respuesta, detalle := decodificarError(t, grabador)
	if respuesta.Codigo != codigoSolicitudInvalida {
		t.Fatalf("codigo = %q", respuesta.Codigo)
	}
	if detalle["campo"] != "delta" || detalle["motivo"] == nil {
		t.Fatalf("detalle = %v", detalle)
	}
	if !strings.Contains(respuesta.Mensaje, "delta") {
		t.Fatalf("mensaje = %q", respuesta.Mensaje)
	}
	if grabador.Header().Get(cabeceraRequestID) == "" {
		t.Fatal("los errores también deben llevar X-Request-Id")
	}
}

func TestRutaConYSinBarraFinal(t *testing.T) {
	for _, ruta := range []string{"/rutas", "/rutas/"} {
		t.Run(ruta, func(t *testing.T) {
			almacen := &almacenFalso{}
			grabador := solicitarJSON(t, RouterRutas(almacen), http.MethodPost, ruta, cuerpoCreacion())
			if grabador.Code != http.StatusCreated {
				t.Fatalf("POST %s = %d: %s", ruta, grabador.Code, grabador.Body.String())
			}
		})
	}
}

func TestRecuperarPanico(t *testing.T) {
	manejador := recuperar(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("fallo inesperado")
	}))
	grabador := httptest.NewRecorder()
	manejador.ServeHTTP(grabador, httptest.NewRequest(http.MethodGet, "/rutas", nil))

	if grabador.Code != http.StatusInternalServerError {
		t.Fatalf("estado = %d, se esperaba 500", grabador.Code)
	}
	respuesta, detalle := decodificarError(t, grabador)
	if respuesta.Codigo != codigoErrorInterno || len(detalle) != 0 {
		t.Fatalf("respuesta = %+v detalle = %v", respuesta, detalle)
	}
	if strings.Contains(grabador.Body.String(), "fallo inesperado") {
		t.Fatal("la respuesta no debe exponer el pánico")
	}
}

func TestSanitizarParaLog(t *testing.T) {
	if limpio := sanitizarParaLog("linea\ninyectada\r\t"); limpio != "lineainyectada" {
		t.Fatalf("sanitizarParaLog = %q", limpio)
	}
}

func TestRutasDesconocidasMantienenFormatoDeError(t *testing.T) {
	manejador := RouterRutas(&almacenFalso{})

	grabador := solicitar(manejador, http.MethodGet, "/desconocida", "", "", "")
	if grabador.Code != http.StatusNotFound {
		t.Fatalf("ruta desconocida: estado = %d", grabador.Code)
	}
	respuesta, detalle := decodificarError(t, grabador)
	if respuesta.Codigo != codigoNoEncontrada || len(detalle) != 0 {
		t.Fatalf("respuesta = %+v detalle = %v", respuesta, detalle)
	}

	grabador = solicitar(manejador, http.MethodPatch, "/rutas/ruta-1a2b3c4d5e6f7a8b", "", "", "")
	if grabador.Code != http.StatusMethodNotAllowed {
		t.Fatalf("método no permitido: estado = %d", grabador.Code)
	}
	respuesta, detalle = decodificarError(t, grabador)
	if respuesta.Codigo != codigoSolicitudInvalida || detalle["metodo"] != http.MethodPatch {
		t.Fatalf("respuesta = %+v detalle = %v", respuesta, detalle)
	}
}

func TestHealthRutasCrud(t *testing.T) {
	grabador := solicitar(RouterRutas(&almacenFalso{}), http.MethodGet, "/health", "", "", "")
	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d", grabador.Code)
	}
	if !bytes.Contains(grabador.Body.Bytes(), []byte(`"service":"rutas-crud"`)) {
		t.Fatalf("cuerpo = %s", grabador.Body.String())
	}
}

func TestListarRutas(t *testing.T) {
	esperada := rutaEjemplo()
	var capturado rutas.FiltroListado
	almacen := &almacenFalso{listar: func(_ context.Context, filtro rutas.FiltroListado) (rutas.Listado, error) {
		capturado = filtro
		return rutas.Listado{Items: []rutas.Ruta{esperada}, Total: 7}, nil
	}}

	grabador := solicitar(RouterRutas(almacen), http.MethodGet,
		"/rutas?conductorId=usr-12&estado=abierta&desde=2030-01-15T00:00:00Z&hasta=2030-01-16T00:00:00-05:00&limit=5&offset=10",
		"", "", "req-listado")

	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if grabador.Header().Get(cabeceraRequestID) != "req-listado" {
		t.Fatal("no se propagó X-Request-Id")
	}
	var cuerpo struct {
		Items  []rutas.Ruta `json:"items"`
		Total  int64        `json:"total"`
		Limit  int          `json:"limit"`
		Offset int          `json:"offset"`
	}
	if err := json.Unmarshal(grabador.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("decodificar el listado: %v (%s)", err, grabador.Body.String())
	}
	if cuerpo.Total != 7 || cuerpo.Limit != 5 || cuerpo.Offset != 10 || len(cuerpo.Items) != 1 {
		t.Fatalf("cuerpo inesperado: %+v", cuerpo)
	}
	if cuerpo.Items[0].ID != esperada.ID {
		t.Fatalf("item inesperado: %+v", cuerpo.Items[0])
	}

	if capturado.ConductorID != "usr-12" || capturado.Estado != rutas.Abierta {
		t.Fatalf("filtro inesperado: %+v", capturado)
	}
	desde, _ := time.Parse(time.RFC3339, "2030-01-15T00:00:00Z")
	hasta, _ := time.Parse(time.RFC3339, "2030-01-16T05:00:00Z")
	if !capturado.Desde.Equal(desde) || !capturado.Hasta.Equal(hasta) {
		t.Fatalf("ventana inesperada: %v - %v", capturado.Desde, capturado.Hasta)
	}
	if capturado.Desde.Location() != time.UTC || capturado.Hasta.Location() != time.UTC {
		t.Fatalf("la ventana no se normalizó a UTC: %v - %v", capturado.Desde, capturado.Hasta)
	}
}

func TestListarRutasPorDefecto(t *testing.T) {
	var capturado rutas.FiltroListado
	almacen := &almacenFalso{listar: func(_ context.Context, filtro rutas.FiltroListado) (rutas.Listado, error) {
		capturado = filtro
		return rutas.Listado{}, nil
	}}

	grabador := solicitar(RouterRutas(almacen), http.MethodGet, "/rutas", "", "", "")
	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if capturado.Limit != rutas.LimitPorDefecto || capturado.Offset != 0 || capturado.ConductorID != "" || capturado.Estado != "" {
		t.Fatalf("filtro por defecto inesperado: %+v", capturado)
	}
	if !strings.Contains(grabador.Body.String(), `"items":[]`) {
		t.Fatalf("items debe serializarse como lista vacía: %s", grabador.Body.String())
	}
}

func TestListarRutasRechazaConsultaInvalida(t *testing.T) {
	casos := []struct {
		nombre   string
		consulta string
		campo    string
	}{
		{"estado desconocido", "estado=rara", "estado"},
		{"limit cero", "limit=0", "limit"},
		{"limit máximo excedido", "limit=101", "limit"},
		{"limit no entero", "limit=abc", "limit"},
		{"offset negativo", "offset=-1", "offset"},
		{"desde con formato inválido", "desde=ayer", "desde"},
		{"hasta con formato inválido", "hasta=2030-01-15", "hasta"},
		{"ventana invertida", "desde=2030-01-16T00:00:00Z&hasta=2030-01-15T00:00:00Z", "desde"},
		{"conductor vacío", "conductorId=%20", "conductorId"},
		{"estado vacío", "estado=", "estado"},
		{"desde vacío", "desde=", "desde"},
		{"hasta vacío", "hasta=", "hasta"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			llamado := false
			almacen := &almacenFalso{listar: func(context.Context, rutas.FiltroListado) (rutas.Listado, error) {
				llamado = true
				return rutas.Listado{}, nil
			}}
			grabador := solicitar(RouterRutas(almacen), http.MethodGet, "/rutas?"+caso.consulta, "", "", "")
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
			if llamado {
				t.Fatal("no se debe consultar el almacén con una consulta inválida")
			}
		})
	}
}

func TestActualizarRuta(t *testing.T) {
	esperada := rutaEjemplo()
	esperada.CuposTotales = 4
	esperada.CuposDisponibles = 4
	esperada.PrecioTotal = 14000
	var (
		idRecibido        string
		identidadRecibida string
		cambiosRecibidos  rutas.ActualizarRuta
	)
	almacen := &almacenFalso{actualizar: func(_ context.Context, id string, cambios rutas.ActualizarRuta, identidad string) (rutas.Ruta, error) {
		idRecibido = id
		identidadRecibida = identidad
		cambiosRecibidos = cambios
		return esperada, nil
	}}

	grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodPut,
		"/rutas/"+esperada.ID, cuerpoActualizacion(), "usr-12")

	if grabador.Code != http.StatusOK {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if idRecibido != esperada.ID || identidadRecibida != "usr-12" {
		t.Fatalf("id = %q, identidad = %q", idRecibido, identidadRecibida)
	}
	if cambiosRecibidos.CuposTotales != 4 || cambiosRecibidos.PrecioTotal != 14000 {
		t.Fatalf("cambios inesperados: %+v", cambiosRecibidos)
	}
	if cambiosRecibidos.Origen.Nombre != "Chapinero" || len(cambiosRecibidos.Trazado.Coordinates) != 3 {
		t.Fatalf("geometría inesperada: %+v", cambiosRecibidos)
	}
	var cuerpo rutas.Ruta
	if err := json.Unmarshal(grabador.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("decodificar la ruta: %v", err)
	}
	if cuerpo.ID != esperada.ID || cuerpo.CuposTotales != 4 {
		t.Fatalf("cuerpo inesperado: %+v", cuerpo)
	}
}

func TestActualizarRutaRechazaEntradasInvalidas(t *testing.T) {
	casos := []struct {
		nombre    string
		modificar func(map[string]any)
		campo     string
	}{
		{"sin cupos", func(m map[string]any) { delete(m, "cuposTotales") }, "cuposTotales"},
		{"cupos cero", func(m map[string]any) { m["cuposTotales"] = 0 }, "cuposTotales"},
		{"siete cupos", func(m map[string]any) { m["cuposTotales"] = 7 }, "cuposTotales"},
		{"sin precio", func(m map[string]any) { delete(m, "precioTotal") }, "precioTotal"},
		{"precio negativo", func(m map[string]any) { m["precioTotal"] = -1 }, "precioTotal"},
		{"salida pasada", func(m map[string]any) {
			m["salida"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
		}, "salida"},
		{"salida inválida", func(m map[string]any) { m["salida"] = "mañana" }, "salida"},
		{"trazado de un punto", func(m map[string]any) {
			m["trazado"].(map[string]any)["coordinates"] = [][]float64{{-74.0645, 4.6486}}
		}, "trazado.coordinates"},
		{"longitud fuera de rango", func(m map[string]any) {
			m["origen"].(map[string]any)["punto"].(map[string]any)["coordinates"] = []float64{181, 4.6486}
		}, "origen.punto.coordinates"},
		{"conductorId no editable", func(m map[string]any) { m["conductorId"] = "usr-01" }, "cuerpo"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			llamado := false
			almacen := &almacenFalso{actualizar: func(context.Context, string, rutas.ActualizarRuta, string) (rutas.Ruta, error) {
				llamado = true
				return rutas.Ruta{}, nil
			}}
			cuerpo := cuerpoActualizacion()
			caso.modificar(cuerpo)
			grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodPut,
				"/rutas/ruta-1a2b3c4d5e6f7a8b", cuerpo, "usr-12")

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
			if llamado {
				t.Fatal("no se debe llamar al almacén con una entrada inválida")
			}
		})
	}
}

func TestActualizarRutaExigeIdentidad(t *testing.T) {
	llamado := false
	almacen := &almacenFalso{actualizar: func(context.Context, string, rutas.ActualizarRuta, string) (rutas.Ruta, error) {
		llamado = true
		return rutas.Ruta{}, nil
	}}
	grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodPut,
		"/rutas/ruta-1a2b3c4d5e6f7a8b", cuerpoActualizacion(), "")

	if grabador.Code != http.StatusForbidden {
		t.Fatalf("estado = %d, se esperaba 403: %s", grabador.Code, grabador.Body.String())
	}
	respuesta, detalle := decodificarError(t, grabador)
	if respuesta.Codigo != codigoNoAutorizado {
		t.Fatalf("codigo = %q", respuesta.Codigo)
	}
	if detalle["rutaId"] != "ruta-1a2b3c4d5e6f7a8b" {
		t.Fatalf("detalle = %v", detalle)
	}
	if llamado {
		t.Fatal("no se debe llamar al almacén sin identidad")
	}
}

func TestActualizarRutaClasificaFallosDelAlmacen(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		estado int
		codigo string
	}{
		{"inexistente", rutas.ErrNoEncontrada, http.StatusNotFound, codigoNoEncontrada},
		{"ajena", rutas.ErrNoAutorizado, http.StatusForbidden, codigoNoAutorizado},
		{"cerrada", rutas.ErrRutaCerrada, http.StatusConflict, codigoRutaCerrada},
		{"comprometidos", rutas.ErrReservasActivas, http.StatusConflict, codigoReservasActivas},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			almacen := &almacenFalso{actualizar: func(context.Context, string, rutas.ActualizarRuta, string) (rutas.Ruta, error) {
				return rutas.Ruta{}, caso.err
			}}
			grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodPut,
				"/rutas/ruta-1a2b3c4d5e6f7a8b", cuerpoActualizacion(), "usr-01")
			if grabador.Code != caso.estado {
				t.Fatalf("estado = %d, se esperaba %d: %s", grabador.Code, caso.estado, grabador.Body.String())
			}
			respuesta, detalle := decodificarError(t, grabador)
			if respuesta.Codigo != caso.codigo {
				t.Fatalf("codigo = %q, se esperaba %q", respuesta.Codigo, caso.codigo)
			}
			if detalle["rutaId"] != "ruta-1a2b3c4d5e6f7a8b" {
				t.Fatalf("detalle = %v", detalle)
			}
		})
	}

	t.Run("fallo interno", func(t *testing.T) {
		almacen := &almacenFalso{actualizar: func(context.Context, string, rutas.ActualizarRuta, string) (rutas.Ruta, error) {
			return rutas.Ruta{}, errors.New("mongo caído")
		}}
		grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodPut,
			"/rutas/ruta-1a2b3c4d5e6f7a8b", cuerpoActualizacion(), "usr-12")
		if grabador.Code != http.StatusInternalServerError {
			t.Fatalf("estado = %d", grabador.Code)
		}
		if strings.Contains(grabador.Body.String(), "mongo caído") {
			t.Fatal("la respuesta interna no debe exponer la causa")
		}
	})
}

func TestCerrarRutaResponde204(t *testing.T) {
	var (
		idRecibido        string
		identidadRecibida string
	)
	almacen := &almacenFalso{cerrar: func(_ context.Context, id, identidad string) error {
		idRecibido = id
		identidadRecibida = identidad
		return nil
	}}
	grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodDelete,
		"/rutas/ruta-1a2b3c4d5e6f7a8b", nil, "usr-12")

	if grabador.Code != http.StatusNoContent {
		t.Fatalf("estado = %d: %s", grabador.Code, grabador.Body.String())
	}
	if grabador.Body.Len() != 0 {
		t.Fatalf("204 no debe llevar cuerpo: %s", grabador.Body.String())
	}
	if grabador.Header().Get(cabeceraRequestID) == "" {
		t.Fatal("falta X-Request-Id")
	}
	if idRecibido != "ruta-1a2b3c4d5e6f7a8b" || identidadRecibida != "usr-12" {
		t.Fatalf("id = %q, identidad = %q", idRecibido, identidadRecibida)
	}
}

func TestCerrarRutaExigeIdentidad(t *testing.T) {
	llamado := false
	almacen := &almacenFalso{cerrar: func(context.Context, string, string) error {
		llamado = true
		return nil
	}}
	grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodDelete,
		"/rutas/ruta-1a2b3c4d5e6f7a8b", nil, "")

	if grabador.Code != http.StatusForbidden {
		t.Fatalf("estado = %d, se esperaba 403", grabador.Code)
	}
	respuesta, _ := decodificarError(t, grabador)
	if respuesta.Codigo != codigoNoAutorizado {
		t.Fatalf("codigo = %q", respuesta.Codigo)
	}
	if llamado {
		t.Fatal("no se debe llamar al almacén sin identidad")
	}
}

func TestCerrarRutaClasificaFallosDelAlmacen(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		estado int
		codigo string
	}{
		{"inexistente", rutas.ErrNoEncontrada, http.StatusNotFound, codigoNoEncontrada},
		{"ajena", rutas.ErrNoAutorizado, http.StatusForbidden, codigoNoAutorizado},
		{"comprometidos", rutas.ErrReservasActivas, http.StatusConflict, codigoReservasActivas},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			almacen := &almacenFalso{cerrar: func(context.Context, string, string) error {
				return caso.err
			}}
			grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodDelete,
				"/rutas/ruta-1a2b3c4d5e6f7a8b", nil, "usr-01")
			if grabador.Code != caso.estado {
				t.Fatalf("estado = %d, se esperaba %d: %s", grabador.Code, caso.estado, grabador.Body.String())
			}
			respuesta, _ := decodificarError(t, grabador)
			if respuesta.Codigo != caso.codigo {
				t.Fatalf("codigo = %q, se esperaba %q", respuesta.Codigo, caso.codigo)
			}
		})
	}
}

func TestCerrarRutaCerradaEsIdempotente(t *testing.T) {
	almacen := &almacenFalso{cerrar: func(context.Context, string, string) error {
		// El repositorio devuelve éxito cuando la ruta ya estaba cerrada.
		return nil
	}}
	manejador := RouterRutas(almacen)
	for intento := 0; intento < 2; intento++ {
		grabador := solicitarConUsuario(t, manejador, http.MethodDelete,
			"/rutas/ruta-1a2b3c4d5e6f7a8b", nil, "usr-12")
		if grabador.Code != http.StatusNoContent || grabador.Body.Len() != 0 {
			t.Fatalf("intento %d: estado = %d cuerpo = %s", intento, grabador.Code, grabador.Body.String())
		}
	}
}

func TestCrearRutaConCabeceraAjena(t *testing.T) {
	llamado := false
	almacen := &almacenFalso{crear: func(context.Context, rutas.Ruta) error {
		llamado = true
		return nil
	}}
	grabador := solicitarConUsuario(t, RouterRutas(almacen), http.MethodPost, "/rutas", cuerpoCreacion(), "usr-01")

	if grabador.Code != http.StatusForbidden {
		t.Fatalf("estado = %d, se esperaba 403: %s", grabador.Code, grabador.Body.String())
	}
	respuesta, _ := decodificarError(t, grabador)
	if respuesta.Codigo != codigoNoAutorizado {
		t.Fatalf("codigo = %q", respuesta.Codigo)
	}
	if llamado {
		t.Fatal("no se debe guardar una ruta con identidad ajena")
	}

	grabador = solicitarConUsuario(t, RouterRutas(almacen), http.MethodPost, "/rutas", cuerpoCreacion(), "usr-12")
	if grabador.Code != http.StatusCreated {
		t.Fatalf("cabecera propia: estado = %d: %s", grabador.Code, grabador.Body.String())
	}
}

func TestSecretoGatewayProtegeElRouter(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "secreto-de-prueba")
	almacen := &almacenFalso{}
	manejador := RouterRutas(almacen)

	grabador := solicitar(manejador, http.MethodGet, "/rutas", "", "", "")
	if grabador.Code != http.StatusUnauthorized {
		t.Fatalf("sin secreto: estado = %d, se esperaba 401", grabador.Code)
	}
	respuesta, _ := decodificarError(t, grabador)
	if respuesta.Codigo != codigoNoAutorizado {
		t.Fatalf("codigo = %q", respuesta.Codigo)
	}

	peticion := httptest.NewRequest(http.MethodGet, "/rutas", nil)
	peticion.Header.Set(cabeceraSecreto, "secreto-de-prueba")
	grabador = httptest.NewRecorder()
	manejador.ServeHTTP(grabador, peticion)
	if grabador.Code != http.StatusOK {
		t.Fatalf("con secreto: estado = %d: %s", grabador.Code, grabador.Body.String())
	}

	grabador = solicitar(manejador, http.MethodGet, "/health", "", "", "")
	if grabador.Code != http.StatusOK {
		t.Fatalf("health sin secreto: estado = %d", grabador.Code)
	}
}

func TestDecodificarRechazaDatosDespuesDelObjeto(t *testing.T) {
	creacion := mustJSON(t, cuerpoCreacion())
	actualizacion := mustJSON(t, cuerpoActualizacion())
	casos := []struct {
		nombre  string
		metodo  string
		ruta    string
		cuerpo  string
		usuario string
	}{
		{"POST objeto", http.MethodPost, "/rutas", creacion + "{}", ""},
		{"POST null", http.MethodPost, "/rutas", creacion + " null", ""},
		{"PUT objeto", http.MethodPut, "/rutas/ruta-1a2b3c4d5e6f7a8b", actualizacion + "{}", "usr-12"},
		{"PUT null", http.MethodPut, "/rutas/ruta-1a2b3c4d5e6f7a8b", actualizacion + " null", "usr-12"},
		{"PATCH objeto", http.MethodPatch, "/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", `{"delta":-1}{}`, ""},
		{"PATCH null", http.MethodPatch, "/rutas/ruta-1a2b3c4d5e6f7a8b/cupos", `{"delta":-1} null`, ""},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			almacen := &almacenFalso{}
			cabeceras := map[string]string{}
			if caso.usuario != "" {
				cabeceras[cabeceraUsuario] = caso.usuario
			}
			grabador := solicitarCrudo(t, RouterRutas(almacen), caso.metodo, caso.ruta, caso.cuerpo, cabeceras)
			if grabador.Code != http.StatusBadRequest {
				t.Fatalf("estado = %d, se esperaba 400: %s", grabador.Code, grabador.Body.String())
			}
			respuesta, detalle := decodificarError(t, grabador)
			if respuesta.Codigo != codigoSolicitudInvalida {
				t.Fatalf("codigo = %q", respuesta.Codigo)
			}
			if campo, _ := detalle["campo"].(string); campo != "cuerpo" {
				t.Fatalf("detalle.campo = %q, se esperaba cuerpo (%v)", campo, detalle)
			}
			if motivo, _ := detalle["motivo"].(string); !strings.Contains(motivo, "después del objeto") {
				t.Fatalf("detalle.motivo = %q", motivo)
			}
		})
	}
}

func TestCrearRutaConGatewayActivo(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "secreto-de-prueba")
	cuerpo := mustJSON(t, cuerpoCreacion())

	casos := []struct {
		nombre  string
		usuario string
		estado  int
		codigo  string
	}{
		{"sin identidad", "", http.StatusForbidden, codigoNoAutorizado},
		{"identidad propia", "usr-12", http.StatusCreated, ""},
		{"identidad ajena", "usr-01", http.StatusForbidden, codigoNoAutorizado},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			llamadas := 0
			almacen := &almacenFalso{crear: func(context.Context, rutas.Ruta) error {
				llamadas++
				return nil
			}}
			cabeceras := map[string]string{cabeceraSecreto: "secreto-de-prueba"}
			if caso.usuario != "" {
				cabeceras[cabeceraUsuario] = caso.usuario
			}
			grabador := solicitarCrudo(t, RouterRutas(almacen), http.MethodPost, "/rutas", cuerpo, cabeceras)
			if grabador.Code != caso.estado {
				t.Fatalf("estado = %d, se esperaba %d: %s", grabador.Code, caso.estado, grabador.Body.String())
			}
			if caso.codigo != "" {
				respuesta, _ := decodificarError(t, grabador)
				if respuesta.Codigo != caso.codigo {
					t.Fatalf("codigo = %q, se esperaba %q", respuesta.Codigo, caso.codigo)
				}
				if llamadas != 0 {
					t.Fatal("no se debe guardar la ruta sin una identidad válida")
				}
			}
		})
	}

	t.Run("sin secreto", func(t *testing.T) {
		almacen := &almacenFalso{}
		grabador := solicitarCrudo(t, RouterRutas(almacen), http.MethodPost, "/rutas", cuerpo,
			map[string]string{cabeceraUsuario: "usr-12"})
		if grabador.Code != http.StatusUnauthorized {
			t.Fatalf("estado = %d, se esperaba 401: %s", grabador.Code, grabador.Body.String())
		}
	})
}
