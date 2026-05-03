package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const quizCellCount = 20

// QuizService handles quiz, submission and leaderboard use cases.
type QuizService struct {
	quizzes     port.QuizRepository
	sessions    port.SessionRepository
	submissions port.SubmissionRepository
	users       port.UserRepository
	hospitals   port.HospitalRepository
	cache       port.CachePort
	events      port.EventPort
	supabaseURL    string
	supabaseBucket string
}

func NewQuizService(
	quizzes port.QuizRepository,
	sessions port.SessionRepository,
	submissions port.SubmissionRepository,
	users port.UserRepository,
	hospitals port.HospitalRepository,
	cache port.CachePort,
	events port.EventPort,
	supabaseURL, supabaseBucket string,
) *QuizService {
	return &QuizService{
		quizzes:        quizzes,
		sessions:       sessions,
		submissions:    submissions,
		users:          users,
		hospitals:      hospitals,
		cache:          cache,
		events:         events,
		supabaseURL:    supabaseURL,
		supabaseBucket: supabaseBucket,
	}
}

func (s *QuizService) buildImageURL(path string) string {
	if s.supabaseURL == "" || path == "" {
		return ""
	}
	return strings.TrimRight(s.supabaseURL, "/") +
		"/storage/v1/object/public/" + s.supabaseBucket + "/" + path
}

// ──────────────────────────────────────────────────────────────────────────
// List
// ──────────────────────────────────────────────────────────────────────────

// QuizListItem is the DTO returned by List.
type QuizListItem struct {
	ID          primitive.ObjectID `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Category    string             `json:"category"`
	CellCount   int                `json:"cellCount"`
	PassPercent int                `json:"passPercent"`
	DurationSec int                `json:"durationSec"`
}

func (s *QuizService) List(ctx context.Context, role, hospitalIDHex string) ([]QuizListItem, error) {
	var quizzes []entity.Quiz
	var err error

	if role == entity.RoleAdmin || role == entity.RoleInstructor {
		quizzes, err = s.quizzes.ListAll(ctx)
	} else {
		var hospID *primitive.ObjectID
		if hospitalIDHex != "" {
			if oid, e := parseOID(hospitalIDHex); e == nil {
				hospID = &oid
			}
		}
		activeIDs, e := s.sessions.FindActiveQuizIDs(ctx, hospID)
		if e != nil {
			return nil, e
		}
		if len(activeIDs) == 0 {
			return []QuizListItem{}, nil
		}
		quizzes, err = s.quizzes.ListByIDs(ctx, activeIDs)
	}
	if err != nil {
		return nil, err
	}

	out := make([]QuizListItem, 0, len(quizzes))
	for _, q := range quizzes {
		out = append(out, QuizListItem{
			ID: q.ID, Title: q.Title, Description: q.Description,
			Category: q.Category, CellCount: len(q.Cells),
			PassPercent: q.PassPercent, DurationSec: q.DurationSec,
		})
	}
	return out, nil
}

// ──────────────────────────────────────────────────────────────────────────
// Get (returns quiz without correct answers, stores attempt in cache)
// ──────────────────────────────────────────────────────────────────────────

// QuizGetOutput is the DTO returned by Get.
type QuizGetOutput struct {
	ID          primitive.ObjectID     `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	PassPercent int                    `json:"passPercent"`
	DurationSec int                    `json:"durationSec"`
	Categories  []entity.CellCategory  `json:"categories"`
	Cells       []entity.CellImage     `json:"cells"`
}

func (s *QuizService) Get(ctx context.Context, quizIDHex, userIDHex, sessionIDHex, role, hospitalIDHex string) (*QuizGetOutput, error) {
	id, err := parseOID(quizIDHex)
	if err != nil {
		return nil, ErrNotFound
	}

	// If session is specified and user is not admin, validate session is running
	if sessionIDHex != "" && role != entity.RoleAdmin {
		sessionOID, err := parseOID(sessionIDHex)
		if err != nil {
			return nil, fmt.Errorf("invalid session id")
		}
		sess, err := s.sessions.FindByID(ctx, sessionOID)
		if err != nil {
			return nil, ErrNotFound
		}
		if sess.Status != entity.SessionRunning {
			return nil, ErrSessionNotRunning
		}
		if hospitalIDHex != "" {
			userHospID, err := parseOID(hospitalIDHex)
			if err != nil || userHospID != sess.HospitalID {
				return nil, ErrForbidden
			}
		}
	}

	// Try cache for quiz metadata
	var quiz entity.Quiz
	metaKey := "quiz:meta:" + id.Hex()
	if cached, err := s.cache.Get(ctx, metaKey); err == nil {
		_ = json.Unmarshal([]byte(cached), &quiz)
	}
	if quiz.ID.IsZero() {
		q, err := s.quizzes.FindByID(ctx, id)
		if err != nil {
			return nil, ErrNotFound
		}
		quiz = *q
		if data, err := json.Marshal(quiz); err == nil {
			_ = s.cache.Set(ctx, metaKey, string(data), 10*time.Minute)
		}
	}

	// Shuffle and pick cells
	cells := make([]entity.CellImage, len(quiz.Cells))
	copy(cells, quiz.Cells)
	mrand.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
	if len(cells) > quizCellCount {
		cells = cells[:quizCellCount]
	}

	// Store attempt in cache for submit validation
	if userIDHex != "" {
		attemptKey := fmt.Sprintf("quiz:attempt:%s:%s", id.Hex(), userIDHex)
		cellIDs := make([]string, len(cells))
		for i, c := range cells {
			cellIDs[i] = c.ID
		}
		if data, err := json.Marshal(cellIDs); err == nil {
			ttl := time.Duration(quiz.DurationSec+300) * time.Second
			_ = s.cache.Set(ctx, attemptKey, string(data), ttl)
		}
	}

	// Build public cells (no CorrectType, inject ImageURL)
	publicCells := make([]entity.CellImage, len(cells))
	for i, cell := range cells {
		publicCells[i] = entity.CellImage{
			ID:       cell.ID,
			ImageURL: s.buildImageURL(cell.Path),
		}
	}

	return &QuizGetOutput{
		ID: quiz.ID, Title: quiz.Title, Description: quiz.Description,
		Category: quiz.Category, PassPercent: quiz.PassPercent, DurationSec: quiz.DurationSec,
		Categories: quiz.Categories, Cells: publicCells,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────
// Submit
// ──────────────────────────────────────────────────────────────────────────

// SubmitInput carries user answers.
type SubmitInput struct {
	QuizIDHex   string
	UserIDHex   string
	SessionID   string
	DurationSec int
	Assignments map[string]string
}

func generateCertificateID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return "DIEQA-" + strings.ToUpper(hex.EncodeToString(b))
}

func (s *QuizService) Submit(ctx context.Context, in SubmitInput) (*entity.Submission, error) {
	quizID, err := parseOID(in.QuizIDHex)
	if err != nil {
		return nil, ErrNotFound
	}
	userID, err := parseOID(in.UserIDHex)
	if err != nil {
		return nil, ErrForbidden
	}

	// Dedupe lock
	dedupeKey := fmt.Sprintf("submit:lock:%s:%s", quizID.Hex(), userID.Hex())
	ok, err := s.cache.SetNX(ctx, dedupeKey, "1", 10*time.Second)
	if err == nil && !ok {
		return nil, ErrDuplicateSubmit
	}

	quiz, err := s.quizzes.FindByID(ctx, quizID)
	if err != nil {
		return nil, ErrNotFound
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, ErrForbidden
	}
	var hospital entity.Hospital
	if h, err := s.hospitals.FindByID(ctx, user.HospitalID); err == nil {
		hospital = *h
	}

	// Retrieve shown cell IDs from cache
	attemptKey := fmt.Sprintf("quiz:attempt:%s:%s", quizID.Hex(), userID.Hex())
	var shownCellIDs []string
	if raw, err := s.cache.Get(ctx, attemptKey); err == nil {
		_ = json.Unmarshal([]byte(raw), &shownCellIDs)
	}

	cellMap := make(map[string]entity.CellImage, len(quiz.Cells))
	for _, cell := range quiz.Cells {
		cellMap[cell.ID] = cell
	}

	if len(shownCellIDs) == 0 {
		for cellID := range in.Assignments {
			if _, ok := cellMap[cellID]; ok {
				shownCellIDs = append(shownCellIDs, cellID)
			}
		}
	}

	validKeys := make(map[string]bool, len(quiz.Categories))
	for _, cat := range quiz.Categories {
		validKeys[cat.Key] = true
	}

	answers := make([]entity.CellAnswer, 0, len(shownCellIDs))
	correct := 0
	userCounts := make(map[string]int)
	trueCounts := make(map[string]int)
	correctPerCat := make(map[string]int)

	for _, cellID := range shownCellIDs {
		cell, ok := cellMap[cellID]
		if !ok {
			continue
		}
		assigned := in.Assignments[cell.ID]
		if !validKeys[assigned] {
			assigned = ""
		}
		isCorrect := assigned != "" && assigned == cell.CorrectType
		if isCorrect {
			correct++
			correctPerCat[assigned]++
		}
		if assigned != "" {
			userCounts[assigned]++
		}
		trueCounts[cell.CorrectType]++
		answers = append(answers, entity.CellAnswer{
			CellID:       cell.ID,
			ImageURL:     s.buildImageURL(cell.Path),
			AssignedType: assigned,
			CorrectType:  cell.CorrectType,
			IsCorrect:    isCorrect,
		})
	}

	categoryStats := make([]entity.CategoryStat, 0, len(quiz.Categories))
	for _, cat := range quiz.Categories {
		categoryStats = append(categoryStats, entity.CategoryStat{
			Key: cat.Key, Label: cat.Label, Color: cat.Color,
			UserCount: userCounts[cat.Key],
			TrueCount: trueCounts[cat.Key],
			Correct:   correctPerCat[cat.Key],
		})
	}

	total := len(answers)
	percent := 0
	if total > 0 {
		percent = int(float64(correct) * 100.0 / float64(total))
	}
	passPercent := quiz.PassPercent
	if passPercent <= 0 {
		passPercent = 80
	}
	passed := percent >= passPercent
	certID := ""
	if passed {
		certID = generateCertificateID()
	}

	var sessionOID *primitive.ObjectID
	if in.SessionID != "" {
		if oid, err := parseOID(in.SessionID); err == nil {
			sessionOID = &oid
		}
	}

	sub := &entity.Submission{
		QuizID: quiz.ID, QuizTitle: quiz.Title,
		SessionID:    sessionOID,
		UserID:       user.ID,
		HospitalID:   user.HospitalID,
		Username:     user.Username,
		FullName:     user.FullName,
		HospitalName: hospital.Name,
		Score:        correct, Total: total, Correct: correct,
		Percent: percent, PassPercent: passPercent,
		Passed: passed, CertificateID: certID,
		Categories:  categoryStats,
		Answers:     answers,
		DurationSec: in.DurationSec,
		SubmittedAt: time.Now(),
	}
	id, err := s.submissions.Create(ctx, sub)
	if err != nil {
		return nil, err
	}
	sub.ID = id

	// Update leaderboard cache & broadcast if session exists
	if sessionOID != nil {
		room := "session:" + sessionOID.Hex()
		zKey := "leaderboard:" + sessionOID.Hex()
		metaKey := "leaderboard:meta:" + sessionOID.Hex()

		_ = s.cache.ZAdd(ctx, zKey, float64(correct), user.ID.Hex())
		meta := mustJSON(map[string]interface{}{
			"userId": user.ID.Hex(), "username": user.Username,
			"fullName": user.FullName, "hospital": hospital.Name,
			"score": correct, "total": total, "correct": correct,
			"percent": percent, "passed": passed,
		})
		_ = s.cache.HSet(ctx, metaKey, user.ID.Hex(), meta)
		_ = s.cache.Expire(ctx, zKey, 24*time.Hour)
		_ = s.cache.Expire(ctx, metaKey, 24*time.Hour)

		s.events.Broadcast(room, port.Event{
			Type: "leaderboard:update",
			Payload: map[string]interface{}{
				"userId": user.ID.Hex(), "username": user.Username,
				"fullName": user.FullName, "hospital": hospital.Name,
				"score": correct, "total": total, "correct": correct,
				"percent": percent, "passed": passed,
			},
		})
		s.events.Broadcast(room, port.Event{
			Type:    "submission:new",
			Payload: map[string]interface{}{"username": user.Username, "score": correct, "total": total, "percent": percent},
		})
	}

	return sub, nil
}

// ──────────────────────────────────────────────────────────────────────────
// Leaderboard
// ──────────────────────────────────────────────────────────────────────────

func (s *QuizService) Leaderboard(ctx context.Context, sessionIDHex string, limit int64) ([]map[string]interface{}, error) {
	sid, err := parseOID(sessionIDHex)
	if err != nil {
		return nil, fmt.Errorf("invalid session id")
	}
	zKey := "leaderboard:" + sid.Hex()
	metaKey := "leaderboard:meta:" + sid.Hex()
	zs, err := s.cache.ZRevRangeWithScores(ctx, zKey, 0, limit-1)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(zs))
	for i, z := range zs {
		entry := map[string]interface{}{
			"rank": i + 1, "userId": z.Member, "score": int(z.Score),
		}
		if data, err := s.cache.HGet(ctx, metaKey, z.Member); err == nil {
			var meta map[string]interface{}
			if json.Unmarshal([]byte(data), &meta) == nil {
				for k, v := range meta {
					entry[k] = v
				}
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// ──────────────────────────────────────────────────────────────────────────
// Submission history
// ──────────────────────────────────────────────────────────────────────────

func (s *QuizService) MyHistory(ctx context.Context, userIDHex string) ([]entity.Submission, error) {
	userID, err := parseOID(userIDHex)
	if err != nil {
		return nil, ErrForbidden
	}
	return s.submissions.FindByUser(ctx, userID, 50)
}

func (s *QuizService) GetSubmission(ctx context.Context, submissionIDHex, userIDHex string) (*entity.Submission, error) {
	subID, err := parseOID(submissionIDHex)
	if err != nil {
		return nil, ErrNotFound
	}
	userID, err := parseOID(userIDHex)
	if err != nil {
		return nil, ErrForbidden
	}
	sub, err := s.submissions.FindByIDAndUser(ctx, subID, userID)
	if err != nil {
		return nil, ErrNotFound
	}
	return sub, nil
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
