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

// AuditLogRepo is the MongoDB implementation of port.AuditLogRepository.
type AuditLogRepo struct {
	coll *mongo.Collection
}

func NewAuditLogRepo(coll *mongo.Collection) *AuditLogRepo {
	return &AuditLogRepo{coll: coll}
}

func (r *AuditLogRepo) Create(ctx context.Context, log *entity.AuditLog) (primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	res, err := r.coll.InsertOne(ctx, log)
	if err != nil {
		return primitive.NilObjectID, err
	}
	return res.InsertedID.(primitive.ObjectID), nil
}

// List returns the most recent audit logs, newest first, with optional action
// filter and total count for client-side pagination.
func (r *AuditLogRepo) List(ctx context.Context, action string, skip, limit int64) ([]entity.AuditLog, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	filter := bson.M{}
	if action != "" {
		filter["action"] = action
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit)

	cur, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	var logs []entity.AuditLog
	if err := cur.All(ctx, &logs); err != nil {
		return nil, 0, err
	}
	if logs == nil {
		logs = []entity.AuditLog{}
	}
	return logs, total, nil
}
