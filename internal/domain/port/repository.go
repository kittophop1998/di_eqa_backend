package port

// Package port defines the secondary ports (driven interfaces) that the
// application layer depends on.  Concrete implementations live in adapter/.

import (
	"context"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserFilter narrows the admin user list.
type UserFilter struct {
	Status     entity.UserStatus
	Role       entity.Role
	HospitalID primitive.ObjectID
	Query      string // matched literally (never as a regex) against username/fullName/email
}

// UserUpdate is a partial update; nil fields are left untouched.
type UserUpdate struct {
	Status       *entity.UserStatus
	Role         *entity.Role
	HospitalID   *primitive.ObjectID
	FullName     *string
	Email        *string
	RejectReason *string
	ApprovedAt   *time.Time
	ApprovedBy   *primitive.ObjectID
	// ClearRejectReason removes a previous rejection reason.
	ClearRejectReason bool
}

// UserRepository — port for user persistence.
type UserRepository interface {
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.User, error)
	FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.User, error)
	// FindByUsername looks up by the lower-cased username.
	FindByUsername(ctx context.Context, usernameLower string) (*entity.User, error)
	// Create returns ErrDuplicate when the username already exists.
	Create(ctx context.Context, user *entity.User) (primitive.ObjectID, error)
	List(ctx context.Context, f UserFilter, p Page) ([]entity.User, int64, error)
	// Update applies upd. When onlyIfStatusIn is non-empty the write is a
	// compare-and-set on the current status and returns ErrConflict if it does
	// not match. It returns the updated user, or ErrNotFound.
	Update(ctx context.Context, id primitive.ObjectID, upd UserUpdate, onlyIfStatusIn []entity.UserStatus) (*entity.User, error)
	CountByHospital(ctx context.Context, hospitalID primitive.ObjectID) (int64, error)
	// CountActiveByHospitals returns active user counts keyed by hospital.
	CountActiveByHospitals(ctx context.Context, hospitalIDs []primitive.ObjectID) (map[primitive.ObjectID]int, error)
	// CountByRoleStatus counts users with role; an empty status matches any status.
	CountByRoleStatus(ctx context.Context, role entity.Role, status entity.UserStatus) (int64, error)
}

// HospitalFilter narrows the admin hospital list.
type HospitalFilter struct {
	Query  string
	Active *bool
}

// HospitalRepository — port for hospital persistence.
type HospitalRepository interface {
	FindByCode(ctx context.Context, code string) (*entity.Hospital, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Hospital, error)
	FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.Hospital, error)
	ListActive(ctx context.Context) ([]entity.Hospital, error)
	List(ctx context.Context, f HospitalFilter, p Page) ([]entity.Hospital, int64, error)
	// Create returns ErrDuplicate for a repeated code.
	Create(ctx context.Context, h *entity.Hospital) (primitive.ObjectID, error)
	// Update returns ErrDuplicate for a repeated code and ErrNotFound if missing.
	Update(ctx context.Context, id primitive.ObjectID, h *entity.Hospital) error
	SetActive(ctx context.Context, id primitive.ObjectID, active bool) error
}

// CellTypeRepository — port for the answer-option library.
type CellTypeRepository interface {
	ListAll(ctx context.Context) ([]entity.CellType, error) // sorted by sortOrder
	ListActive(ctx context.Context) ([]entity.CellType, error)
}

// ImageFilter narrows the admin image list.
type ImageFilter struct {
	TypeKey string
	Active  *bool
}

// CellImageRepository — port for the image pool.
type CellImageRepository interface {
	// ActiveIDs returns ids of active images whose type key is in typeKeys.
	ActiveIDs(ctx context.Context, typeKeys []string) ([]primitive.ObjectID, error)
	FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.CellImage, error)
	List(ctx context.Context, f ImageFilter, p Page) ([]entity.CellImage, int64, error)
	CountByType(ctx context.Context) (map[string]entity.CellTypeCounts, error)
}

// QuizFilter narrows the admin quiz list.
type QuizFilter struct {
	Status entity.QuizStatus
	Query  string
}

// QuizRepository — port for v2 quiz persistence. Only v2 documents are visible.
type QuizRepository interface {
	Create(ctx context.Context, q *entity.Quiz) (primitive.ObjectID, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Quiz, error)
	FindByIDs(ctx context.Context, ids []primitive.ObjectID) ([]entity.Quiz, error)
	List(ctx context.Context, f QuizFilter, p Page) ([]entity.Quiz, int64, error)
	// UpdateParams replaces title/description/parameters and bumps updatedAt.
	UpdateParams(ctx context.Context, q *entity.Quiz) error
	// TransitionStatus is a compare-and-set on status; ErrConflict if the
	// current status is not in from.
	TransitionStatus(ctx context.Context, id primitive.ObjectID, from []entity.QuizStatus, to entity.QuizStatus, at time.Time) error
	Delete(ctx context.Context, id primitive.ObjectID) error
}

// AssignmentRepository — port for quiz↔hospital assignments.
type AssignmentRepository interface {
	CreateMany(ctx context.Context, as []entity.HospitalAssignment) error
	Find(ctx context.Context, quizID, hospitalID primitive.ObjectID) (*entity.HospitalAssignment, error)
	ListByQuiz(ctx context.Context, quizID primitive.ObjectID) ([]entity.HospitalAssignment, error)
	ListByQuizzes(ctx context.Context, quizIDs []primitive.ObjectID) ([]entity.HospitalAssignment, error)
	ListByHospital(ctx context.Context, hospitalID primitive.ObjectID) ([]entity.HospitalAssignment, error)
	DeleteMany(ctx context.Context, quizID primitive.ObjectID, hospitalIDs []primitive.ObjectID) error
	DeleteByQuiz(ctx context.Context, quizID primitive.ObjectID) error
	// SetStatus sets the status (and openedAt/closedAt as appropriate) and
	// returns the updated assignment, or ErrNotFound.
	SetStatus(ctx context.Context, quizID, hospitalID primitive.ObjectID, status entity.AssignmentStatus, at time.Time) (*entity.HospitalAssignment, error)
}

// AttemptStats are per-hospital counters for the admin assignment table.
type AttemptStats struct {
	Started   int
	Submitted int
	Passed    int
}

// SubmitResult is what the atomic submit transition writes.
type SubmitResult struct {
	Answers     map[string]string
	SubmittedAt time.Time
	Correct     int
	Total       int
	Percent     float64
	Passed      bool
	PerType     []entity.TypeScore
}

// AttemptRepository — port for attempt persistence. All state changes are
// conditional writes (BR-20); there is no Redis lock.
type AttemptRepository interface {
	// Create returns ErrDuplicate when the partial-unique in_progress index or
	// the (quiz,user,attemptNo) index rejects the insert.
	Create(ctx context.Context, a *entity.Attempt) (primitive.ObjectID, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Attempt, error)
	ListByUserQuizzes(ctx context.Context, userID primitive.ObjectID, quizIDs []primitive.ObjectID) ([]entity.Attempt, error)
	ListByUser(ctx context.Context, userID primitive.ObjectID, quizID *primitive.ObjectID, p Page) ([]entity.Attempt, int64, error)
	// MergeAnswers sets/clears answers only while the attempt is in progress and
	// now <= expiresAt+grace. It returns the updated attempt or ErrConflict.
	MergeAnswers(ctx context.Context, id primitive.ObjectID, set map[string]string, clear []string, now time.Time) (*entity.Attempt, error)
	// Submit moves in_progress -> submitted atomically (only if now <= expiresAt+grace).
	// Returns ErrConflict when the attempt is no longer in that state.
	Submit(ctx context.Context, id primitive.ObjectID, res SubmitResult, now time.Time) (*entity.Attempt, error)
	// Expire moves in_progress -> expired atomically, only if past grace at now.
	// Returns ErrConflict when it did not match.
	Expire(ctx context.Context, id primitive.ObjectID, now time.Time) (*entity.Attempt, error)
	SetCertificate(ctx context.Context, id primitive.ObjectID, certID primitive.ObjectID, certNo string) error
	ExistsForQuizHospital(ctx context.Context, quizID, hospitalID primitive.ObjectID) (bool, error)
	ExistsForQuiz(ctx context.Context, quizID primitive.ObjectID) (bool, error)
	// StatsByQuizzes returns counters keyed by quiz then hospital.
	StatsByQuizzes(ctx context.Context, quizIDs []primitive.ObjectID) (map[primitive.ObjectID]map[primitive.ObjectID]AttemptStats, error)
}

// CertificateRepository — port for certificate persistence.
type CertificateRepository interface {
	// Create returns ErrDuplicate when certNo, attemptId or (quizId,userId) already exists.
	// Callers disambiguate by looking the certificate up by attempt / quiz+user.
	Create(ctx context.Context, c *entity.Certificate) (primitive.ObjectID, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (*entity.Certificate, error)
	FindByAttempt(ctx context.Context, attemptID primitive.ObjectID) (*entity.Certificate, error)
	FindByQuizUser(ctx context.Context, quizID, userID primitive.ObjectID) (*entity.Certificate, error)
	ListByUser(ctx context.Context, userID primitive.ObjectID) ([]entity.Certificate, error)
	ListByUserQuizzes(ctx context.Context, userID primitive.ObjectID, quizIDs []primitive.ObjectID) ([]entity.Certificate, error)
}

// AuditLogRepository — port for append-only audit log persistence.
type AuditLogRepository interface {
	Create(ctx context.Context, log *entity.AuditLog) (primitive.ObjectID, error)
	List(ctx context.Context, action string, skip, limit int64) ([]entity.AuditLog, int64, error)
}
