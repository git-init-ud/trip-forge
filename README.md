# Trip Forge

APIs de rutas y conductores del grupo A de Súbete. El proyecto usa Go, chi, MongoDB y JetBrains GoLand, y corre como dos procesos independientes:

- **rutas-crud** es el único dueño de MongoDB. Publica, lista, edita, cierra y ajusta cupos de rutas.
- **rutas-mid** busca rutas compatibles y arma el panel del conductor. Consulta el CRUD por HTTP y el puntaje de Pasajeros y reputación por el gateway; no toca MongoDB.

Las reglas de cupos se aplican en una sola escritura condicional y la geometría/ranking del MID se prueba sin HTTP ni base de datos. Los contratos están en [`api/`](api/) y los acuerdos pendientes con el clúster en la [propuesta](PROPUESTA-GRUPO-A.md).

## Arranque local

Necesitas Go 1.27.x, Docker y, si usas el IDE, GoLand. Desde la raíz:

```bash
go mod download
docker compose up -d --wait
```

MongoDB escucha en `127.0.0.1:27017` y conserva los datos en un volumen local. Para cambiar el puerto, copia `.env.example` a `.env` y ajusta `MONGO_PORT` antes de iniciar Compose. `docker compose stop` lo detiene.

Abre la carpeta que contiene `go.mod` en GoLand y selecciona el SDK en *Settings → Go → GOROOT*. En el selector de ejecución están **rutas-crud**, **rutas-mid**, **seed**, **docs** y **Pruebas**. Desde una terminal:

| Servicio | Comando | Comprobación |
| --- | --- | --- |
| CRUD | `go run ./cmd/rutas-crud` | `curl http://127.0.0.1:8081/health` |
| MID | `go run ./cmd/rutas-mid` | `curl http://127.0.0.1:8082/health` |
| Docs | `go run ./cmd/docs` | `curl http://127.0.0.1:8090/health` |
| Semilla | `go run ./cmd/seed` | imprime las 15 rutas cargadas |

En PowerShell, `curl` se reemplaza por `Invoke-RestMethod`, por ejemplo `Invoke-RestMethod http://127.0.0.1:8081/health`. Cada proceso lee `HTTP_ADDR` para cambiar su dirección.

### Variables de entorno

Los procesos Go no cargan `.env` automáticamente: exporta las variables en la terminal o configúralas en la ejecución de GoLand. `.env` solo lo usa Compose para `MONGO_PORT`.

| Variable | Proceso | Defecto | Uso |
| --- | --- | --- | --- |
| `HTTP_ADDR` | ambos | `127.0.0.1:8081` / `:8082` | Dirección HTTP del proceso. |
| `MONGODB_URI` | CRUD, seed | `mongodb://127.0.0.1:27017` | Conexión a MongoDB. |
| `MONGODB_DB` | CRUD, seed | `trip_forge` | Base de datos del servicio. |
| `RUTAS_CRUD_URL` | MID | `http://127.0.0.1:8081` | CRUD que consulta el MID. |
| `REPUTACION_URL` | MID | `http://127.0.0.1:8083` | Servicio de Pasajeros y reputación o su doble. |
| `RADIO_M` | MID | `800` | Radio máximo de caminata al origen y al destino. |
| `RUTAS_TIMEOUT_MS` | MID | `3000` | Plazo de las llamadas al CRUD. |
| `REPUTACION_TIMEOUT_MS` | MID | `2000` | Plazo de las llamadas a reputación. |
| `REPUTACION_CONCURRENCIA` | MID | `4` | Consultas de reputación simultáneas por búsqueda. |
| `GATEWAY_SECRET` | ambos | vacío | Si se define, exige `X-Cluster-Secret` en toda petición distinta de `/health`. |
| `CORS_ORIGINS` | ambos | visor local en `8090` | Orígenes exactos autorizados; el comodín `*` no se acepta. |
| `MONGODB_TEST_URI` | pruebas | vacío | Habilita las pruebas de integración; si queda vacío se omiten. |

## Datos semilla

`go run ./cmd/seed` carga de forma idempotente 15 rutas con ids fijos `ruta-0001` a `ruta-0015`. No borra otras rutas. Las salidas son futuras y se calculan en la zona de Bogotá, pero se guardan en UTC. La semilla incluye rutas abiertas, una llena, una cerrada, un trazado en sentido inverso y una ruta que pasa cerca del destino pero lejos del origen del corredor de demostración.

Los conductores usan ids `usr-01` a `usr-12`; las rutas semilla y esos perfiles deben confirmarse con los grupos B y C antes de la demo. Los cupos ocupados de la semilla son datos de demostración: no representan reservas reales hasta que exista la integración con Reservas.

## API CRUD de rutas

`rutas-crud` publica el [contrato OpenAPI](api/rutas-crud.openapi.yaml):

| Método y ruta | Resultado |
| --- | --- |
| `GET /rutas` | Lista paginada con filtros `conductorId`, `estado`, `desde`, `hasta`, `limit` y `offset`; devuelve `{items, total, limit, offset}`. |
| `POST /rutas` | Crea una ruta `abierta` y responde 201 con `Location`. |
| `GET /rutas/{id}` | Detalle; 404 si no existe. |
| `PUT /rutas/{id}` | Edita los datos editables y conserva los cupos comprometidos; 403/404/409 según propiedad, existencia o conflicto. |
| `DELETE /rutas/{id}` | Cierre lógico; 204, o 409 `reservas_activas` si hay cupos comprometidos. |
| `PATCH /rutas/{id}/cupos` | Descuenta o devuelve cupos; 409 `sin_cupos`, `cupos_excedidos` o `ruta_cerrada`. |

Una publicación mínima:

```bash
curl -X POST http://127.0.0.1:8081/rutas \
  -H 'Content-Type: application/json' \
  -d '{
    "conductorId": "usr-12",
    "origen": {"nombre":"Chapinero","punto":{"type":"Point","coordinates":[-74.0645,4.6486]}},
    "destino": {"nombre":"Universidad","punto":{"type":"Point","coordinates":[-74.0415,4.7012]}},
    "trazado": {"type":"LineString","coordinates":[[-74.0645,4.6486],[-74.0600,4.6650],[-74.0415,4.7012]]},
    "salida": "2030-01-15T06:30:00Z",
    "cuposTotales": 3,
    "precioTotal": 12000
  }'
```

Los cupos se ajustan con `PATCH` y `{"delta": -1}`. El cambio de disponibilidad y estado ocurre en una sola escritura condicional: dos solicitudes simultáneas por el último cupo producen una respuesta 200 y otra 409. Al llegar a cero la ruta pasa a `llena`; al devolver un cupo vuelve a `abierta`.

### Identidad y gateway (provisional)

`PUT` y `DELETE` exigen la cabecera `X-Usuario-Id` y comprueban que coincida con el `conductorId`. `POST` la acepta de forma opcional mientras no exista el gateway: si llega, también debe coincidir. `GET /rutas`, `GET /rutas/{id}` y `PATCH /rutas/{id}/cupos` no usan identidad de usuario: el gateway debe aplicar `rutas:leer` y `rutas:escribir` a los consumidores entre servicios.

Si `GATEWAY_SECRET` está definida, ambos procesos rechazan con 401 cualquier petición distinta de `/health` que no traiga `X-Cluster-Secret` correcta. Los nombres de ambas cabeceras son provisionales y deben confirmarse con el líder del gateway.

## API MID de rutas

`rutas-mid` publica el [contrato OpenAPI](api/rutas-mid.openapi.yaml):

| Método y ruta | Resultado |
| --- | --- |
| `GET /rutas/compatibles` | Rutas `abiertas` con cupos, salida en la ventana y trazado a menos de `RADIO_M` del origen y del destino, en el sentido correcto. Devuelve distancias, puntaje, `puntajeConductor` e insignia, ordenados. |
| `GET /conductores/{id}/panel` | Puntaje e insignia del conductor y sus rutas con `ocupados = cuposTotales - cuposDisponibles`. Perfil inexistente: 404. Sin rutas: 200 con lista vacía. |

Ejemplo de búsqueda:

```bash
curl 'http://127.0.0.1:8082/rutas/compatibles?origenLat=4.6486&origenLng=-74.0645&destinoLat=4.7012&destinoLng=-74.0415&desde=2030-01-15T06:00:00Z&hasta=2030-01-15T09:00:00Z'
```

El orden usa `(distanciaOrigenM + distanciaDestinoM) / 100 - puntajeConductor * 2` ascendente y desempata por `id`. Se excluyen los conductores `en_observacion`; un 404 de reputación omite al conductor y un timeout o 5xx devuelve 503 `dependencia_no_disponible` en lugar de un ranking incompleto. Sin candidatos no se consulta reputación.

El panel y la búsqueda necesitan que el CRUD esté arriba y que `REPUTACION_URL` responda con `{ "puntaje": 0..5, "insignia": "nuevo|confiable|regular|en_observacion" }`. Para desarrollo puede usarse un doble HTTP; las pruebas de integración levantan uno.

## Documentación OpenAPI ejecutable

El visor sirve los dos contratos con Swagger UI y permite ejecutar sus operaciones desde el navegador. Arranca MongoDB y cada servicio en terminales separadas:

```bash
docker compose up -d --wait
go run ./cmd/rutas-crud
go run ./cmd/rutas-mid
go run ./cmd/docs
```

Abre `http://127.0.0.1:8090`, elige **CRUD** o **MID** y usa **Try it out → Execute**. Las operaciones del CRUD incluyen listado, publicación, detalle, edición, cierre y cupos. Las del MID incluyen búsqueda y panel; para que respondan, `REPUTACION_URL` debe apuntar a un servicio o doble disponible.

El navegador exige CORS porque el visor corre en el puerto 8090 y las APIs en 8081 y 8082. Ambos servicios autorizan por defecto `http://127.0.0.1:8090` y `http://localhost:8090`. Si abres el visor desde otro origen, define `CORS_ORIGINS` con la lista de orígenes exactos separados por comas; `*` se ignora con una advertencia. La variable se lee al arrancar cada proceso y la configuración de ejecución no la recarga.

El visor también reúne los documentos del repositorio con enlaces entre ellos. Los YAML quedan en `/api/` y los Markdown en `/md/`. Cambia la dirección del visor con `HTTP_ADDR` y la raíz con `DOCS_ROOT`; si cambias el puerto, agrega el nuevo origen a `CORS_ORIGINS`. Si cambias el `HTTP_ADDR` de una API, actualiza la URL en `servers` de su contrato. Swagger UI y el renderizador de Markdown se cargan desde CDN; sin internet, los archivos crudos siguen disponibles.

## Errores y seguridad

Las respuestas de error usan el formato común del clúster:

```json
{ "codigo": "sin_cupos", "mensaje": "La ruta no tiene cupos disponibles", "detalle": {} }
```

Cada respuesta incluye `X-Request-Id`; el MID lo propaga al CRUD y a reputación. Los errores internos no exponen causas, tokens ni datos de pasajeros. Los clientes HTTP del MID imponen plazos, cierran los cuerpos, no siguen redirecciones y no registran credenciales. La identidad y el secreto del gateway se mantienen en el entorno.

## Verificación

```bash
gofmt -l cmd internal test
go vet ./...
docker compose up -d --wait
MONGODB_TEST_URI=mongodb://127.0.0.1:27017 go test -race ./...
go build ./...
docker compose config --quiet
```

GitHub Actions levanta MongoDB 8, comprueba el formato de `cmd`, `internal` y `test`, ejecuta `go vet`, las pruebas con detector de carreras, la compilación y valida Compose. Las pruebas de integración usan bases temporales y verifican la competencia por el último cupo, la edición concurrente, el cierre con cupos comprometidos, la búsqueda con reputación simulada y el panel. En Windows, `-race` requiere CGO y un compilador C; el pipeline lo ejecuta en Linux.

## Acuerdos pendientes con el clúster

- **Gateway:** nombres definitivos de `X-Usuario-Id` y `X-Cluster-Secret`, emisión de tokens y scopes `rutas:leer`, `rutas:escribir` y `perfiles:leer`. Hoy la identidad y el secreto son provisionales y la comprobación del secreto está desactivada si `GATEWAY_SECRET` queda vacía.
- **Reservas:** clave de idempotencia para `PATCH /rutas/{id}/cupos`, responsable de reconciliar devoluciones fallidas y confirmación de que `cuposTotales - cuposDisponibles` representa exactamente los cupos comprometidos. El cierre bloquea cupos ya descontados, no reservas pendientes sin descuento.
- **Pasajeros y reputación:** confirmar que `puntaje` e `insignia` son obligatorios, que los campos adicionales son opcionales y que el 404 significa perfil inexistente. Falta la integración real con el gateway.
- **Semilla:** confirmar los ids `ruta-0001..ruta-0015` y `usr-01..usr-12` con B y C.
- **API de tercero:** la generación vial con openrouteservice y su operación interna siguen pendientes; el MID todavía no consume ese proveedor.

- [Requisitos del clúster](X1.cluster-subete.md) y [propuesta y secuencias](PROPUESTA-GRUPO-A.md).
- [Contrato CRUD](api/rutas-crud.openapi.yaml) y [contrato MID](api/rutas-mid.openapi.yaml).
- [Convenciones de desarrollo](AGENTS.md).

## Autores

- Jhojan Stiven Aragón Ramírez
- Yader Ibraldo Quiroga Torres
- Kevin Emmanuel Tovar Lizarazo
