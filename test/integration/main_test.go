package integration

import (
	"os"
	"testing"
)

// TestMain evita que un GATEWAY_SECRET exportado para la demo haga fallar las
// pruebas de integración. Los casos que necesitan el secreto lo activan con
// t.Setenv dentro de la prueba correspondiente.
func TestMain(m *testing.M) {
	_ = os.Unsetenv("GATEWAY_SECRET")
	os.Exit(m.Run())
}
