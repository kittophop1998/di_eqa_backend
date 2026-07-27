package port

// Package port defines the secondary ports (driven interfaces) that the
// application layer depends on.  Concrete implementations live in adapter/.

import (
	"context"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserRepository — port for user persistence
type UserRepository interface {
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.User, error)
	FindByUsername(ctx context.Context, username string) (*entity.User, error)
	FindAdminByUsername(ctx context.Context, username string) (*entity.User, error)
	FindByUsernameAndHospital(ctx context.Context, username string, hospitalID primitive.ObjectID) (*entity.User, error)
	FindExternalByUsername(ctx context.Context, username string) (*entity.User, error)
	CountByUsername(ctx context.Context, username string) (int64, error)
	CountByUsernameAndHospital(ctx context.Context, username string, hospitalID primitive.ObjectID) (int64, error)
	CountExternalByUsername(ctx context.Context, username string) (int64, error)
	CountByHospital(ctx context.Context, hospitalID primitive.ObjectID) (int64, error)
	Create(ctx context.Context, user *entity.User) (primitive.ObjectID, error)
	ListAll(ctx context.Context, search string, skip, limit int64) ([]entity.User, int64, error)
	UpdateRole(ctx context.Context, id primitive.ObjectID, role string) error
}

// HospitalRepository — port for hospital persistence
type HospitalRepository interface {
	List(ctx context.Context, query string) ([]entity.Hospital, error)
	FindByCode(ctx context.Context, code string) (*entity.Hospital, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Hospital, error)
	Create(ctx context.Context, h *entity.Hospital) (primitive.ObjectID, error)
	Update(ctx context.Context, id primitive.ObjectID, h *entity.Hospital) error
	Delete(ctx context.Context, id primitive.ObjectID) error
}

// QuizRepository — port for quiz persistence
type QuizRepository interface {
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Quiz, error)
	ListAll(ctx context.Context) ([]entity.Quiz, error)
	ListByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.Quiz, error)
}

// SessionRepository — port for session persistence
type SessionRepository interface {
	Create(ctx context.Context, session *entity.Session) (primitive.ObjectID, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Session, error)
	FindByCode(ctx context.Context, code string) (*entity.Session, error)
	ListActive(ctx context.Context, hospitalID *primitive.ObjectID) ([]entity.Session, error)
	UpdateToRunning(ctx context.Context, id primitive.ObjectID) (*entity.Session, error)
	UpdateToEnded(ctx context.Context, id primitive.ObjectID) (*entity.Session, error)
	FindActiveQuizIDs(ctx context.Context, hospitalID *primitive.ObjectID) ([]primitive.ObjectID, error)
}

// SubmissionRepository — port for submission persistence
type SubmissionRepository interface {
	Create(ctx context.Context, sub *entity.Submission) (primitive.ObjectID, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Submission, error)
	FindByIDAndUser(ctx context.Context, id, userID primitive.ObjectID) (*entity.Submission, error)
	FindByUser(ctx context.Context, userID primitive.ObjectID, limit int64) ([]entity.Submission, error)
}

// AuditLogRepository — port for append-only audit log persistence.
// The repository is intentionally minimal: log writes must never block the
// hot path, so callers are expected to invoke Create from a goroutine.
type AuditLogRepository interface {
	Create(ctx context.Context, log *entity.AuditLog) (primitive.ObjectID, error)
	List(ctx context.Context, action string, skip, limit int64) ([]entity.AuditLog, int64, error)
}
