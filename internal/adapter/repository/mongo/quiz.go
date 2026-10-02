package mongo

import (
	"context"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// QuizRepo is the MongoDB implementation of port.QuizRepository.
type QuizRepo struct {
	coll *mongodriver.Collection
}

func NewQuizRepo(coll *mongodriver.Collection) *QuizRepo {
	return &QuizRepo{coll: coll}
}

func (r *QuizRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Quiz, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var q entity.Quiz
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&q); err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *QuizRepo) ListAll(ctx context.Context) ([]entity.Quiz, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := r.coll.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var list []entity.Quiz
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *QuizRepo) ListByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.Quiz, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := r.coll.Find(ctx,
		bson.M{"_id": bson.M{"$in": ids}},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var list []entity.Quiz
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}
