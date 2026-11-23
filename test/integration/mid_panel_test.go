package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/mid"
)

func TestMIDPanelConOcupacionYReputacion(t *testing.T) {
	entorno := midNuevoEntorno(t)
	base := time.Now().UTC().Add(24 * time.Hour)

	primera := midCrearRuta(t, entorno.crud, "usr-12", 3, base.Add(30*time.Hour), midTrazado(true))
	segunda := midCrearRuta(t, entorno.crud, "usr-12", 2, base.Add(50*time.Hour), midTrazado(true))

	ajuste := entorno.crud.peticion(t, http.MethodPatch, "/rutas/"+primera.ID+"/cupos",
		map[string]any{"delta": -1})
	if ajuste.estado != http.StatusOK {
		t.Fatalf("descontar un cupo = %d: %s", ajuste.estado, ajuste.cuerpo)
	}

	viajes := 12
	tasa := 0.05
	total := 9
	entorno.reputacion.configurar("usr-12", midPerfil{
		puntaje:             4.7,
		insignia:            "confiable",
		viajesCompletados:   &viajes,
		tasaCancelacion:     &tasa,
		totalCalificaciones: &total,
	})

	respuesta := midPeticion(t, entorno.servidor, http.MethodGet, "/conductores/usr-12/panel")
	if respuesta.estado != http.StatusOK {
		t.Fatalf("GET panel = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	var panel mid.Panel
	if err := json.Unmarshal(respuesta.cuerpo, &panel); err != nil {
		t.Fatalf("decodificar el panel: %v (%s)", err, respuesta.cuerpo)
	}
	if panel.ConductorID != "usr-12" || panel.Puntaje != 4.7 || panel.Insignia != "confiable" {
		t.Fatalf("panel = %+v", panel)
	}
	if panel.ViajesCompletados == nil || *panel.ViajesCompletados != 12 {
		t.Fatalf("viajesCompletados = %v", panel.ViajesCompletados)
	}
	if panel.TasaCancelacion == nil || *panel.TasaCancelacion != 0.05 {
		t.Fatalf("tasaCancelacion = %v", panel.TasaCancelacion)
	}
	if len(panel.Rutas) != 2 {
		t.Fatalf("rutas = %+v", panel.Rutas)
	}
	if panel.Rutas[0].Ruta.ID != primera.ID || panel.Rutas[0].Ocupados != 1 {
		t.Fatalf("primera ruta = %+v", panel.Rutas[0])
	}
	if panel.Rutas[1].Ruta.ID != segunda.ID || panel.Rutas[1].Ocupados != 0 {
		t.Fatalf("segunda ruta = %+v", panel.Rutas[1])
	}
}

func TestMIDPanelSinRutasRespondeListaVacia(t *testing.T) {
	entorno := midNuevoEntorno(t)
	entorno.reputacion.configurar("usr-50", midPerfil{puntaje: 5, insignia: "confiable"})

	respuesta := midPeticion(t, entorno.servidor, http.MethodGet, "/conductores/usr-50/panel")
	if respuesta.estado != http.StatusOK {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	var panel mid.Panel
	if err := json.Unmarshal(respuesta.cuerpo, &panel); err != nil {
		t.Fatalf("decodificar el panel: %v (%s)", err, respuesta.cuerpo)
	}
	if panel.ConductorID != "usr-50" || panel.Rutas == nil || len(panel.Rutas) != 0 {
		t.Fatalf("panel = %+v", panel)
	}
}

func TestMIDPanelPerfilInexistenteResponde404(t *testing.T) {
	entorno := midNuevoEntorno(t)

	respuesta := midPeticion(t, entorno.servidor, http.MethodGet, "/conductores/usr-404/panel")
	if respuesta.estado != http.StatusNotFound {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cuerpo := decodificarError(t, respuesta)
	if cuerpo.Codigo != "no_encontrada" || cuerpo.Detalle["conductorId"] != "usr-404" {
		t.Fatalf("error = %+v", cuerpo)
	}
}

func TestMIDPanelConReputacionCaidaResponde503(t *testing.T) {
	entorno := midNuevoEntorno(t)
	entorno.reputacion.configurar("usr-12", midPerfil{puntaje: 4.7, insignia: "confiable"})
	entorno.reputacion.marcarCaido(true)

	respuesta := midPeticion(t, entorno.servidor, http.MethodGet, "/conductores/usr-12/panel")
	if respuesta.estado != http.StatusServiceUnavailable {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cuerpo := decodificarError(t, respuesta)
	if cuerpo.Codigo != "dependencia_no_disponible" || cuerpo.Detalle["servicio"] != "reputacion" {
		t.Fatalf("error = %+v", cuerpo)
	}
}

func TestMIDPanelConCRUPCaidoResponde503(t *testing.T) {
	reputacion := midNuevaReputacion()
	reputacion.configurar("usr-12", midPerfil{puntaje: 4.7, insignia: "confiable"})
	servidorReputacion := httptest.NewServer(reputacion)
	defer servidorReputacion.Close()

	caido := httptest.NewServer(http.NewServeMux())
	caido.Close()

	servidor := midServidorHacia(t, caido.URL, servidorReputacion.URL)
	respuesta := midPeticion(t, servidor, http.MethodGet, "/conductores/usr-12/panel")
	if respuesta.estado != http.StatusServiceUnavailable {
		t.Fatalf("estado = %d: %s", respuesta.estado, respuesta.cuerpo)
	}
	cuerpo := decodificarError(t, respuesta)
	if cuerpo.Codigo != "dependencia_no_disponible" || cuerpo.Detalle["servicio"] != "rutas-crud" {
		t.Fatalf("error = %+v", cuerpo)
	}
}
