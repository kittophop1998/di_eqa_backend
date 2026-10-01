package service

import (
	"errors"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Machine-readable error codes (backlog §5.2). The HTTP adapter maps each code
// to a status; services never know about HTTP.
const (
	CodeValidation         = "VALIDATION_ERROR"
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeTokenExpired       = "TOKEN_EXPIRED"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeAccountPending     = "ACCOUNT_PENDING"
	CodeAccountRejected    = "ACCOUNT_REJECTED"
	CodeAccountDisabled    = "ACCOUNT_DISABLED"
	CodeForbidden          = "FORBIDDEN"
	CodeNotFound           = "NOT_FOUND"
	CodeUsernameTaken      = "USERNAME_TAKEN"
	CodeInvalidState       = "INVALID_STATE"
	CodeQuizNotOpen        = "QUIZ_NOT_OPEN"
	CodeMaxAttempts        = "MAX_ATTEMPTS_REACHED"
	CodeAlreadyPassed      = "ALREADY_PASSED"
	CodeAttemptNotInProg   = "ATTEMPT_NOT_IN_PROGRESS"
	CodeAttemptExpired     = "ATTEMPT_EXPIRED"
	CodeAttemptNotSubmit   = "ATTEMPT_NOT_SUBMITTED"
	CodeQuizNotPublished   = "QUIZ_NOT_PUBLISHED"
	CodeQuizWindowEnded    = "QUIZ_WINDOW_ENDED"
	CodePoolTooSmall       = "POOL_TOO_SMALL"
	CodeAssignmentAttempts = "ASSIGNMENT_HAS_ATTEMPTS"
	CodeHospitalInUse      = "HOSPITAL_IN_USE"
	CodeLastSuperAdmin     = "LAST_SUPER_ADMIN"
	CodeDuplicate          = "DUPLICATE"
	CodeInternal           = "INTERNAL"
)

// Error is an application error carrying a stable code, a Thai user-safe
// message and optional structured details. It never wraps driver errors.
type Error struct {
	Code    string
	Message string
	Details map[string]any
	cause   error
}

// Unwrap exposes the private cause for logging only.
func (e *Error) Unwrap() error { return e.cause }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Is lets errors.Is(err, ErrX) match on the code alone.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

func newErr(code, msg string, details map[string]any) *Error {
	return &Error{Code: code, Message: msg, Details: details}
}

// Sentinels for errors.Is comparisons.
var (
	ErrNotFound  = newErr(CodeNotFound, "ไม่พบข้อมูลที่ต้องการ", nil)
	ErrForbidden = newErr(CodeForbidden, "คุณไม่มีสิทธิ์ดำเนินการนี้", nil)
)

// CodeOf returns the application error code of err, or CodeInternal if err is
// not an application error.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}

func notFound() *Error {
	return newErr(CodeNotFound, "ไม่พบข้อมูลที่ต้องการ", nil)
}

func forbidden() *Error {
	return newErr(CodeForbidden, "คุณไม่มีสิทธิ์ดำเนินการนี้", nil)
}

func invalidState(msg string) *Error { return newErr(CodeInvalidState, msg, nil) }

func validation(issues ...entity.FieldIssue) *Error {
	fields := make([]map[string]string, 0, len(issues))
	for _, i := range issues {
		fields = append(fields, map[string]string{"field": i.Field, "issue": i.Issue})
	}
	return newErr(CodeValidation, "ข้อมูลไม่ถูกต้อง", map[string]any{"fields": fields})
}

func validationOne(field, issue string) *Error {
	return validation(entity.FieldIssue{Field: field, Issue: issue})
}

// internal wraps an unexpected failure. The cause is intentionally dropped
// from the message so driver text can never reach a client (BR-52); handlers
// log the original error via errors.Unwrap.
func internal(cause error) error {
	e := newErr(CodeInternal, "เกิดข้อผิดพลาดภายในระบบ", nil)
	e.cause = cause
	return e
}

// Paged is the common list envelope (§5.1).
type Paged[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
}

// MaxPageSize is the largest accepted pageSize.
const MaxPageSize = 100

// DefaultPageSize is used when pageSize is missing or invalid.
const DefaultPageSize = 20

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return page, size
}

// parseOID parses a hex ObjectID string.
func parseOID(hex string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(hex)
}

// Actor identifies the authenticated caller for audit records.
type Actor struct {
	ID        primitive.ObjectID
	Name      string
	Role      entity.Role
	IP        string
	UserAgent string
}

// Principal is the authenticated identity, loaded from the database on every
// request (BR-02). Nothing in it comes from the token except the subject.
type Principal struct {
	UserID     primitive.ObjectID
	Username   string
	FullName   string
	Role       entity.Role
	Status     entity.UserStatus
	HospitalID primitive.ObjectID
}

// HospitalRef is {id,name} (§5.3).
type HospitalRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func hospitalRef(h *entity.Hospital) *HospitalRef {
	if h == nil {
		return nil
	}
	return &HospitalRef{ID: h.ID.Hex(), Name: h.Name}
}

// UserRef is {id,fullName}.
type UserRef struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
}

func isNotFound(err error) bool { return errors.Is(err, port.ErrNotFound) }
