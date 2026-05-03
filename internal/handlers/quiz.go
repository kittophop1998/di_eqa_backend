package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/models"
	"github.com/di-eqa/backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const quizCellCount = 20

type QuizHandler struct {
	QuizColl       *mongo.Collection
	SubmissionColl *mongo.Collection
	HospitalColl   *mongo.Collection
	UsersColl      *mongo.Collection
	SessionColl    *mongo.Collection
	Redis          *redis.Client
	Hub            *ws.Hub
	SupabaseURL    string // base URL เช่น https://xxxx.supabase.co
	SupabaseBucket string // ชื่อ bucket เช่น "cell-images"
}

// buildImageURL สร้าง public URL จาก relative path บน Supabase
func (h *QuizHandler) buildImageURL(path string) string {
	if h.SupabaseURL == "" || path == "" {
		return ""
	}
	return strings.TrimRight(h.SupabaseURL, "/") +
		"/storage/v1/object/public/" + h.SupabaseBucket + "/" + path
}

// List คืนรายการข้อสอบ — แสดง cellCount แทน questionCount
func (h *QuizHandler) List(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cur, err := h.QuizColl.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(ctx)

	var quizzes []models.Quiz
	if err := cur.All(ctx, &quizzes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type item struct {
		ID          primitive.ObjectID `json:"id"`
		Title       string             `json:"title"`
		Description string             `json:"description"`
		Category    string             `json:"category"`
		CellCount   int                `json:"cellCount"`
		PassPercent int                `json:"passPercent"`
		DurationSec int                `json:"durationSec"`
	}
	out := make([]item, 0, len(quizzes))
	for _, q := range quizzes {
		out = append(out, item{
			ID: q.ID, Title: q.Title, Description: q.Description, Category: q.Category,
			CellCount: len(q.Cells), PassPercent: q.PassPercent, DurationSec: q.DurationSec,
		})
	}
	c.JSON(http.StatusOK, out)
}

// Get คืนข้อสอบ "แบบไม่มีเฉลย" — สุ่ม 20 เซลล์และเก็บ attempt ใน Redis
func (h *QuizHandler) Get(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	uid, _ := c.Get("userId")
	userIDStr, _ := uid.(string)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Cache ข้อมูล quiz (ไม่รวม cells ที่สุ่ม) เพื่อลด DB query
	quizMetaKey := "quiz:meta:" + id.Hex()
	var quiz models.Quiz
	if cached, err := h.Redis.Get(ctx, quizMetaKey).Result(); err == nil {
		if err2 := json.Unmarshal([]byte(cached), &quiz); err2 != nil {
			if err3 := h.QuizColl.FindOne(ctx, bson.M{"_id": id}).Decode(&quiz); err3 != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "quiz not found"})
				return
			}
		}
	} else {
		if err := h.QuizColl.FindOne(ctx, bson.M{"_id": id}).Decode(&quiz); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "quiz not found"})
			return
		}
		if data, err := json.Marshal(quiz); err == nil {
			_ = h.Redis.Set(ctx, quizMetaKey, data, 10*time.Minute).Err()
		}
	}

	// สุ่มเลือก quizCellCount เซลล์จากทั้งหมด
	cells := make([]models.CellImage, len(quiz.Cells))
	copy(cells, quiz.Cells)
	mrand.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
	if len(cells) > quizCellCount {
		cells = cells[:quizCellCount]
	}

	// เก็บ cell IDs ที่สุ่มได้ใน Redis สำหรับใช้ตอน Submit
	if userIDStr != "" {
		attemptKey := fmt.Sprintf("quiz:attempt:%s:%s", id.Hex(), userIDStr)
		cellIDs := make([]string, len(cells))
		for i, c := range cells {
			cellIDs[i] = c.ID
		}
		if data, err := json.Marshal(cellIDs); err == nil {
			ttl := time.Duration(quiz.DurationSec+300) * time.Second
			_ = h.Redis.Set(ctx, attemptKey, data, ttl).Err()
		}
	}

	// ตัด CorrectType ออกก่อนส่ง client และ inject ImageURL จาก Path
	publicCells := make([]models.CellImage, len(cells))
	for i, cell := range cells {
		publicCells[i] = models.CellImage{
			ID:       cell.ID,
			ImageURL: h.buildImageURL(cell.Path),
		}
	}

	publicQuiz := struct {
		ID          primitive.ObjectID    `json:"id"`
		Title       string                `json:"title"`
		Description string                `json:"description"`
		Category    string                `json:"category"`
		PassPercent int                   `json:"passPercent"`
		DurationSec int                   `json:"durationSec"`
		Categories  []models.CellCategory `json:"categories"`
		Cells       []models.CellImage    `json:"cells"`
	}{
		ID: quiz.ID, Title: quiz.Title, Description: quiz.Description,
		Category: quiz.Category, PassPercent: quiz.PassPercent, DurationSec: quiz.DurationSec,
		Categories: quiz.Categories,
		Cells:      publicCells,
	}

	c.JSON(http.StatusOK, publicQuiz)
}

type submitInput struct {
	SessionID string `json:"sessionId"`
	Duration  int    `json:"durationSec"`
	// Assignments map[cellId] = categoryKey ที่ผู้ใช้เลือก
	// เซลล์ที่ไม่ได้แปะจะถือว่าผิดอัตโนมัติ
	Assignments map[string]string `json:"assignments"`
}

// generateCertificateID — รหัสใบ cert แบบสั้น เช่น "DIEQA-A1B2C3"
func generateCertificateID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return "DIEQA-" + strings.ToUpper(hex.EncodeToString(b))
}

func (h *QuizHandler) Submit(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid quiz id"})
		return
	}

	var in submitInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Assignments == nil {
		in.Assignments = map[string]string{}
	}

	uid, _ := c.Get("userId")
	userID, _ := primitive.ObjectIDFromHex(uid.(string))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	dedupeKey := fmt.Sprintf("submit:lock:%s:%s", id.Hex(), userID.Hex())
	ok, err := h.Redis.SetNX(ctx, dedupeKey, "1", 10*time.Second).Result()
	if err == nil && !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "กำลังประมวลผลคำตอบของคุณอยู่ กรุณารอสักครู่"})
		return
	}

	var quiz models.Quiz
	if err := h.QuizColl.FindOne(ctx, bson.M{"_id": id}).Decode(&quiz); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "quiz not found"})
		return
	}

	var user models.User
	if err := h.UsersColl.FindOne(ctx, bson.M{"_id": userID}).Decode(&user); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	var hospital models.Hospital
	_ = h.HospitalColl.FindOne(ctx, bson.M{"_id": user.HospitalID}).Decode(&hospital)

	// ดึง cell IDs ที่สุ่มให้ user ครั้งนี้จาก Redis
	attemptKey := fmt.Sprintf("quiz:attempt:%s:%s", id.Hex(), userID.Hex())
	var shownCellIDs []string
	if raw, err := h.Redis.Get(ctx, attemptKey).Result(); err == nil {
		_ = json.Unmarshal([]byte(raw), &shownCellIDs)
	}

	// สร้าง map ของ cell ทั้งหมดใน quiz เพื่อ lookup CorrectType
	cellMap := make(map[string]models.CellImage, len(quiz.Cells))
	for _, cell := range quiz.Cells {
		cellMap[cell.ID] = cell
	}

	// ถ้าไม่มี attempt ใน Redis (หมดเวลา/ไม่ได้เปิดผ่าน GET) fallback ใช้ cells ใน Assignments
	if len(shownCellIDs) == 0 {
		for cellID := range in.Assignments {
			if _, ok := cellMap[cellID]; ok {
				shownCellIDs = append(shownCellIDs, cellID)
			}
		}
	}

	// validKeys — กันผู้ใช้ส่ง assignedType ที่ไม่อยู่ใน categories ของข้อสอบ
	validKeys := make(map[string]bool, len(quiz.Categories))
	for _, cat := range quiz.Categories {
		validKeys[cat.Key] = true
	}

	answers := make([]models.CellAnswer, 0, len(shownCellIDs))
	correct := 0
	userCounts := make(map[string]int, len(quiz.Categories))
	trueCounts := make(map[string]int, len(quiz.Categories))
	correctPerCat := make(map[string]int, len(quiz.Categories))

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
		answers = append(answers, models.CellAnswer{
			CellID:       cell.ID,
			ImageURL:     cell.ImageURL,
			AssignedType: assigned,
			CorrectType:  cell.CorrectType,
			IsCorrect:    isCorrect,
		})
	}

	categoryStats := make([]models.CategoryStat, 0, len(quiz.Categories))
	for _, cat := range quiz.Categories {
		categoryStats = append(categoryStats, models.CategoryStat{
			Key:       cat.Key,
			Label:     cat.Label,
			Color:     cat.Color,
			UserCount: userCounts[cat.Key],
			TrueCount: trueCounts[cat.Key],
			Correct:   correctPerCat[cat.Key],
		})
	}

	total := len(answers)
	score := correct
	percent := 0
	if total > 0 {
		percent = int(float64(score) * 100.0 / float64(total))
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

	var sessionID *primitive.ObjectID
	if in.SessionID != "" {
		if oid, err := primitive.ObjectIDFromHex(in.SessionID); err == nil {
			sessionID = &oid
		}
	}

	sub := models.Submission{
		QuizID:        quiz.ID,
		QuizTitle:     quiz.Title,
		SessionID:     sessionID,
		UserID:        user.ID,
		HospitalID:    user.HospitalID,
		Username:      user.Username,
		FullName:      user.FullName,
		HospitalName:  hospital.Name,
		Score:         score,
		Total:         total,
		Correct:       correct,
		Percent:       percent,
		PassPercent:   passPercent,
		Passed:        passed,
		CertificateID: certID,
		Categories:    categoryStats,
		Answers:       answers,
		DurationSec:   in.Duration,
		SubmittedAt:   time.Now(),
	}
	res, err := h.SubmissionColl.InsertOne(ctx, sub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sub.ID = res.InsertedID.(primitive.ObjectID)

	if sessionID != nil {
		room := "session:" + sessionID.Hex()
		zKey := "leaderboard:" + sessionID.Hex()
		_ = h.Redis.ZAdd(ctx, zKey, redis.Z{
			Score:  float64(score),
			Member: user.ID.Hex(),
		}).Err()
		_ = h.Redis.HSet(ctx, "leaderboard:meta:"+sessionID.Hex(), user.ID.Hex(), mustJSON(map[string]interface{}{
			"userId":   user.ID.Hex(),
			"username": user.Username,
			"fullName": user.FullName,
			"hospital": hospital.Name,
			"score":    score,
			"total":    total,
			"correct":  correct,
			"percent":  percent,
			"passed":   passed,
		})).Err()
		_ = h.Redis.Expire(ctx, zKey, 24*time.Hour).Err()
		_ = h.Redis.Expire(ctx, "leaderboard:meta:"+sessionID.Hex(), 24*time.Hour).Err()

		h.Hub.Broadcast(room, ws.Message{
			Type: "leaderboard:update",
			Payload: map[string]interface{}{
				"userId":   user.ID.Hex(),
				"username": user.Username,
				"fullName": user.FullName,
				"hospital": hospital.Name,
				"score":    score,
				"total":    total,
				"correct":  correct,
				"percent":  percent,
				"passed":   passed,
			},
		})
		h.Hub.Broadcast(room, ws.Message{
			Type:    "submission:new",
			Payload: gin.H{"username": user.Username, "score": score, "total": total, "percent": percent},
		})
	}

	c.JSON(http.StatusOK, sub)
}

func (h *QuizHandler) MyHistory(c *gin.Context) {
	uid, _ := c.Get("userId")
	userID, _ := primitive.ObjectIDFromHex(uid.(string))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cur, err := h.SubmissionColl.Find(ctx, bson.M{"userId": userID},
		options.Find().SetSort(bson.D{{Key: "submittedAt", Value: -1}}).SetLimit(50))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(ctx)
	var subs []models.Submission
	if err := cur.All(ctx, &subs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if subs == nil {
		subs = []models.Submission{}
	}
	c.JSON(http.StatusOK, subs)
}

func (h *QuizHandler) GetSubmission(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	uid, _ := c.Get("userId")
	userID, _ := primitive.ObjectIDFromHex(uid.(string))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var sub models.Submission
	if err := h.SubmissionColl.FindOne(ctx, bson.M{"_id": id, "userId": userID}).Decode(&sub); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, sub)
}

func (h *QuizHandler) Leaderboard(c *gin.Context) {
	sid, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	zKey := "leaderboard:" + sid.Hex()
	metaKey := "leaderboard:meta:" + sid.Hex()
	zs, err := h.Redis.ZRevRangeWithScores(ctx, zKey, 0, int64(limit-1)).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	out := make([]map[string]interface{}, 0, len(zs))
	for i, z := range zs {
		uid, _ := z.Member.(string)
		entry := map[string]interface{}{
			"rank":   i + 1,
			"userId": uid,
			"score":  int(z.Score),
		}
		if data, err := h.Redis.HGet(ctx, metaKey, uid).Result(); err == nil {
			var meta map[string]interface{}
			if json.Unmarshal([]byte(data), &meta) == nil {
				for k, v := range meta {
					entry[k] = v
				}
			}
		}
		out = append(out, entry)
	}
	c.JSON(http.StatusOK, out)
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
