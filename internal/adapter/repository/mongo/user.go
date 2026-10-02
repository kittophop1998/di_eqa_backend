package mongo

import (
	"context"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

// FindByUsername performs a global username lookup — used for the simplified
// login flow that does not require a hospitalCode.
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*entity.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var u entity.User
	if err := r.coll.FindOne(ctx, bson.M{"username": username}).Decode(&u); err != nil {
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
		"role":     bson.M{"$in": []string{entity.RoleAdmin, entity.RoleSuperAdmin}},
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

// FindExternalByUsername finds an external member (no hospital) by username.
func (r *UserRepo) FindExternalByUsername(ctx context.Context, username string) (*entity.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var u entity.User
	if err := r.coll.FindOne(ctx, bson.M{
		"username":           username,
		"profile.memberType": entity.MemberExternal,
	}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// CountByUsername counts users that have the given username globally.
func (r *UserRepo) CountByUsername(ctx context.Context, username string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.coll.CountDocuments(ctx, bson.M{"username": username})
}

func (r *UserRepo) CountByUsernameAndHospital(ctx context.Context, username string, hospitalID primitive.ObjectID) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.coll.CountDocuments(ctx, bson.M{
		"hospitalId": hospitalID,
		"username":   username,
	})
}

func (r *UserRepo) CountExternalByUsername(ctx context.Context, username string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.coll.CountDocuments(ctx, bson.M{
		"username":           username,
		"profile.memberType": entity.MemberExternal,
	})
}

// CountByHospital counts the users currently attached to a hospital. Used to
// block deletion of a hospital that still has members.
func (r *UserRepo) CountByHospital(ctx context.Context, hospitalID primitive.ObjectID) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.coll.CountDocuments(ctx, bson.M{"hospitalId": hospitalID})
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

// ListAll returns a paginated+searchable list of all users, plus the total count.
func (r *UserRepo) ListAll(ctx context.Context, search string, skip, limit int64) ([]entity.User, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	filter := bson.M{}
	if search != "" {
		filter = bson.M{
			"$or": []bson.M{
				{"username": bson.M{"$regex": search, "$options": "i"}},
				{"fullName": bson.M{"$regex": search, "$options": "i"}},
				{"email": bson.M{"$regex": search, "$options": "i"}},
			},
		}
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit).
		SetProjection(bson.M{"password": 0}) // never send hashed password

	cur, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	var users []entity.User
	if err := cur.All(ctx, &users); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// UpdateRole atomically updates a user's role field.
func (r *UserRepo) UpdateRole(ctx context.Context, id primitive.ObjectID, role string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := r.coll.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"role": role}},
	)
	return err
}
