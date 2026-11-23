package matching

import "testing"

func TestPuntajeDelContrato(t *testing.T) {
	obtenido := Puntaje(120, 80, 4.5)
	esperado := (120+80)/100.0 - 4.5*2
	if obtenido != esperado {
		t.Fatalf("puntaje = %v, se esperaba %v", obtenido, esperado)
	}
}

func TestOrdenarCandidatosPorPuntaje(t *testing.T) {
	// Mismas distancias: la reputación define el orden.
	mejor := NuevoCandidato("ruta-b", 100, 100, 4.8, InsigniaConfiable)
	peor := NuevoCandidato("ruta-a", 100, 100, 3.0, InsigniaRegular)

	candidatos := []Candidato{peor, mejor}
	OrdenarCandidatos(candidatos)
	if candidatos[0].RutaID != "ruta-b" {
		t.Fatalf("el conductor con mejor puntaje debe ir primero: %+v", candidatos)
	}
}

func TestOrdenarCandidatosDesempataPorID(t *testing.T) {
	primero := NuevoCandidato("ruta-aaa", 100, 100, 4.0, InsigniaConfiable)
	segundo := NuevoCandidato("ruta-bbb", 100, 100, 4.0, InsigniaConfiable)

	candidatos := []Candidato{segundo, primero}
	OrdenarCandidatos(candidatos)
	if candidatos[0].RutaID != "ruta-aaa" || candidatos[1].RutaID != "ruta-bbb" {
		t.Fatalf("el desempate debe ser por id ascendente: %+v", candidatos)
	}
}

func TestCandidatoEnObservacionNoEsElegible(t *testing.T) {
	candidato := NuevoCandidato("ruta-a", 10, 20, 1.0, InsigniaEnObservacion)
	if candidato.Elegible() {
		t.Fatal("en_observacion no debe ser elegible")
	}
	for _, insignia := range []Insignia{InsigniaNuevo, InsigniaConfiable, InsigniaRegular, InsigniaEnObservacion} {
		if !insignia.Valida() {
			t.Fatalf("la insignia %q debería ser válida", insignia)
		}
	}
	if Insignia("excelente").Valida() {
		t.Fatal("una insignia desconocida no debe ser válida")
	}
}
