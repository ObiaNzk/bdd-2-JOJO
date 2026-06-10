package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/ObiaNzk/bdd-2-JOJO/internal/model"
)

type MongoRepository struct {
	coll   *mongo.Collection // event_results: per-event type-specific outcomes
	wrColl *mongo.Collection // world_records: standing-WR ledger per discipline+metric
}

func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{
		coll:   db.Collection("event_results"),
		wrColl: db.Collection("world_records"),
	}
}

// EnsureIndexes creates the indexes backing the result and record queries.
// Safe to call on every startup.
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	if _, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "disciplineId", Value: 1}}},
		{Keys: bson.D{{Key: "eventId", Value: 1}}},
		{Keys: bson.D{{Key: "records.athleteId", Value: 1}}},
	}); err != nil {
		return err
	}
	// One world-record document per (discipline, metric).
	_, err := r.wrColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "disciplineId", Value: 1}, {Key: "metric", Value: 1}},
		Options: options.Index().SetUnique(true),
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

// GetWorldRecord returns the standing-world-record ledger for a discipline and
// metric, or nil when none has been set yet (so the first event inaugurates it).
func (r *MongoRepository) GetWorldRecord(ctx context.Context, disciplineID int64, metric string) (*model.WorldRecord, error) {
	var wr model.WorldRecord
	err := r.wrColl.FindOne(ctx, bson.M{"disciplineId": disciplineID, "metric": metric}).Decode(&wr)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &wr, nil
}

// UpsertWorldRecord persists the world-record ledger, replacing the document for
// its (discipline, metric) key or inserting it the first time.
func (r *MongoRepository) UpsertWorldRecord(ctx context.Context, wr *model.WorldRecord) error {
	// Store _id as a hex string (like event_results) so reads decode into the
	// string ID field; on insert Mongo would otherwise assign a raw ObjectID.
	if wr.ID == "" {
		wr.ID = bson.NewObjectID().Hex()
	}
	filter := bson.M{"disciplineId": wr.DisciplineID, "metric": wr.Metric}
	_, err := r.wrColl.ReplaceOne(ctx, filter, wr, options.Replace().SetUpsert(true))
	return err
}

// ListWorldRecords returns every world-record ledger (one per discipline+metric),
// each with its full holder timeline.
func (r *MongoRepository) ListWorldRecords(ctx context.Context) ([]model.WorldRecord, error) {
	cur, err := r.wrColl.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var out []model.WorldRecord
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
