package mongo

import (
	"context"
	"strings"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HospitalRepo is the MongoDB implementation of port.HospitalRepository.
type HospitalRepo struct{ coll *mongodriver.Collection }

func NewHospitalRepo(coll *mongodriver.Collection) *HospitalRepo { return &HospitalRepo{coll: coll} }

func (r *HospitalRepo) FindByCode(ctx context.Context, code string) (*entity.Hospital, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var h entity.Hospital
	if err := r.coll.FindOne(ctx, bson.M{"code": strings.ToUpper(strings.TrimSpace(code))}).Decode(&h); err != nil {
		return nil, mapErr(err)
	}
	return &h, nil
}

func (r *HospitalRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Hospital, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var h entity.Hospital
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&h); err != nil {
		return nil, mapErr(err)
	}
	return &h, nil
}

func (r *HospitalRepo) FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.Hospital, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, bson.M{"_id": idsIn(ids)})
	if err != nil {
		return nil, err
	}
	return collect[entity.Hospital](ctx, cur)
}

func (r *HospitalRepo) ListActive(ctx context.Context) ([]entity.Hospital, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, bson.M{"active": true}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return collect[entity.Hospital](ctx, cur)
}

func (r *HospitalRepo) List(ctx context.Context, f port.HospitalFilter, p port.Page) ([]entity.Hospital, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{}
	if f.Active != nil {
		filter["active"] = *f.Active
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		filter["$or"] = []bson.M{
			{"name": literal(q)}, {"code": literal(q)}, {"province": literal(q)},
			{"district": literal(q)}, {"subDistrict": literal(q)}, {"postalCode": literal(q)},
		}
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := r.coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).SetSkip(p.Skip()).SetLimit(p.Limit()))
	if err != nil {
		return nil, 0, err
	}
	list, err := collect[entity.Hospital](ctx, cur)
	return list, total, err
}

func (r *HospitalRepo) Create(ctx context.Context, h *entity.Hospital) (primitive.ObjectID, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.coll.InsertOne(ctx, h)
	if err != nil {
		return primitive.NilObjectID, mapErr(err)
	}
	return res.InsertedID.(primitive.ObjectID), nil
}

func (r *HospitalRepo) Update(ctx context.Context, id primitive.ObjectID, h *entity.Hospital) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{
		"code": h.Code, "name": h.Name, "logo": h.Logo, "province": h.Province,
		"district": h.District, "subDistrict": h.SubDistrict, "postalCode": h.PostalCode, "active": h.Active,
	}})
	if err != nil {
		return mapErr(err)
	}
	if res.MatchedCount == 0 {
		return port.ErrNotFound
	}
	return nil
}

func (r *HospitalRepo) SetActive(ctx context.Context, id primitive.ObjectID, active bool) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"active": active}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return port.ErrNotFound
	}
	return nil
}
