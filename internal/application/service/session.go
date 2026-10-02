package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SessionService handles session lifecycle use cases.
type SessionService struct {
	sessions  port.SessionRepository
	quizzes   port.QuizRepository
	hospitals port.HospitalRepository
	events    port.EventPort
}

func NewSessionService(
	sessions port.SessionRepository,
	quizzes port.QuizRepository,
	hospitals port.HospitalRepository,
	events port.EventPort,
) *SessionService {
	return &SessionService{sessions: sessions, quizzes: quizzes, hospitals: hospitals, events: events}
}

func generateSessionCode() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

// CreateSessionInput carries data needed to create a session.
type CreateSessionInput struct {
	QuizIDHex    string
	HospitalCode string
	HospitalID   string
	HostIDHex    string
	Role         string
	CallerHospID string // hospitalId from JWT (for non-admin fallback)
}

func (s *SessionService) Create(ctx context.Context, in CreateSessionInput) (*entity.Session, error) {
	quizID, err := parseOID(in.QuizIDHex)
	if err != nil {
		return nil, errBadRequest("invalid quiz id")
	}

	// Resolve hospital ID
	var hospID primitive.ObjectID
	switch {
	case in.HospitalCode != "":
		hosp, err := s.hospitals.FindByCode(ctx, strings.ToUpper(strings.TrimSpace(in.HospitalCode)))
		if err != nil {
			return nil, errBadRequest("hospital not found for code: " + in.HospitalCode)
		}
		hospID = hosp.ID
	case in.HospitalID != "":
		parsed, err := parseOID(in.HospitalID)
		if err != nil {
			return nil, errBadRequest("invalid hospital id")
		}
		hospID = parsed
	default:
		if in.Role == entity.RoleAdmin || in.Role == entity.RoleSuperAdmin {
			return nil, errBadRequest("admin must provide hospitalCode")
		}
		hospID, _ = parseOID(in.CallerHospID)
	}

	// Verify quiz exists
	if _, err := s.quizzes.FindByID(ctx, quizID); err != nil {
		return nil, errBadRequest("quiz not found")
	}

	hostID, _ := parseOID(in.HostIDHex)
	session := &entity.Session{
		Code:       generateSessionCode(),
		QuizID:     quizID,
		HostID:     hostID,
		HospitalID: hospID,
		Status:     entity.SessionPending,
		CreatedAt:  time.Now(),
	}
	id, err := s.sessions.Create(ctx, session)
	if err != nil {
		return nil, err
	}
	session.ID = id
	return session, nil
}

func (s *SessionService) Start(ctx context.Context, sessionIDHex string) (*entity.Session, error) {
	id, err := parseOID(sessionIDHex)
	if err != nil {
		return nil, ErrNotFound
	}
	session, err := s.sessions.UpdateToRunning(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	now := time.Now()
	s.events.Broadcast("session:"+session.ID.Hex(), port.Event{
		Type: "session:start",
		Payload: map[string]interface{}{
			"sessionId": session.ID.Hex(),
			"quizId":    session.QuizID.Hex(),
			"startedAt": now,
		},
	})
	s.events.Broadcast("hospital:"+session.HospitalID.Hex(), port.Event{
		Type: "session:start",
		Payload: map[string]interface{}{
			"sessionId": session.ID.Hex(),
			"quizId":    session.QuizID.Hex(),
			"startedAt": now,
		},
	})
	return session, nil
}

func (s *SessionService) End(ctx context.Context, sessionIDHex string) (*entity.Session, error) {
	id, err := parseOID(sessionIDHex)
	if err != nil {
		return nil, ErrNotFound
	}
	session, err := s.sessions.UpdateToEnded(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	now := time.Now()
	s.events.Broadcast("session:"+session.ID.Hex(), port.Event{
		Type: "session:end",
		Payload: map[string]interface{}{
			"sessionId": session.ID.Hex(),
			"endedAt":   now,
		},
	})
	return session, nil
}

func (s *SessionService) Get(ctx context.Context, sessionIDHex string) (*entity.Session, error) {
	id, err := parseOID(sessionIDHex)
	if err != nil {
		return nil, ErrNotFound
	}
	sess, err := s.sessions.FindByID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return sess, nil
}

func (s *SessionService) GetByCode(ctx context.Context, code string) (*entity.Session, error) {
	sess, err := s.sessions.FindByCode(ctx, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, ErrNotFound
	}
	return sess, nil
}

func (s *SessionService) ListActive(ctx context.Context, role, hospitalIDHex string) ([]entity.Session, error) {
	var hospID *primitive.ObjectID
	if role != entity.RoleAdmin && role != entity.RoleSuperAdmin && hospitalIDHex != "" {
		if oid, err := parseOID(hospitalIDHex); err == nil {
			hospID = &oid
		}
	}
	return s.sessions.ListActive(ctx, hospID)
}

// ──────────────────────────────────────────────────────────────────────────
// Internal helpers
// ──────────────────────────────────────────────────────────────────────────

type badRequestError struct{ msg string }

func (e *badRequestError) Error() string { return e.msg }

func errBadRequest(msg string) error { return &badRequestError{msg: msg} }

// IsBadRequest returns true if the error originated from a bad request.
func IsBadRequest(err error) bool {
	_, ok := err.(*badRequestError)
	return ok
}
