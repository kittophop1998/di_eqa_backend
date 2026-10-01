package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// QuizSchemaVersion tags v2 documents in the shared "quizzes" collection so
// legacy documents (no "v" field) never show up in v2 queries.
const QuizSchemaVersion = 2

// QuizStatus is the publication state of a quiz (BR-07).
type QuizStatus string

const (
	QuizDraft     QuizStatus = "draft"
	QuizPublished QuizStatus = "published"
	QuizArchived  QuizStatus = "archived"
)

// DefaultQuestionCount is used when a create request omits questionCount.
const DefaultQuestionCount = 20

// MinDurationSec is the smallest allowed durationSec (BR-08).
const MinDurationSec = 60

// Quiz is a hospital-assignable quiz round (v2).
type Quiz struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	SchemaVersion int                `bson:"v"             json:"-"`
	Title         string             `bson:"title"         json:"title"`
	Description   string             `bson:"description"   json:"description"`
	QuestionCount int                `bson:"questionCount" json:"questionCount"`
	PassPercent   int                `bson:"passPercent"   json:"passPercent"`
	DurationSec   int                `bson:"durationSec"   json:"durationSec"`
	MaxAttempts   int                `bson:"maxAttempts"   json:"maxAttempts"`
	OpensAt       time.Time          `bson:"opensAt"       json:"opensAt"`
	ClosesAt      time.Time          `bson:"closesAt"      json:"closesAt"`
	Status        QuizStatus         `bson:"status"        json:"status"`
	CreatedBy     primitive.ObjectID `bson:"createdBy"     json:"createdBy"`
	CreatedAt     time.Time          `bson:"createdAt"     json:"createdAt"`
	UpdatedAt     time.Time          `bson:"updatedAt"     json:"updatedAt"`
}

// FieldIssue describes one invalid input field.
type FieldIssue struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// QuizParams are the editable quiz parameters validated by BR-08.
type QuizParams struct {
	Title         string
	QuestionCount int
	PassPercent   int
	DurationSec   int
	MaxAttempts   int
	OpensAt       time.Time
	ClosesAt      time.Time
}

// Validate checks BR-08 and returns one issue per invalid field.
func (p QuizParams) Validate() []FieldIssue {
	var out []FieldIssue
	if len(p.Title) == 0 {
		out = append(out, FieldIssue{"title", "is required"})
	}
	if len([]rune(p.Title)) > 200 {
		out = append(out, FieldIssue{"title", "must be at most 200 characters"})
	}
	if p.QuestionCount < 1 {
		out = append(out, FieldIssue{"questionCount", "must be an integer >= 1"})
	}
	if p.PassPercent < 1 || p.PassPercent > 100 {
		out = append(out, FieldIssue{"passPercent", "must be between 1 and 100"})
	}
	if p.DurationSec < MinDurationSec {
		out = append(out, FieldIssue{"durationSec", "must be >= 60"})
	}
	if p.MaxAttempts < 1 {
		out = append(out, FieldIssue{"maxAttempts", "must be an integer >= 1"})
	}
	if p.OpensAt.IsZero() {
		out = append(out, FieldIssue{"opensAt", "is required"})
	}
	if p.ClosesAt.IsZero() {
		out = append(out, FieldIssue{"closesAt", "is required"})
	}
	if !p.OpensAt.IsZero() && !p.ClosesAt.IsZero() && !p.ClosesAt.After(p.OpensAt) {
		out = append(out, FieldIssue{"closesAt", "must be after opensAt"})
	}
	return out
}

// AssignmentStatus is the per-hospital admin switch (BR-09).
type AssignmentStatus string

const (
	AssignmentScheduled AssignmentStatus = "scheduled"
	AssignmentOpen      AssignmentStatus = "open"
	AssignmentClosed    AssignmentStatus = "closed"
)

// HospitalAssignment links a quiz to a hospital and holds its Open/Close state.
type HospitalAssignment struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	QuizID     primitive.ObjectID `bson:"quizId"        json:"-"`
	HospitalID primitive.ObjectID `bson:"hospitalId"    json:"-"`
	Status     AssignmentStatus   `bson:"status"        json:"-"`
	OpenedAt   *time.Time         `bson:"openedAt,omitempty" json:"-"`
	ClosedAt   *time.Time         `bson:"closedAt,omitempty" json:"-"`
	CreatedAt  time.Time          `bson:"createdAt"     json:"-"`
}

// Availability is the computed, never-stored state a user sees (BR-10a).
type Availability string

const (
	AvailabilityOpen     Availability = "open"
	AvailabilityUpcoming Availability = "upcoming"
	AvailabilityClosed   Availability = "closed"
)
