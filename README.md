# Trip Forge

APIs de rutas y conductores del grupo A de Súbete. El proyecto usa Go, chi, MongoDB y JetBrains GoLand. Esta base de arranque sirve `GET /health` en dos procesos independientes. Las historias R-HU1 a R-HU6 y la persistencia de rutas están planificadas en la [propuesta](PROPUESTA-GRUPO-A.md).

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

Abre la carpeta que contiene `go.mod` en GoLand y selecciona el SDK instalado en *Settings → Go → GOROOT*. En el selector de ejecución inicia **rutas-crud** y **rutas-mid**. **Pruebas** ejecuta las pruebas del módulo. Desde una terminal por servicio puedes usar los mismos puntos de entrada:

| Servicio | Comando | Comprobación en PowerShell |
| --- | --- | --- |
| CRUD | `go run ./cmd/rutas-crud` | `Invoke-RestMethod http://127.0.0.1:8081/health` |
| MID | `go run ./cmd/rutas-mid` | `Invoke-RestMethod http://127.0.0.1:8082/health` |

Cada respuesta contiene `status: ok` y el nombre del proceso. Este chequeo mide disponibilidad HTTP. Para cambiar la dirección de un proceso, define `HTTP_ADDR` en su configuración de ejecución.

## Verificación y primer hito

```powershell
go vet ./...
go test ./...
go build ./...
docker compose config --quiet
```

GitHub Actions comprueba además el formato y ejecuta las pruebas con el detector de carreras en Linux. El primer hito funcional es publicar una ruta, consultarla y descontar un cupo. Antes de integrarlo, el equipo debe acordar con el gateway la identidad, los scopes y la cabecera de confianza, y con Reservas los IDs, los errores y el manejo de reintentos.

- [Requisitos del clúster](X1.cluster-subete.md) y [propuesta y secuencias](PROPUESTA-GRUPO-A.md).
- [Contrato CRUD](api/rutas-crud.openapi.yaml) y [contrato MID](api/rutas-mid.openapi.yaml).
- [Convenciones de desarrollo](AGENTS.md).

## Autores

- Jhojan Stiven Aragón Ramírez
- Yader Ibraldo Quiroga Torres
- Kevin Emmanuel Tovar Lizarazo
