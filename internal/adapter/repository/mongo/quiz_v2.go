package mongo

import (
	"context"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// QuizRepo is the MongoDB implementation of port.QuizRepository. It only ever
// sees v2 documents (field "v" == 2); legacy documents are invisible.
type QuizRepo struct{ coll *mongodriver.Collection }

func NewQuizRepo(coll *mongodriver.Collection) *QuizRepo { return &QuizRepo{coll: coll} }

func v2(extra bson.M) bson.M {
	f := bson.M{"v": entity.QuizSchemaVersion}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

func (r *QuizRepo) Create(ctx context.Context, q *entity.Quiz) (primitive.ObjectID, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	q.SchemaVersion = entity.QuizSchemaVersion
	res, err := r.coll.InsertOne(ctx, q)
	if err != nil {
		return primitive.NilObjectID, mapErr(err)
	}
	return res.InsertedID.(primitive.ObjectID), nil
}

func (r *QuizRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Quiz, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var q entity.Quiz
	if err := r.coll.FindOne(ctx, v2(bson.M{"_id": id})).Decode(&q); err != nil {
		return nil, mapErr(err)
	}
	return &q, nil
}

func (r *QuizRepo) FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.Quiz, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, v2(bson.M{"_id": idsIn(ids)}))
	if err != nil {
		return nil, err
	}
	return collect[entity.Quiz](ctx, cur)
}

func (r *QuizRepo) List(ctx context.Context, f port.QuizFilter, p port.Page) ([]entity.Quiz, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := v2(nil)
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.Query != "" {
		filter["title"] = literal(f.Query)
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := r.coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).SetSkip(p.Skip()).SetLimit(p.Limit()))
	if err != nil {
		return nil, 0, err
	}
	list, err := collect[entity.Quiz](ctx, cur)
	return list, total, err
}

func (r *QuizRepo) UpdateParams(ctx context.Context, q *entity.Quiz) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.coll.UpdateOne(ctx, v2(bson.M{"_id": q.ID}), bson.M{"$set": bson.M{
		"title": q.Title, "description": q.Description, "questionCount": q.QuestionCount,
		"passPercent": q.PassPercent, "durationSec": q.DurationSec, "maxAttempts": q.MaxAttempts,
		"opensAt": q.OpensAt, "closesAt": q.ClosesAt, "updatedAt": q.UpdatedAt,
	}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return port.ErrNotFound
	}
	return nil
}

func (r *QuizRepo) TransitionStatus(ctx context.Context, id primitive.ObjectID, from []entity.QuizStatus, to entity.QuizStatus, at time.Time) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.coll.UpdateOne(ctx, v2(bson.M{"_id": id, "status": bson.M{"$in": from}}),
		bson.M{"$set": bson.M{"status": to, "updatedAt": at}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		if _, ferr := r.FindByID(ctx, id); ferr == nil {
			return port.ErrConflict
		}
		return port.ErrNotFound
	}
	return nil
}

func (r *QuizRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.coll.DeleteOne(ctx, v2(bson.M{"_id": id}))
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return port.ErrNotFound
	}
	return nil
}
