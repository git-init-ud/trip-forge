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

// Obtener devuelve la ruta o rutas.ErrNoEncontrada.
func (r *Repositorio) Obtener(ctx context.Context, id string) (rutas.Ruta, error) {
	ctx, cancelar := context.WithTimeout(ctx, plazoOperacion)
	defer cancelar()
	return r.obtener(ctx, id)
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
