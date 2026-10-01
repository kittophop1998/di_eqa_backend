package service

import (
	"context"
	"sort"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/eligibility"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MyQuizService implements the user's quiz catalog (B-07). Every query is
// scoped to the hospital stored on the caller's user document (BR-04, BR-05).
type MyQuizService struct {
	quizzes     port.QuizRepository
	assignments port.AssignmentRepository
	attempts    port.AttemptRepository
	certs       port.CertificateRepository
	clock       applicationport.Clock
}

func NewMyQuizService(
	q port.QuizRepository, a port.AssignmentRepository, at port.AttemptRepository,
	c port.CertificateRepository, clock applicationport.Clock,
) *MyQuizService {
	return &MyQuizService{quizzes: q, assignments: a, attempts: at, certs: c, clock: clock}
}

// InProgressRef points at the caller's running attempt.
type InProgressRef struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// BestResult is the caller's best submitted attempt (BR-35).
type BestResult struct {
	AttemptID string  `json:"attemptId"`
	Percent   float64 `json:"percent"`
	Passed    bool    `json:"passed"`
}

// CertRef is the {id,certNo} pair shown on cards.
type CertRef struct {
	ID     string `json:"id"`
	CertNo string `json:"certNo"`
}

// QuizCard is what a user sees for one quiz (§5.5).
type QuizCard struct {
	ID                string              `json:"id"`
	Title             string              `json:"title"`
	Description       *string             `json:"description"`
	QuestionCount     int                 `json:"questionCount"`
	PassPercent       int                 `json:"passPercent"`
	DurationSec       int                 `json:"durationSec"`
	MaxAttempts       int                 `json:"maxAttempts"`
	OpensAt           time.Time           `json:"opensAt"`
	ClosesAt          time.Time           `json:"closesAt"`
	Availability      entity.Availability `json:"availability"`
	AttemptsUsed      int                 `json:"attemptsUsed"`
	AttemptsRemaining int                 `json:"attemptsRemaining"`
	InProgressAttempt *InProgressRef      `json:"inProgressAttempt"`
	BestResult        *BestResult         `json:"bestResult"`
	Passed            bool                `json:"passed"`
	Certificate       *CertRef            `json:"certificate"`
}

// QuizListOutput is GET /me/quizzes.
type QuizListOutput struct {
	ServerTime time.Time  `json:"serverTime"`
	Items      []QuizCard `json:"items"`
}

// buildCard is pure: it derives a card from already-loaded facts.
func buildCard(q *entity.Quiz, assignment entity.AssignmentStatus, attempts []entity.Attempt, cert *entity.Certificate, now time.Time) QuizCard {
	used := 0
	passed := false
	var inProg *entity.Attempt
	var best *entity.Attempt
	for i := range attempts {
		a := &attempts[i]
		if a.CountsAsUsed() {
			used++
		}
		switch a.Status {
		case entity.AttemptInProgress:
			if !a.PastGrace(now) {
				inProg = a
			}
		case entity.AttemptSubmitted:
			if a.Passed {
				passed = true
			}
			if a.Percent != nil {
				if best == nil || *a.Percent > *best.Percent ||
					(*a.Percent == *best.Percent && a.StartedAt.After(best.StartedAt)) {
					best = a
				}
			}
		}
	}
	card := QuizCard{
		ID: q.ID.Hex(), Title: q.Title, QuestionCount: q.QuestionCount, PassPercent: q.PassPercent,
		DurationSec: q.DurationSec, MaxAttempts: q.MaxAttempts, OpensAt: q.OpensAt, ClosesAt: q.ClosesAt,
		Availability: eligibility.Availability(q, assignment, now),
		AttemptsUsed: used, AttemptsRemaining: eligibility.Remaining(q.MaxAttempts, used),
		Passed: passed,
	}
	if q.Description != "" {
		d := q.Description
		card.Description = &d
	}
	if inProg != nil {
		card.InProgressAttempt = &InProgressRef{ID: inProg.ID.Hex(), ExpiresAt: inProg.ExpiresAt}
	}
	if best != nil {
		card.BestResult = &BestResult{AttemptID: best.ID.Hex(), Percent: *best.Percent, Passed: best.Passed}
	}
	if cert != nil {
		card.Certificate = &CertRef{ID: cert.ID.Hex(), CertNo: cert.CertNo}
	}
	return card
}

// List returns published quizzes assigned to the caller's hospital. A caller
// without a hospital gets an empty list, never another hospital's quizzes.
func (s *MyQuizService) List(ctx context.Context, p *Principal) (*QuizListOutput, error) {
	now := s.clock.Now()
	out := &QuizListOutput{ServerTime: now.UTC(), Items: []QuizCard{}}
	if p.HospitalID.IsZero() {
		return out, nil
	}
	as, err := s.assignments.ListByHospital(ctx, p.HospitalID)
	if err != nil {
		return nil, internal(err)
	}
	if len(as) == 0 {
		return out, nil
	}
	status := map[primitive.ObjectID]entity.AssignmentStatus{}
	ids := make([]primitive.ObjectID, 0, len(as))
	for _, a := range as {
		status[a.QuizID] = a.Status
		ids = append(ids, a.QuizID)
	}
	quizzes, err := s.quizzes.FindByIDs(ctx, ids)
	if err != nil {
		return nil, internal(err)
	}
	published := quizzes[:0:0]
	pids := make([]primitive.ObjectID, 0, len(quizzes))
	for _, q := range quizzes {
		if q.Status == entity.QuizPublished {
			published = append(published, q)
			pids = append(pids, q.ID)
		}
	}
	if len(published) == 0 {
		return out, nil
	}
	attempts, err := s.attempts.ListByUserQuizzes(ctx, p.UserID, pids)
	if err != nil {
		return nil, internal(err)
	}
	certs, err := s.certs.ListByUserQuizzes(ctx, p.UserID, pids)
	if err != nil {
		return nil, internal(err)
	}
	byQuiz := map[primitive.ObjectID][]entity.Attempt{}
	for _, a := range attempts {
		byQuiz[a.QuizID] = append(byQuiz[a.QuizID], a)
	}
	certBy := map[primitive.ObjectID]*entity.Certificate{}
	for i := range certs {
		certBy[certs[i].QuizID] = &certs[i]
	}
	for i := range published {
		q := &published[i]
		out.Items = append(out.Items, buildCard(q, status[q.ID], byQuiz[q.ID], certBy[q.ID], now))
	}
	rank := map[entity.Availability]int{entity.AvailabilityOpen: 0, entity.AvailabilityUpcoming: 1, entity.AvailabilityClosed: 2}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if rank[a.Availability] != rank[b.Availability] {
			return rank[a.Availability] < rank[b.Availability]
		}
		return a.OpensAt.After(b.OpensAt)
	})
	return out, nil
}

// Get returns one card, or 404 when the quiz is unknown, unpublished or not
// assigned to the caller's hospital (BR-05).
func (s *MyQuizService) Get(ctx context.Context, p *Principal, quizIDHex string) (*QuizCard, error) {
	qid, err := parseOID(quizIDHex)
	if err != nil || p.HospitalID.IsZero() {
		return nil, notFound()
	}
	q, err := s.quizzes.FindByID(ctx, qid)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if q.Status != entity.QuizPublished {
		return nil, notFound()
	}
	a, err := s.assignments.Find(ctx, qid, p.HospitalID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	attempts, err := s.attempts.ListByUserQuizzes(ctx, p.UserID, []primitive.ObjectID{qid})
	if err != nil {
		return nil, internal(err)
	}
	cert, err := s.certs.FindByQuizUser(ctx, qid, p.UserID)
	if err != nil && !isNotFound(err) {
		return nil, internal(err)
	}
	card := buildCard(q, a.Status, attempts, cert, s.clock.Now())
	return &card, nil
}
