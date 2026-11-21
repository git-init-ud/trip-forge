# Trabajar en Trip Forge

Consulta el enunciado en `X1.cluster-subete.md` para criterios de aceptación y `PROPUESTA-GRUPO-A.md` para decisiones y acuerdos pendientes. Los contratos en `api/` describen las operaciones entregadas. Mantén contrato, handler y pruebas en el mismo cambio.

## Límites de las APIs

- El CRUD es el único dueño de MongoDB. El MID consulta rutas por HTTP y consume reputación por el gateway. Mantén el cálculo de distancias y ranking independiente de HTTP y Mongo.
- Representa GeoJSON en orden longitud, latitud y las fechas de salida en UTC. Conserva metros como unidad en el dominio. Valida rangos, sentido del trayecto y límites de cupos en la entrada.
- Cambia disponibilidad y estado en una sola escritura condicional. Cubre la competencia por el último cupo con Mongo real. Coordina con Reservas la idempotencia antes de reintentar escrituras.
- En operaciones de negocio, verifica identidad, propietario, scopes y cabecera del gateway según el contrato acordado. Mantén las claves en el entorno. Registra identificadores de solicitud y errores sin volcar tokens ni datos de pasajeros.

## Cambios en Go

Mantén `cmd/` pequeño. Pasa el contexto de la solicitud a Mongo y a cada llamada HTTP, configura plazos y cierra los cuerpos de respuesta. Distingue errores de dominio, indisponibilidad externa y errores internos al construir la respuesta. Usa dobles HTTP para reputación y el proveedor vial en pruebas deterministas.

Ejecuta `go fmt ./...`, `go vet ./...`, `go test ./...` y `go build ./...` desde la raíz. En CI Linux se ejecuta también `go test -race ./...`. Revisa el diff antes de entregar y especifica qué resultados observaste y qué depende de otro grupo.

## Trabajo entre agentes

El orquestador asigna una historia, archivos y criterio verificable a cada agente. Reserva un único editor para cada archivo compartido, incluidos go.mod, go.sum y contratos. Integra los cambios sobre el estado actual del repositorio conservando el trabajo del equipo. Usa la skill de cupos o de matching cuando corresponda.
