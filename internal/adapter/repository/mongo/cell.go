package mongo

import (
	"context"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CellTypeRepo is the MongoDB implementation of port.CellTypeRepository.
type CellTypeRepo struct{ coll *mongodriver.Collection }

func NewCellTypeRepo(coll *mongodriver.Collection) *CellTypeRepo { return &CellTypeRepo{coll: coll} }

func (r *CellTypeRepo) list(ctx context.Context, filter bson.M) ([]entity.CellType, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "sortOrder", Value: 1}, {Key: "key", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return collect[entity.CellType](ctx, cur)
}

func (r *CellTypeRepo) ListAll(ctx context.Context) ([]entity.CellType, error) {
	return r.list(ctx, bson.M{})
}

func (r *CellTypeRepo) ListActive(ctx context.Context) ([]entity.CellType, error) {
	return r.list(ctx, bson.M{"active": true})
}

// CellImageRepo is the MongoDB implementation of port.CellImageRepository.
type CellImageRepo struct{ coll *mongodriver.Collection }

func NewCellImageRepo(coll *mongodriver.Collection) *CellImageRepo { return &CellImageRepo{coll: coll} }

func (r *CellImageRepo) ActiveIDs(ctx context.Context, typeKeys []string) ([]primitive.ObjectID, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	if len(typeKeys) == 0 {
		return []primitive.ObjectID{}, nil
	}
	cur, err := r.coll.Find(ctx, bson.M{"active": true, "typeKey": bson.M{"$in": typeKeys}},
		options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	rows, err := collect[struct {
		ID primitive.ObjectID `bson:"_id"`
	}](ctx, cur)
	if err != nil {
		return nil, err
	}
	out := make([]primitive.ObjectID, len(rows))
	for i, row := range rows {
		out[i] = row.ID
	}
	return out, nil
}

func (r *CellImageRepo) FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.CellImage, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, bson.M{"_id": idsIn(ids)})
	if err != nil {
		return nil, err
	}
	return collect[entity.CellImage](ctx, cur)
}

func (r *CellImageRepo) List(ctx context.Context, f port.ImageFilter, p port.Page) ([]entity.CellImage, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{}
	if f.TypeKey != "" {
		filter["typeKey"] = f.TypeKey
	}
	if f.Active != nil {
		filter["active"] = *f.Active
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := r.coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).SetSkip(p.Skip()).SetLimit(p.Limit()))
	if err != nil {
		return nil, 0, err
	}
	list, err := collect[entity.CellImage](ctx, cur)
	return list, total, err
}

func (r *CellImageRepo) CountByType(ctx context.Context) (map[string]entity.CellTypeCounts, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Aggregate(ctx, mongodriver.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":    "$typeKey",
			"total":  bson.M{"$sum": 1},
			"active": bson.M{"$sum": bson.M{"$cond": bson.A{"$active", 1, 0}}},
		}}},
	})
	if err != nil {
		return nil, err
	}
	rows, err := collect[struct {
		Key    string `bson:"_id"`
		Total  int    `bson:"total"`
		Active int    `bson:"active"`
	}](ctx, cur)
	if err != nil {
		return nil, err
	}
	out := make(map[string]entity.CellTypeCounts, len(rows))
	for _, row := range rows {
		out[row.Key] = entity.CellTypeCounts{Total: row.Total, Active: row.Active}
	}
	return out, nil
}
