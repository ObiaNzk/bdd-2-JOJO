package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

type MongoRepository struct {
	coll *mongo.Collection
}

func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{coll: db.Collection("event_results")}
}

// EnsureIndexes creates the indexes backing the result and record queries.
// Safe to call on every startup.
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "disciplineId", Value: 1}}},
		{Keys: bson.D{{Key: "eventId", Value: 1}}},
		{Keys: bson.D{{Key: "records.athleteId", Value: 1}}},
	})
	return err
}

// RegisterEventResult stores the full, type-specific result of one event.
func (r *MongoRepository) RegisterEventResult(ctx context.Context, res *model.EventResult) error {
	if res.ID == "" {
		res.ID = bson.NewObjectID().Hex()
	}
	if res.Date.IsZero() {
		res.Date = time.Now().UTC()
	}
	_, err := r.coll.InsertOne(ctx, res)
	return err
}

// ListEventResults returns every stored event result document.
func (r *MongoRepository) ListEventResults(ctx context.Context) ([]model.EventResult, error) {
	return r.find(ctx, bson.M{})
}

// ListEventResultsByDiscipline returns the raw result documents of a discipline.
func (r *MongoRepository) ListEventResultsByDiscipline(ctx context.Context, disciplineID int64) ([]model.EventResult, error) {
	return r.find(ctx, bson.M{"disciplineId": disciplineID})
}

// ListRecordHolders resolves case 2 (athletes that hold olympic records) by
// flattening the embedded records of every event result.
func (r *MongoRepository) ListRecordHolders(ctx context.Context) ([]model.RecordHolder, error) {
	return r.recordHolders(ctx, nil)
}

// ListRecordHoldersByDiscipline resolves case 2 filtered by discipline.
func (r *MongoRepository) ListRecordHoldersByDiscipline(ctx context.Context, disciplineID int64) ([]model.RecordHolder, error) {
	return r.recordHolders(ctx, bson.D{{Key: "disciplineId", Value: disciplineID}})
}

// HasOlympicRecord reports whether an athlete holds any olympic record (case 7, part B).
func (r *MongoRepository) HasOlympicRecord(ctx context.Context, athleteID int64) (bool, error) {
	n, err := r.coll.CountDocuments(ctx, bson.M{"records.athleteId": athleteID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *MongoRepository) recordHolders(ctx context.Context, match bson.D) ([]model.RecordHolder, error) {
	pipeline := mongo.Pipeline{}
	if match != nil {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: match}})
	}
	pipeline = append(pipeline,
		bson.D{{Key: "$unwind", Value: "$records"}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "athleteId", Value: "$records.athleteId"},
			{Key: "athleteName", Value: "$records.athleteName"},
			{Key: "disciplineId", Value: "$disciplineId"},
			{Key: "disciplineName", Value: "$disciplineName"},
			{Key: "sport", Value: "$sport"},
			{Key: "eventId", Value: "$eventId"},
			{Key: "gameName", Value: "$gameName"},
			{Key: "record_type", Value: "$records.record_type"},
			{Key: "metric", Value: "$records.metric"},
			{Key: "value", Value: "$records.value"},
		}}},
	)

	cur, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var out []model.RecordHolder
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MongoRepository) find(ctx context.Context, filter bson.M) ([]model.EventResult, error) {
	cur, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var out []model.EventResult
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
