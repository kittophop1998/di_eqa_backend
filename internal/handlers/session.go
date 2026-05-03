package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/models"
	"github.com/di-eqa/backend/internal/ws"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SessionHandler struct {
	SessionColl *mongo.Collection
	QuizColl    *mongo.Collection
	Hub         *ws.Hub
}

func generateCode() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

type createSessionInput struct {
	QuizID     string `json:"quizId"     binding:"required"`
	HospitalID string `json:"hospitalId"` // admin เลือก รพ. ได้เอง; ถ้าไม่ระบุใช้ รพ. ของ host
}

func (s *SessionHandler) Create(c *gin.Context) {
	var in createSessionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	quizID, err := primitive.ObjectIDFromHex(in.QuizID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid quiz id"})
		return
	}

	uid, _ := c.Get("userId")
	hid, _ := c.Get("hospitalId")
	hostID, _ := primitive.ObjectIDFromHex(uid.(string))

	// ถ้า admin ระบุ hospitalId มาใน body ให้ใช้ค่านั้น; ไม่งั้น fallback ไปใช้ของ host
	var hospID primitive.ObjectID
	if in.HospitalID != "" {
		parsed, err := primitive.ObjectIDFromHex(in.HospitalID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid hospital id"})
			return
		}
		hospID = parsed
	} else {
		hospID, _ = primitive.ObjectIDFromHex(hid.(string))
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := s.QuizColl.FindOne(ctx, bson.M{"_id": quizID}).Err(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quiz not found"})
		return
	}

	session := models.Session{
		Code:       generateCode(),
		QuizID:     quizID,
		HostID:     hostID,
		HospitalID: hospID,
		Status:     models.SessionPending,
		CreatedAt:  time.Now(),
	}
	res, err := s.SessionColl.InsertOne(ctx, session)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	session.ID = res.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusOK, session)
}

func (s *SessionHandler) Start(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	now := time.Now()
	res := s.SessionColl.FindOneAndUpdate(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": models.SessionRunning, "startedAt": now}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var session models.Session
	if err := res.Decode(&session); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	s.Hub.Broadcast("session:"+session.ID.Hex(), ws.Message{
		Type: "session:start",
		Payload: map[string]interface{}{
			"sessionId": session.ID.Hex(),
			"quizId":    session.QuizID.Hex(),
			"startedAt": now,
		},
	})

	// broadcast ไปที่ hospital room เพื่อให้เฉพาะ user ของ รพ. นั้นเห็น banner
	s.Hub.Broadcast("hospital:"+session.HospitalID.Hex(), ws.Message{
		Type: "session:start",
		Payload: map[string]interface{}{
			"sessionId": session.ID.Hex(),
			"quizId":    session.QuizID.Hex(),
			"startedAt": now,
		},
	})

	c.JSON(http.StatusOK, session)
}

func (s *SessionHandler) End(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	now := time.Now()
	res := s.SessionColl.FindOneAndUpdate(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": models.SessionEnded, "endedAt": now}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var session models.Session
	if err := res.Decode(&session); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	s.Hub.Broadcast("session:"+session.ID.Hex(), ws.Message{
		Type: "session:end",
		Payload: map[string]interface{}{
			"sessionId": session.ID.Hex(),
			"endedAt":   now,
		},
	})

	c.JSON(http.StatusOK, session)
}

func (s *SessionHandler) Get(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var session models.Session
	if err := s.SessionColl.FindOne(ctx, bson.M{"_id": id}).Decode(&session); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, session)
}

func (s *SessionHandler) GetByCode(c *gin.Context) {
	code := strings.ToUpper(strings.TrimSpace(c.Param("code")))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var session models.Session
	if err := s.SessionColl.FindOne(ctx, bson.M{"code": code}).Decode(&session); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, session)
}

func (s *SessionHandler) ListActive(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// กรอง session ตาม hospitalId ของ user ที่ request
	hid, _ := c.Get("hospitalId")

	filter := bson.M{"status": bson.M{"$in": []string{models.SessionPending, models.SessionRunning}}}

	// admin/instructor เห็นทุก session ของ รพ. ตัวเอง, user ทั่วไปก็เห็นของ รพ. ตัวเองเช่นกัน
	if hidStr, ok := hid.(string); ok && hidStr != "" {
		hospID, err := primitive.ObjectIDFromHex(hidStr)
		if err == nil {
			filter["hospitalId"] = hospID
		}
	}

	cur, err := s.SessionColl.Find(ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(50),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(ctx)
	var list []models.Session
	if err := cur.All(ctx, &list); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []models.Session{}
	}
	c.JSON(http.StatusOK, list)
}
