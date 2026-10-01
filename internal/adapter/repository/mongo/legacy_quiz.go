package mongo

import (
	"context"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
)

// LegacyQuizRepo reads pre-v2 quiz documents for the unrouted legacy session code.
type LegacyQuizRepo struct{ coll *mongodriver.Collection }

func NewLegacyQuizRepo(coll *mongodriver.Collection) *LegacyQuizRepo {
	return &LegacyQuizRepo{coll: coll}
}

func (r *LegacyQuizRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.LegacyQuiz, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var q entity.LegacyQuiz
	if err := r.coll.FindOne(ctx, bson.M{"_id": id, "v": bson.M{"$exists": false}}).Decode(&q); err != nil {
		return nil, mapErr(err)
	}
	return &q, nil
}
