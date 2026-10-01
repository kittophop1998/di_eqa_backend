package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var ctx = context.Background()

// world wires every service to in-memory fakes and seeds two hospitals, a
// staff account, one user per hospital and an unbalanced image pool.
type world struct {
	t     *testing.T
	db    *memDB
	clock *fakeClock
	rnd   *fakeRandom
	imgs  *fakeImages

	auth       *service.AuthService
	hospitals  *service.HospitalService
	adminUsers *service.AdminUserService
	cells      *service.CellLibraryService
	quizAdmin  *service.QuizAdminService
	myQuiz     *service.MyQuizService
	attempts   *service.AttemptService
	certs      *service.CertificateService

	hospA, hospB *entity.Hospital
	admin, super *entity.User
	userA, userB *entity.User
}

func newWorld(t *testing.T) *world {
	t.Helper()
	d := newMemDB()
	w := &world{t: t, db: d, clock: newClock(), rnd: newRandom(), imgs: &fakeImages{}}
	audit := service.NewAuditService(auditRepo{d})
	users, hosp, types, images := userRepo{d}, hospitalRepo{d}, cellTypeRepo{d}, cellImageRepo{d}
	quizzes, asg, att, certs := quizRepo{d}, assignmentRepo{d}, attemptRepo{d}, certRepo{d}

	w.auth = service.NewAuthService(users, hosp, audit, fakeTokens{}, fakeHasher{}, w.clock)
	w.hospitals = service.NewHospitalService(hosp, users, asg, audit, w.clock)
	w.adminUsers = service.NewAdminUserService(users, hosp, audit, w.clock)
	w.cells = service.NewCellLibraryService(types, images, w.imgs)
	w.quizAdmin = service.NewQuizAdminService(quizzes, asg, hosp, users, att, types, images, audit, w.clock)
	w.myQuiz = service.NewMyQuizService(quizzes, asg, att, certs, w.clock)
	w.attempts = service.NewAttemptService(quizzes, asg, att, certs, hosp, types, images, w.imgs, w.rnd, w.clock)
	w.certs = service.NewCertificateService(certs)

	w.hospA = w.addHospital("HOSP-A", "Hospital A")
	w.hospB = w.addHospital("HOSP-B", "Hospital B")
	w.admin = w.addUser("admin1", entity.RoleAdmin, entity.UserActive, primitive.NilObjectID)
	w.super = w.addUser("super1", entity.RoleSuperAdmin, entity.UserActive, primitive.NilObjectID)
	w.userA = w.addUser("usera", entity.RoleUser, entity.UserActive, w.hospA.ID)
	w.userB = w.addUser("userb", entity.RoleUser, entity.UserActive, w.hospB.ID)

	// Unbalanced pool: 40/5/3/2 active images over 4 active types, plus noise
	// that must never be drawn (inactive image, inactive type).
	for i, ty := range []struct {
		key    string
		n      int
		active bool
	}{{"neutrophil", 40, true}, {"lymphocyte", 5, true}, {"monocyte", 3, true}, {"eosinophil", 2, true}, {"basophil", 10, false}} {
		d.types = append(d.types, entity.CellType{Key: ty.key, Label: ty.key, SortOrder: (i + 1) * 10, Active: ty.active})
		for j := 0; j < ty.n; j++ {
			w.addImage(ty.key, fmt.Sprintf("%s/%03d.jpg", ty.key, j), true)
		}
	}
	w.addImage("neutrophil", "neutrophil/inactive.jpg", false)
	return w
}

func (w *world) addHospital(code, name string) *entity.Hospital {
	h := &entity.Hospital{ID: primitive.NewObjectID(), Code: code, Name: name, Active: true}
	w.db.hospitals[h.ID] = h
	return h
}

func (w *world) addUser(name string, role entity.Role, st entity.UserStatus, hosp primitive.ObjectID) *entity.User {
	u := &entity.User{ID: primitive.NewObjectID(), Username: name, UsernameLower: name, FullName: "Name " + name,
		Password: "hash:pw-" + name, Role: role, Status: st, HospitalID: hosp}
	w.db.users[u.ID] = u
	return u
}

func (w *world) addImage(key, path string, active bool) {
	im := &entity.CellImage{ID: primitive.NewObjectID(), TypeKey: key, Path: path, Active: active}
	w.db.images[im.ID] = im
}

func prin(u *entity.User) *service.Principal {
	return &service.Principal{UserID: u.ID, Username: u.Username, FullName: u.FullName, Role: u.Role, Status: u.Status, HospitalID: u.HospitalID}
}

func actor(u *entity.User) service.Actor {
	return service.Actor{ID: u.ID, Name: u.FullName, Role: u.Role}
}

// quizInput is a valid create payload: 10 questions, 60% to pass, 10 minutes, 2 attempts.
func (w *world) quizInput(hospitals ...*entity.Hospital) service.QuizInput {
	qc := 10
	var ids []string
	for _, h := range hospitals {
		ids = append(ids, h.ID.Hex())
	}
	now := w.clock.Now()
	return service.QuizInput{
		Title: "Quiz 1", QuestionCount: &qc, PassPercent: 60, DurationSec: 600, MaxAttempts: 2,
		OpensAt: now.Add(-time.Hour).Format(time.RFC3339), ClosesAt: now.Add(24 * time.Hour).Format(time.RFC3339),
		HospitalIDs: ids,
	}
}

// publishedQuiz creates and publishes a quiz for the hospitals, then opens it for `open`.
func (w *world) publishedQuiz(hospitals []*entity.Hospital, open ...*entity.Hospital) string {
	w.t.Helper()
	q, err := w.quizAdmin.Create(ctx, w.quizInput(hospitals...), actor(w.admin))
	if err != nil {
		w.t.Fatalf("create quiz: %v", err)
	}
	if _, err := w.quizAdmin.Publish(ctx, q.ID, actor(w.admin)); err != nil {
		w.t.Fatalf("publish quiz: %v", err)
	}
	for _, h := range open {
		if _, err := w.quizAdmin.OpenAssignment(ctx, q.ID, h.ID.Hex(), actor(w.admin)); err != nil {
			w.t.Fatalf("open assignment: %v", err)
		}
	}
	return q.ID
}

// answerAll answers every question with its correct type (reading the answer
// key straight from the fake store, as the API itself never exposes it).
func (w *world) answerKey(attemptID string) map[string]string {
	id, _ := primitive.ObjectIDFromHex(attemptID)
	a := w.db.attempts[id]
	key := map[string]string{}
	for _, q := range a.Questions {
		key[q.ID] = q.CorrectType
	}
	return key
}

func strp(s string) *string { return &s }

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %s, got nil", code)
	}
	if got := service.CodeOf(err); got != code {
		t.Fatalf("expected error %s, got %s (%v)", code, got, err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
