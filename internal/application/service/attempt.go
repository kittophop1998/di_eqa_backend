package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/draw"
	"github.com/di-eqa/backend/internal/domain/eligibility"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"github.com/di-eqa/backend/internal/domain/scoring"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AttemptService implements the attempt lifecycle: start/resume, autosave,
// atomic submit, expiry, result and image streaming (B-03..B-06).
type AttemptService struct {
	quizzes     port.QuizRepository
	assignments port.AssignmentRepository
	attempts    port.AttemptRepository
	certs       port.CertificateRepository
	hospitals   port.HospitalRepository
	types       port.CellTypeRepository
	images      port.CellImageRepository
	store       applicationport.ImageStore
	rnd         applicationport.Randomness
	clock       applicationport.Clock
}

func NewAttemptService(
	q port.QuizRepository, a port.AssignmentRepository, at port.AttemptRepository,
	c port.CertificateRepository, h port.HospitalRepository, t port.CellTypeRepository,
	i port.CellImageRepository, store applicationport.ImageStore,
	rnd applicationport.Randomness, clock applicationport.Clock,
) *AttemptService {
	return &AttemptService{
		quizzes: q, assignments: a, attempts: at, certs: c, hospitals: h,
		types: t, images: i, store: store, rnd: rnd, clock: clock,
	}
}

// QuestionView is one question as shown before submit: opaque id, position
// and the opaque image endpoint. Nothing here reveals the answer (BR-14).
type QuestionView struct {
	ID       string `json:"id"`
	Index    int    `json:"index"` // 1-based
	ImageURL string `json:"imageUrl"`
}

// AttemptView is an in-progress attempt (§5.5).
type AttemptView struct {
	ID            string                  `json:"id"`
	QuizID        string                  `json:"quizId"`
	QuizTitle     string                  `json:"quizTitle"`
	Status        entity.AttemptStatus    `json:"status"`
	StartedAt     time.Time               `json:"startedAt"`
	ExpiresAt     time.Time               `json:"expiresAt"`
	ServerTime    time.Time               `json:"serverTime"`
	DurationSec   int                     `json:"durationSec"`
	QuestionCount int                     `json:"questionCount"`
	CellTypes     []entity.CellTypeOption `json:"cellTypes"`
	Questions     []QuestionView          `json:"questions"`
	Answers       map[string]string       `json:"answers"`
}

// CertBrief is the certificate summary inside a result.
type CertBrief struct {
	ID       string    `json:"id"`
	CertNo   string    `json:"certNo"`
	IssuedAt time.Time `json:"issuedAt"`
}

// AttemptResult is the post-submit view. It has no per-question data (BR-33).
type AttemptResult struct {
	AttemptID         string               `json:"attemptId"`
	QuizID            string               `json:"quizId"`
	QuizTitle         string               `json:"quizTitle"`
	Hospital          HospitalRef          `json:"hospital"`
	AttemptNo         int                  `json:"attemptNo"`
	Status            entity.AttemptStatus `json:"status"`
	StartedAt         time.Time            `json:"startedAt"`
	SubmittedAt       *time.Time           `json:"submittedAt"`
	Correct           *int                 `json:"correct"`
	Total             int                  `json:"total"`
	Percent           *float64             `json:"percent"`
	Unanswered        *int                 `json:"unanswered"`
	PassPercent       int                  `json:"passPercent"`
	Passed            bool                 `json:"passed"`
	PerType           []entity.TypeScore   `json:"perType"`
	AttemptsUsed      int                  `json:"attemptsUsed"`
	AttemptsRemaining int                  `json:"attemptsRemaining"`
	CanRetry          bool                 `json:"canRetry"`
	Certificate       *CertBrief           `json:"certificate"`
}

// AttemptState is the result of GET /me/attempts/{id}: exactly one field is set.
type AttemptState struct {
	View   *AttemptView
	Result *AttemptResult
}

// ---------------------------------------------------------------- start

// Start returns the caller's in-progress attempt (created=false) or draws a
// fresh one (created=true) (BR-21).
func (s *AttemptService) Start(ctx context.Context, p *Principal, quizIDHex string) (view *AttemptView, created bool, err error) {
	qid, perr := parseOID(quizIDHex)
	if perr != nil || p.HospitalID.IsZero() {
		return nil, false, notFound()
	}
	quiz, err := s.quizzes.FindByID(ctx, qid)
	if err != nil {
		return nil, false, mapNotFound(err)
	}
	if quiz.Status != entity.QuizPublished {
		return nil, false, notFound() // BR-05: users only see published quizzes
	}
	asg, err := s.assignments.Find(ctx, qid, p.HospitalID)
	if err != nil {
		return nil, false, mapNotFound(err) // other hospital's quiz => 404, not 403
	}

	for try := 0; try < 3; try++ {
		now := s.clock.Now()
		list, err := s.attempts.ListByUserQuizzes(ctx, p.UserID, []primitive.ObjectID{qid})
		if err != nil {
			return nil, false, internal(err)
		}
		used, passed := 0, false
		for i := range list {
			a := &list[i]
			if a.Status == entity.AttemptInProgress {
				if !a.PastGrace(now) {
					return s.view(a, now), false, nil // resume: same set, same order
				}
				// Stale attempt: close it so it stops blocking (still counts as used).
				if _, err := s.attempts.Expire(ctx, a.ID, now); err != nil && !errors.Is(err, port.ErrConflict) {
					return nil, false, internal(err)
				}
			}
			if a.CountsAsUsed() {
				used++
			}
			if a.Status == entity.AttemptSubmitted && a.Passed {
				passed = true
			}
		}

		avail := eligibility.Availability(quiz, asg.Status, now)
		if derr := eligibility.CanStart(eligibility.StartInput{
			Availability: avail, Passed: passed, AttemptsUsed: used, MaxAttempts: quiz.MaxAttempts,
		}); derr != nil {
			return nil, false, startError(derr, quiz, avail)
		}

		a, err := s.newAttempt(ctx, p, quiz, used+1, now)
		if err != nil {
			return nil, false, err
		}
		id, err := s.attempts.Create(ctx, a)
		if err == nil {
			a.ID = id
			return s.view(a, now), true, nil
		}
		if !errors.Is(err, port.ErrDuplicate) {
			return nil, false, internal(err)
		}
		// Lost a race with a concurrent start: loop, which resumes theirs.
	}
	return nil, false, internal(errors.New("could not start attempt after retries"))
}

func startError(derr error, quiz *entity.Quiz, avail entity.Availability) error {
	switch {
	case errors.Is(derr, eligibility.ErrNotOpen):
		reason := "closed"
		if avail == entity.AvailabilityUpcoming {
			reason = "upcoming"
		}
		return newErr(CodeQuizNotOpen, "แบบทดสอบนี้ยังไม่เปิดให้ทำ", map[string]any{
			"reason": reason, "opensAt": quiz.OpensAt, "closesAt": quiz.ClosesAt,
		})
	case errors.Is(derr, eligibility.ErrAlreadyPassed):
		return newErr(CodeAlreadyPassed, "คุณผ่านแบบทดสอบนี้แล้ว", nil)
	default:
		return newErr(CodeMaxAttempts, "คุณใช้สิทธิ์ทำแบบทดสอบครบแล้ว", map[string]any{"maxAttempts": quiz.MaxAttempts})
	}
}

// newAttempt draws the question set (pure random, BR-11) and snapshots
// parameters and answer options (BR-12, BR-15).
func (s *AttemptService) newAttempt(ctx context.Context, p *Principal, quiz *entity.Quiz, attemptNo int, now time.Time) (*entity.Attempt, error) {
	types, err := s.types.ListActive(ctx)
	if err != nil {
		return nil, internal(err)
	}
	keys := make([]string, 0, len(types))
	options := make([]entity.CellTypeOption, 0, len(types))
	for _, t := range types {
		keys = append(keys, t.Key)
		options = append(options, entity.CellTypeOption{Key: t.Key, Label: t.Label})
	}
	ids, err := s.images.ActiveIDs(ctx, keys)
	if err != nil {
		return nil, internal(err)
	}
	picked, err := draw.Sample(ids, quiz.QuestionCount, s.rnd.Intn)
	if err != nil {
		if errors.Is(err, draw.ErrPoolTooSmall) {
			return nil, newErr(CodePoolTooSmall, "ขณะนี้ยังไม่สามารถเริ่มทำแบบทดสอบได้ กรุณาติดต่อผู้ดูแลระบบ", nil)
		}
		return nil, internal(err)
	}
	docs, err := s.images.FindByIDs(ctx, picked)
	if err != nil {
		return nil, internal(err)
	}
	byID := make(map[primitive.ObjectID]entity.CellImage, len(docs))
	for _, d := range docs {
		byID[d.ID] = d
	}
	qids, err := draw.UniqueIDs(len(picked), s.rnd.QuestionID)
	if err != nil {
		return nil, internal(err)
	}
	questions := make([]entity.AttemptQuestion, 0, len(picked))
	for i, id := range picked {
		d, ok := byID[id]
		if !ok {
			return nil, internal(fmt.Errorf("drawn image %s vanished", id.Hex()))
		}
		questions = append(questions, entity.AttemptQuestion{ID: qids[i], ImageID: d.ID, Path: d.Path, CorrectType: d.TypeKey})
	}

	hospName := ""
	if h, err := s.hospitals.FindByID(ctx, p.HospitalID); err == nil {
		hospName = h.Name
	} else if !isNotFound(err) {
		return nil, internal(err)
	}
	return &entity.Attempt{
		QuizID: quiz.ID, UserID: p.UserID, HospitalID: p.HospitalID, AttemptNo: attemptNo,
		Status: entity.AttemptInProgress, QuizTitle: quiz.Title, HospitalName: hospName, RecipientName: p.FullName,
		StartedAt: now, ExpiresAt: now.Add(time.Duration(quiz.DurationSec) * time.Second), // server decides (BR-22)
		QuestionCount: quiz.QuestionCount, PassPercent: quiz.PassPercent,
		DurationSec: quiz.DurationSec, MaxAttempts: quiz.MaxAttempts,
		CellTypes: options, Questions: questions, Answers: map[string]string{}, Total: len(questions),
	}, nil
}

func (s *AttemptService) view(a *entity.Attempt, now time.Time) *AttemptView {
	qs := make([]QuestionView, 0, len(a.Questions))
	for i, q := range a.Questions {
		qs = append(qs, QuestionView{ID: q.ID, Index: i + 1, ImageURL: imagePath(a.ID, q.ID)})
	}
	ans := make(map[string]string, len(a.Answers))
	for k, v := range a.Answers {
		ans[k] = v
	}
	return &AttemptView{
		ID: a.ID.Hex(), QuizID: a.QuizID.Hex(), QuizTitle: a.QuizTitle, Status: a.Status,
		StartedAt: a.StartedAt, ExpiresAt: a.ExpiresAt, ServerTime: now.UTC(),
		DurationSec: a.DurationSec, QuestionCount: a.QuestionCount,
		CellTypes: a.CellTypes, Questions: qs, Answers: ans,
	}
}

func imagePath(attemptID primitive.ObjectID, qid string) string {
	return "/api/me/attempts/" + attemptID.Hex() + "/questions/" + qid + "/image"
}

// ---------------------------------------------------------------- access

// loadOwned loads an attempt the caller owns (BR-27) and lazily expires it
// when it is past the grace period (BR-23). Someone else's attempt is a 404.
func (s *AttemptService) loadOwned(ctx context.Context, p *Principal, idHex string, now time.Time) (*entity.Attempt, error) {
	id, err := parseOID(idHex)
	if err != nil {
		return nil, notFound()
	}
	a, err := s.attempts.FindByID(ctx, id)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if a.UserID != p.UserID {
		return nil, notFound()
	}
	if a.Status == entity.AttemptInProgress && a.PastGrace(now) {
		return s.expire(ctx, a, now)
	}
	return a, nil
}

func (s *AttemptService) expire(ctx context.Context, a *entity.Attempt, now time.Time) (*entity.Attempt, error) {
	upd, err := s.attempts.Expire(ctx, a.ID, now)
	if err == nil {
		return upd, nil
	}
	if !errors.Is(err, port.ErrConflict) {
		return nil, internal(err)
	}
	cur, ferr := s.attempts.FindByID(ctx, a.ID) // someone else transitioned it
	if ferr != nil {
		return nil, mapNotFound(ferr)
	}
	return cur, nil
}

// Get returns AttemptView while in progress, AttemptResult afterwards.
func (s *AttemptService) Get(ctx context.Context, p *Principal, idHex string) (*AttemptState, error) {
	now := s.clock.Now()
	a, err := s.loadOwned(ctx, p, idHex, now)
	if err != nil {
		return nil, err
	}
	if a.Status == entity.AttemptInProgress {
		return &AttemptState{View: s.view(a, now)}, nil
	}
	res, err := s.result(ctx, a, now)
	if err != nil {
		return nil, err
	}
	return &AttemptState{Result: res}, nil
}

// Result returns the result of a finished attempt; 409 while in progress.
func (s *AttemptService) Result(ctx context.Context, p *Principal, idHex string) (*AttemptResult, error) {
	now := s.clock.Now()
	a, err := s.loadOwned(ctx, p, idHex, now)
	if err != nil {
		return nil, err
	}
	if a.Status == entity.AttemptInProgress {
		return nil, newErr(CodeAttemptNotSubmit, "ยังไม่ได้ส่งแบบทดสอบ", nil)
	}
	return s.result(ctx, a, now)
}

// ---------------------------------------------------------------- autosave

// SaveOutput is the PUT answers response.
type SaveOutput struct {
	SavedAt       time.Time `json:"savedAt"`
	AnsweredCount int       `json:"answeredCount"`
	ExpiresAt     time.Time `json:"expiresAt"`
	ServerTime    time.Time `json:"serverTime"`
}

// validateAnswers checks question ids and answer keys against the attempt and
// splits the update into set/clear parts (BR-15, BR-26). A nil value clears.
func validateAnswers(a *entity.Attempt, in map[string]*string) (set map[string]string, clear []string, err error) {
	var issues []entity.FieldIssue
	set = map[string]string{}
	for qid, v := range in {
		if _, ok := a.Question(qid); !ok {
			issues = append(issues, entity.FieldIssue{Field: "answers." + qid, Issue: "unknown question id"})
			continue
		}
		if v == nil || *v == "" {
			clear = append(clear, qid)
			continue
		}
		if !a.HasCellType(*v) {
			issues = append(issues, entity.FieldIssue{Field: "answers." + qid, Issue: "unknown cell type key"})
			continue
		}
		set[qid] = *v
	}
	if len(issues) > 0 {
		return nil, nil, validation(issues...)
	}
	return set, clear, nil
}

// notAnswerable maps a non-answerable attempt to the right 409 (BR-23, BR-26).
func notAnswerable(a *entity.Attempt) error {
	if a.Status == entity.AttemptExpired {
		return newErr(CodeAttemptExpired, "หมดเวลาทำแบบทดสอบแล้ว", nil)
	}
	return newErr(CodeAttemptNotInProg, "แบบทดสอบนี้ถูกส่งหรือปิดไปแล้ว", nil)
}

// SaveAnswers merges answers into an in-progress attempt.
func (s *AttemptService) SaveAnswers(ctx context.Context, p *Principal, idHex string, in map[string]*string) (*SaveOutput, error) {
	now := s.clock.Now()
	a, err := s.loadOwned(ctx, p, idHex, now)
	if err != nil {
		return nil, err
	}
	if a.Status != entity.AttemptInProgress {
		return nil, notAnswerable(a)
	}
	set, clear, err := validateAnswers(a, in)
	if err != nil {
		return nil, err
	}
	cur := a
	if len(set) > 0 || len(clear) > 0 {
		cur, err = s.attempts.MergeAnswers(ctx, a.ID, set, clear, now)
		if err != nil {
			if errors.Is(err, port.ErrConflict) {
				latest, ferr := s.attempts.FindByID(ctx, a.ID)
				if ferr != nil {
					return nil, mapNotFound(ferr)
				}
				if latest.Status == entity.AttemptInProgress { // grace passed while saving
					if latest, ferr = s.expire(ctx, latest, now); ferr != nil {
						return nil, ferr
					}
				}
				return nil, notAnswerable(latest)
			}
			return nil, internal(err)
		}
	}
	return &SaveOutput{SavedAt: now.UTC(), AnsweredCount: len(cur.Answers), ExpiresAt: cur.ExpiresAt, ServerTime: now.UTC()}, nil
}

// ---------------------------------------------------------------- submit

// Submit merges any final answers, scores and atomically closes the attempt.
// Repeating it on a submitted attempt returns the stored result (BR-25).
func (s *AttemptService) Submit(ctx context.Context, p *Principal, idHex string, in map[string]*string) (*AttemptResult, error) {
	now := s.clock.Now()
	a, err := s.loadOwned(ctx, p, idHex, now)
	if err != nil {
		return nil, err
	}
	switch a.Status {
	case entity.AttemptSubmitted:
		return s.result(ctx, a, now)
	case entity.AttemptExpired:
		return nil, notAnswerable(a)
	}

	set, clear, err := validateAnswers(a, in)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]string, len(a.Answers)+len(set))
	for k, v := range a.Answers {
		merged[k] = v
	}
	for k, v := range set {
		merged[k] = v
	}
	for _, k := range clear {
		delete(merged, k)
	}

	sc := scoring.Score(a.Questions, merged, a.CellTypes, a.PassPercent)
	updated, err := s.attempts.Submit(ctx, a.ID, port.SubmitResult{
		Answers: merged, SubmittedAt: now, Correct: sc.Correct, Total: sc.Total,
		Percent: sc.Percent, Passed: sc.Passed, PerType: sc.PerType,
	}, now)
	if err != nil {
		if !errors.Is(err, port.ErrConflict) {
			return nil, internal(err)
		}
		// Another request already moved it: report its outcome.
		latest, ferr := s.attempts.FindByID(ctx, a.ID)
		if ferr != nil {
			return nil, mapNotFound(ferr)
		}
		if latest.Status == entity.AttemptSubmitted {
			return s.result(ctx, latest, now)
		}
		if latest.Status == entity.AttemptInProgress {
			if latest, ferr = s.expire(ctx, latest, now); ferr != nil {
				return nil, ferr
			}
		}
		return nil, notAnswerable(latest)
	}
	return s.result(ctx, updated, now)
}

// ---------------------------------------------------------------- result

// result builds AttemptResult, issuing a missing certificate for a passed
// attempt first (idempotent self-heal, BR-25, BR-36).
func (s *AttemptService) result(ctx context.Context, a *entity.Attempt, now time.Time) (*AttemptResult, error) {
	var cert *entity.Certificate
	if a.Status == entity.AttemptSubmitted && a.Passed {
		c, err := s.ensureCertificate(ctx, a)
		if err != nil {
			return nil, err
		}
		cert = c
	}

	res := &AttemptResult{
		AttemptID: a.ID.Hex(), QuizID: a.QuizID.Hex(), QuizTitle: a.QuizTitle,
		Hospital:  HospitalRef{ID: a.HospitalID.Hex(), Name: a.HospitalName},
		AttemptNo: a.AttemptNo, Status: a.Status, StartedAt: a.StartedAt, SubmittedAt: a.SubmittedAt,
		Total: a.Total, PassPercent: a.PassPercent, Passed: a.Status == entity.AttemptSubmitted && a.Passed,
		PerType: []entity.TypeScore{},
	}
	if a.Status == entity.AttemptSubmitted {
		res.Correct, res.Percent = a.Correct, a.Percent
		answered := 0
		for _, q := range a.Questions {
			if v, ok := a.Answers[q.ID]; ok && v != "" {
				answered++
			}
		}
		un := a.Total - answered
		res.Unanswered = &un
		if a.PerType != nil {
			res.PerType = a.PerType
		}
	}
	if cert != nil {
		res.Certificate = &CertBrief{ID: cert.ID.Hex(), CertNo: cert.CertNo, IssuedAt: cert.IssuedAt}
	}

	// Attempts left and retry flag use the live quiz, since a new attempt would.
	maxAttempts := a.MaxAttempts
	avail := entity.AvailabilityClosed
	if quiz, err := s.quizzes.FindByID(ctx, a.QuizID); err == nil {
		maxAttempts = quiz.MaxAttempts
		if quiz.Status == entity.QuizPublished {
			if asg, err := s.assignments.Find(ctx, a.QuizID, a.HospitalID); err == nil {
				avail = eligibility.Availability(quiz, asg.Status, now)
			} else if !isNotFound(err) {
				return nil, internal(err)
			}
		}
	} else if !isNotFound(err) {
		return nil, internal(err)
	}
	all, err := s.attempts.ListByUserQuizzes(ctx, a.UserID, []primitive.ObjectID{a.QuizID})
	if err != nil {
		return nil, internal(err)
	}
	used, passedAny := 0, false
	for i := range all {
		if all[i].CountsAsUsed() {
			used++
		}
		if all[i].Status == entity.AttemptSubmitted && all[i].Passed {
			passedAny = true
		}
	}
	res.AttemptsUsed = used
	res.AttemptsRemaining = eligibility.Remaining(maxAttempts, used)
	res.CanRetry = eligibility.CanRetry(passedAny, res.AttemptsRemaining, avail)
	return res, nil
}

// certNoRetries bounds certNo collision retries (BR-37).
const certNoRetries = 5

// ensureCertificate returns the certificate of a passed attempt, creating it
// if needed. It is safe to call repeatedly and concurrently: attemptId and
// (quizId,userId) are unique, so at most one certificate exists.
func (s *AttemptService) ensureCertificate(ctx context.Context, a *entity.Attempt) (*entity.Certificate, error) {
	existing, err := s.certs.FindByAttempt(ctx, a.ID)
	if err == nil {
		return s.link(ctx, a, existing)
	}
	if !isNotFound(err) {
		return nil, internal(err)
	}
	issuedAt := s.clock.Now()
	if a.SubmittedAt != nil {
		issuedAt = *a.SubmittedAt
	}
	percent := 0.0
	if a.Percent != nil {
		percent = *a.Percent
	}
	for i := 0; i < certNoRetries; i++ {
		no, err := s.rnd.CertNo(issuedAt)
		if err != nil {
			return nil, internal(err)
		}
		c := &entity.Certificate{
			CertNo: no, AttemptID: a.ID, QuizID: a.QuizID, UserID: a.UserID, HospitalID: a.HospitalID,
			RecipientName: a.RecipientName, HospitalName: a.HospitalName, QuizTitle: a.QuizTitle,
			Percent: percent, IssuedAt: issuedAt,
		}
		id, err := s.certs.Create(ctx, c)
		if err == nil {
			c.ID = id
			return s.link(ctx, a, c)
		}
		if !errors.Is(err, port.ErrDuplicate) {
			return nil, internal(err)
		}
		// Duplicate: a concurrent request may have won, or certNo collided.
		if won, ferr := s.certs.FindByAttempt(ctx, a.ID); ferr == nil {
			return s.link(ctx, a, won)
		} else if !isNotFound(ferr) {
			return nil, internal(ferr)
		}
		if won, ferr := s.certs.FindByQuizUser(ctx, a.QuizID, a.UserID); ferr == nil {
			return won, nil
		} else if !isNotFound(ferr) {
			return nil, internal(ferr)
		}
	}
	return nil, internal(errors.New("could not allocate a unique certificate number"))
}

func (s *AttemptService) link(ctx context.Context, a *entity.Attempt, c *entity.Certificate) (*entity.Certificate, error) {
	if a.CertificateID != c.ID {
		if err := s.attempts.SetCertificate(ctx, a.ID, c.ID, c.CertNo); err != nil {
			return nil, internal(err)
		}
		a.CertificateID, a.CertNo = c.ID, c.CertNo
	}
	return c, nil
}

// ---------------------------------------------------------------- image

// Image returns the bytes of one question image. It is available only to the
// owner of an in-progress attempt before expiresAt; anything else is a 404
// (BR-14). The response never carries the cell type or the storage path.
func (s *AttemptService) Image(ctx context.Context, p *Principal, idHex, qid string) (*applicationport.Image, error) {
	now := s.clock.Now()
	a, err := s.loadOwned(ctx, p, idHex, now)
	if err != nil {
		return nil, err
	}
	if !eligibility.ImageAccess(a, now) {
		return nil, notFound()
	}
	q, ok := a.Question(qid)
	if !ok {
		return nil, notFound()
	}
	img, err := s.store.Get(ctx, q.Path)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return nil, internal(fmt.Errorf("image missing in storage for attempt %s", a.ID.Hex()))
		}
		return nil, internal(err)
	}
	return img, nil
}

// ---------------------------------------------------------------- history

// HistoryItem is one row of GET /me/attempts (no per-question data).
type HistoryItem struct {
	ID          string               `json:"id"`
	QuizID      string               `json:"quizId"`
	QuizTitle   string               `json:"quizTitle"`
	AttemptNo   int                  `json:"attemptNo"`
	Status      entity.AttemptStatus `json:"status"`
	StartedAt   time.Time            `json:"startedAt"`
	SubmittedAt *time.Time           `json:"submittedAt"`
	Percent     *float64             `json:"percent"`
	Passed      bool                 `json:"passed"`
	CertNo      *string              `json:"certNo"`
}

// History lists the caller's attempts, newest first.
func (s *AttemptService) History(ctx context.Context, p *Principal, quizIDHex string, page, pageSize int) (*Paged[HistoryItem], error) {
	var quizID *primitive.ObjectID
	if quizIDHex != "" {
		id, err := parseOID(quizIDHex)
		if err != nil {
			return nil, validationOne("quizId", "invalid id")
		}
		quizID = &id
	}
	page, size := normalizePage(page, pageSize)
	list, total, err := s.attempts.ListByUser(ctx, p.UserID, quizID, port.Page{Page: page, PageSize: size})
	if err != nil {
		return nil, internal(err)
	}
	items := make([]HistoryItem, 0, len(list))
	for _, a := range list {
		it := HistoryItem{
			ID: a.ID.Hex(), QuizID: a.QuizID.Hex(), QuizTitle: a.QuizTitle, AttemptNo: a.AttemptNo,
			Status: a.Status, StartedAt: a.StartedAt, SubmittedAt: a.SubmittedAt,
			Percent: a.Percent, Passed: a.Status == entity.AttemptSubmitted && a.Passed,
		}
		if a.CertNo != "" {
			c := a.CertNo
			it.CertNo = &c
		}
		items = append(items, it)
	}
	return &Paged[HistoryItem]{Items: items, Page: page, PageSize: size, Total: total}, nil
}
