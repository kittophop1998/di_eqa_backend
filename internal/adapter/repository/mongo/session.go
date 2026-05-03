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

// SessionRepo is the MongoDB implementation of port.SessionRepository.
type SessionRepo struct {
	coll *mongodriver.Collection
}

func NewSessionRepo(coll *mongodriver.Collection) *SessionRepo {
	return &SessionRepo{coll: coll}
}

func (r *SessionRepo) Create(ctx context.Context, session *entity.Session) (primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := r.coll.InsertOne(ctx, session)
	if err != nil {
		return primitive.NilObjectID, err
	}
	return res.InsertedID.(primitive.ObjectID), nil
}

func (r *SessionRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var s entity.Session
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepo) FindByCode(ctx context.Context, code string) (*entity.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var s entity.Session
	if err := r.coll.FindOne(ctx, bson.M{"code": code}).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepo) ListActive(ctx context.Context, hospitalID *primitive.ObjectID) ([]entity.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filter := bson.M{"status": bson.M{"$in": []string{entity.SessionPending, entity.SessionRunning}}}
	if hospitalID != nil {
		filter["hospitalId"] = *hospitalID
	}

	cur, err := r.coll.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(50))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var list []entity.Session
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []entity.Session{}
	}
	return list, nil
}

func (r *SessionRepo) UpdateToRunning(ctx context.Context, id primitive.ObjectID) (*entity.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := time.Now()
	res := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": entity.SessionRunning, "startedAt": now}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var s entity.Session
	if err := res.Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepo) UpdateToEnded(ctx context.Context, id primitive.ObjectID) (*entity.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := time.Now()
	res := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": entity.SessionEnded, "endedAt": now}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var s entity.Session
	if err := res.Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepo) FindActiveQuizIDs(ctx context.Context, hospitalID *primitive.ObjectID) ([]primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filter := bson.M{"status": bson.M{"$in": []string{entity.SessionPending, entity.SessionRunning}}}
	if hospitalID != nil {
		filter["hospitalId"] = *hospitalID
	}

	cur, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	seen := map[primitive.ObjectID]struct{}{}
	var ids []primitive.ObjectID
	for cur.Next(ctx) {
		var s entity.Session
		if err := cur.Decode(&s); err == nil {
			if _, exists := seen[s.QuizID]; !exists {
				seen[s.QuizID] = struct{}{}
				ids = append(ids, s.QuizID)
			}
		}
	}
	return ids, nil
}
