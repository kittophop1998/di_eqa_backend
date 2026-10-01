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

// UserRepo is the MongoDB implementation of port.UserRepository.
type UserRepo struct{ coll *mongodriver.Collection }

func NewUserRepo(coll *mongodriver.Collection) *UserRepo { return &UserRepo{coll: coll} }

func (r *UserRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var u entity.User
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepo) FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, bson.M{"_id": idsIn(ids)}, options.Find().SetProjection(bson.M{"password": 0}))
	if err != nil {
		return nil, err
	}
	return collect[entity.User](ctx, cur)
}

func (r *UserRepo) FindByUsername(ctx context.Context, usernameLower string) (*entity.User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var u entity.User
	if err := r.coll.FindOne(ctx, bson.M{"usernameLower": strings.ToLower(usernameLower)}).Decode(&u); err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepo) Create(ctx context.Context, user *entity.User) (primitive.ObjectID, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	user.UsernameLower = strings.ToLower(user.Username)
	res, err := r.coll.InsertOne(ctx, user)
	if err != nil {
		return primitive.NilObjectID, mapErr(err)
	}
	return res.InsertedID.(primitive.ObjectID), nil
}

func (r *UserRepo) List(ctx context.Context, f port.UserFilter, p port.Page) ([]entity.User, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.Role != "" {
		filter["role"] = f.Role
	}
	if !f.HospitalID.IsZero() {
		filter["hospitalId"] = f.HospitalID
	}
	if f.Query != "" {
		filter["$or"] = []bson.M{
			{"username": literal(f.Query)}, {"fullName": literal(f.Query)}, {"email": literal(f.Query)},
		}
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := r.coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(p.Skip()).SetLimit(p.Limit()).
		SetProjection(bson.M{"password": 0}))
	if err != nil {
		return nil, 0, err
	}
	users, err := collect[entity.User](ctx, cur)
	return users, total, err
}

func (r *UserRepo) Update(ctx context.Context, id primitive.ObjectID, upd port.UserUpdate, onlyIfStatusIn []entity.UserStatus) (*entity.User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	set := bson.M{}
	unset := bson.M{}
	if upd.Status != nil {
		set["status"] = *upd.Status
	}
	if upd.Role != nil {
		set["role"] = *upd.Role
	}
	if upd.HospitalID != nil {
		set["hospitalId"] = *upd.HospitalID
	}
	if upd.FullName != nil {
		set["fullName"] = *upd.FullName
	}
	if upd.Email != nil {
		set["email"] = *upd.Email
	}
	if upd.RejectReason != nil {
		set["rejectReason"] = *upd.RejectReason
	}
	if upd.ApprovedAt != nil {
		set["approvedAt"] = *upd.ApprovedAt
	}
	if upd.ApprovedBy != nil {
		set["approvedBy"] = *upd.ApprovedBy
	}
	if upd.ClearRejectReason && upd.RejectReason == nil {
		unset["rejectReason"] = ""
	}
	doc := bson.M{}
	if len(set) > 0 {
		doc["$set"] = set
	}
	if len(unset) > 0 {
		doc["$unset"] = unset
	}
	filter := bson.M{"_id": id}
	if len(onlyIfStatusIn) > 0 {
		filter["status"] = bson.M{"$in": onlyIfStatusIn}
	}
	if len(doc) == 0 {
		return r.FindByID(ctx, id)
	}
	var u entity.User
	err := r.coll.FindOneAndUpdate(ctx, filter, doc,
		options.FindOneAndUpdate().SetReturnDocument(options.After).SetProjection(bson.M{"password": 0})).Decode(&u)
	if err != nil {
		if err == mongodriver.ErrNoDocuments && len(onlyIfStatusIn) > 0 {
			if _, ferr := r.FindByID(ctx, id); ferr == nil {
				return nil, port.ErrConflict
			}
		}
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *UserRepo) CountByHospital(ctx context.Context, hospitalID primitive.ObjectID) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.coll.CountDocuments(ctx, bson.M{"hospitalId": hospitalID})
}

func (r *UserRepo) CountActiveByHospitals(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]int, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Aggregate(ctx, mongodriver.Pipeline{
		{{Key: "$match", Value: bson.M{"hospitalId": idsIn(ids), "status": entity.UserActive, "role": entity.RoleUser}}},
		{{Key: "$group", Value: bson.M{"_id": "$hospitalId", "n": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	rows, err := collect[struct {
		ID primitive.ObjectID `bson:"_id"`
		N  int                `bson:"n"`
	}](ctx, cur)
	if err != nil {
		return nil, err
	}
	out := make(map[primitive.ObjectID]int, len(rows))
	for _, row := range rows {
		out[row.ID] = row.N
	}
	return out, nil
}

func (r *UserRepo) CountByRoleStatus(ctx context.Context, role entity.Role, status entity.UserStatus) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	f := bson.M{"role": role}
	if status != "" {
		f["status"] = status
	}
	return r.coll.CountDocuments(ctx, f)
}
