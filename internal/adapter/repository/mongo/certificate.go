package mongo

import (
	"context"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CertificateRepo is the MongoDB implementation of port.CertificateRepository.
type CertificateRepo struct{ coll *mongodriver.Collection }

func NewCertificateRepo(coll *mongodriver.Collection) *CertificateRepo {
	return &CertificateRepo{coll: coll}
}

func (r *CertificateRepo) Create(ctx context.Context, c *entity.Certificate) (primitive.ObjectID, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.coll.InsertOne(ctx, c)
	if err != nil {
		return primitive.NilObjectID, mapErr(err)
	}
	return res.InsertedID.(primitive.ObjectID), nil
}

func (r *CertificateRepo) findOne(ctx context.Context, filter bson.M) (*entity.Certificate, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var c entity.Certificate
	if err := r.coll.FindOne(ctx, filter).Decode(&c); err != nil {
		return nil, mapErr(err)
	}
	return &c, nil
}

func (r *CertificateRepo) FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Certificate, error) {
	return r.findOne(ctx, bson.M{"_id": id})
}

func (r *CertificateRepo) FindByAttempt(ctx context.Context, attemptID primitive.ObjectID) (*entity.Certificate, error) {
	return r.findOne(ctx, bson.M{"attemptId": attemptID})
}

func (r *CertificateRepo) FindByQuizUser(ctx context.Context, quizID, userID primitive.ObjectID) (*entity.Certificate, error) {
	return r.findOne(ctx, bson.M{"quizId": quizID, "userId": userID})
}

func (r *CertificateRepo) list(ctx context.Context, filter bson.M) ([]entity.Certificate, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cur, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "issuedAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	return collect[entity.Certificate](ctx, cur)
}

func (r *CertificateRepo) ListByUser(ctx context.Context, userID primitive.ObjectID) ([]entity.Certificate, error) {
	return r.list(ctx, bson.M{"userId": userID})
}

func (r *CertificateRepo) ListByUserQuizzes(ctx context.Context, userID primitive.ObjectID, quizIDs []primitive.ObjectID) ([]entity.Certificate, error) {
	return r.list(ctx, bson.M{"userId": userID, "quizId": idsIn(quizIDs)})
}
