package mongo

import (
	"context"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// UserRepo is the MongoDB implementation of port.UserRepository.
type UserRepo struct {
	coll *mongo.Collection
}

func NewUserRepo(coll *mongo.Collection) *UserRepo {
	return &UserRepo{coll: coll}
}

func (r *UserRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var u entity.User
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) FindAdminByUsername(ctx context.Context, username string) (*entity.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var u entity.User
	if err := r.coll.FindOne(ctx, bson.M{
		"username": username,
		"role":     entity.RoleAdmin,
	}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) FindByUsernameAndHospital(ctx context.Context, username string, hospitalID primitive.ObjectID) (*entity.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var u entity.User
	if err := r.coll.FindOne(ctx, bson.M{
		"hospitalId": hospitalID,
		"username":   username,
	}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) CountByUsernameAndHospital(ctx context.Context, username string, hospitalID primitive.ObjectID) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.coll.CountDocuments(ctx, bson.M{
		"hospitalId": hospitalID,
		"username":   username,
	})
}

func (r *UserRepo) Create(ctx context.Context, user *entity.User) (primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	res, err := r.coll.InsertOne(ctx, user)
	if err != nil {
		return primitive.NilObjectID, err
	}
	return res.InsertedID.(primitive.ObjectID), nil
}
