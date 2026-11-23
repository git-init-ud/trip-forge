// Package mongorutas implementa la persistencia MongoDB del CRUD de rutas.
//
// El paquete es el único que habla con la base de datos del servicio. Cada
// operación recibe el contexto de la solicitud y le impone un plazo propio, de
// modo que una petición no queda esperando al driver indefinidamente.
package mongorutas

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/git-init-ud/trip-forge/internal/rutas"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// ColeccionRutas es la colección donde vive el documento de cada ruta.
	ColeccionRutas = "rutas"

	// plazoOperacion acota cada lectura o escritura del driver.
	plazoOperacion = 5 * time.Second
	// plazoArranque acota la conexión, el ping y la creación de índices.
	plazoArranque = 10 * time.Second
)

// Repositorio implementa el almacenamiento que necesita el CRUD de rutas.
type Repositorio struct {
	coleccion *mongo.Collection
}

// Nuevo envuelve la colección de rutas de la base indicada.
func Nuevo(cliente *mongo.Client, base string) *Repositorio {
	return &Repositorio{coleccion: cliente.Database(base).Collection(ColeccionRutas)}
}

// Abrir conecta al servidor indicado y comprueba que responde antes de
// devolver el repositorio. El llamador es responsable de cerrar el cliente.
func Abrir(ctx context.Context, uri, base string) (*mongo.Client, *Repositorio, error) {
	cliente, err := mongo.Connect(options.Client().
		ApplyURI(uri).
		SetConnectTimeout(plazoOperacion).
		SetServerSelectionTimeout(plazoOperacion))
	if err != nil {
		return nil, nil, fmt.Errorf("conectar a mongodb: %w", err)
	}
	pingCtx, cancelar := context.WithTimeout(ctx, plazoArranque)
	defer cancelar()
	if err := cliente.Ping(pingCtx, nil); err != nil {
		_ = cliente.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("verificar mongodb: %w", err)
	}
	return cliente, Nuevo(cliente, base), nil
}

// AsegurarIndices deja listo el índice geoespacial del trazado y los índices de
// las consultas por fecha de salida y por conductor.
func (r *Repositorio) AsegurarIndices(ctx context.Context) error {
	ctx, cancelar := context.WithTimeout(ctx, plazoArranque)
	defer cancelar()
	_, err := r.coleccion.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "trazado", Value: "2dsphere"}},
			Options: options.Index().SetName("trazado_2dsphere"),
		},
		{
			Keys:    bson.D{{Key: "salida", Value: 1}},
			Options: options.Index().SetName("salida_1"),
		},
		{
			Keys:    bson.D{{Key: "conductorId", Value: 1}},
			Options: options.Index().SetName("conductorId_1"),
		},
	})
	if err != nil {
		return fmt.Errorf("crear los índices de rutas: %w", err)
	}
	return nil
}

// Crear inserta una ruta ya validada. El identificador viaja en "_id".
func (r *Repositorio) Crear(ctx context.Context, ruta rutas.Ruta) error {
	ctx, cancelar := context.WithTimeout(ctx, plazoOperacion)
	defer cancelar()
	if _, err := r.coleccion.InsertOne(ctx, ruta); err != nil {
		return fmt.Errorf("guardar la ruta %s: %w", ruta.ID, err)
	}
	return nil
}

// Reemplazar guarda la ruta por su identificador y crea el documento si no
// existe. La semilla de demostración lo usa para poder repetirse sin duplicar
// rutas ni borrar las que ya estén en la base.
func (r *Repositorio) Reemplazar(ctx context.Context, ruta rutas.Ruta) error {
	ctx, cancelar := context.WithTimeout(ctx, plazoOperacion)
	defer cancelar()
	_, err := r.coleccion.ReplaceOne(ctx,
		bson.D{{Key: "_id", Value: ruta.ID}},
		ruta,
		options.Replace().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("reemplazar la ruta %s: %w", ruta.ID, err)
	}
	return nil
}

// Listar devuelve una página de rutas que cumplen el filtro, ordenadas por
// salida ascendente y con el identificador como desempate. El total se cuenta
// sobre el mismo filtro con CountDocuments, sin traer los documentos a memoria.
func (r *Repositorio) Listar(ctx context.Context, filtro rutas.FiltroListado) (rutas.Listado, error) {
	if err := filtro.Validar(); err != nil {
		return rutas.Listado{}, err
	}
	filtro = filtro.Normalizar()
	ctx, cancelar := context.WithTimeout(ctx, plazoOperacion)
	defer cancelar()

	condicion := filtroDeListado(filtro)
	total, err := r.coleccion.CountDocuments(ctx, condicion)
	if err != nil {
		return rutas.Listado{}, fmt.Errorf("contar las rutas: %w", err)
	}

	opciones := options.Find().
		SetSort(bson.D{{Key: "salida", Value: 1}, {Key: "_id", Value: 1}}).
		SetSkip(int64(filtro.Offset)).
		SetLimit(int64(filtro.Limit))
	cursor, err := r.coleccion.Find(ctx, condicion, opciones)
	if err != nil {
		return rutas.Listado{}, fmt.Errorf("listar las rutas: %w", err)
	}
	items := make([]rutas.Ruta, 0, filtro.Limit)
	if err := cursor.All(ctx, &items); err != nil {
		return rutas.Listado{}, fmt.Errorf("leer las rutas: %w", err)
	}
	return rutas.Listado{Items: items, Total: total}, nil
}

// Obtener devuelve la ruta o rutas.ErrNoEncontrada.
func (r *Repositorio) Obtener(ctx context.Context, id string) (rutas.Ruta, error) {
	ctx, cancelar := context.WithTimeout(ctx, plazoOperacion)
	defer cancelar()
	return r.obtener(ctx, id)
}

// Actualizar reemplaza los campos editables en una sola escritura condicional.
// El filtro exige el identificador, la identidad propietaria, que la ruta no
// esté cerrada y que el nuevo total no quede por debajo de los cupos
// comprometidos. El pipeline recalcula los disponibles conservando esos cupos y
// deriva el estado. Si no hay coincidencia se relee el documento para
// clasificar el fallo entre 404, 403 y 409 sin una lectura previa que abra una
// carrera con PATCH /cupos.
func (r *Repositorio) Actualizar(ctx context.Context, id string, cambios rutas.ActualizarRuta, identidad string) (rutas.Ruta, error) {
	identidad = strings.TrimSpace(identidad)
	ctx, cancelar := context.WithTimeout(ctx, plazoOperacion)
	defer cancelar()

	filtro, actualizacion := actualizacionDeRuta(id, identidad, cambios)
	opciones := options.FindOneAndUpdate().SetReturnDocument(options.After)
	for intento := 0; intento < 3; intento++ {
		var actualizada rutas.Ruta
		err := r.coleccion.FindOneAndUpdate(ctx, filtro, actualizacion, opciones).Decode(&actualizada)
		if err == nil {
			return actualizada, nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return rutas.Ruta{}, fmt.Errorf("actualizar la ruta %s: %w", id, err)
		}
		actual, errLectura := r.obtener(ctx, id)
		if errLectura != nil {
			return rutas.Ruta{}, errLectura
		}
		if conflicto := conflictoDeActualizacion(actual, cambios, identidad); conflicto != nil {
			return rutas.Ruta{}, conflicto
		}
		// Otra escritura cambió el documento entre el intento y la lectura y
		// las condiciones vuelven a cumplirse: reintentar con el mismo filtro
		// es seguro porque la escritura sigue siendo condicional.
	}

	actual, errLectura := r.obtener(ctx, id)
	if errLectura != nil {
		return rutas.Ruta{}, errLectura
	}
	if conflicto := conflictoDeActualizacion(actual, cambios, identidad); conflicto != nil {
		return rutas.Ruta{}, conflicto
	}
	return rutas.Ruta{}, fmt.Errorf("actualizar la ruta %s: la condición no se pudo aplicar", id)
}

// Cerrar marca la ruta como cerrada sin borrar el documento. La escritura exige
// propiedad y cero cupos comprometidos; el cierre de una ruta ya cerrada es
// idempotente. Si la condición no coincide, se relee para clasificar el fallo.
func (r *Repositorio) Cerrar(ctx context.Context, id, identidad string) error {
	identidad = strings.TrimSpace(identidad)
	ctx, cancelar := context.WithTimeout(ctx, plazoOperacion)
	defer cancelar()

	filtro, actualizacion := cierreDeRuta(id, identidad)
	for intento := 0; intento < 3; intento++ {
		resultado, err := r.coleccion.UpdateOne(ctx, filtro, actualizacion)
		if err != nil {
			return fmt.Errorf("cerrar la ruta %s: %w", id, err)
		}
		if resultado.MatchedCount > 0 {
			return nil
		}
		actual, errLectura := r.obtener(ctx, id)
		if errLectura != nil {
			return errLectura
		}
		if conflicto := conflictoDeCierre(actual, identidad); conflicto != nil {
			return conflicto
		}
		if actual.Estado == rutas.Cerrada {
			return nil
		}
		// Otra escritura cambió el documento entre el intento y la lectura:
		// reintentar mantiene la condición de cero comprometidos.
	}

	actual, errLectura := r.obtener(ctx, id)
	if errLectura != nil {
		return errLectura
	}
	if conflicto := conflictoDeCierre(actual, identidad); conflicto != nil {
		return conflicto
	}
	if actual.Estado == rutas.Cerrada {
		return nil
	}
	return fmt.Errorf("cerrar la ruta %s: la condición no se pudo aplicar", id)
}

// AjustarCupos cambia cuposDisponibles y estado en una sola escritura
// condicional. Si la condición no se cumple, vuelve a leer el documento para
// distinguir entre ruta inexistente, falta de cupos, exceso sobre el total y
// ruta cerrada.
func (r *Repositorio) AjustarCupos(ctx context.Context, id string, delta int) (rutas.Ruta, error) {
	if err := rutas.ValidarDelta(delta); err != nil {
		return rutas.Ruta{}, err
	}
	ctx, cancelar := context.WithTimeout(ctx, plazoOperacion)
	defer cancelar()

	filtro, actualizacion := ajuste(id, delta)
	opciones := options.FindOneAndUpdate().SetReturnDocument(options.After)
	for intento := 0; intento < 3; intento++ {
		var actualizada rutas.Ruta
		err := r.coleccion.FindOneAndUpdate(ctx, filtro, actualizacion, opciones).Decode(&actualizada)
		if err == nil {
			return actualizada, nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return rutas.Ruta{}, fmt.Errorf("ajustar los cupos de la ruta %s: %w", id, err)
		}
		actual, errLectura := r.obtener(ctx, id)
		if errLectura != nil {
			return rutas.Ruta{}, errLectura
		}
		conflicto := conflictoDeAjuste(actual, delta)
		if conflicto != nil {
			return rutas.Ruta{}, conflicto
		}
		// La lectura muestra que la condición ya se cumple: otra operación pudo
		// cambiar el documento entre la escritura y la lectura. Reintentar evita
		// reportar un conflicto que ya no existe, pero el número de intentos es
		// acotado para no quedarse esperando a un documento muy disputado.
	}

	actual, errLectura := r.obtener(ctx, id)
	if errLectura != nil {
		return rutas.Ruta{}, errLectura
	}
	if conflicto := conflictoDeAjuste(actual, delta); conflicto != nil {
		return rutas.Ruta{}, conflicto
	}
	// Se agotaron los reintentos con la condición aún satisfecha. Se responde
	// como conflicto de inventario, nunca como error interno: el cliente puede
	// reintentar y el documento no quedó en un estado inconsistente.
	if delta < 0 {
		return rutas.Ruta{}, rutas.ErrSinCupos
	}
	return rutas.Ruta{}, rutas.ErrCuposExcedidos
}

func (r *Repositorio) obtener(ctx context.Context, id string) (rutas.Ruta, error) {
	var ruta rutas.Ruta
	err := r.coleccion.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&ruta)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return rutas.Ruta{}, rutas.ErrNoEncontrada
	}
	if err != nil {
		return rutas.Ruta{}, fmt.Errorf("consultar la ruta %s: %w", id, err)
	}
	return ruta, nil
}

// filtroDeListado traduce el filtro validado a la condición de Mongo. La
// ventana se expresa como un único rango inclusivo sobre "salida".
func filtroDeListado(filtro rutas.FiltroListado) bson.D {
	condicion := bson.D{}
	if filtro.ConductorID != "" {
		condicion = append(condicion, bson.E{Key: "conductorId", Value: filtro.ConductorID})
	}
	if filtro.Estado != "" {
		condicion = append(condicion, bson.E{Key: "estado", Value: string(filtro.Estado)})
	}
	ventana := bson.D{}
	if !filtro.Desde.IsZero() {
		ventana = append(ventana, bson.E{Key: "$gte", Value: filtro.Desde})
	}
	if !filtro.Hasta.IsZero() {
		ventana = append(ventana, bson.E{Key: "$lte", Value: filtro.Hasta})
	}
	if len(ventana) > 0 {
		condicion = append(condicion, bson.E{Key: "salida", Value: ventana})
	}
	return condicion
}

// actualizacionDeRuta construye la escritura condicional de la edición. Los
// cupos comprometidos se calculan con el documento almacenado, no con una
// lectura del cliente, y la disponibilidad resultante se escribe junto al
// estado en la misma operación.
func actualizacionDeRuta(id, identidad string, cambios rutas.ActualizarRuta) (bson.D, mongo.Pipeline) {
	comprometidos := bson.D{{Key: "$subtract", Value: bson.A{"$cuposTotales", "$cuposDisponibles"}}}
	disponibles := bson.D{{Key: "$subtract", Value: bson.A{cambios.CuposTotales, comprometidos}}}

	filtro := bson.D{
		{Key: "_id", Value: id},
		{Key: "conductorId", Value: identidad},
		{Key: "estado", Value: bson.D{{Key: "$ne", Value: string(rutas.Cerrada)}}},
		{Key: "$expr", Value: bson.D{{Key: "$gte", Value: bson.A{cambios.CuposTotales, comprometidos}}}},
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "origen", Value: cambios.Origen},
			{Key: "destino", Value: cambios.Destino},
			{Key: "trazado", Value: cambios.Trazado},
			{Key: "salida", Value: cambios.Salida.UTC()},
			{Key: "cuposTotales", Value: cambios.CuposTotales},
			{Key: "cuposDisponibles", Value: disponibles},
			{Key: "precioTotal", Value: cambios.PrecioTotal},
			{Key: "estado", Value: estadoSegun(disponibles)},
		}}},
	}
	return filtro, pipeline
}

// cierreDeRuta exige propiedad y cero cupos comprometidos en la misma escritura
// condicional.
func cierreDeRuta(id, identidad string) (bson.D, bson.D) {
	filtro := bson.D{
		{Key: "_id", Value: id},
		{Key: "conductorId", Value: identidad},
		{Key: "estado", Value: bson.D{{Key: "$ne", Value: string(rutas.Cerrada)}}},
		{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$cuposDisponibles", "$cuposTotales"}}}},
	}
	actualizacion := bson.D{{Key: "$set", Value: bson.D{{Key: "estado", Value: string(rutas.Cerrada)}}}}
	return filtro, actualizacion
}

// conflictoDeActualizacion clasifica una escritura de edición que no encontró
// coincidencia. Devuelve nil cuando las condiciones ya se cumplen, señal de que
// otra escritura cambió el documento y conviene reintentar.
func conflictoDeActualizacion(actual rutas.Ruta, cambios rutas.ActualizarRuta, identidad string) error {
	if identidad == "" || actual.ConductorID != identidad {
		return rutas.ErrNoAutorizado
	}
	if actual.Estado == rutas.Cerrada {
		return rutas.ErrRutaCerrada
	}
	if cambios.CuposTotales < actual.Ocupados() {
		return rutas.ErrReservasActivas
	}
	return nil
}

// conflictoDeCierre clasifica una escritura de cierre que no encontró
// coincidencia, comprobando primero la propiedad para no revelar el estado de
// una ruta ajena.
func conflictoDeCierre(actual rutas.Ruta, identidad string) error {
	if identidad == "" || actual.ConductorID != identidad {
		return rutas.ErrNoAutorizado
	}
	return rutas.ValidarCierre(actual)
}

// ajuste construye el filtro condicional y el pipeline que actualiza cupos y
// estado juntos. Para descuentos exige ruta abierta y disponibilidad
// suficiente. Para devoluciones acepta rutas abiertas o llenas y comprueba en
// el propio servidor que el resultado no supere cuposTotales.
func ajuste(id string, delta int) (bson.D, mongo.Pipeline) {
	if delta < 0 {
		requeridos := -delta
		nuevosCupos := bson.D{{Key: "$subtract", Value: bson.A{"$cuposDisponibles", requeridos}}}
		filtro := bson.D{
			{Key: "_id", Value: id},
			{Key: "estado", Value: string(rutas.Abierta)},
			{Key: "cuposDisponibles", Value: bson.D{{Key: "$gte", Value: requeridos}}},
		}
		return filtro, mongo.Pipeline{
			bson.D{{Key: "$set", Value: bson.D{
				{Key: "cuposDisponibles", Value: nuevosCupos},
				{Key: "estado", Value: estadoSegun(nuevosCupos)},
			}}},
		}
	}

	nuevosCupos := bson.D{{Key: "$add", Value: bson.A{"$cuposDisponibles", delta}}}
	filtro := bson.D{
		{Key: "_id", Value: id},
		{Key: "estado", Value: bson.D{{Key: "$in", Value: bson.A{string(rutas.Abierta), string(rutas.Llena)}}}},
		{Key: "$expr", Value: bson.D{{Key: "$lte", Value: bson.A{nuevosCupos, "$cuposTotales"}}}},
	}
	return filtro, mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "cuposDisponibles", Value: nuevosCupos},
			{Key: "estado", Value: estadoSegun(nuevosCupos)},
		}}},
	}
}

func estadoSegun(cupos any) bson.D {
	return bson.D{{Key: "$cond", Value: bson.A{
		bson.D{{Key: "$eq", Value: bson.A{cupos, 0}}},
		string(rutas.Llena),
		string(rutas.Abierta),
	}}}
}

func conflictoDeAjuste(actual rutas.Ruta, delta int) error {
	if delta < 0 {
		if actual.Estado != rutas.Abierta || actual.CuposDisponibles < -delta {
			return rutas.ErrSinCupos
		}
		return nil
	}
	if actual.Estado == rutas.Cerrada {
		return rutas.ErrRutaCerrada
	}
	if actual.CuposDisponibles+delta > actual.CuposTotales {
		return rutas.ErrCuposExcedidos
	}
	return nil
}
