package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
)

// ErrSinRutaVial indica que el proveedor no encontró un trayecto entre puntos.
var ErrSinRutaVial = errors.New("no existe un trayecto vial entre los puntos")

// GeneradorVial produce un trazado GeoJSON sin conocer HTTP ni MongoDB.
type GeneradorVial interface {
	Generar(ctx context.Context, origen, destino rutas.Punto) (rutas.LineString, error)
}

type entradaCache struct {
	trazado     rutas.LineString
	vencimiento time.Time
}

// ClienteORS consulta directions/geojson de openrouteservice. Guarda trazados
// válidos por coordenadas y perfil durante un TTL configurable.
type ClienteORS struct {
	url    string
	clave  string
	perfil string
	plazo  time.Duration
	ttl    time.Duration
	http   *http.Client
	mu     sync.Mutex
	cache  map[[4]float64]entradaCache
}

func NuevoClienteORS(baseURL, clave, perfil string, plazo, ttl time.Duration) (*ClienteORS, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || base == nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("openrouteservice: URL base inválida")
	}
	if perfil != "driving-car" && perfil != "driving-hgv" {
		return nil, errors.New("openrouteservice: perfil vial inválido")
	}
	if plazo <= 0 || ttl <= 0 {
		return nil, errors.New("openrouteservice: plazo y TTL deben ser positivos")
	}
	return &ClienteORS{
		url:   strings.TrimRight(base.String(), "/") + "/v2/directions/" + perfil + "/geojson",
		clave: strings.TrimSpace(clave), perfil: perfil, plazo: plazo, ttl: ttl,
		http:  &http.Client{Timeout: plazo, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		cache: make(map[[4]float64]entradaCache),
	}, nil
}

func (c *ClienteORS) Generar(ctx context.Context, origen, destino rutas.Punto) (rutas.LineString, error) {
	if err := rutas.ValidarPunto("origen", origen); err != nil {
		return rutas.LineString{}, err
	}
	if err := rutas.ValidarPunto("destino", destino); err != nil {
		return rutas.LineString{}, err
	}
	clave := [4]float64{origen.Coordinates[0], origen.Coordinates[1], destino.Coordinates[0], destino.Coordinates[1]}
	c.mu.Lock()
	if entrada, ok := c.cache[clave]; ok && time.Now().Before(entrada.vencimiento) {
		c.mu.Unlock()
		return copiarTrazado(entrada.trazado), nil
	}
	c.mu.Unlock()
	if c.clave == "" {
		return rutas.LineString{}, fmt.Errorf("%w: falta ORS_API_KEY", ErrNoDisponible)
	}
	ctx, cancelar := context.WithTimeout(ctx, c.plazo)
	defer cancelar()
	cuerpo, _ := json.Marshal(map[string]any{"coordinates": [][]float64{origen.Coordinates, destino.Coordinates}})
	for intento := 0; intento < 2; intento++ {
		trazado, transitorio, err := c.consultar(ctx, cuerpo)
		if err == nil {
			c.mu.Lock()
			if len(c.cache) >= 1024 {
				// La caché es pequeña y temporal; se vacía para acotar memoria.
				c.cache = make(map[[4]float64]entradaCache)
			}
			c.cache[clave] = entradaCache{trazado: copiarTrazado(trazado), vencimiento: time.Now().Add(c.ttl)}
			c.mu.Unlock()
			return trazado, nil
		}
		if !transitorio || intento == 1 {
			return rutas.LineString{}, err
		}
		select {
		case <-ctx.Done():
			return rutas.LineString{}, fmt.Errorf("%w: plazo agotado", ErrNoDisponible)
		case <-time.After(100 * time.Millisecond):
		}
	}
	return rutas.LineString{}, ErrNoDisponible
}

func (c *ClienteORS) consultar(ctx context.Context, cuerpo []byte) (rutas.LineString, bool, error) {
	peticion, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(cuerpo))
	if err != nil {
		return rutas.LineString{}, false, fmt.Errorf("%w: petición vial inválida", ErrNoDisponible)
	}
	peticion.Header.Set("Authorization", c.clave)
	peticion.Header.Set("Content-Type", "application/json")
	peticion.Header.Set("Accept", "application/geo+json")
	respuesta, err := c.http.Do(peticion)
	if err != nil {
		return rutas.LineString{}, ctx.Err() == nil, fmt.Errorf("%w: proveedor vial sin respuesta", ErrNoDisponible)
	}
	defer respuesta.Body.Close()
	if respuesta.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(respuesta.Body, 4096))
		switch respuesta.StatusCode {
		case 404, 422:
			return rutas.LineString{}, false, ErrSinRutaVial
		case 429, 500, 502, 503, 504:
			return rutas.LineString{}, true, fmt.Errorf("%w: proveedor vial respondió %d", ErrNoDisponible, respuesta.StatusCode)
		default:
			return rutas.LineString{}, false, fmt.Errorf("%w: proveedor vial respondió %d", ErrNoDisponible, respuesta.StatusCode)
		}
	}
	datosCrudos, err := io.ReadAll(io.LimitReader(respuesta.Body, maxRespuesta+1))
	if err != nil || len(datosCrudos) > maxRespuesta {
		return rutas.LineString{}, false, fmt.Errorf("%w: respuesta vial demasiado grande o ilegible", ErrNoDisponible)
	}
	var datos struct {
		Features []struct {
			Geometry rutas.LineString `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(datosCrudos, &datos); err != nil || len(datos.Features) == 0 {
		return rutas.LineString{}, false, fmt.Errorf("%w: respuesta vial inválida", ErrNoDisponible)
	}
	trazado := datos.Features[0].Geometry
	if err := rutas.ValidarTrazado(trazado); err != nil {
		return rutas.LineString{}, false, fmt.Errorf("%w: trazado vial inválido", ErrNoDisponible)
	}
	return trazado, false, nil
}

func copiarTrazado(original rutas.LineString) rutas.LineString {
	copia := rutas.LineString{Type: original.Type, Coordinates: make([][]float64, len(original.Coordinates))}
	for i, punto := range original.Coordinates {
		copia.Coordinates[i] = append([]float64(nil), punto...)
	}
	return copia
}

// ClienteGeneradorMID llama al MID desde el CRUD para solicitar un trazado.
type ClienteGeneradorMID struct {
	url, secreto string
	http         *http.Client
}

func NuevoClienteGeneradorMID(baseURL, secreto string, plazo time.Duration) (*ClienteGeneradorMID, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || base == nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || plazo <= 0 {
		return nil, errors.New("mid vial: URL o plazo inválido")
	}
	return &ClienteGeneradorMID{
		url:     strings.TrimRight(base.String(), "/") + "/interno/trazados",
		secreto: strings.TrimSpace(secreto),
		http:    &http.Client{Timeout: plazo, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (c *ClienteGeneradorMID) Generar(ctx context.Context, origen, destino rutas.Punto) (rutas.LineString, error) {
	cuerpo, _ := json.Marshal(map[string]any{"origen": origen, "destino": destino})
	peticion, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(cuerpo))
	if err != nil {
		return rutas.LineString{}, fmt.Errorf("%w: petición interna inválida", ErrNoDisponible)
	}
	peticion.Header.Set("Content-Type", "application/json")
	if c.secreto != "" {
		peticion.Header.Set(cabeceraSecreto, c.secreto)
	}
	if id := RequestIDDe(ctx); id != "" {
		peticion.Header.Set(cabeceraRequestID, id)
	}
	respuesta, err := c.http.Do(peticion)
	if err != nil {
		return rutas.LineString{}, fmt.Errorf("%w: MID vial sin respuesta", ErrNoDisponible)
	}
	defer respuesta.Body.Close()
	if respuesta.StatusCode == 422 {
		return rutas.LineString{}, ErrSinRutaVial
	}
	if respuesta.StatusCode != 200 {
		return rutas.LineString{}, fmt.Errorf("%w: MID vial respondió %d", ErrNoDisponible, respuesta.StatusCode)
	}
	var trazado rutas.LineString
	if err := json.NewDecoder(io.LimitReader(respuesta.Body, maxRespuesta+1)).Decode(&trazado); err != nil || rutas.ValidarTrazado(trazado) != nil {
		return rutas.LineString{}, fmt.Errorf("%w: MID devolvió un trazado inválido", ErrNoDisponible)
	}
	return trazado, nil
}
