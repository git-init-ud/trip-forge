package integration

import (
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestCRUDListadoFiltrosYPaginacion(t *testing.T) {
	ambiente := crudAmbiente(t)
	base := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)

	creadas := 0
	for _, salida := range []time.Duration{0, 2 * time.Hour, 4 * time.Hour} {
		crudCrear(t, ambiente, "usr-01", 3, base.Add(salida))
		creadas++
	}
	for _, salida := range []time.Duration{time.Hour, 3 * time.Hour} {
		crudCrear(t, ambiente, "usr-02", 2, base.Add(salida))
		creadas++
	}
	if creadas != 5 {
		t.Fatalf("rutas preparadas = %d", creadas)
	}

	t.Run("todas ordenadas por salida", func(t *testing.T) {
		respuesta := ambiente.peticion(t, http.MethodGet, "/rutas", nil)
		if respuesta.estado != http.StatusOK {
			t.Fatalf("GET /rutas = %d: %s", respuesta.estado, respuesta.cuerpo)
		}
		listado := crudDecodificarListado(t, respuesta)
		if listado.Total != 5 || listado.Limit != 20 || listado.Offset != 0 || len(listado.Items) != 5 {
			t.Fatalf("listado inesperado: total=%d limit=%d offset=%d items=%d",
				listado.Total, listado.Limit, listado.Offset, len(listado.Items))
		}
		for i := 1; i < len(listado.Items); i++ {
			if listado.Items[i].Salida.Before(listado.Items[i-1].Salida) {
				t.Fatalf("el listado no está ordenado por salida: %v antes de %v",
					listado.Items[i-1].Salida, listado.Items[i].Salida)
			}
		}
	})

	t.Run("filtro por conductor", func(t *testing.T) {
		respuesta := ambiente.peticion(t, http.MethodGet, crudURL("conductorId=usr-01"), nil)
		if respuesta.estado != http.StatusOK {
			t.Fatalf("GET filtrado = %d: %s", respuesta.estado, respuesta.cuerpo)
		}
		listado := crudDecodificarListado(t, respuesta)
		if listado.Total != 3 || len(listado.Items) != 3 {
			t.Fatalf("total=%d items=%d, se esperaban 3", listado.Total, len(listado.Items))
		}
		for _, ruta := range listado.Items {
			if ruta.ConductorID != "usr-01" {
				t.Fatalf("conductor inesperado: %+v", ruta)
			}
		}
	})

	t.Run("ventana inclusiva", func(t *testing.T) {
		desde := base.Add(30 * time.Minute)
		hasta := base.Add(3*time.Hour + 30*time.Minute)
		respuesta := ambiente.peticion(t, http.MethodGet, crudURL(crudVentana(desde, hasta)), nil)
		if respuesta.estado != http.StatusOK {
			t.Fatalf("GET ventana = %d: %s", respuesta.estado, respuesta.cuerpo)
		}
		listado := crudDecodificarListado(t, respuesta)
		if listado.Total != 3 || len(listado.Items) != 3 {
			t.Fatalf("total=%d items=%d, se esperaban 3", listado.Total, len(listado.Items))
		}
		for _, ruta := range listado.Items {
			if ruta.Salida.Before(desde) || ruta.Salida.After(hasta) {
				t.Fatalf("ruta fuera de la ventana: %v", ruta.Salida)
			}
		}
	})

	t.Run("paginación", func(t *testing.T) {
		todas := crudDecodificarListado(t, ambiente.peticion(t, http.MethodGet, "/rutas", nil))
		if len(todas.Items) != 5 {
			t.Fatalf("items = %d", len(todas.Items))
		}

		respuesta := ambiente.peticion(t, http.MethodGet, crudURL("limit=2&offset=2"), nil)
		if respuesta.estado != http.StatusOK {
			t.Fatalf("GET paginado = %d: %s", respuesta.estado, respuesta.cuerpo)
		}
		listado := crudDecodificarListado(t, respuesta)
		if listado.Total != 5 || listado.Limit != 2 || listado.Offset != 2 || len(listado.Items) != 2 {
			t.Fatalf("página inesperada: total=%d limit=%d offset=%d items=%d",
				listado.Total, listado.Limit, listado.Offset, len(listado.Items))
		}
		if listado.Items[0].ID != todas.Items[2].ID || listado.Items[1].ID != todas.Items[3].ID {
			t.Fatalf("página inesperada: %s,%s; se esperaban %s,%s",
				listado.Items[0].ID, listado.Items[1].ID, todas.Items[2].ID, todas.Items[3].ID)
		}
	})

	casos := []struct {
		nombre   string
		consulta string
		campo    string
	}{
		{"estado desconocido", "estado=pausada", "estado"},
		{"limit excedido", "limit=101", "limit"},
		{"limit no entero", "limit=abc", "limit"},
		{"offset negativo", "offset=-1", "offset"},
		{"ventana invertida", "desde=2030-01-16T00:00:00Z&hasta=2030-01-15T00:00:00Z", "desde"},
		{"desde inválido", "desde=ayer", "desde"},
		{"conductor vacío", "conductorId=%20", "conductorId"},
		{"estado vacío", "estado=", "estado"},
		{"desde vacío", "desde=", "desde"},
		{"hasta vacío", "hasta=", "hasta"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			respuesta := ambiente.peticion(t, http.MethodGet, crudURL(caso.consulta), nil)
			if respuesta.estado != http.StatusBadRequest {
				t.Fatalf("estado = %d, se esperaba 400: %s", respuesta.estado, respuesta.cuerpo)
			}
			cuerpo := decodificarError(t, respuesta)
			if cuerpo.Codigo != "solicitud_invalida" {
				t.Fatalf("codigo = %q", cuerpo.Codigo)
			}
			if campo, _ := cuerpo.Detalle["campo"].(string); campo != caso.campo {
				t.Fatalf("detalle.campo = %q, se esperaba %q (%v)", campo, caso.campo, cuerpo.Detalle)
			}
		})
	}
}

func TestCRUDListadoPorEstadoYDesempate(t *testing.T) {
	ambiente := crudAmbiente(t)
	base := time.Now().UTC().Truncate(time.Second).Add(30 * time.Hour)

	llena := crudCrear(t, ambiente, "usr-12", 1, base)
	if respuesta := ambiente.ajustar(t, llena.ID, -1); respuesta.estado != http.StatusOK {
		t.Fatalf("preparar ruta llena = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cerrada := crudCrear(t, ambiente, "usr-12", 2, base.Add(time.Hour))
	if respuesta := crudCerrar(t, ambiente, cerrada.ID, "usr-12"); respuesta.estado != http.StatusNoContent {
		t.Fatalf("cerrar la ruta = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	abierta := crudCrear(t, ambiente, "usr-12", 2, base.Add(2*time.Hour))

	respuesta := ambiente.peticion(t, http.MethodGet, crudURL("estado=llena"), nil)
	if respuesta.estado != http.StatusOK {
		t.Fatalf("GET llena = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	listado := crudDecodificarListado(t, respuesta)
	if listado.Total != 1 || len(listado.Items) != 1 || listado.Items[0].ID != llena.ID {
		t.Fatalf("listado llena inesperado: %+v", listado)
	}

	respuesta = ambiente.peticion(t, http.MethodGet, crudURL("estado=cerrada"), nil)
	if respuesta.estado != http.StatusOK {
		t.Fatalf("GET cerrada = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	listado = crudDecodificarListado(t, respuesta)
	if listado.Total != 1 || len(listado.Items) != 1 || listado.Items[0].ID != cerrada.ID {
		t.Fatalf("listado cerrada inesperado: %+v", listado)
	}

	respuesta = ambiente.peticion(t, http.MethodGet, crudURL("estado=abierta&conductorId=usr-12"), nil)
	if respuesta.estado != http.StatusOK {
		t.Fatalf("GET abierta = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	listado = crudDecodificarListado(t, respuesta)
	if listado.Total != 1 || len(listado.Items) != 1 || listado.Items[0].ID != abierta.ID {
		t.Fatalf("listado abierta inesperado: %+v", listado)
	}

	// Tres rutas con la misma salida deben desempatar por id ascendente.
	var ids []string
	for i := 0; i < 3; i++ {
		ruta := crudCrear(t, ambiente, "usr-03", 2, base.Add(3*time.Hour))
		ids = append(ids, ruta.ID)
	}
	respuesta = ambiente.peticion(t, http.MethodGet, crudURL("conductorId=usr-03"), nil)
	if respuesta.estado != http.StatusOK {
		t.Fatalf("GET desempate = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	listado = crudDecodificarListado(t, respuesta)
	if listado.Total != 3 || len(listado.Items) != 3 {
		t.Fatalf("listado desempate inesperado: %+v", listado)
	}
	sort.Strings(ids)
	for i, ruta := range listado.Items {
		if ruta.ID != ids[i] {
			t.Fatalf("orden por id inesperado en posición %d: %q, se esperaba %q", i, ruta.ID, ids[i])
		}
	}
}

func TestCRUDListadoRechazaFechaMalFormada(t *testing.T) {
	ambiente := crudAmbiente(t)
	respuesta := ambiente.peticion(t, http.MethodGet, crudURL("hasta=2030-01-15"), nil)
	if respuesta.estado != http.StatusBadRequest {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cuerpo := decodificarError(t, respuesta)
	if !strings.Contains(strings.ToLower(cuerpo.Mensaje), "hasta") {
		t.Fatalf("mensaje = %q", cuerpo.Mensaje)
	}
}
