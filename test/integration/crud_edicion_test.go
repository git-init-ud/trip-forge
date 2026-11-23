package integration

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

func TestCRUDEdicionConservaComprometidos(t *testing.T) {
	ambiente := crudAmbiente(t)
	base := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)

	ruta := crudCrear(t, ambiente, "usr-12", 4, base)
	descuento := ambiente.ajustar(t, ruta.ID, -1)
	if descuento.estado != http.StatusOK {
		t.Fatalf("PATCH -1 = %d: %s", descuento.estado, descuento.cuerpo)
	}
	if actual := decodificarRuta(t, descuento); actual.CuposDisponibles != 3 {
		t.Fatalf("disponibles = %d", actual.CuposDisponibles)
	}

	nuevaSalida := base.Add(2 * time.Hour)
	respuesta := crudEditar(t, ambiente, ruta.ID, "usr-12", 6, nuevaSalida)
	if respuesta.estado != http.StatusOK {
		t.Fatalf("PUT = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	editada := decodificarRuta(t, respuesta)
	if editada.ID != ruta.ID || editada.ConductorID != "usr-12" {
		t.Fatalf("identidad inesperada: %+v", editada)
	}
	if editada.CuposTotales != 6 || editada.CuposDisponibles != 5 {
		t.Fatalf("cupos = %d/%d, se esperaban 6/5", editada.CuposDisponibles, editada.CuposTotales)
	}
	if editada.Estado != rutas.Abierta {
		t.Fatalf("estado = %q", editada.Estado)
	}
	if editada.PrecioTotal != 14000 {
		t.Fatalf("precio = %d", editada.PrecioTotal)
	}
	if !editada.Salida.Equal(nuevaSalida) || editada.Salida.Location() != time.UTC {
		t.Fatalf("salida = %v, se esperaba %v en UTC", editada.Salida, nuevaSalida)
	}
	documento := ambiente.leerDocumento(t, ruta.ID)
	if documento.CuposTotales != 6 || documento.CuposDisponibles != 5 || documento.Estado != "abierta" {
		t.Fatalf("documento inesperado: %+v", documento)
	}

	ajena := crudEditar(t, ambiente, ruta.ID, "usr-01", 3, base.Add(3*time.Hour))
	if ajena.estado != http.StatusForbidden {
		t.Fatalf("edición ajena = %d: %s", ajena.estado, ajena.cuerpo)
	}
	if codigo := decodificarError(t, ajena).Codigo; codigo != "no_autorizado" {
		t.Fatalf("codigo = %q", codigo)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.CuposTotales != 6 || documento.CuposDisponibles != 5 {
		t.Fatalf("la edición ajena alteró el documento: %+v", documento)
	}

	inexistente := crudEditar(t, ambiente, "ruta-0000000000000000", "usr-12", 3, base)
	if inexistente.estado != http.StatusNotFound {
		t.Fatalf("edición inexistente = %d: %s", inexistente.estado, inexistente.cuerpo)
	}
	if codigo := decodificarError(t, inexistente).Codigo; codigo != "no_encontrada" {
		t.Fatalf("codigo = %q", codigo)
	}

	alLimite := crudEditar(t, ambiente, ruta.ID, "usr-12", 1, base.Add(4*time.Hour))
	if alLimite.estado != http.StatusOK {
		t.Fatalf("PUT al límite = %d: %s", alLimite.estado, alLimite.cuerpo)
	}
	llena := decodificarRuta(t, alLimite)
	if llena.CuposTotales != 1 || llena.CuposDisponibles != 0 || llena.Estado != rutas.Llena {
		t.Fatalf("ruta al límite inesperada: %+v", llena)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.CuposTotales != 1 || documento.CuposDisponibles != 0 || documento.Estado != "llena" {
		t.Fatalf("documento al límite inesperado: %+v", documento)
	}
}

func TestCRUDEdicionPorDebajoDeComprometidos(t *testing.T) {
	ambiente := crudAmbiente(t)
	base := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)

	ruta := crudCrear(t, ambiente, "usr-12", 4, base)
	for i := 0; i < 2; i++ {
		if respuesta := ambiente.ajustar(t, ruta.ID, -1); respuesta.estado != http.StatusOK {
			t.Fatalf("preparar comprometidos = %d: %s", respuesta.estado, respuesta.cuerpo)
		}
	}
	antes := ambiente.leerDocumento(t, ruta.ID)
	if antes.CuposTotales != 4 || antes.CuposDisponibles != 2 {
		t.Fatalf("documento preparado inesperado: %+v", antes)
	}

	respuesta := crudEditar(t, ambiente, ruta.ID, "usr-12", 1, base.Add(time.Hour))
	if respuesta.estado != http.StatusConflict {
		t.Fatalf("PUT por debajo de comprometidos = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cuerpo := decodificarError(t, respuesta)
	if cuerpo.Codigo != "reservas_activas" {
		t.Fatalf("codigo = %q", cuerpo.Codigo)
	}
	if cuerpo.Detalle["rutaId"] != ruta.ID {
		t.Fatalf("detalle = %v", cuerpo.Detalle)
	}

	despues := ambiente.leerDocumento(t, ruta.ID)
	if despues != antes {
		t.Fatalf("el documento cambió con una edición rechazada: %+v -> %+v", antes, despues)
	}
	if actual := crudRuta(t, ambiente, ruta.ID); actual.CuposTotales != 4 || actual.CuposDisponibles != 2 || actual.Estado != rutas.Abierta {
		t.Fatalf("la ruta cambió: %+v", actual)
	}
}

func TestCRUDEditarRutaCerradaYCierreIdempotente(t *testing.T) {
	ambiente := crudAmbiente(t)
	base := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)

	ruta := crudCrear(t, ambiente, "usr-12", 2, base)
	if respuesta := crudCerrar(t, ambiente, ruta.ID, "usr-12"); respuesta.estado != http.StatusNoContent {
		t.Fatalf("DELETE = %d: %s", respuesta.estado, respuesta.cuerpo)
	}

	respuesta := crudEditar(t, ambiente, ruta.ID, "usr-12", 3, base.Add(time.Hour))
	if respuesta.estado != http.StatusConflict {
		t.Fatalf("PUT sobre cerrada = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	if codigo := decodificarError(t, respuesta).Codigo; codigo != "ruta_cerrada" {
		t.Fatalf("codigo = %q", codigo)
	}
	documento := ambiente.leerDocumento(t, ruta.ID)
	if documento.Estado != "cerrada" || documento.CuposTotales != 2 || documento.CuposDisponibles != 2 {
		t.Fatalf("documento inesperado: %+v", documento)
	}

	for intento := 0; intento < 2; intento++ {
		cierre := crudCerrar(t, ambiente, ruta.ID, "usr-12")
		if cierre.estado != http.StatusNoContent || len(cierre.cuerpo) != 0 {
			t.Fatalf("cierre idempotente %d = %d: %s", intento, cierre.estado, cierre.cuerpo)
		}
	}

	if respuesta := crudCerrar(t, ambiente, "ruta-0000000000000000", "usr-12"); respuesta.estado != http.StatusNotFound {
		t.Fatalf("DELETE inexistente = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
}

func TestCRUDCierreExigePropietario(t *testing.T) {
	ambiente := crudAmbiente(t)
	base := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)
	ruta := crudCrear(t, ambiente, "usr-12", 2, base)

	respuesta := crudCerrar(t, ambiente, ruta.ID, "usr-01")
	if respuesta.estado != http.StatusForbidden {
		t.Fatalf("DELETE ajeno = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	if codigo := decodificarError(t, respuesta).Codigo; codigo != "no_autorizado" {
		t.Fatalf("codigo = %q", codigo)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.Estado != "abierta" {
		t.Fatalf("el cierre ajeno cambió el estado: %+v", documento)
	}

	if cierre := crudCerrar(t, ambiente, ruta.ID, "usr-12"); cierre.estado != http.StatusNoContent {
		t.Fatalf("DELETE propio = %d: %s", cierre.estado, cierre.cuerpo)
	}
	if actual := crudRuta(t, ambiente, ruta.ID); actual.Estado != rutas.Cerrada {
		t.Fatalf("estado final = %q", actual.Estado)
	}
}

func TestCRUDCerrarConComprometidosYBloquearCupos(t *testing.T) {
	ambiente := crudAmbiente(t)
	base := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)

	ruta := crudCrear(t, ambiente, "usr-12", 2, base)
	if respuesta := ambiente.ajustar(t, ruta.ID, -1); respuesta.estado != http.StatusOK {
		t.Fatalf("PATCH -1 = %d: %s", respuesta.estado, respuesta.cuerpo)
	}

	cierre := crudCerrar(t, ambiente, ruta.ID, "usr-12")
	if cierre.estado != http.StatusConflict {
		t.Fatalf("cierre con comprometidos = %d: %s", cierre.estado, cierre.cuerpo)
	}
	cuerpo := decodificarError(t, cierre)
	if cuerpo.Codigo != "reservas_activas" {
		t.Fatalf("codigo = %q", cuerpo.Codigo)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.Estado != "abierta" || documento.CuposDisponibles != 1 {
		t.Fatalf("el cierre rechazado alteró el documento: %+v", documento)
	}

	devolucion := ambiente.ajustar(t, ruta.ID, 1)
	if devolucion.estado != http.StatusOK {
		t.Fatalf("devolución = %d: %s", devolucion.estado, devolucion.cuerpo)
	}
	if cierre := crudCerrar(t, ambiente, ruta.ID, "usr-12"); cierre.estado != http.StatusNoContent {
		t.Fatalf("cierre sin comprometidos = %d: %s", cierre.estado, cierre.cuerpo)
	}
	if actual := crudRuta(t, ambiente, ruta.ID); actual.Estado != rutas.Cerrada || actual.CuposDisponibles != 2 {
		t.Fatalf("ruta cerrada inesperada: %+v", actual)
	}

	descuento := ambiente.ajustar(t, ruta.ID, -1)
	if descuento.estado != http.StatusConflict {
		t.Fatalf("descuento tras el cierre = %d: %s", descuento.estado, descuento.cuerpo)
	}
	if codigo := decodificarError(t, descuento).Codigo; codigo != "sin_cupos" {
		t.Fatalf("codigo del descuento = %q", codigo)
	}
	devolucion = ambiente.ajustar(t, ruta.ID, 1)
	if devolucion.estado != http.StatusConflict {
		t.Fatalf("devolución tras el cierre = %d: %s", devolucion.estado, devolucion.cuerpo)
	}
	if codigo := decodificarError(t, devolucion).Codigo; codigo != "ruta_cerrada" {
		t.Fatalf("codigo de la devolución = %q", codigo)
	}
	if documento := ambiente.leerDocumento(t, ruta.ID); documento.Estado != "cerrada" || documento.CuposDisponibles != 2 {
		t.Fatalf("documento tras intentos bloqueados: %+v", documento)
	}
}

func TestCRUDConcurrenciaDescuentoYEdicion(t *testing.T) {
	ambiente := crudAmbiente(t)
	base := time.Now().UTC().Truncate(time.Second).Add(24 * time.Hour)

	ruta := crudCrear(t, ambiente, "usr-12", 3, base)
	if respuesta := ambiente.ajustar(t, ruta.ID, -1); respuesta.estado != http.StatusOK {
		t.Fatalf("PATCH inicial = %d: %s", respuesta.estado, respuesta.cuerpo)
	}

	cuerpoEdicion, err := json.Marshal(crudCuerpoActualizacion(2, base.Add(time.Hour)))
	if err != nil {
		t.Fatalf("serializar la edición: %v", err)
	}

	resultados := make([]resultadoPeticion, 2)
	inicio := make(chan struct{})
	var espera sync.WaitGroup
	espera.Add(2)
	go func() {
		defer espera.Done()
		<-inicio
		resultados[0] = crudPeticionSinTesting(ambiente, http.MethodPatch,
			"/rutas/"+ruta.ID+"/cupos", `{"delta":-1}`, "")
	}()
	go func() {
		defer espera.Done()
		<-inicio
		resultados[1] = crudPeticionSinTesting(ambiente, http.MethodPut,
			"/rutas/"+ruta.ID, string(cuerpoEdicion), "usr-12")
	}()
	close(inicio)
	espera.Wait()

	for i, resultado := range resultados {
		if resultado.err != nil {
			t.Fatalf("solicitud %d: %v", i, resultado.err)
		}
		if resultado.estado != http.StatusOK {
			t.Fatalf("solicitud %d = %d: %s", i, resultado.estado, resultado.cuerpo)
		}
		actual := decodificarRuta(t, respuestaHTTP{cuerpo: resultado.cuerpo})
		if actual.CuposDisponibles < 0 || actual.CuposDisponibles > actual.CuposTotales {
			t.Fatalf("solicitud %d dejó cupos fuera de rango: %+v", i, actual)
		}
	}

	documento := ambiente.leerDocumento(t, ruta.ID)
	if documento.CuposTotales != 2 {
		t.Fatalf("cuposTotales = %d, se esperaban 2", documento.CuposTotales)
	}
	if documento.CuposDisponibles < 0 || documento.CuposDisponibles > documento.CuposTotales {
		t.Fatalf("cupos fuera de rango: %+v", documento)
	}
	if documento.CuposDisponibles == 0 && documento.Estado != "llena" {
		t.Fatalf("estado incoherente: %+v", documento)
	}
	if documento.CuposDisponibles > 0 && documento.Estado != "abierta" {
		t.Fatalf("estado incoherente: %+v", documento)
	}

	actual := crudRuta(t, ambiente, ruta.ID)
	if actual.CuposTotales != documento.CuposTotales || actual.CuposDisponibles != documento.CuposDisponibles {
		t.Fatalf("la API y el documento no coinciden: %+v vs %+v", actual, documento)
	}

	// Con los dos cupos comprometidos, un descuento más debe rechazarse.
	descuento := ambiente.ajustar(t, ruta.ID, -1)
	if descuento.estado != http.StatusConflict {
		t.Fatalf("descuento posterior = %d: %s", descuento.estado, descuento.cuerpo)
	}
	if final := ambiente.leerDocumento(t, ruta.ID); final.CuposDisponibles < 0 {
		t.Fatalf("cupos negativos: %+v", final)
	}
}
