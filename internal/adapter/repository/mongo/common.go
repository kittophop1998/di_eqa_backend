// Package mongo contains the MongoDB implementations of the domain ports.
package mongo

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
)

const opTimeout = 8 * time.Second

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, opTimeout)
}

// mapErr converts driver errors into the port errors the application knows.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mongodriver.ErrNoDocuments):
		return port.ErrNotFound
	case mongodriver.IsDuplicateKeyError(err):
		return port.ErrDuplicate
	default:
		return err
	}
}

// literal builds a case-insensitive regex that matches q literally, so user
// input can never act as a regex (no injection / ReDoS).
func literal(q string) bson.M {
	return bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
}

func idsIn(ids []primitive.ObjectID) bson.M { return bson.M{"$in": ids} }

// collect decodes every document of a cursor.
func collect[T any](ctx context.Context, cur *mongodriver.Cursor) ([]T, error) {
	defer cur.Close(ctx)
	var out []T
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []T{}
	}
	return out, nil
}
