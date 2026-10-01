package service_test

import (
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStart_DrawsDistinctRandomQuestionsFromUnbalancedActivePool(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA, w.hospB}, w.hospA)

	v, created, err := w.attempts.Start(ctx, prin(w.userA), quizID)
	if err != nil || !created {
		t.Fatalf("start: created=%v err=%v", created, err)
	}
	if len(v.Questions) != 10 || v.QuestionCount != 10 {
		t.Fatalf("want 10 questions, got %d", len(v.Questions))
	}
	seen := map[string]bool{}
	id, _ := primitive.ObjectIDFromHex(v.ID)
	stored := w.db.attempts[id]
	imgs := map[primitive.ObjectID]bool{}
	for i, q := range v.Questions {
		if seen[q.ID] {
			t.Fatalf("duplicate question id %s", q.ID)
		}
		seen[q.ID] = true
		if len(q.ID) < 8 {
			t.Fatalf("question id %q shorter than 8 chars", q.ID)
		}
		if q.Index != i+1 {
			t.Fatalf("index %d at position %d", q.Index, i)
		}
		if want := "/api/me/attempts/" + v.ID + "/questions/" + q.ID + "/image"; q.ImageURL != want {
			t.Fatalf("imageUrl %q want %q", q.ImageURL, want)
		}
		sq := stored.Questions[i]
		if imgs[sq.ImageID] {
			t.Fatalf("same image drawn twice")
		}
		imgs[sq.ImageID] = true
		if q.ID == sq.ImageID.Hex() {
			t.Fatalf("question id equals image id")
		}
		if sq.Path == "" || sq.CorrectType == "" {
			t.Fatalf("snapshot missing path/correctType")
		}
		if sq.CorrectType == "basophil" || sq.Path == "neutrophil/inactive.jpg" {
			t.Fatalf("drew from inactive type/image: %+v", sq)
		}
	}
	// Answer options are the active types only, ordered by sortOrder.
	var keys []string
	for _, c := range v.CellTypes {
		keys = append(keys, c.Key)
	}
	want := []string{"neutrophil", "lymphocyte", "monocyte", "eosinophil"}
	if len(keys) != len(want) {
		t.Fatalf("cell types %v", keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("cell types %v want %v", keys, want)
		}
	}
	if got := v.ExpiresAt.Sub(v.StartedAt); got != 600*time.Second {
		t.Fatalf("expiresAt-startedAt = %v", got)
	}
	if !v.ServerTime.Equal(w.clock.Now()) {
		t.Fatalf("serverTime not set")
	}
}

func TestStart_ResumeReturnsSameSetAndOrderAndDoesNotRedraw(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)

	first, created, err := w.attempts.Start(ctx, p, quizID)
	if err != nil || !created {
		t.Fatal(err)
	}
	if _, err := w.attempts.SaveAnswers(ctx, p, first.ID, map[string]*string{first.Questions[0].ID: strp("neutrophil")}); err != nil {
		t.Fatal(err)
	}
	w.clock.Advance(2 * time.Minute)
	again, created, err := w.attempts.Start(ctx, p, quizID)
	if err != nil || created {
		t.Fatalf("resume: created=%v err=%v", created, err)
	}
	if again.ID != first.ID || !again.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("resume returned a different attempt or moved expiresAt")
	}
	for i := range first.Questions {
		if first.Questions[i] != again.Questions[i] {
			t.Fatalf("question %d changed on resume", i)
		}
	}
	if again.Answers[first.Questions[0].ID] != "neutrophil" {
		t.Fatalf("saved answer lost on resume")
	}
	// GET returns the same view (refresh).
	st, err := w.attempts.Get(ctx, p, first.ID)
	if err != nil || st.View == nil || st.View.Questions[3] != first.Questions[3] {
		t.Fatalf("get after refresh: %+v %v", st, err)
	}
	if len(w.db.attempts) != 1 {
		t.Fatalf("resume created %d attempts", len(w.db.attempts))
	}
}

func TestStart_HospitalScopingAndAvailability(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA, w.hospB}, w.hospA) // only A opened

	// Other hospital, still scheduled: visible quiz but QUIZ_NOT_OPEN(upcoming).
	_, _, err := w.attempts.Start(ctx, prin(w.userB), quizID)
	wantCode(t, err, service.CodeQuizNotOpen)
	if d := err.(*service.Error).Details; d["reason"] != "upcoming" {
		t.Fatalf("details %v", d)
	}

	// A hospital that was never assigned gets NOT_FOUND, not FORBIDDEN.
	hospC := w.addHospital("HOSP-C", "Hospital C")
	userC := w.addUser("userc", entity.RoleUser, entity.UserActive, hospC.ID)
	_, _, err = w.attempts.Start(ctx, prin(userC), quizID)
	wantCode(t, err, service.CodeNotFound)

	// A user without a hospital never sees anything.
	orphan := w.addUser("orphan", entity.RoleUser, entity.UserActive, primitive.NilObjectID)
	_, _, err = w.attempts.Start(ctx, prin(orphan), quizID)
	wantCode(t, err, service.CodeNotFound)
	list, err := w.myQuiz.List(ctx, prin(orphan))
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("orphan list: %+v %v", list, err)
	}

	// Unknown / malformed ids.
	_, _, err = w.attempts.Start(ctx, prin(w.userA), "nope")
	wantCode(t, err, service.CodeNotFound)

	// Closing A blocks a new start immediately.
	if _, err := w.quizAdmin.CloseAssignment(ctx, quizID, w.hospA.ID.Hex(), actor(w.admin)); err != nil {
		t.Fatal(err)
	}
	_, _, err = w.attempts.Start(ctx, prin(w.userA), quizID)
	wantCode(t, err, service.CodeQuizNotOpen)
	if d := err.(*service.Error).Details; d["reason"] != "closed" {
		t.Fatalf("details %v", d)
	}
}

func TestStart_DraftAndArchivedQuizAreInvisibleToUsers(t *testing.T) {
	w := newWorld(t)
	q, err := w.quizAdmin.Create(ctx, w.quizInput(w.hospA), actor(w.admin))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = w.attempts.Start(ctx, prin(w.userA), q.ID)
	wantCode(t, err, service.CodeNotFound)
	if l, _ := w.myQuiz.List(ctx, prin(w.userA)); len(l.Items) != 0 {
		t.Fatalf("draft visible: %+v", l.Items)
	}
	if _, err := w.myQuiz.Get(ctx, prin(w.userA), q.ID); err == nil {
		t.Fatal("draft visible via Get")
	}
}

func TestStart_OutsideWindowIsNotOpenEvenIfAssignmentOpen(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	w.clock.Advance(25 * time.Hour) // past closesAt
	_, _, err := w.attempts.Start(ctx, prin(w.userA), quizID)
	wantCode(t, err, service.CodeQuizNotOpen)
}

func TestStart_MaxAttemptsAndAlreadyPassed(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA) // maxAttempts=2
	p := prin(w.userA)

	for i := 1; i <= 2; i++ {
		v, created, err := w.attempts.Start(ctx, p, quizID)
		if err != nil || !created {
			t.Fatalf("attempt %d: %v", i, err)
		}
		res, err := w.attempts.Submit(ctx, p, v.ID, nil) // nothing answered => fail
		if err != nil || res.Passed || res.AttemptNo != i {
			t.Fatalf("submit %d: %+v %v", i, res, err)
		}
	}
	_, _, err := w.attempts.Start(ctx, p, quizID)
	wantCode(t, err, service.CodeMaxAttempts)
	if d := err.(*service.Error).Details; d["maxAttempts"] != 2 {
		t.Fatalf("details %v", d)
	}

	// Second user passes on the first try, then cannot retry.
	p2 := prin(w.addUser("usera2", entity.RoleUser, entity.UserActive, w.hospA.ID))
	v, _, _ := w.attempts.Start(ctx, p2, quizID)
	res, err := w.attempts.Submit(ctx, p2, v.ID, toPtrs(w.answerKey(v.ID)))
	if err != nil || !res.Passed {
		t.Fatalf("pass: %+v %v", res, err)
	}
	_, _, err = w.attempts.Start(ctx, p2, quizID)
	wantCode(t, err, service.CodeAlreadyPassed)
}

func TestStart_PoolTooSmallAtStart(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	for id, im := range w.db.images { // shrink the pool below questionCount
		if len(w.db.images) > 5 {
			delete(w.db.images, id)
			_ = im
		}
	}
	_, _, err := w.attempts.Start(ctx, prin(w.userA), quizID)
	wantCode(t, err, service.CodePoolTooSmall)
	if len(w.db.attempts) != 0 {
		t.Fatal("attempt persisted despite failed draw")
	}
}

func TestStart_SnapshotsSurviveLaterQuizAndPoolEdits(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	before := answerKeyCopy(w, v.ID)

	// Admin edits the quiz and deactivates images/types afterwards.
	in := w.quizInput()
	in.PassPercent, in.DurationSec, in.MaxAttempts = 100, 3600, 9
	if _, err := w.quizAdmin.Update(ctx, quizID, in, actor(w.admin)); err != nil {
		t.Fatal(err)
	}
	for _, im := range w.db.images {
		im.Active, im.TypeKey = false, "changed"
	}
	for i := range w.db.types {
		w.db.types[i].Active = false
	}

	// The running attempt is unchanged and still gradable.
	got, _ := w.attempts.Get(ctx, p, v.ID)
	if got.View.DurationSec != 600 || len(got.View.CellTypes) != 4 || len(got.View.Questions) != 10 {
		t.Fatalf("attempt view changed: %+v", got.View)
	}
	res, err := w.attempts.Submit(ctx, p, v.ID, toPtrs(before))
	if err != nil {
		t.Fatal(err)
	}
	if res.PassPercent != 60 || !res.Passed || res.Correct == nil || *res.Correct != 10 {
		t.Fatalf("result uses live data instead of snapshot: %+v", res)
	}
}

func answerKeyCopy(w *world, id string) map[string]string { return w.answerKey(id) }

func toPtrs(m map[string]string) map[string]*string {
	out := make(map[string]*string, len(m))
	for k, v := range m {
		v := v
		out[k] = &v
	}
	return out
}

func TestStart_ConcurrentStartsCreateOneAttempt(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)

	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, _, err := w.attempts.Start(ctx, p, quizID)
			if err != nil {
				t.Errorf("start %d: %v", i, err)
				return
			}
			ids[i] = v.ID
		}(i)
	}
	wg.Wait()
	if len(w.db.attempts) != 1 {
		t.Fatalf("%d attempts created by concurrent starts", len(w.db.attempts))
	}
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("callers got different attempts: %v", ids)
		}
	}
}

func TestStart_StalePastGraceAttemptExpiresAndConsumesATry(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	w.clock.Advance(10*time.Minute + 31*time.Second) // past expiresAt + grace

	v2, created, err := w.attempts.Start(ctx, p, quizID)
	if err != nil || !created || v2.ID == v.ID {
		t.Fatalf("new attempt expected: %+v created=%v err=%v", v2, created, err)
	}
	id, _ := primitive.ObjectIDFromHex(v.ID)
	if w.db.attempts[id].Status != entity.AttemptExpired {
		t.Fatalf("stale attempt not expired: %s", w.db.attempts[id].Status)
	}
	if w.db.attempts[primitive.NilObjectID] != nil {
		t.Fatal("unexpected")
	}
	// Both tries are now used.
	w.clock.Advance(11 * time.Minute)
	_, _, err = w.attempts.Start(ctx, p, quizID)
	wantCode(t, err, service.CodeMaxAttempts)
}

// ------------------------------------------------------------- autosave

func TestSaveAnswers_ValidationMergeClearAndOwnership(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA, w.hospB}, w.hospA, w.hospB)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	q0, q1 := v.Questions[0].ID, v.Questions[1].ID

	out, err := w.attempts.SaveAnswers(ctx, p, v.ID, map[string]*string{q0: strp("neutrophil"), q1: strp("monocyte")})
	if err != nil || out.AnsweredCount != 2 || !out.ExpiresAt.Equal(v.ExpiresAt) {
		t.Fatalf("save: %+v %v", out, err)
	}
	// merge one more and clear one with null
	out, err = w.attempts.SaveAnswers(ctx, p, v.ID, map[string]*string{q0: nil, v.Questions[2].ID: strp("lymphocyte")})
	if err != nil || out.AnsweredCount != 2 {
		t.Fatalf("merge/clear: %+v %v", out, err)
	}

	// Unknown question id and unknown type key are 400; basophil is inactive => not an option.
	_, err = w.attempts.SaveAnswers(ctx, p, v.ID, map[string]*string{"zzzzzzzz": strp("neutrophil")})
	wantCode(t, err, service.CodeValidation)
	_, err = w.attempts.SaveAnswers(ctx, p, v.ID, map[string]*string{q0: strp("basophil")})
	wantCode(t, err, service.CodeValidation)
	_, err = w.attempts.SaveAnswers(ctx, p, v.ID, map[string]*string{q0: strp("nonsense")})
	wantCode(t, err, service.CodeValidation)

	// Someone else's attempt is 404 (same hospital or not).
	_, err = w.attempts.SaveAnswers(ctx, prin(w.userB), v.ID, map[string]*string{q0: strp("neutrophil")})
	wantCode(t, err, service.CodeNotFound)
	_, err = w.attempts.Get(ctx, prin(w.userB), v.ID)
	wantCode(t, err, service.CodeNotFound)
	_, err = w.attempts.Result(ctx, prin(w.userB), v.ID)
	wantCode(t, err, service.CodeNotFound)
	_, err = w.attempts.Submit(ctx, prin(w.userB), v.ID, nil)
	wantCode(t, err, service.CodeNotFound)
	_, err = w.attempts.Image(ctx, prin(w.userB), v.ID, q0)
	wantCode(t, err, service.CodeNotFound)
}

func TestSaveAnswers_GraceAndExpiry(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	q0 := v.Questions[0].ID

	w.clock.Advance(10*time.Minute + 20*time.Second) // inside grace
	if _, err := w.attempts.SaveAnswers(ctx, p, v.ID, map[string]*string{q0: strp("neutrophil")}); err != nil {
		t.Fatalf("save inside grace: %v", err)
	}
	w.clock.Advance(11 * time.Second) // now past expiresAt+30s
	_, err := w.attempts.SaveAnswers(ctx, p, v.ID, map[string]*string{q0: strp("lymphocyte")})
	wantCode(t, err, service.CodeAttemptExpired)

	// Already-submitted attempts reject autosave with NOT_IN_PROGRESS.
	w2 := newWorld(t)
	q2 := w2.publishedQuiz([]*entity.Hospital{w2.hospA}, w2.hospA)
	pa := prin(w2.userA)
	v2, _, _ := w2.attempts.Start(ctx, pa, q2)
	if _, err := w2.attempts.Submit(ctx, pa, v2.ID, nil); err != nil {
		t.Fatal(err)
	}
	_, err = w2.attempts.SaveAnswers(ctx, pa, v2.ID, map[string]*string{v2.Questions[0].ID: strp("neutrophil")})
	wantCode(t, err, service.CodeAttemptNotInProg)
}

// ------------------------------------------------------------- submit / scoring

func TestSubmit_OneCorrectOfTenIsTenPercentNotHundred(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	key := w.answerKey(v.ID)
	q0 := v.Questions[0].ID

	res, err := w.attempts.Submit(ctx, p, v.ID, map[string]*string{q0: strp(key[q0])})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 10 || res.Correct == nil || *res.Correct != 1 || res.Percent == nil || *res.Percent != 10 {
		t.Fatalf("scoring wrong: %+v", res)
	}
	if res.Unanswered == nil || *res.Unanswered != 9 || res.Passed {
		t.Fatalf("unanswered/passed wrong: %+v", res)
	}
	if res.Certificate != nil {
		t.Fatal("certificate issued for a failed attempt")
	}
	if res.AttemptsUsed != 1 || res.AttemptsRemaining != 1 || !res.CanRetry {
		t.Fatalf("attempts bookkeeping: %+v", res)
	}
	if len(res.PerType) == 0 {
		t.Fatal("perType missing")
	}
	total := 0
	for _, pt := range res.PerType {
		total += pt.Total
	}
	if total != 10 {
		t.Fatalf("perType totals %d", total)
	}
}

func TestSubmit_PassIssuesExactlyOneCertificateAndIsIdempotent(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)

	res, err := w.attempts.Submit(ctx, p, v.ID, toPtrs(w.answerKey(v.ID)))
	if err != nil || !res.Passed || res.Certificate == nil || res.Certificate.CertNo == "" {
		t.Fatalf("pass: %+v %v", res, err)
	}
	if res.CanRetry {
		t.Fatal("canRetry after passing")
	}
	again, err := w.attempts.Submit(ctx, p, v.ID, map[string]*string{v.Questions[0].ID: strp("nonsense")})
	if err != nil {
		t.Fatalf("idempotent resubmit: %v", err)
	}
	if again.Certificate.ID != res.Certificate.ID || again.Certificate.CertNo != res.Certificate.CertNo ||
		*again.Correct != *res.Correct || *again.Percent != *res.Percent {
		t.Fatalf("resubmit changed the result: %+v vs %+v", again, res)
	}
	if len(w.db.certs) != 1 {
		t.Fatalf("%d certificates", len(w.db.certs))
	}
	// Snapshot fields.
	for _, c := range w.db.certs {
		if c.RecipientName != w.userA.FullName || c.HospitalName != "Hospital A" || c.QuizTitle != "Quiz 1" || c.Percent != 100 {
			t.Fatalf("certificate snapshot: %+v", c)
		}
	}
	// Reading via Get/Result gives the same thing.
	st, _ := w.attempts.Get(ctx, p, v.ID)
	r2, err := w.attempts.Result(ctx, p, v.ID)
	if err != nil || st.Result == nil || r2.Certificate.CertNo != res.Certificate.CertNo {
		t.Fatalf("get/result: %+v %+v %v", st, r2, err)
	}
}

func TestSubmit_ConcurrentSubmitsProduceOneResultAndOneCertificate(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	answers := toPtrs(w.answerKey(v.ID))

	var wg sync.WaitGroup
	nos := make([]string, 10)
	for i := range nos {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := w.attempts.Submit(ctx, p, v.ID, answers)
			if err != nil {
				t.Errorf("submit %d: %v", i, err)
				return
			}
			nos[i] = r.Certificate.CertNo
		}(i)
	}
	wg.Wait()
	if len(w.db.certs) != 1 {
		t.Fatalf("%d certificates after concurrent submits", len(w.db.certs))
	}
	for _, n := range nos {
		if n != nos[0] {
			t.Fatalf("different cert numbers: %v", nos)
		}
	}
}

func TestSubmit_ExpiredAttemptIsNotScoredCountsAsTryAndHasNoCertificate(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)

	w.clock.Advance(10*time.Minute + 31*time.Second)
	_, err := w.attempts.Submit(ctx, p, v.ID, toPtrs(w.answerKey(v.ID)))
	wantCode(t, err, service.CodeAttemptExpired)

	id, _ := primitive.ObjectIDFromHex(v.ID)
	if w.db.attempts[id].Status != entity.AttemptExpired {
		t.Fatalf("status %s", w.db.attempts[id].Status)
	}
	if len(w.db.certs) != 0 {
		t.Fatal("certificate for an expired attempt")
	}
	// GET now returns an expired AttemptResult with null score fields.
	st, err := w.attempts.Get(ctx, p, v.ID)
	if err != nil || st.Result == nil {
		t.Fatalf("get expired: %+v %v", st, err)
	}
	r := st.Result
	if r.Status != entity.AttemptExpired || r.Correct != nil || r.Percent != nil || r.Unanswered != nil || r.Passed || r.Total != 10 {
		t.Fatalf("expired result: %+v", r)
	}
	if r.AttemptsUsed != 1 || r.AttemptsRemaining != 1 {
		t.Fatalf("expired attempt must count as a try: %+v", r)
	}
	// Submitting again is still ATTEMPT_EXPIRED, and images are gone.
	_, err = w.attempts.Submit(ctx, p, v.ID, nil)
	wantCode(t, err, service.CodeAttemptExpired)
	_, err = w.attempts.Image(ctx, p, v.ID, v.Questions[0].ID)
	wantCode(t, err, service.CodeNotFound)
	// Result endpoint works for expired attempts.
	if _, err := w.attempts.Result(ctx, p, v.ID); err != nil {
		t.Fatalf("result of expired: %v", err)
	}
}

func TestSubmit_WithinGraceIsAccepted(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	w.clock.Advance(10*time.Minute + 29*time.Second)
	res, err := w.attempts.Submit(ctx, p, v.ID, toPtrs(w.answerKey(v.ID)))
	if err != nil || !res.Passed {
		t.Fatalf("submit inside grace: %+v %v", res, err)
	}
}

func TestSubmit_InvalidAnswersRejectedAndAttemptStaysOpen(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	_, err := w.attempts.Submit(ctx, p, v.ID, map[string]*string{"bogusbogus": strp("neutrophil")})
	wantCode(t, err, service.CodeValidation)
	_, err = w.attempts.Result(ctx, p, v.ID)
	wantCode(t, err, service.CodeAttemptNotSubmit)
}

func TestSubmit_PassBoundaryUsesIntegerMath(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA) // 10 questions, pass 60%
	p := prin(w.userA)

	for _, tc := range []struct {
		correct int
		pass    bool
	}{{5, false}, {6, true}} {
		u := prin(w.addUser(time.Now().Format("150405.000000")+string(rune('a'+tc.correct)), entity.RoleUser, entity.UserActive, w.hospA.ID))
		v, _, _ := w.attempts.Start(ctx, u, quizID)
		key := w.answerKey(v.ID)
		ans := map[string]*string{}
		for i, q := range v.Questions {
			if i < tc.correct {
				ans[q.ID] = strp(key[q.ID])
			}
		}
		res, err := w.attempts.Submit(ctx, u, v.ID, ans)
		if err != nil || res.Passed != tc.pass {
			t.Fatalf("correct=%d: passed=%v err=%v", tc.correct, res.Passed, err)
		}
	}
	_ = p
}

func TestCertificateNumberCollisionRetries(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	// Occupy a certNo, then make the generator return it twice before a fresh one.
	w.db.certs[primitive.NewObjectID()] = &entity.Certificate{CertNo: "DIEQA-2026-TAKEN000", AttemptID: primitive.NewObjectID(), QuizID: primitive.NewObjectID(), UserID: primitive.NewObjectID()}
	w.rnd.certNos = []string{"DIEQA-2026-TAKEN000", "DIEQA-2026-TAKEN000", "DIEQA-2026-FRESH000"}

	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	res, err := w.attempts.Submit(ctx, p, v.ID, toPtrs(w.answerKey(v.ID)))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if res.Certificate.CertNo != "DIEQA-2026-FRESH000" {
		t.Fatalf("certNo %s", res.Certificate.CertNo)
	}
}

func TestCertificateNumberCollisionGivesUpAfterFiveTries(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	w.db.certs[primitive.NewObjectID()] = &entity.Certificate{CertNo: "DIEQA-2026-TAKEN000", AttemptID: primitive.NewObjectID(), QuizID: primitive.NewObjectID(), UserID: primitive.NewObjectID()}
	w.rnd.certNos = []string{"DIEQA-2026-TAKEN000", "DIEQA-2026-TAKEN000", "DIEQA-2026-TAKEN000", "DIEQA-2026-TAKEN000", "DIEQA-2026-TAKEN000"}

	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	_, err := w.attempts.Submit(ctx, p, v.ID, toPtrs(w.answerKey(v.ID)))
	wantCode(t, err, service.CodeInternal)
	if len(err.Error()) > 0 && containsDriverText(err.Error()) {
		t.Fatalf("internal error leaks detail: %v", err)
	}
	// The attempt is submitted; a retry self-heals by issuing the certificate.
	res, err := w.attempts.Submit(ctx, p, v.ID, nil)
	if err != nil || res.Certificate == nil {
		t.Fatalf("self-heal: %+v %v", res, err)
	}
}

func containsDriverText(s string) bool {
	return len(s) > 0 && (contains(s, "mongo") || contains(s, "duplicate key"))
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------- images / history / privacy

func TestImage_OpaqueAccessOnlyForOwnerWhileInProgress(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	q := v.Questions[0]

	img, err := w.attempts.Image(ctx, p, v.ID, q.ID)
	if err != nil || img.ContentType != "image/jpeg" || len(img.Data) == 0 {
		t.Fatalf("image: %+v %v", img, err)
	}
	if _, err := w.attempts.Image(ctx, p, v.ID, "unknownqid"); err == nil {
		t.Fatal("unknown qid served")
	}
	// Past expiresAt (but inside grace) images stop.
	w.clock.Advance(10*time.Minute + time.Second)
	_, err = w.attempts.Image(ctx, p, v.ID, q.ID)
	wantCode(t, err, service.CodeNotFound)

	// After submit, images are gone too.
	w2 := newWorld(t)
	q2 := w2.publishedQuiz([]*entity.Hospital{w2.hospA}, w2.hospA)
	pa := prin(w2.userA)
	v2, _, _ := w2.attempts.Start(ctx, pa, q2)
	if _, err := w2.attempts.Submit(ctx, pa, v2.ID, nil); err != nil {
		t.Fatal(err)
	}
	_, err = w2.attempts.Image(ctx, pa, v2.ID, v2.Questions[0].ID)
	wantCode(t, err, service.CodeNotFound)
}

func TestViewsNeverContainAnswerKeyOrStoragePaths(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)
	v, _, _ := w.attempts.Start(ctx, p, quizID)

	assertClean := func(label string, v any) {
		t.Helper()
		b := mustJSON(t, v)
		id, _ := primitive.ObjectIDFromHex(v0ID(label, b))
		_ = id
		for _, secret := range []string{"correctType", "imageId", "typeKey", ".jpg", "inactive", "path"} {
			if contains(b, secret) {
				t.Fatalf("%s leaks %q: %s", label, secret, b)
			}
		}
	}
	assertClean("start view", v)
	res, err := w.attempts.Submit(ctx, p, v.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertClean("result", res)
	hist, _ := w.attempts.History(ctx, p, "", 1, 20)
	assertClean("history", hist)
	card, _ := w.myQuiz.Get(ctx, p, quizID)
	assertClean("card", card)
}

func v0ID(string, string) string { return "" }

func TestHistoryAndCardsReflectAttempts(t *testing.T) {
	w := newWorld(t)
	quizID := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	p := prin(w.userA)

	card, err := w.myQuiz.Get(ctx, p, quizID)
	if err != nil || card.Availability != entity.AvailabilityOpen || card.AttemptsRemaining != 2 || card.InProgressAttempt != nil {
		t.Fatalf("fresh card: %+v %v", card, err)
	}
	v, _, _ := w.attempts.Start(ctx, p, quizID)
	card, _ = w.myQuiz.Get(ctx, p, quizID)
	if card.InProgressAttempt == nil || card.InProgressAttempt.ID != v.ID || card.AttemptsUsed != 1 {
		t.Fatalf("card in progress: %+v", card)
	}
	if _, err := w.attempts.Submit(ctx, p, v.ID, toPtrs(w.answerKey(v.ID))); err != nil {
		t.Fatal(err)
	}
	card, _ = w.myQuiz.Get(ctx, p, quizID)
	if !card.Passed || card.BestResult == nil || card.BestResult.Percent != 100 || card.Certificate == nil {
		t.Fatalf("passed card: %+v", card)
	}
	hist, err := w.attempts.History(ctx, p, quizID, 1, 20)
	if err != nil || hist.Total != 1 || hist.Items[0].CertNo == nil || !hist.Items[0].Passed {
		t.Fatalf("history: %+v %v", hist, err)
	}
	// Another user's history is empty and certificates are private.
	h2, _ := w.attempts.History(ctx, prin(w.userB), "", 1, 20)
	if h2.Total != 0 {
		t.Fatal("history leaked across users")
	}
	mine, _ := w.certs.ListMine(ctx, p)
	theirs, _ := w.certs.ListMine(ctx, prin(w.userB))
	if len(mine) != 1 || len(theirs) != 0 {
		t.Fatalf("certs mine=%d theirs=%d", len(mine), len(theirs))
	}
	if _, err := w.certs.GetMine(ctx, prin(w.userB), mine[0].ID); err == nil {
		t.Fatal("certificate readable by another user")
	}
	if c, err := w.certs.GetMine(ctx, p, mine[0].ID); err != nil || c.CertNo != card.Certificate.CertNo {
		t.Fatalf("own cert: %+v %v", c, err)
	}
}

func TestMyQuizList_OrderAndScopingAcrossHospitals(t *testing.T) {
	w := newWorld(t)
	closed := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	if _, err := w.quizAdmin.CloseAssignment(ctx, closed, w.hospA.ID.Hex(), actor(w.admin)); err != nil {
		t.Fatal(err)
	}
	upcoming := w.publishedQuiz([]*entity.Hospital{w.hospA})
	open := w.publishedQuiz([]*entity.Hospital{w.hospA}, w.hospA)
	other := w.publishedQuiz([]*entity.Hospital{w.hospB}, w.hospB)

	out, err := w.myQuiz.List(ctx, prin(w.userA))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range out.Items {
		got = append(got, c.ID)
	}
	want := []string{open, upcoming, closed}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		sort.Strings(got)
		t.Fatalf("order/scoping wrong: got %v want %v (other hospital quiz %s must be absent)", got, want, other)
	}
	if !out.ServerTime.Equal(w.clock.Now()) {
		t.Fatal("serverTime")
	}
	if _, err := w.myQuiz.Get(ctx, prin(w.userA), other); err == nil {
		t.Fatal("quiz of another hospital readable")
	}
}

var _ = service.CodeInternal
