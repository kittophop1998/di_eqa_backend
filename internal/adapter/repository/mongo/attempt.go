package mongo

import (
	"context"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AttemptRepo is the MongoDB implementation of port.AttemptRepository. Every
// state change is one conditional findOneAndUpdate (BR-20).
type AttemptRepo struct{ coll *mongodriver.Collection }

func NewAttemptRepo(coll *mongodriver.Collection) *AttemptRepo { return &AttemptRepo{coll: coll} }

func (r *AttemptRepo) Create(ctx context.Context, a *entity.Attempt) (primitive.ObjectID, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.coll.InsertOne(ctx, a)
	if err != nil {
		return primitive.NilObjectID, mapErr(err)
	}
	return res.InsertedID.(primitive.ObjectID), nil
}

func (r *AttemptRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Attempt, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var a entity.Attempt
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&a); err != nil {
		return nil, mapErr(err)
	}
	return &a, nil
}

func (r *AttemptRepo) ListByUserQuizzes(ctx context.Context, userID primitive.ObjectID, quizIDs []primitive.ObjectID) ([]entity.Attempt, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, bson.M{"userId": userID, "quizId": idsIn(quizIDs)},
		options.Find().SetSort(bson.D{{Key: "startedAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return collect[entity.Attempt](ctx, cur)
}

func (r *AttemptRepo) ListByUser(ctx context.Context, userID primitive.ObjectID, quizID *primitive.ObjectID, p port.Page) ([]entity.Attempt, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{"userId": userID}
	if quizID != nil {
		filter["quizId"] = *quizID
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := r.coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "startedAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(p.Skip()).SetLimit(p.Limit()).
		SetProjection(bson.M{"questions": 0, "answers": 0})) // history never needs per-question data
	if err != nil {
		return nil, 0, err
	}
	list, err := collect[entity.Attempt](ctx, cur)
	return list, total, err
}

// graceCutoff is the earliest expiresAt that is still inside the grace period.
func graceCutoff(now time.Time) time.Time { return now.Add(-entity.SubmitGrace) }

func (r *AttemptRepo) MergeAnswers(ctx context.Context, id primitive.ObjectID, set map[string]string, clear []string, now time.Time) (*entity.Attempt, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	upd := bson.M{}
	if len(set) > 0 {
		s := bson.M{}
		for qid, v := range set {
			s["answers."+qid] = v
		}
		upd["$set"] = s
	}
	if len(clear) > 0 {
		u := bson.M{}
		for _, qid := range clear {
			u["answers."+qid] = ""
		}
		upd["$unset"] = u
	}
	var a entity.Attempt
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "status": entity.AttemptInProgress, "expiresAt": bson.M{"$gte": graceCutoff(now)}},
		upd, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&a)
	if err != nil {
		if err == mongodriver.ErrNoDocuments {
			return nil, port.ErrConflict
		}
		return nil, err
	}
	return &a, nil
}

func (r *AttemptRepo) Submit(ctx context.Context, id primitive.ObjectID, res port.SubmitResult, now time.Time) (*entity.Attempt, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var a entity.Attempt
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "status": entity.AttemptInProgress, "expiresAt": bson.M{"$gte": graceCutoff(now)}},
		bson.M{"$set": bson.M{
			"status": entity.AttemptSubmitted, "submittedAt": res.SubmittedAt, "answers": res.Answers,
			"correct": res.Correct, "total": res.Total, "percent": res.Percent,
			"passed": res.Passed, "perType": res.PerType,
		}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&a)
	if err != nil {
		if err == mongodriver.ErrNoDocuments {
			return nil, port.ErrConflict
		}
		return nil, err
	}
	return &a, nil
}

func (r *AttemptRepo) Expire(ctx context.Context, id primitive.ObjectID, now time.Time) (*entity.Attempt, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var a entity.Attempt
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "status": entity.AttemptInProgress, "expiresAt": bson.M{"$lt": graceCutoff(now)}},
		bson.M{"$set": bson.M{"status": entity.AttemptExpired}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&a)
	if err != nil {
		if err == mongodriver.ErrNoDocuments {
			return nil, port.ErrConflict
		}
		return nil, err
	}
	return &a, nil
}

func (r *AttemptRepo) SetCertificate(ctx context.Context, id, certID primitive.ObjectID, certNo string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"certificateId": certID, "certNo": certNo}})
	return err
}

func (r *AttemptRepo) exists(ctx context.Context, filter bson.M) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	n, err := r.coll.CountDocuments(ctx, filter, options.Count().SetLimit(1))
	return n > 0, err
}

func (r *AttemptRepo) ExistsForQuizHospital(ctx context.Context, quizID, hospitalID primitive.ObjectID) (bool, error) {
	return r.exists(ctx, bson.M{"quizId": quizID, "hospitalId": hospitalID})
}

func (r *AttemptRepo) ExistsForQuiz(ctx context.Context, quizID primitive.ObjectID) (bool, error) {
	return r.exists(ctx, bson.M{"quizId": quizID})
}

// StatsByQuizzes counts distinct users per (quiz,hospital): started, with a
// submitted attempt, and with a passing attempt.
func (r *AttemptRepo) StatsByQuizzes(ctx context.Context, quizIDs []primitive.ObjectID) (map[primitive.ObjectID]map[primitive.ObjectID]port.AttemptStats, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Aggregate(ctx, mongodriver.Pipeline{
		{{Key: "$match", Value: bson.M{"quizId": idsIn(quizIDs)}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{"q": "$quizId", "h": "$hospitalId", "u": "$userId"},
			"sub": bson.M{"$max": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", entity.AttemptSubmitted}}, 1, 0}}},
			"pas": bson.M{"$max": bson.M{"$cond": bson.A{bson.M{"$and": bson.A{
				bson.M{"$eq": bson.A{"$status", entity.AttemptSubmitted}}, bson.M{"$eq": bson.A{"$passed", true}},
			}}, 1, 0}}},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":       bson.M{"q": "$_id.q", "h": "$_id.h"},
			"started":   bson.M{"$sum": 1},
			"submitted": bson.M{"$sum": "$sub"},
			"passed":    bson.M{"$sum": "$pas"},
		}}},
	})
	if err != nil {
		return nil, err
	}
	rows, err := collect[struct {
		ID struct {
			Q primitive.ObjectID `bson:"q"`
			H primitive.ObjectID `bson:"h"`
		} `bson:"_id"`
		Started   int `bson:"started"`
		Submitted int `bson:"submitted"`
		Passed    int `bson:"passed"`
	}](ctx, cur)
	if err != nil {
		return nil, err
	}
	out := map[primitive.ObjectID]map[primitive.ObjectID]port.AttemptStats{}
	for _, row := range rows {
		if out[row.ID.Q] == nil {
			out[row.ID.Q] = map[primitive.ObjectID]port.AttemptStats{}
		}
		out[row.ID.Q][row.ID.H] = port.AttemptStats{Started: row.Started, Submitted: row.Submitted, Passed: row.Passed}
	}
	return out, nil
}
