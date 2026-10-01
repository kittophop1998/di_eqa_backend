package port

import (
	"context"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The ports below serve only the legacy live-session code that is unrouted
// and scheduled for removal in Epic G.

// LegacyQuizRepository reads pre-v2 quiz documents.
type LegacyQuizRepository interface {
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.LegacyQuiz, error)
}

// SessionRepository — port for legacy session persistence.
type SessionRepository interface {
	Create(ctx context.Context, session *entity.Session) (primitive.ObjectID, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Session, error)
	FindByCode(ctx context.Context, code string) (*entity.Session, error)
	ListActive(ctx context.Context, hospitalID *primitive.ObjectID) ([]entity.Session, error)
	UpdateToRunning(ctx context.Context, id primitive.ObjectID) (*entity.Session, error)
	UpdateToEnded(ctx context.Context, id primitive.ObjectID) (*entity.Session, error)
	FindActiveQuizIDs(ctx context.Context, hospitalID *primitive.ObjectID) ([]primitive.ObjectID, error)
}

// SubmissionRepository — port for legacy submission persistence (read-only archive).
type SubmissionRepository interface {
	Create(ctx context.Context, sub *entity.Submission) (primitive.ObjectID, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Submission, error)
	FindByIDAndUser(ctx context.Context, id, userID primitive.ObjectID) (*entity.Submission, error)
	FindByUser(ctx context.Context, userID primitive.ObjectID, limit int64) ([]entity.Submission, error)
}
