package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	for _, service := range []string{"rutas-crud", "rutas-mid"} {
		t.Run(service, func(t *testing.T) {
			w := httptest.NewRecorder()
			router(service).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusOK || body["status"] != "ok" || body["service"] != service {
				t.Fatalf("unexpected health response: %d %v", w.Code, body)
			}
		})
	}
}
