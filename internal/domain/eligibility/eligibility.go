// Package eligibility holds the pure availability and start-eligibility rules
// (BR-10a, BR-21, BR-34).
package eligibility

import (
	"errors"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
)

// Reasons a user may not start a new attempt.
var (
	ErrNotOpen       = errors.New("quiz is not open")
	ErrAlreadyPassed = errors.New("quiz already passed")
	ErrMaxAttempts   = errors.New("max attempts reached")
)

// Availability computes the user-facing availability of a quiz for a hospital.
// It is derived on every call and never stored (BR-10a).
//
//	closed   : quiz archived, assignment closed, or now > closesAt
//	open     : quiz published AND assignment open AND opensAt <= now <= closesAt
//	upcoming : everything else
func Availability(quiz *entity.Quiz, assignment entity.AssignmentStatus, now time.Time) entity.Availability {
	if quiz.Status == entity.QuizArchived || assignment == entity.AssignmentClosed || now.After(quiz.ClosesAt) {
		return entity.AvailabilityClosed
	}
	if quiz.Status == entity.QuizPublished && assignment == entity.AssignmentOpen && !now.Before(quiz.OpensAt) {
		return entity.AvailabilityOpen
	}
	return entity.AvailabilityUpcoming
}

// StartInput is what CanStart needs to decide (BR-21).
type StartInput struct {
	Availability entity.Availability
	Passed       bool
	AttemptsUsed int
	MaxAttempts  int
}

// CanStart decides whether a brand-new attempt may be created. It does not
// cover resuming an in-progress attempt, which is handled before this check.
func CanStart(in StartInput) error {
	if in.Availability != entity.AvailabilityOpen {
		return ErrNotOpen
	}
	if in.Passed {
		return ErrAlreadyPassed
	}
	if in.AttemptsUsed >= in.MaxAttempts {
		return ErrMaxAttempts
	}
	return nil
}

// Remaining is max(0, maxAttempts - used) (BR-24).
func Remaining(maxAttempts, used int) int {
	if r := maxAttempts - used; r > 0 {
		return r
	}
	return 0
}

// CanRetry mirrors the AttemptResult.canRetry rule: !passed && remaining>0 && open.
func CanRetry(passed bool, remaining int, avail entity.Availability) bool {
	return !passed && remaining > 0 && avail == entity.AvailabilityOpen
}

// CanOpenAssignment checks BR-09 for the admin Open action.
func CanOpenAssignment(quiz *entity.Quiz, now time.Time) error {
	if quiz.Status != entity.QuizPublished {
		return ErrQuizNotPublished
	}
	if now.After(quiz.ClosesAt) {
		return ErrWindowEnded
	}
	return nil
}

// Errors for CanOpenAssignment.
var (
	ErrQuizNotPublished = errors.New("quiz is not published")
	ErrWindowEnded      = errors.New("quiz window has ended")
)

// ImageAccess reports whether an attempt's images may still be served (BR-14):
// in progress and not past expiresAt (no grace for images).
func ImageAccess(a *entity.Attempt, now time.Time) bool {
	return a.Status == entity.AttemptInProgress && !now.After(a.ExpiresAt)
}
