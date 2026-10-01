package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SubmitGrace is the extra time after ExpiresAt during which answers/submit
// are still accepted (BR-23).
const SubmitGrace = 30 * time.Second

// AttemptStatus is the lifecycle state of an attempt (BR-20).
type AttemptStatus string

const (
	AttemptInProgress AttemptStatus = "in_progress"
	AttemptSubmitted  AttemptStatus = "submitted"
	AttemptExpired    AttemptStatus = "expired"
)

// AttemptQuestion is one drawn question. CorrectType is a snapshot of the
// answer key and must never leave the server for role=user (BR-14).
type AttemptQuestion struct {
	ID          string             `bson:"id"          json:"id"`
	ImageID     primitive.ObjectID `bson:"imageId"     json:"-"`
	Path        string             `bson:"path"        json:"-"`
	CorrectType string             `bson:"correctType" json:"-"`
}

// TypeScore is the per-cell-type breakdown of a scored attempt (BR-33).
type TypeScore struct {
	Key     string  `bson:"key"     json:"key"`
	Label   string  `bson:"label"   json:"label"`
	Total   int     `bson:"total"   json:"total"`
	Correct int     `bson:"correct" json:"correct"`
	Percent float64 `bson:"percent" json:"percent"`
}

// Attempt is one try of a quiz by a user. It stores the drawn questions and
// parameter snapshots, so later quiz/pool edits never change it (BR-10c, BR-12).
type Attempt struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	QuizID     primitive.ObjectID `bson:"quizId"`
	UserID     primitive.ObjectID `bson:"userId"`
	HospitalID primitive.ObjectID `bson:"hospitalId"`
	AttemptNo  int                `bson:"attemptNo"`
	Status     AttemptStatus      `bson:"status"`

	QuizTitle     string `bson:"quizTitle"`
	HospitalName  string `bson:"hospitalName"`
	RecipientName string `bson:"recipientName"`

	StartedAt   time.Time  `bson:"startedAt"`
	ExpiresAt   time.Time  `bson:"expiresAt"`
	SubmittedAt *time.Time `bson:"submittedAt,omitempty"`

	QuestionCount int `bson:"questionCount"`
	PassPercent   int `bson:"passPercent"`
	DurationSec   int `bson:"durationSec"`
	MaxAttempts   int `bson:"maxAttempts"`

	CellTypes []CellTypeOption  `bson:"cellTypes"`
	Questions []AttemptQuestion `bson:"questions"`
	Answers   map[string]string `bson:"answers"`

	// Result fields, set only when Status == submitted.
	Correct *int        `bson:"correct,omitempty"`
	Total   int         `bson:"total"`
	Percent *float64    `bson:"percent,omitempty"`
	Passed  bool        `bson:"passed"`
	PerType []TypeScore `bson:"perType,omitempty"`

	CertificateID primitive.ObjectID `bson:"certificateId,omitempty"`
	CertNo        string             `bson:"certNo,omitempty"`
}

// Deadline is the last instant at which answers or a submit are accepted.
func (a *Attempt) Deadline() time.Time { return a.ExpiresAt.Add(SubmitGrace) }

// PastGrace reports whether the attempt can no longer be answered or submitted.
func (a *Attempt) PastGrace(now time.Time) bool { return now.After(a.Deadline()) }

// CountsAsUsed reports whether the attempt consumes one of the user's tries
// (BR-24): every status does.
func (a *Attempt) CountsAsUsed() bool {
	switch a.Status {
	case AttemptInProgress, AttemptSubmitted, AttemptExpired:
		return true
	}
	return false
}

// Question returns the drawn question with the given opaque id.
func (a *Attempt) Question(id string) (AttemptQuestion, bool) {
	for _, q := range a.Questions {
		if q.ID == id {
			return q, true
		}
	}
	return AttemptQuestion{}, false
}

// HasCellType reports whether key is one of the answer options snapshot at start (BR-15).
func (a *Attempt) HasCellType(key string) bool {
	for _, t := range a.CellTypes {
		if t.Key == key {
			return true
		}
	}
	return false
}
