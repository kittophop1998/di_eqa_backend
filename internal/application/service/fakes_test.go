package service_test

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---------------------------------------------------------------- clock / random

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
}
func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type fakeRandom struct {
	mu      sync.Mutex
	rng     *rand.Rand
	qn      int
	certNos []string // returned in order first, then generated
	certN   int
}

func newRandom() *fakeRandom { return &fakeRandom{rng: rand.New(rand.NewSource(1))} }
func (r *fakeRandom) Intn(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rng.Intn(n)
}
func (r *fakeRandom) QuestionID() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.qn++
	return fmt.Sprintf("q%07x", r.qn*7919), nil
}
func (r *fakeRandom) CertNo(now time.Time) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.certN++
	if len(r.certNos) > 0 {
		v := r.certNos[0]
		r.certNos = r.certNos[1:]
		return v, nil
	}
	return fmt.Sprintf("DIEQA-%d-GEN%05d", now.Year(), r.certN), nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(p string) (string, error) { return "hash:" + p, nil }
func (fakeHasher) Compare(h, p string) error {
	if h != "hash:"+p {
		return fmt.Errorf("mismatch")
	}
	return nil
}

type fakeTokens struct{}

func (fakeTokens) Issue(c applicationport.TokenClaims) (string, time.Time, error) {
	return "tok-" + c.UserID, time.Date(2026, 10, 1, 21, 0, 0, 0, time.UTC), nil
}

type fakeImages struct {
	mu     sync.Mutex
	served []string
}

func (f *fakeImages) Get(_ context.Context, p string) (*applicationport.Image, error) {
	f.mu.Lock()
	f.served = append(f.served, p)
	f.mu.Unlock()
	return &applicationport.Image{Data: []byte("IMG:" + p), ContentType: "image/jpeg"}, nil
}
func (f *fakeImages) PreviewURL(p string) string { return "https://cdn.example/" + p }

// ---------------------------------------------------------------- in-memory db

type memDB struct {
	mu          sync.Mutex
	users       map[primitive.ObjectID]*entity.User
	hospitals   map[primitive.ObjectID]*entity.Hospital
	types       []entity.CellType
	images      map[primitive.ObjectID]*entity.CellImage
	quizzes     map[primitive.ObjectID]*entity.Quiz
	assignments []entity.HospitalAssignment
	attempts    map[primitive.ObjectID]*entity.Attempt
	certs       map[primitive.ObjectID]*entity.Certificate
	audit       []entity.AuditLog
}

func newMemDB() *memDB {
	return &memDB{
		users: map[primitive.ObjectID]*entity.User{}, hospitals: map[primitive.ObjectID]*entity.Hospital{},
		images: map[primitive.ObjectID]*entity.CellImage{}, quizzes: map[primitive.ObjectID]*entity.Quiz{},
		attempts: map[primitive.ObjectID]*entity.Attempt{}, certs: map[primitive.ObjectID]*entity.Certificate{},
	}
}

func (d *memDB) auditActions() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, a := range d.audit {
		out = append(out, a.Action)
	}
	return out
}

// users ----------------------------------------------------------------

type userRepo struct{ d *memDB }

func cloneUser(u *entity.User) *entity.User { c := *u; return &c }

func (r userRepo) FindByID(_ context.Context, id primitive.ObjectID) (*entity.User, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if u, ok := r.d.users[id]; ok {
		return cloneUser(u), nil
	}
	return nil, port.ErrNotFound
}
func (r userRepo) FindByIDs(_ context.Context, ids []primitive.ObjectID) ([]entity.User, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.User
	for _, id := range ids {
		if u, ok := r.d.users[id]; ok {
			out = append(out, *u)
		}
	}
	return out, nil
}
func (r userRepo) FindByUsername(_ context.Context, name string) (*entity.User, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, u := range r.d.users {
		if u.UsernameLower == strings.ToLower(name) {
			return cloneUser(u), nil
		}
	}
	return nil, port.ErrNotFound
}
func (r userRepo) Create(_ context.Context, u *entity.User) (primitive.ObjectID, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, e := range r.d.users {
		if e.UsernameLower == strings.ToLower(u.Username) {
			return primitive.NilObjectID, port.ErrDuplicate
		}
	}
	c := cloneUser(u)
	c.ID = primitive.NewObjectID()
	c.UsernameLower = strings.ToLower(u.Username)
	r.d.users[c.ID] = c
	return c.ID, nil
}
func (r userRepo) List(_ context.Context, f port.UserFilter, p port.Page) ([]entity.User, int64, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.User
	for _, u := range r.d.users {
		if f.Status != "" && u.Status != f.Status {
			continue
		}
		if f.Role != "" && u.Role != f.Role {
			continue
		}
		if !f.HospitalID.IsZero() && u.HospitalID != f.HospitalID {
			continue
		}
		if f.Query != "" && !strings.Contains(u.Username+u.FullName+u.Email, f.Query) {
			continue
		}
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, int64(len(out)), nil
}
func (r userRepo) Update(_ context.Context, id primitive.ObjectID, upd port.UserUpdate, only []entity.UserStatus) (*entity.User, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	u, ok := r.d.users[id]
	if !ok {
		return nil, port.ErrNotFound
	}
	if len(only) > 0 {
		match := false
		for _, s := range only {
			match = match || u.Status == s
		}
		if !match {
			return nil, port.ErrConflict
		}
	}
	if upd.Status != nil {
		u.Status = *upd.Status
	}
	if upd.Role != nil {
		u.Role = *upd.Role
	}
	if upd.HospitalID != nil {
		u.HospitalID = *upd.HospitalID
	}
	if upd.FullName != nil {
		u.FullName = *upd.FullName
	}
	if upd.Email != nil {
		u.Email = *upd.Email
	}
	if upd.RejectReason != nil {
		u.RejectReason = *upd.RejectReason
	}
	if upd.ClearRejectReason {
		u.RejectReason = ""
	}
	if upd.ApprovedAt != nil {
		u.ApprovedAt = upd.ApprovedAt
	}
	if upd.ApprovedBy != nil {
		u.ApprovedBy = *upd.ApprovedBy
	}
	return cloneUser(u), nil
}
func (r userRepo) CountByHospital(_ context.Context, h primitive.ObjectID) (int64, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var n int64
	for _, u := range r.d.users {
		if u.HospitalID == h {
			n++
		}
	}
	return n, nil
}
func (r userRepo) CountActiveByHospitals(_ context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]int, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	out := map[primitive.ObjectID]int{}
	for _, u := range r.d.users {
		if u.Status == entity.UserActive && u.Role == entity.RoleUser {
			out[u.HospitalID]++
		}
	}
	return out, nil
}
func (r userRepo) CountByRoleStatus(_ context.Context, role entity.Role, st entity.UserStatus) (int64, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var n int64
	for _, u := range r.d.users {
		if u.Role == role && (st == "" || u.Status == st) {
			n++
		}
	}
	return n, nil
}

// hospitals ------------------------------------------------------------

type hospitalRepo struct{ d *memDB }

func (r hospitalRepo) FindByCode(_ context.Context, code string) (*entity.Hospital, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, h := range r.d.hospitals {
		if h.Code == code {
			c := *h
			return &c, nil
		}
	}
	return nil, port.ErrNotFound
}
func (r hospitalRepo) FindByID(_ context.Context, id primitive.ObjectID) (*entity.Hospital, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if h, ok := r.d.hospitals[id]; ok {
		c := *h
		return &c, nil
	}
	return nil, port.ErrNotFound
}
func (r hospitalRepo) FindByIDs(_ context.Context, ids []primitive.ObjectID) ([]entity.Hospital, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.Hospital
	for _, id := range ids {
		if h, ok := r.d.hospitals[id]; ok {
			out = append(out, *h)
		}
	}
	return out, nil
}
func (r hospitalRepo) ListActive(_ context.Context) ([]entity.Hospital, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.Hospital
	for _, h := range r.d.hospitals {
		if h.Active {
			out = append(out, *h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (r hospitalRepo) List(_ context.Context, f port.HospitalFilter, p port.Page) ([]entity.Hospital, int64, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.Hospital
	for _, h := range r.d.hospitals {
		if f.Active != nil && h.Active != *f.Active {
			continue
		}
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, int64(len(out)), nil
}
func (r hospitalRepo) Create(_ context.Context, h *entity.Hospital) (primitive.ObjectID, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, e := range r.d.hospitals {
		if e.Code == h.Code {
			return primitive.NilObjectID, port.ErrDuplicate
		}
	}
	c := *h
	c.ID = primitive.NewObjectID()
	r.d.hospitals[c.ID] = &c
	return c.ID, nil
}
func (r hospitalRepo) Update(_ context.Context, id primitive.ObjectID, h *entity.Hospital) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if _, ok := r.d.hospitals[id]; !ok {
		return port.ErrNotFound
	}
	for _, e := range r.d.hospitals {
		if e.ID != id && e.Code == h.Code {
			return port.ErrDuplicate
		}
	}
	c := *h
	c.ID = id
	r.d.hospitals[id] = &c
	return nil
}
func (r hospitalRepo) SetActive(_ context.Context, id primitive.ObjectID, a bool) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	h, ok := r.d.hospitals[id]
	if !ok {
		return port.ErrNotFound
	}
	h.Active = a
	return nil
}

// cell library ---------------------------------------------------------

type cellTypeRepo struct{ d *memDB }

func (r cellTypeRepo) ListAll(context.Context) ([]entity.CellType, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	out := append([]entity.CellType(nil), r.d.types...)
	sort.Slice(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out, nil
}
func (r cellTypeRepo) ListActive(ctx context.Context) ([]entity.CellType, error) {
	all, _ := r.ListAll(ctx)
	var out []entity.CellType
	for _, t := range all {
		if t.Active {
			out = append(out, t)
		}
	}
	return out, nil
}

type cellImageRepo struct{ d *memDB }

func (r cellImageRepo) ActiveIDs(_ context.Context, keys []string) ([]primitive.ObjectID, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	set := map[string]bool{}
	for _, k := range keys {
		set[k] = true
	}
	var out []primitive.ObjectID
	for _, im := range r.d.images {
		if im.Active && set[im.TypeKey] {
			out = append(out, im.ID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hex() < out[j].Hex() })
	return out, nil
}
func (r cellImageRepo) FindByIDs(_ context.Context, ids []primitive.ObjectID) ([]entity.CellImage, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.CellImage
	for _, id := range ids {
		if im, ok := r.d.images[id]; ok {
			out = append(out, *im)
		}
	}
	return out, nil
}
func (r cellImageRepo) List(_ context.Context, f port.ImageFilter, p port.Page) ([]entity.CellImage, int64, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.CellImage
	for _, im := range r.d.images {
		if f.TypeKey != "" && im.TypeKey != f.TypeKey {
			continue
		}
		if f.Active != nil && im.Active != *f.Active {
			continue
		}
		out = append(out, *im)
	}
	return out, int64(len(out)), nil
}
func (r cellImageRepo) CountByType(context.Context) (map[string]entity.CellTypeCounts, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	out := map[string]entity.CellTypeCounts{}
	for _, im := range r.d.images {
		c := out[im.TypeKey]
		c.Total++
		if im.Active {
			c.Active++
		}
		out[im.TypeKey] = c
	}
	return out, nil
}

// quizzes / assignments ------------------------------------------------

type quizRepo struct{ d *memDB }

func (r quizRepo) Create(_ context.Context, q *entity.Quiz) (primitive.ObjectID, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	c := *q
	c.ID = primitive.NewObjectID()
	r.d.quizzes[c.ID] = &c
	return c.ID, nil
}
func (r quizRepo) FindByID(_ context.Context, id primitive.ObjectID) (*entity.Quiz, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if q, ok := r.d.quizzes[id]; ok {
		c := *q
		return &c, nil
	}
	return nil, port.ErrNotFound
}
func (r quizRepo) FindByIDs(_ context.Context, ids []primitive.ObjectID) ([]entity.Quiz, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.Quiz
	for _, id := range ids {
		if q, ok := r.d.quizzes[id]; ok {
			out = append(out, *q)
		}
	}
	return out, nil
}
func (r quizRepo) List(_ context.Context, f port.QuizFilter, p port.Page) ([]entity.Quiz, int64, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.Quiz
	for _, q := range r.d.quizzes {
		if f.Status == "" || q.Status == f.Status {
			out = append(out, *q)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, int64(len(out)), nil
}
func (r quizRepo) UpdateParams(_ context.Context, q *entity.Quiz) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	cur, ok := r.d.quizzes[q.ID]
	if !ok {
		return port.ErrNotFound
	}
	c := *q
	c.Status = cur.Status
	r.d.quizzes[q.ID] = &c
	return nil
}
func (r quizRepo) TransitionStatus(_ context.Context, id primitive.ObjectID, from []entity.QuizStatus, to entity.QuizStatus, at time.Time) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	q, ok := r.d.quizzes[id]
	if !ok {
		return port.ErrNotFound
	}
	for _, s := range from {
		if q.Status == s {
			q.Status, q.UpdatedAt = to, at
			return nil
		}
	}
	return port.ErrConflict
}
func (r quizRepo) Delete(_ context.Context, id primitive.ObjectID) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if _, ok := r.d.quizzes[id]; !ok {
		return port.ErrNotFound
	}
	delete(r.d.quizzes, id)
	return nil
}

type assignmentRepo struct{ d *memDB }

func (r assignmentRepo) CreateMany(_ context.Context, as []entity.HospitalAssignment) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, a := range as {
		for _, e := range r.d.assignments {
			if e.QuizID == a.QuizID && e.HospitalID == a.HospitalID {
				return port.ErrDuplicate
			}
		}
		a.ID = primitive.NewObjectID()
		r.d.assignments = append(r.d.assignments, a)
	}
	return nil
}
func (r assignmentRepo) Find(_ context.Context, q, h primitive.ObjectID) (*entity.HospitalAssignment, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, a := range r.d.assignments {
		if a.QuizID == q && a.HospitalID == h {
			c := a
			return &c, nil
		}
	}
	return nil, port.ErrNotFound
}
func (r assignmentRepo) filter(pred func(entity.HospitalAssignment) bool) []entity.HospitalAssignment {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.HospitalAssignment
	for _, a := range r.d.assignments {
		if pred(a) {
			out = append(out, a)
		}
	}
	return out
}
func (r assignmentRepo) ListByQuiz(_ context.Context, q primitive.ObjectID) ([]entity.HospitalAssignment, error) {
	return r.filter(func(a entity.HospitalAssignment) bool { return a.QuizID == q }), nil
}
func (r assignmentRepo) ListByQuizzes(_ context.Context, qs []primitive.ObjectID) ([]entity.HospitalAssignment, error) {
	return r.filter(func(a entity.HospitalAssignment) bool {
		for _, q := range qs {
			if a.QuizID == q {
				return true
			}
		}
		return false
	}), nil
}
func (r assignmentRepo) ListByHospital(_ context.Context, h primitive.ObjectID) ([]entity.HospitalAssignment, error) {
	return r.filter(func(a entity.HospitalAssignment) bool { return a.HospitalID == h }), nil
}
func (r assignmentRepo) DeleteMany(_ context.Context, q primitive.ObjectID, hs []primitive.ObjectID) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var keep []entity.HospitalAssignment
	for _, a := range r.d.assignments {
		drop := false
		if a.QuizID == q {
			for _, h := range hs {
				drop = drop || a.HospitalID == h
			}
		}
		if !drop {
			keep = append(keep, a)
		}
	}
	r.d.assignments = keep
	return nil
}
func (r assignmentRepo) DeleteByQuiz(_ context.Context, q primitive.ObjectID) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var keep []entity.HospitalAssignment
	for _, a := range r.d.assignments {
		if a.QuizID != q {
			keep = append(keep, a)
		}
	}
	r.d.assignments = keep
	return nil
}
func (r assignmentRepo) SetStatus(_ context.Context, q, h primitive.ObjectID, st entity.AssignmentStatus, at time.Time) (*entity.HospitalAssignment, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for i := range r.d.assignments {
		a := &r.d.assignments[i]
		if a.QuizID == q && a.HospitalID == h {
			a.Status = st
			if st == entity.AssignmentOpen {
				a.OpenedAt, a.ClosedAt = &at, nil
			} else {
				a.ClosedAt = &at
			}
			c := *a
			return &c, nil
		}
	}
	return nil, port.ErrNotFound
}

// attempts -------------------------------------------------------------

type attemptRepo struct{ d *memDB }

func cloneAttempt(a *entity.Attempt) *entity.Attempt {
	c := *a
	c.Answers = map[string]string{}
	for k, v := range a.Answers {
		c.Answers[k] = v
	}
	return &c
}

func (r attemptRepo) Create(_ context.Context, a *entity.Attempt) (primitive.ObjectID, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, e := range r.d.attempts {
		if e.QuizID == a.QuizID && e.UserID == a.UserID &&
			((a.Status == entity.AttemptInProgress && e.Status == entity.AttemptInProgress) || e.AttemptNo == a.AttemptNo) {
			return primitive.NilObjectID, port.ErrDuplicate // partial unique / attemptNo unique
		}
	}
	c := cloneAttempt(a)
	c.ID = primitive.NewObjectID()
	r.d.attempts[c.ID] = c
	return c.ID, nil
}
func (r attemptRepo) FindByID(_ context.Context, id primitive.ObjectID) (*entity.Attempt, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if a, ok := r.d.attempts[id]; ok {
		return cloneAttempt(a), nil
	}
	return nil, port.ErrNotFound
}
func (r attemptRepo) ListByUserQuizzes(_ context.Context, u primitive.ObjectID, qs []primitive.ObjectID) ([]entity.Attempt, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.Attempt
	for _, a := range r.d.attempts {
		if a.UserID != u {
			continue
		}
		for _, q := range qs {
			if a.QuizID == q {
				out = append(out, *cloneAttempt(a))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AttemptNo < out[j].AttemptNo })
	return out, nil
}
func (r attemptRepo) ListByUser(_ context.Context, u primitive.ObjectID, q *primitive.ObjectID, p port.Page) ([]entity.Attempt, int64, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.Attempt
	for _, a := range r.d.attempts {
		if a.UserID == u && (q == nil || a.QuizID == *q) {
			out = append(out, *cloneAttempt(a))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AttemptNo > out[j].AttemptNo })
	return out, int64(len(out)), nil
}
func (r attemptRepo) MergeAnswers(_ context.Context, id primitive.ObjectID, set map[string]string, clear []string, now time.Time) (*entity.Attempt, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	a, ok := r.d.attempts[id]
	if !ok || a.Status != entity.AttemptInProgress || a.PastGrace(now) {
		return nil, port.ErrConflict
	}
	if a.Answers == nil {
		a.Answers = map[string]string{}
	}
	for k, v := range set {
		a.Answers[k] = v
	}
	for _, k := range clear {
		delete(a.Answers, k)
	}
	return cloneAttempt(a), nil
}
func (r attemptRepo) Submit(_ context.Context, id primitive.ObjectID, res port.SubmitResult, now time.Time) (*entity.Attempt, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	a, ok := r.d.attempts[id]
	if !ok || a.Status != entity.AttemptInProgress || a.PastGrace(now) {
		return nil, port.ErrConflict
	}
	a.Status = entity.AttemptSubmitted
	t := res.SubmittedAt
	a.SubmittedAt = &t
	a.Answers = res.Answers
	c, p := res.Correct, res.Percent
	a.Correct, a.Percent, a.Total, a.Passed, a.PerType = &c, &p, res.Total, res.Passed, res.PerType
	return cloneAttempt(a), nil
}
func (r attemptRepo) Expire(_ context.Context, id primitive.ObjectID, now time.Time) (*entity.Attempt, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	a, ok := r.d.attempts[id]
	if !ok || a.Status != entity.AttemptInProgress || !a.PastGrace(now) {
		return nil, port.ErrConflict
	}
	a.Status = entity.AttemptExpired
	return cloneAttempt(a), nil
}
func (r attemptRepo) SetCertificate(_ context.Context, id, cid primitive.ObjectID, no string) error {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if a, ok := r.d.attempts[id]; ok {
		a.CertificateID, a.CertNo = cid, no
	}
	return nil
}
func (r attemptRepo) ExistsForQuizHospital(_ context.Context, q, h primitive.ObjectID) (bool, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, a := range r.d.attempts {
		if a.QuizID == q && a.HospitalID == h {
			return true, nil
		}
	}
	return false, nil
}
func (r attemptRepo) ExistsForQuiz(_ context.Context, q primitive.ObjectID) (bool, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, a := range r.d.attempts {
		if a.QuizID == q {
			return true, nil
		}
	}
	return false, nil
}
func (r attemptRepo) StatsByQuizzes(_ context.Context, qs []primitive.ObjectID) (map[primitive.ObjectID]map[primitive.ObjectID]port.AttemptStats, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	type key struct{ q, h, u primitive.ObjectID }
	sub, pas, started := map[key]bool{}, map[key]bool{}, map[key]bool{}
	for _, a := range r.d.attempts {
		k := key{a.QuizID, a.HospitalID, a.UserID}
		started[k] = true
		if a.Status == entity.AttemptSubmitted {
			sub[k] = true
			if a.Passed {
				pas[k] = true
			}
		}
	}
	out := map[primitive.ObjectID]map[primitive.ObjectID]port.AttemptStats{}
	for k := range started {
		if out[k.q] == nil {
			out[k.q] = map[primitive.ObjectID]port.AttemptStats{}
		}
		s := out[k.q][k.h]
		s.Started++
		if sub[k] {
			s.Submitted++
		}
		if pas[k] {
			s.Passed++
		}
		out[k.q][k.h] = s
	}
	return out, nil
}

// certificates ---------------------------------------------------------

type certRepo struct{ d *memDB }

func (r certRepo) Create(_ context.Context, c *entity.Certificate) (primitive.ObjectID, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, e := range r.d.certs {
		if e.CertNo == c.CertNo || e.AttemptID == c.AttemptID || (e.QuizID == c.QuizID && e.UserID == c.UserID) {
			return primitive.NilObjectID, port.ErrDuplicate
		}
	}
	cp := *c
	cp.ID = primitive.NewObjectID()
	r.d.certs[cp.ID] = &cp
	return cp.ID, nil
}
func (r certRepo) find(pred func(*entity.Certificate) bool) (*entity.Certificate, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	for _, c := range r.d.certs {
		if pred(c) {
			cp := *c
			return &cp, nil
		}
	}
	return nil, port.ErrNotFound
}
func (r certRepo) FindByID(_ context.Context, id primitive.ObjectID) (*entity.Certificate, error) {
	return r.find(func(c *entity.Certificate) bool { return c.ID == id })
}
func (r certRepo) FindByAttempt(_ context.Context, id primitive.ObjectID) (*entity.Certificate, error) {
	return r.find(func(c *entity.Certificate) bool { return c.AttemptID == id })
}
func (r certRepo) FindByQuizUser(_ context.Context, q, u primitive.ObjectID) (*entity.Certificate, error) {
	return r.find(func(c *entity.Certificate) bool { return c.QuizID == q && c.UserID == u })
}
func (r certRepo) ListByUser(_ context.Context, u primitive.ObjectID) ([]entity.Certificate, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	var out []entity.Certificate
	for _, c := range r.d.certs {
		if c.UserID == u {
			out = append(out, *c)
		}
	}
	return out, nil
}
func (r certRepo) ListByUserQuizzes(_ context.Context, u primitive.ObjectID, qs []primitive.ObjectID) ([]entity.Certificate, error) {
	all, _ := r.ListByUser(context.Background(), u)
	var out []entity.Certificate
	for _, c := range all {
		for _, q := range qs {
			if c.QuizID == q {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

type auditRepo struct{ d *memDB }

func (r auditRepo) Create(_ context.Context, l *entity.AuditLog) (primitive.ObjectID, error) {
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	r.d.audit = append(r.d.audit, *l)
	return primitive.NewObjectID(), nil
}
func (r auditRepo) List(context.Context, string, int64, int64) ([]entity.AuditLog, int64, error) {
	return nil, 0, nil
}
