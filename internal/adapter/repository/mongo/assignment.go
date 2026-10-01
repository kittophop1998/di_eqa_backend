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

// AssignmentRepo is the MongoDB implementation of port.AssignmentRepository.
type AssignmentRepo struct{ coll *mongodriver.Collection }

func NewAssignmentRepo(coll *mongodriver.Collection) *AssignmentRepo {
	return &AssignmentRepo{coll: coll}
}

func (r *AssignmentRepo) CreateMany(ctx context.Context, as []entity.HospitalAssignment) error {
	if len(as) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	docs := make([]any, len(as))
	for i := range as {
		docs[i] = as[i]
	}
	_, err := r.coll.InsertMany(ctx, docs)
	return mapErr(err)
}

func (r *AssignmentRepo) Find(ctx context.Context, quizID, hospitalID primitive.ObjectID) (*entity.HospitalAssignment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var a entity.HospitalAssignment
	if err := r.coll.FindOne(ctx, bson.M{"quizId": quizID, "hospitalId": hospitalID}).Decode(&a); err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

func (r *AssignmentRepo) list(ctx context.Context, filter bson.M) ([]entity.HospitalAssignment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return collect[entity.HospitalAssignment](ctx, cur)
}

func (r *AssignmentRepo) ListByQuiz(ctx context.Context, quizID primitive.ObjectID) ([]entity.HospitalAssignment, error) {
	return r.list(ctx, bson.M{"quizId": quizID})
}

func (r *AssignmentRepo) ListByQuizzes(ctx context.Context, quizIDs []primitive.ObjectID) ([]entity.HospitalAssignment, error) {
	return r.list(ctx, bson.M{"quizId": idsIn(quizIDs)})
}

func (r *AssignmentRepo) ListByHospital(ctx context.Context, hospitalID primitive.ObjectID) ([]entity.HospitalAssignment, error) {
	return r.list(ctx, bson.M{"hospitalId": hospitalID})
}

func (r *AssignmentRepo) DeleteMany(ctx context.Context, quizID primitive.ObjectID, hospitalIDs []primitive.ObjectID) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.coll.DeleteMany(ctx, bson.M{"quizId": quizID, "hospitalId": idsIn(hospitalIDs)})
	return err
}

func (r *AssignmentRepo) DeleteByQuiz(ctx context.Context, quizID primitive.ObjectID) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.coll.DeleteMany(ctx, bson.M{"quizId": quizID})
	return err
}

func (r *AssignmentRepo) SetStatus(ctx context.Context, quizID, hospitalID primitive.ObjectID, status entity.AssignmentStatus, at time.Time) (*entity.HospitalAssignment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var upd bson.M
	if status == entity.AssignmentOpen {
		upd = bson.M{"$set": bson.M{"status": status, "openedAt": at}, "$unset": bson.M{"closedAt": ""}}
	} else {
		upd = bson.M{"$set": bson.M{"status": status, "closedAt": at}}
	}
	var a entity.HospitalAssignment
	err := r.coll.FindOneAndUpdate(ctx, bson.M{"quizId": quizID, "hospitalId": hospitalID}, upd,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&a)
	if err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}
