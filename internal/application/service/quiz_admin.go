package service

import (
	"context"
	"errors"
	"strings"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/eligibility"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// QuizAdminService implements admin quiz management, assignments and the
// per-hospital Open/Close switch (C-03, C-04).
type QuizAdminService struct {
	quizzes     port.QuizRepository
	assignments port.AssignmentRepository
	hospitals   port.HospitalRepository
	users       port.UserRepository
	attempts    port.AttemptRepository
	types       port.CellTypeRepository
	images      port.CellImageRepository
	audit       *AuditService
	clock       applicationport.Clock
}

func NewQuizAdminService(
	q port.QuizRepository, a port.AssignmentRepository, h port.HospitalRepository,
	u port.UserRepository, at port.AttemptRepository, t port.CellTypeRepository,
	i port.CellImageRepository, audit *AuditService, clock applicationport.Clock,
) *QuizAdminService {
	return &QuizAdminService{quizzes: q, assignments: a, hospitals: h, users: u, attempts: at, types: t, images: i, audit: audit, clock: clock}
}

// AdminAssignment is one hospital row of the quiz detail table (§5.6).
type AdminAssignment struct {
	Hospital     HospitalRef             `json:"hospital"`
	Status       entity.AssignmentStatus `json:"status"`
	Availability entity.Availability     `json:"availability"`
	OpenedAt     *time.Time              `json:"openedAt"`
	ClosedAt     *time.Time              `json:"closedAt"`
	Stats        AssignmentStats         `json:"stats"`
}

// AssignmentStats are the per-hospital counters.
type AssignmentStats struct {
	EligibleUsers int `json:"eligibleUsers"`
	Started       int `json:"started"`
	Submitted     int `json:"submitted"`
	Passed        int `json:"passed"`
}

// AdminQuiz is the admin view of a quiz (§5.6).
type AdminQuiz struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Description   *string           `json:"description"`
	QuestionCount int               `json:"questionCount"`
	PassPercent   int               `json:"passPercent"`
	DurationSec   int               `json:"durationSec"`
	MaxAttempts   int               `json:"maxAttempts"`
	OpensAt       time.Time         `json:"opensAt"`
	ClosesAt      time.Time         `json:"closesAt"`
	Status        entity.QuizStatus `json:"status"`
	Assignments   []AdminAssignment `json:"assignments"`
	CreatedBy     UserRef           `json:"createdBy"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// QuizInput is the create/update payload (§5.6). Times are RFC3339 strings so
// parse problems are reported as field issues.
type QuizInput struct {
	Title         string
	Description   string
	QuestionCount *int
	PassPercent   int
	DurationSec   int
	MaxAttempts   int
	OpensAt       string
	ClosesAt      string
	HospitalIDs   []string // create only
}

func parseTime(field, v string) (time.Time, *entity.FieldIssue) {
	if strings.TrimSpace(v) == "" {
		return time.Time{}, &entity.FieldIssue{Field: field, Issue: "is required"}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, &entity.FieldIssue{Field: field, Issue: "must be an RFC3339 timestamp"}
	}
	return t.UTC().Truncate(time.Millisecond), nil
}

func (in QuizInput) params() (entity.QuizParams, []entity.FieldIssue) {
	var issues []entity.FieldIssue
	opens, i1 := parseTime("opensAt", in.OpensAt)
	closes, i2 := parseTime("closesAt", in.ClosesAt)
	if i1 != nil {
		issues = append(issues, *i1)
	}
	if i2 != nil {
		issues = append(issues, *i2)
	}
	qc := entity.DefaultQuestionCount
	if in.QuestionCount != nil {
		qc = *in.QuestionCount
	}
	p := entity.QuizParams{
		Title: strings.TrimSpace(in.Title), QuestionCount: qc, PassPercent: in.PassPercent,
		DurationSec: in.DurationSec, MaxAttempts: in.MaxAttempts, OpensAt: opens, ClosesAt: closes,
	}
	if len(issues) == 0 {
		issues = append(issues, p.Validate()...)
	} else {
		// Time parse failed: still report the other field problems.
		for _, is := range p.Validate() {
			if is.Field != "opensAt" && is.Field != "closesAt" {
				issues = append(issues, is)
			}
		}
	}
	return p, issues
}

// resolveHospitals validates hospital ids (deduplicated, must be active).
func (s *QuizAdminService) resolveHospitals(ctx context.Context, ids []string) ([]entity.Hospital, *Error, error) {
	seen := map[primitive.ObjectID]bool{}
	var oids []primitive.ObjectID
	for _, h := range ids {
		id, err := parseOID(h)
		if err != nil {
			return nil, validationOne("hospitalIds", "contains an invalid id"), nil
		}
		if !seen[id] {
			seen[id] = true
			oids = append(oids, id)
		}
	}
	if len(oids) == 0 {
		return nil, nil, nil
	}
	list, err := s.hospitals.FindByIDs(ctx, oids)
	if err != nil {
		return nil, nil, internal(err)
	}
	if len(list) != len(oids) {
		return nil, validationOne("hospitalIds", "contains an unknown hospital"), nil
	}
	for _, h := range list {
		if !h.Active {
			return nil, validationOne("hospitalIds", "contains an inactive hospital"), nil
		}
	}
	return list, nil, nil
}

// Create makes a draft quiz with scheduled assignments (BR-07..09).
func (s *QuizAdminService) Create(ctx context.Context, in QuizInput, actor Actor) (*AdminQuiz, error) {
	p, issues := in.params()
	if len(issues) > 0 {
		return nil, validation(issues...)
	}
	hospitals, verr, err := s.resolveHospitals(ctx, in.HospitalIDs)
	if err != nil {
		return nil, err
	}
	if verr != nil {
		return nil, verr
	}
	now := s.clock.Now()
	q := &entity.Quiz{
		SchemaVersion: entity.QuizSchemaVersion,
		Title:         p.Title, Description: strings.TrimSpace(in.Description),
		QuestionCount: p.QuestionCount, PassPercent: p.PassPercent, DurationSec: p.DurationSec,
		MaxAttempts: p.MaxAttempts, OpensAt: p.OpensAt, ClosesAt: p.ClosesAt,
		Status: entity.QuizDraft, CreatedBy: actor.ID, CreatedAt: now, UpdatedAt: now,
	}
	id, err := s.quizzes.Create(ctx, q)
	if err != nil {
		return nil, internal(err)
	}
	q.ID = id
	if len(hospitals) > 0 {
		as := make([]entity.HospitalAssignment, 0, len(hospitals))
		for _, h := range hospitals {
			as = append(as, entity.HospitalAssignment{QuizID: id, HospitalID: h.ID, Status: entity.AssignmentScheduled, CreatedAt: now})
		}
		if err := s.assignments.CreateMany(ctx, as); err != nil {
			return nil, internal(err)
		}
	}
	s.audit.Log(ctx, entity.AuditActionQuizCreate, actor, id, q.Title, map[string]any{"hospitals": len(hospitals)})
	return s.get(ctx, q)
}

// Update edits parameters. Allowed for draft and published quizzes; running
// attempts keep their snapshots (BR-10c).
func (s *QuizAdminService) Update(ctx context.Context, idHex string, in QuizInput, actor Actor) (*AdminQuiz, error) {
	q, err := s.load(ctx, idHex)
	if err != nil {
		return nil, err
	}
	if q.Status == entity.QuizArchived {
		return nil, invalidState("แบบทดสอบที่เก็บถาวรแล้วไม่สามารถแก้ไขได้")
	}
	p, issues := in.params()
	if len(issues) > 0 {
		return nil, validation(issues...)
	}
	if q.Status == entity.QuizPublished && p.QuestionCount != q.QuestionCount {
		if perr := s.checkPool(ctx, p.QuestionCount); perr != nil {
			return nil, perr
		}
	}
	q.Title, q.Description = p.Title, strings.TrimSpace(in.Description)
	q.QuestionCount, q.PassPercent, q.DurationSec, q.MaxAttempts = p.QuestionCount, p.PassPercent, p.DurationSec, p.MaxAttempts
	q.OpensAt, q.ClosesAt, q.UpdatedAt = p.OpensAt, p.ClosesAt, s.clock.Now()
	if err := s.quizzes.UpdateParams(ctx, q); err != nil {
		return nil, mapNotFound(err)
	}
	s.audit.Log(ctx, entity.AuditActionQuizUpdate, actor, q.ID, q.Title, nil)
	return s.get(ctx, q)
}

// ListQuizzesInput drives GET /admin/quizzes.
type ListQuizzesInput struct {
	Status   string
	Query    string
	Page     int
	PageSize int
}

func (s *QuizAdminService) List(ctx context.Context, in ListQuizzesInput) (*Paged[AdminQuiz], error) {
	f := port.QuizFilter{Query: strings.TrimSpace(in.Query)}
	if in.Status != "" {
		switch st := entity.QuizStatus(in.Status); st {
		case entity.QuizDraft, entity.QuizPublished, entity.QuizArchived:
			f.Status = st
		default:
			return nil, validationOne("status", "must be draft, published or archived")
		}
	}
	page, size := normalizePage(in.Page, in.PageSize)
	quizzes, total, err := s.quizzes.List(ctx, f, port.Page{Page: page, PageSize: size})
	if err != nil {
		return nil, internal(err)
	}
	items, err := s.assemble(ctx, quizzes)
	if err != nil {
		return nil, err
	}
	return &Paged[AdminQuiz]{Items: items, Page: page, PageSize: size, Total: total}, nil
}

func (s *QuizAdminService) Get(ctx context.Context, idHex string) (*AdminQuiz, error) {
	q, err := s.load(ctx, idHex)
	if err != nil {
		return nil, err
	}
	return s.get(ctx, q)
}

// SetHospitals replaces the target hospital set. Removing a hospital that has
// attempts is refused (BR-10c).
func (s *QuizAdminService) SetHospitals(ctx context.Context, idHex string, ids []string, actor Actor) (*AdminQuiz, error) {
	q, err := s.load(ctx, idHex)
	if err != nil {
		return nil, err
	}
	if q.Status == entity.QuizArchived {
		return nil, invalidState("แบบทดสอบที่เก็บถาวรแล้วไม่สามารถแก้ไขได้")
	}
	target, verr, err := s.resolveHospitals(ctx, ids)
	if err != nil {
		return nil, err
	}
	if verr != nil {
		return nil, verr
	}
	current, err := s.assignments.ListByQuiz(ctx, q.ID)
	if err != nil {
		return nil, internal(err)
	}
	want := map[primitive.ObjectID]bool{}
	for _, h := range target {
		want[h.ID] = true
	}
	have := map[primitive.ObjectID]bool{}
	var remove []primitive.ObjectID
	for _, a := range current {
		have[a.HospitalID] = true
		if !want[a.HospitalID] {
			remove = append(remove, a.HospitalID)
		}
	}
	var blocked []string
	for _, hid := range remove {
		used, err := s.attempts.ExistsForQuizHospital(ctx, q.ID, hid)
		if err != nil {
			return nil, internal(err)
		}
		if used {
			blocked = append(blocked, hid.Hex())
		}
	}
	if len(blocked) > 0 {
		return nil, newErr(CodeAssignmentAttempts, "ลบโรงพยาบาลที่มีผู้เริ่มทำแล้วไม่ได้ ให้ปิด (Close) แทน",
			map[string]any{"hospitalIds": blocked})
	}
	now := s.clock.Now()
	var add []entity.HospitalAssignment
	for _, h := range target {
		if !have[h.ID] {
			add = append(add, entity.HospitalAssignment{QuizID: q.ID, HospitalID: h.ID, Status: entity.AssignmentScheduled, CreatedAt: now})
		}
	}
	if len(add) > 0 {
		if err := s.assignments.CreateMany(ctx, add); err != nil {
			return nil, internal(err)
		}
	}
	if len(remove) > 0 {
		if err := s.assignments.DeleteMany(ctx, q.ID, remove); err != nil {
			return nil, internal(err)
		}
	}
	s.audit.Log(ctx, entity.AuditActionQuizHospitals, actor, q.ID, q.Title,
		map[string]any{"added": len(add), "removed": len(remove)})
	return s.get(ctx, q)
}

// checkPool verifies the active pool can satisfy questionCount (BR-10b).
func (s *QuizAdminService) checkPool(ctx context.Context, questionCount int) error {
	types, err := s.types.ListActive(ctx)
	if err != nil {
		return internal(err)
	}
	counts, err := s.images.CountByType(ctx)
	if err != nil {
		return internal(err)
	}
	pool := 0
	for _, t := range types {
		pool += counts[t.Key].Active
	}
	if len(types) < 2 || pool < questionCount {
		return newErr(CodePoolTooSmall, "คลังภาพไม่เพียงพอสำหรับจำนวนข้อที่กำหนด",
			map[string]any{"activeImages": pool, "required": questionCount, "activeCellTypes": len(types)})
	}
	return nil
}

// Publish moves draft -> published after the BR-10b checks. Idempotent.
func (s *QuizAdminService) Publish(ctx context.Context, idHex string, actor Actor) (*AdminQuiz, error) {
	q, err := s.load(ctx, idHex)
	if err != nil {
		return nil, err
	}
	switch q.Status {
	case entity.QuizPublished:
		return s.get(ctx, q)
	case entity.QuizArchived:
		return nil, invalidState("แบบทดสอบที่เก็บถาวรแล้วไม่สามารถเผยแพร่ได้")
	}
	as, err := s.assignments.ListByQuiz(ctx, q.ID)
	if err != nil {
		return nil, internal(err)
	}
	now := s.clock.Now()
	var issues []entity.FieldIssue
	if len(as) == 0 {
		issues = append(issues, entity.FieldIssue{Field: "hospitalIds", Issue: "at least one hospital is required"})
	}
	if !q.ClosesAt.After(now) {
		issues = append(issues, entity.FieldIssue{Field: "closesAt", Issue: "must be in the future"})
	}
	if len(issues) > 0 {
		return nil, validation(issues...)
	}
	if perr := s.checkPool(ctx, q.QuestionCount); perr != nil {
		return nil, perr
	}
	if err := s.quizzes.TransitionStatus(ctx, q.ID, []entity.QuizStatus{entity.QuizDraft}, entity.QuizPublished, now); err != nil {
		if errors.Is(err, port.ErrConflict) {
			return nil, invalidState("สถานะแบบทดสอบเปลี่ยนไปแล้ว")
		}
		return nil, mapNotFound(err)
	}
	q.Status = entity.QuizPublished
	s.audit.Log(ctx, entity.AuditActionQuizPublish, actor, q.ID, q.Title, nil)
	return s.get(ctx, q)
}

// Archive moves draft/published -> archived. Idempotent.
func (s *QuizAdminService) Archive(ctx context.Context, idHex string, actor Actor) (*AdminQuiz, error) {
	q, err := s.load(ctx, idHex)
	if err != nil {
		return nil, err
	}
	if q.Status != entity.QuizArchived {
		if err := s.quizzes.TransitionStatus(ctx, q.ID, []entity.QuizStatus{entity.QuizDraft, entity.QuizPublished}, entity.QuizArchived, s.clock.Now()); err != nil {
			if errors.Is(err, port.ErrConflict) {
				return nil, invalidState("สถานะแบบทดสอบเปลี่ยนไปแล้ว")
			}
			return nil, mapNotFound(err)
		}
		q.Status = entity.QuizArchived
		s.audit.Log(ctx, entity.AuditActionQuizArchive, actor, q.ID, q.Title, nil)
	}
	return s.get(ctx, q)
}

// Delete removes a draft quiz that has no attempts (BR-10c).
func (s *QuizAdminService) Delete(ctx context.Context, idHex string, actor Actor) error {
	q, err := s.load(ctx, idHex)
	if err != nil {
		return err
	}
	if q.Status != entity.QuizDraft {
		return invalidState("ลบได้เฉพาะแบบทดสอบฉบับร่าง")
	}
	used, err := s.attempts.ExistsForQuiz(ctx, q.ID)
	if err != nil {
		return internal(err)
	}
	if used {
		return invalidState("มีผู้เริ่มทำแบบทดสอบนี้แล้ว จึงลบไม่ได้")
	}
	if err := s.assignments.DeleteByQuiz(ctx, q.ID); err != nil {
		return internal(err)
	}
	if err := s.quizzes.Delete(ctx, q.ID); err != nil {
		return mapNotFound(err)
	}
	s.audit.Log(ctx, entity.AuditActionQuizDelete, actor, q.ID, q.Title, nil)
	return nil
}

// OpenAssignment opens a quiz for one hospital (BR-09). Idempotent.
func (s *QuizAdminService) OpenAssignment(ctx context.Context, quizIDHex, hospitalIDHex string, actor Actor) (*AdminAssignment, error) {
	return s.switchAssignment(ctx, quizIDHex, hospitalIDHex, entity.AssignmentOpen, actor)
}

// CloseAssignment closes a quiz for one hospital. Idempotent; always allowed.
func (s *QuizAdminService) CloseAssignment(ctx context.Context, quizIDHex, hospitalIDHex string, actor Actor) (*AdminAssignment, error) {
	return s.switchAssignment(ctx, quizIDHex, hospitalIDHex, entity.AssignmentClosed, actor)
}

func (s *QuizAdminService) switchAssignment(ctx context.Context, quizIDHex, hospitalIDHex string, to entity.AssignmentStatus, actor Actor) (*AdminAssignment, error) {
	q, err := s.load(ctx, quizIDHex)
	if err != nil {
		return nil, err
	}
	hid, perr := parseOID(hospitalIDHex)
	if perr != nil {
		return nil, notFound()
	}
	cur, ferr := s.assignments.Find(ctx, q.ID, hid)
	if ferr != nil {
		return nil, mapNotFound(ferr)
	}
	now := s.clock.Now()
	if to == entity.AssignmentOpen {
		if cerr := eligibility.CanOpenAssignment(q, now); cerr != nil {
			if errors.Is(cerr, eligibility.ErrQuizNotPublished) {
				return nil, newErr(CodeQuizNotPublished, "ต้องเผยแพร่แบบทดสอบก่อนจึงจะเปิดได้", nil)
			}
			return nil, newErr(CodeQuizWindowEnded, "เลยช่วงเวลาของแบบทดสอบแล้ว ไม่สามารถเปิดได้",
				map[string]any{"closesAt": q.ClosesAt})
		}
	}
	res := cur
	if cur.Status != to { // idempotent: repeating the same action changes nothing
		res, ferr = s.assignments.SetStatus(ctx, q.ID, hid, to, now)
		if ferr != nil {
			return nil, mapNotFound(ferr)
		}
		action := entity.AuditActionAssignClose
		if to == entity.AssignmentOpen {
			action = entity.AuditActionAssignOpen
		}
		s.audit.Log(ctx, action, actor, q.ID, q.Title, map[string]any{"hospitalId": hid.Hex()})
	}
	rows, err := s.assignmentRows(ctx, q, []entity.HospitalAssignment{*res}, now)
	if err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func (s *QuizAdminService) load(ctx context.Context, idHex string) (*entity.Quiz, error) {
	id, err := parseOID(idHex)
	if err != nil {
		return nil, notFound()
	}
	q, err := s.quizzes.FindByID(ctx, id)
	if err != nil {
		return nil, mapNotFound(err)
	}
	return q, nil
}

func (s *QuizAdminService) get(ctx context.Context, q *entity.Quiz) (*AdminQuiz, error) {
	items, err := s.assemble(ctx, []entity.Quiz{*q})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// assemble builds AdminQuiz views with batched lookups.
func (s *QuizAdminService) assemble(ctx context.Context, quizzes []entity.Quiz) ([]AdminQuiz, error) {
	if len(quizzes) == 0 {
		return []AdminQuiz{}, nil
	}
	qids := make([]primitive.ObjectID, 0, len(quizzes))
	creators := map[primitive.ObjectID]bool{}
	for _, q := range quizzes {
		qids = append(qids, q.ID)
		if !q.CreatedBy.IsZero() {
			creators[q.CreatedBy] = true
		}
	}
	all, err := s.assignments.ListByQuizzes(ctx, qids)
	if err != nil {
		return nil, internal(err)
	}
	byQuiz := map[primitive.ObjectID][]entity.HospitalAssignment{}
	for _, a := range all {
		byQuiz[a.QuizID] = append(byQuiz[a.QuizID], a)
	}
	users := map[primitive.ObjectID]*entity.User{}
	if len(creators) > 0 {
		list, err := s.users.FindByIDs(ctx, keys(creators))
		if err != nil {
			return nil, internal(err)
		}
		for i := range list {
			users[list[i].ID] = &list[i]
		}
	}
	now := s.clock.Now()
	out := make([]AdminQuiz, 0, len(quizzes))
	for i := range quizzes {
		q := &quizzes[i]
		rows, err := s.assignmentRows(ctx, q, byQuiz[q.ID], now)
		if err != nil {
			return nil, err
		}
		aq := AdminQuiz{
			ID: q.ID.Hex(), Title: q.Title, QuestionCount: q.QuestionCount, PassPercent: q.PassPercent,
			DurationSec: q.DurationSec, MaxAttempts: q.MaxAttempts, OpensAt: q.OpensAt, ClosesAt: q.ClosesAt,
			Status: q.Status, Assignments: rows, CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
			CreatedBy: UserRef{ID: q.CreatedBy.Hex()},
		}
		if q.Description != "" {
			d := q.Description
			aq.Description = &d
		}
		if u := users[q.CreatedBy]; u != nil {
			aq.CreatedBy.FullName = u.FullName
		}
		out = append(out, aq)
	}
	return out, nil
}

func (s *QuizAdminService) assignmentRows(ctx context.Context, q *entity.Quiz, as []entity.HospitalAssignment, now time.Time) ([]AdminAssignment, error) {
	rows := make([]AdminAssignment, 0, len(as))
	if len(as) == 0 {
		return rows, nil
	}
	hids := make([]primitive.ObjectID, 0, len(as))
	for _, a := range as {
		hids = append(hids, a.HospitalID)
	}
	hs, err := s.hospitals.FindByIDs(ctx, hids)
	if err != nil {
		return nil, internal(err)
	}
	names := map[primitive.ObjectID]string{}
	for _, h := range hs {
		names[h.ID] = h.Name
	}
	eligible, err := s.users.CountActiveByHospitals(ctx, hids)
	if err != nil {
		return nil, internal(err)
	}
	stats, err := s.attempts.StatsByQuizzes(ctx, []primitive.ObjectID{q.ID})
	if err != nil {
		return nil, internal(err)
	}
	for _, a := range as {
		st := stats[q.ID][a.HospitalID]
		rows = append(rows, AdminAssignment{
			Hospital: HospitalRef{ID: a.HospitalID.Hex(), Name: names[a.HospitalID]},
			Status:   a.Status, Availability: eligibility.Availability(q, a.Status, now),
			OpenedAt: a.OpenedAt, ClosedAt: a.ClosedAt,
			Stats: AssignmentStats{EligibleUsers: eligible[a.HospitalID], Started: st.Started, Submitted: st.Submitted, Passed: st.Passed},
		})
	}
	sortAssignments(rows)
	return rows, nil
}

func sortAssignments(rows []AdminAssignment) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Hospital.Name < rows[j-1].Hospital.Name; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}
