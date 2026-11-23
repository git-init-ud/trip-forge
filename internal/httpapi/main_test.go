package httpapi

import (
	"os"
	"testing"
)

// TestMain aísla las pruebas de la configuración del desarrollador. El
// middleware del gateway se prueba explícitamente con t.Setenv; una variable
// GATEWAY_SECRET exportada no debe romper el resto de la suite.
func TestMain(m *testing.M) {
	_ = os.Unsetenv("GATEWAY_SECRET")
	os.Exit(m.Run())
}
