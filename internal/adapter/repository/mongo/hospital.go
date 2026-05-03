package mongo

import (
	"context"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HospitalRepo is the MongoDB implementation of port.HospitalRepository.
type HospitalRepo struct {
	coll *mongodriver.Collection
}

func NewHospitalRepo(coll *mongodriver.Collection) *HospitalRepo {
	return &HospitalRepo{coll: coll}
}

func (r *HospitalRepo) List(ctx context.Context, query string) ([]entity.Hospital, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := strings.TrimSpace(query)
	filter := bson.M{}
	if q != "" {
		filter = bson.M{"$or": []bson.M{
			{"name": bson.M{"$regex": q, "$options": "i"}},
			{"code": bson.M{"$regex": q, "$options": "i"}},
		}}
	}

	cur, err := r.coll.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}).SetLimit(200))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var list []entity.Hospital
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []entity.Hospital{}
	}
	return list, nil
}

func (r *HospitalRepo) FindByCode(ctx context.Context, code string) (*entity.Hospital, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var h entity.Hospital
	if err := r.coll.FindOne(ctx, bson.M{"code": strings.ToUpper(strings.TrimSpace(code))}).Decode(&h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *HospitalRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Hospital, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var h entity.Hospital
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&h); err != nil {
		return nil, err
	}
	return &h, nil
}
