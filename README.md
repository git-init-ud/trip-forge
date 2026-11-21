# Trip Forge

APIs de rutas y conductores del grupo A de Súbete. El proyecto usa Go, chi, MongoDB y JetBrains GoLand. Corre como dos procesos independientes. El primer hito publica, consulta y ajusta cupos de rutas en MongoDB; las historias restantes están planificadas en la [propuesta](PROPUESTA-GRUPO-A.md).

## Arranque local en Windows

Necesitas el SDK Go 1.27.x, GoLand y Docker Desktop con contenedores Linux. Para instalar herramientas en un equipo nuevo puedes usar `winget install --id GoLang.Go -e`, `winget install --id JetBrains.GoLand -e` y `winget install --id Docker.DockerDesktop -e`. Abre una terminal nueva tras la instalación e inicia Docker Desktop.

Desde la raíz del repositorio:

```powershell
go version
go mod download
docker compose up -d --wait
docker compose exec -T mongo mongosh --quiet --eval "db.adminCommand({ping:1})"
```

MongoDB escucha en `127.0.0.1:27017` y conserva los datos en un volumen local. Para cambiar el puerto, copia `.env.example` a `.env` y ajusta `MONGO_PORT` antes de iniciar Compose. Usa `docker compose stop` para detenerlo.

Abre la carpeta que contiene `go.mod` en GoLand y selecciona el SDK instalado en *Settings → Go → GOROOT*. En el selector de ejecución inicia **rutas-crud**, **rutas-mid** y **docs**. **Pruebas** ejecuta las pruebas del módulo. Desde una terminal por servicio puedes usar los mismos puntos de entrada:

| Servicio | Comando | Comprobación en PowerShell |
| --- | --- | --- |
| CRUD | `go run ./cmd/rutas-crud` | `Invoke-RestMethod http://127.0.0.1:8081/health` |
| MID | `go run ./cmd/rutas-mid` | `Invoke-RestMethod http://127.0.0.1:8082/health` |
| Docs | `go run ./cmd/docs` | `Invoke-RestMethod http://127.0.0.1:8090/health` |

Cada respuesta contiene `status: ok` y el nombre del proceso. Este chequeo mide disponibilidad HTTP. Para cambiar la dirección de un proceso, define `HTTP_ADDR` en su configuración de ejecución.

## API CRUD de rutas (primer hito)

`rutas-crud` es el único dueño de MongoDB. Expone estas operaciones del [contrato](api/rutas-crud.openapi.yaml):

| Método y ruta | Resultado |
| --- | --- |
| `POST /rutas` | Crea una ruta `abierta`; responde 201 con `Location`. |
| `GET /rutas/{id}` | Devuelve el detalle; 404 si no existe. |
| `PATCH /rutas/{id}/cupos` | Descuenta o devuelve cupos; 409 `sin_cupos`, `cupos_excedidos` o `ruta_cerrada` según la condición. |

Una publicación mínima se ve así:

```powershell
$cuerpo = @{
  conductorId = 'usr-12'
  origen = @{ nombre = 'Chapinero'; punto = @{ type = 'Point'; coordinates = @(-74.0645, 4.6486) } }
  destino = @{ nombre = 'Universidad'; punto = @{ type = 'Point'; coordinates = @(-74.0415, 4.7012) } }
  trazado = @{ type = 'LineString'; coordinates = @(@(-74.0645, 4.6486), @(-74.0415, 4.7012)) }
  salida = (Get-Date).AddDays(2).ToUniversalTime().ToString('o')
  cuposTotales = 3
  precioTotal = 12000
} | ConvertTo-Json -Depth 6
Invoke-RestMethod -Method Post http://127.0.0.1:8081/rutas -ContentType 'application/json' -Body $cuerpo
```

Los cupos se ajustan con `Invoke-RestMethod -Method Patch http://127.0.0.1:8081/rutas/<id>/cupos -ContentType 'application/json' -Body '{"delta":-1}'`. El cambio de disponibilidad y estado ocurre en una sola escritura condicional: dos solicitudes simultáneas por el último cupo producen una respuesta 200 y otra 409. Los errores usan `{ "codigo", "mensaje", "detalle" }`.

Variables leídas por el CRUD: `HTTP_ADDR`, `MONGODB_URI` y `MONGODB_DB`. `MONGODB_TEST_URI` habilita las pruebas de integración; si queda vacío, se omiten. Copia `.env.example` como referencia.

## Documentación en el navegador

```powershell
go run ./cmd/docs
```

Abre `http://127.0.0.1:8090`. El visor reúne los dos contratos OpenAPI con Swagger UI y los documentos del repositorio con enlaces entre ellos. Los YAML también quedan como archivos en `/api/` y los Markdown en `/md/`. Cambia la dirección con `HTTP_ADDR` y la raíz con `DOCS_ROOT`. Swagger UI y el renderizador de Markdown se cargan desde CDN; si no hay internet, los archivos crudos siguen disponibles.

## Verificación

```powershell
go vet ./...
docker compose up -d --wait
$env:MONGODB_TEST_URI = 'mongodb://127.0.0.1:27017'
go test -race ./...
go build ./...
docker compose config --quiet
```

GitHub Actions levanta un servicio MongoDB 8, comprueba el formato de `cmd`, `internal` y `test`, ejecuta `go vet`, las pruebas con detector de carreras, la compilación y valida Compose. Las pruebas de integración usan una base de datos temporal y verifican la competencia por el último cupo contra Mongo real. En Windows, `-race` requiere CGO y un compilador C; el pipeline lo ejecuta en Linux.

Antes de integrar el hito con el clúster, el equipo debe acordar con el gateway la identidad, los scopes y la cabecera de confianza, y con Reservas los IDs, los errores y el manejo de reintentos. La autorización de propietario y las 15 rutas semilla de R-HU1 quedan para la siguiente entrega.

- [Requisitos del clúster](X1.cluster-subete.md) y [propuesta y secuencias](PROPUESTA-GRUPO-A.md).
- [Contrato CRUD](api/rutas-crud.openapi.yaml) y [contrato MID](api/rutas-mid.openapi.yaml).
- [Convenciones de desarrollo](AGENTS.md).

## Autores

- Jhojan Stiven Aragón Ramírez
- Yader Ibraldo Quiroga Torres
- Kevin Emmanuel Tovar Lizarazo
