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

// SubmissionRepo is the MongoDB implementation of port.SubmissionRepository.
type SubmissionRepo struct {
	coll *mongodriver.Collection
}

func NewSubmissionRepo(coll *mongodriver.Collection) *SubmissionRepo {
	return &SubmissionRepo{coll: coll}
}

func (r *SubmissionRepo) Create(ctx context.Context, sub *entity.Submission) (primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := r.coll.InsertOne(ctx, sub)
	if err != nil {
		return primitive.NilObjectID, err
	}
	return res.InsertedID.(primitive.ObjectID), nil
}

func (r *SubmissionRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Submission, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var s entity.Submission
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SubmissionRepo) FindByIDAndUser(ctx context.Context, id, userID primitive.ObjectID) (*entity.Submission, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var s entity.Submission
	if err := r.coll.FindOne(ctx, bson.M{"_id": id, "userId": userID}).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SubmissionRepo) FindByUser(ctx context.Context, userID primitive.ObjectID, limit int64) ([]entity.Submission, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cur, err := r.coll.Find(ctx, bson.M{"userId": userID},
		options.Find().SetSort(bson.D{{Key: "submittedAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var list []entity.Submission
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []entity.Submission{}
	}
	return list, nil
}
