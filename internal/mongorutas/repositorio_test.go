package mongorutas

import (
	"errors"
	"testing"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFiltroDeListado(t *testing.T) {
	desde := time.Date(2030, 1, 15, 0, 0, 0, 0, time.UTC)
	hasta := time.Date(2030, 1, 16, 0, 0, 0, 0, time.UTC)

	filtro := filtroDeListado(rutas.FiltroListado{
		ConductorID: "usr-12",
		Estado:      rutas.Llena,
		Desde:       desde,
		Hasta:       hasta,
	})
	if len(filtro) != 3 {
		t.Fatalf("campos del filtro = %d, se esperaban 3: %v", len(filtro), filtro)
	}
	if filtro[0].Key != "conductorId" || filtro[0].Value != "usr-12" {
		t.Fatalf("conductorId = %v", filtro[0])
	}
	if filtro[1].Key != "estado" || filtro[1].Value != "llena" {
		t.Fatalf("estado = %v", filtro[1])
	}
	ventana, ok := filtro[2].Value.(bson.D)
	if !ok || filtro[2].Key != "salida" || len(ventana) != 2 {
		t.Fatalf("ventana = %v", filtro[2])
	}
	if ventana[0].Key != "$gte" || !ventana[0].Value.(time.Time).Equal(desde) {
		t.Fatalf("desde = %v", ventana[0])
	}
	if ventana[1].Key != "$lte" || !ventana[1].Value.(time.Time).Equal(hasta) {
		t.Fatalf("hasta = %v", ventana[1])
	}

	if vacio := filtroDeListado(rutas.FiltroListado{}); len(vacio) != 0 {
		t.Fatalf("un filtro vacío no debe agregar condiciones: %v", vacio)
	}

	soloDesde := filtroDeListado(rutas.FiltroListado{Desde: desde})
	ventana = soloDesde[0].Value.(bson.D)
	if len(ventana) != 1 || ventana[0].Key != "$gte" {
		t.Fatalf("ventana con solo desde = %v", ventana)
	}
}

func TestActualizacionDeRutaConstruyeEscrituraCondicional(t *testing.T) {
	cambios := rutas.ActualizarRuta{
		Origen:       rutas.Lugar{Nombre: "Chapinero"},
		Destino:      rutas.Lugar{Nombre: "Universidad"},
		Trazado:      rutas.LineString{Type: rutas.TipoLineString},
		Salida:       time.Date(2030, 1, 15, 6, 30, 0, 0, time.UTC),
		CuposTotales: 4,
		PrecioTotal:  14000,
	}
	filtro, pipeline := actualizacionDeRuta("ruta-1a2b", "usr-12", cambios)
	if len(filtro) != 4 {
		t.Fatalf("filtro = %v", filtro)
	}
	if filtro[0].Key != "_id" || filtro[1].Key != "conductorId" || filtro[1].Value != "usr-12" {
		t.Fatalf("filtro de propiedad inesperado: %v", filtro)
	}
	estado, ok := filtro[2].Value.(bson.D)
	if !ok || filtro[2].Key != "estado" || estado[0].Key != "$ne" || estado[0].Value != "cerrada" {
		t.Fatalf("condición de estado inesperada: %v", filtro[2])
	}
	if filtro[3].Key != "$expr" {
		t.Fatalf("falta la condición de cupos comprometidos: %v", filtro[3])
	}

	if len(pipeline) != 1 {
		t.Fatalf("etapas del pipeline = %d", len(pipeline))
	}
	if pipeline[0][0].Key != "$set" {
		t.Fatalf("la etapa no es $set: %v", pipeline[0])
	}
	conjunto, ok := pipeline[0][0].Value.(bson.D)
	if !ok {
		t.Fatalf("el $set no es un documento: %v", pipeline[0][0].Value)
	}
	campos := map[string]any{}
	for _, campo := range conjunto {
		campos[campo.Key] = campo.Value
	}
	for _, campo := range []string{"origen", "destino", "trazado", "salida", "cuposTotales", "cuposDisponibles", "precioTotal", "estado"} {
		if _, existe := campos[campo]; !existe {
			t.Fatalf("el $set no incluye %q: %v", campo, campos)
		}
	}
	if campos["cuposTotales"] != 4 {
		t.Fatalf("cuposTotales = %v", campos["cuposTotales"])
	}
}

func TestCierreDeRutaConstruyeEscrituraCondicional(t *testing.T) {
	filtro, actualizacion := cierreDeRuta("ruta-1a2b", "usr-12")
	if len(filtro) != 4 {
		t.Fatalf("filtro = %v", filtro)
	}
	condicionCupos, ok := filtro[3].Value.(bson.D)
	if !ok || filtro[3].Key != "$expr" || condicionCupos[0].Key != "$eq" {
		t.Fatalf("condición de cupos = %v", filtro[3])
	}
	if len(actualizacion) != 1 || actualizacion[0].Key != "$set" {
		t.Fatalf("actualización de cierre = %v", actualizacion)
	}
	campos, ok := actualizacion[0].Value.(bson.D)
	if !ok || len(campos) != 1 || campos[0].Key != "estado" || campos[0].Value != "cerrada" {
		t.Fatalf("actualización de cierre = %v", actualizacion)
	}
}

func TestConflictoDeActualizacion(t *testing.T) {
	actual := rutas.Ruta{
		ID:               "ruta-1a2b",
		ConductorID:      "usr-12",
		CuposTotales:     4,
		CuposDisponibles: 2,
		Estado:           rutas.Abierta,
	}
	cambios := rutas.ActualizarRuta{CuposTotales: 2}

	if err := conflictoDeActualizacion(actual, cambios, "usr-01"); !errors.Is(err, rutas.ErrNoAutorizado) {
		t.Fatalf("identidad ajena: %v", err)
	}
	if err := conflictoDeActualizacion(actual, cambios, ""); !errors.Is(err, rutas.ErrNoAutorizado) {
		t.Fatalf("identidad vacía: %v", err)
	}
	if err := conflictoDeActualizacion(actual, cambios, "usr-12"); err != nil {
		t.Fatalf("condiciones cumplidas: %v", err)
	}

	cerrada := actual
	cerrada.Estado = rutas.Cerrada
	if err := conflictoDeActualizacion(cerrada, cambios, "usr-12"); !errors.Is(err, rutas.ErrRutaCerrada) {
		t.Fatalf("ruta cerrada: %v", err)
	}

	debajo := cambios
	debajo.CuposTotales = 1
	if err := conflictoDeActualizacion(actual, debajo, "usr-12"); !errors.Is(err, rutas.ErrReservasActivas) {
		t.Fatalf("cupos comprometidos: %v", err)
	}
}

func TestConflictoDeCierre(t *testing.T) {
	actual := rutas.Ruta{
		ID:               "ruta-1a2b",
		ConductorID:      "usr-12",
		CuposTotales:     3,
		CuposDisponibles: 3,
		Estado:           rutas.Abierta,
	}
	if err := conflictoDeCierre(actual, "usr-01"); !errors.Is(err, rutas.ErrNoAutorizado) {
		t.Fatalf("identidad ajena: %v", err)
	}
	if err := conflictoDeCierre(actual, "usr-12"); err != nil {
		t.Fatalf("cierre posible: %v", err)
	}

	comprometida := actual
	comprometida.CuposDisponibles = 2
	if err := conflictoDeCierre(comprometida, "usr-12"); !errors.Is(err, rutas.ErrReservasActivas) {
		t.Fatalf("cupos comprometidos: %v", err)
	}

	cerradaConComprometidos := comprometida
	cerradaConComprometidos.Estado = rutas.Cerrada
	if err := conflictoDeCierre(cerradaConComprometidos, "usr-12"); !errors.Is(err, rutas.ErrReservasActivas) {
		t.Fatalf("una ruta cerrada con comprometidos debe rechazarse: %v", err)
	}
	if err := conflictoDeCierre(cerradaConComprometidos, "usr-01"); !errors.Is(err, rutas.ErrNoAutorizado) {
		t.Fatalf("una ruta cerrada ajena no debe revelarse: %v", err)
	}

	cerrada := actual
	cerrada.Estado = rutas.Cerrada
	if err := conflictoDeCierre(cerrada, "usr-12"); err != nil {
		t.Fatalf("cierre idempotente: %v", err)
	}
}
