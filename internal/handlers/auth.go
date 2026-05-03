package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/config"
	"github.com/di-eqa/backend/internal/models"
	"github.com/di-eqa/backend/internal/utils"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	Cfg          *config.Config
	UsersColl    *mongo.Collection
	HospitalColl *mongo.Collection
}

type registerInput struct {
	HospitalCode string `json:"hospitalCode" binding:"required"`
	Username     string `json:"username" binding:"required,min=3,max=32"`
	FullName     string `json:"fullName" binding:"required"`
	Email        string `json:"email"`
	Password     string `json:"password" binding:"required,min=6"`
}

func (a *AuthHandler) Register(c *gin.Context) {
	var in registerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	var hospital models.Hospital
	if err := a.HospitalColl.FindOne(ctx, bson.M{"code": strings.ToUpper(in.HospitalCode)}).Decode(&hospital); err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่พบรหัสโรงพยาบาลนี้"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	username := strings.ToLower(strings.TrimSpace(in.Username))
	count, err := a.UsersColl.CountDocuments(ctx, bson.M{
		"hospitalId": hospital.ID,
		"username":   username,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "username นี้มีอยู่แล้วในโรงพยาบาลของคุณ"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user := models.User{
		HospitalID: hospital.ID,
		Username:   username,
		FullName:   strings.TrimSpace(in.FullName),
		Email:      strings.TrimSpace(in.Email),
		Password:   string(hash),
		Role:       models.RoleUser,
		CreatedAt:  time.Now(),
	}
	res, err := a.UsersColl.InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	user.ID = res.InsertedID.(primitive.ObjectID)

	token, err := utils.GenerateToken(a.Cfg.JWTSecret, user.ID.Hex(), hospital.ID.Hex(), user.Role, a.Cfg.JWTExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user.ToPublic(&hospital),
	})
}

type loginInput struct {
	HospitalCode string `json:"hospitalCode"`
	Username     string `json:"username" binding:"required"`
	Password     string `json:"password" binding:"required"`
	IsAdmin      bool   `json:"isAdmin"`
}

func (a *AuthHandler) Login(c *gin.Context) {
	var in loginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	username := strings.ToLower(strings.TrimSpace(in.Username))

	// ---- Admin login: ไม่ต้องใช้ HospitalCode ----
	if in.IsAdmin || in.HospitalCode == "" {
		var user models.User
		if err := a.UsersColl.FindOne(ctx, bson.M{
			"username": username,
			"role":     models.RoleAdmin,
		}).Decode(&user); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "รหัสผ่านไม่ถูกต้อง"})
			return
		}

		// admin ไม่มี hospitalId → ส่ง "" เข้า JWT (role=admin ไม่ต้องอิง รพ.)
		token, err := utils.GenerateToken(a.Cfg.JWTSecret, user.ID.Hex(), "", user.Role, a.Cfg.JWTExpiry)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token": token,
			"user":  user.ToPublic(nil),
		})
		return
	}

	// ---- User / Instructor login: ต้องใช้ HospitalCode ----
	var hospital models.Hospital
	if err := a.HospitalColl.FindOne(ctx, bson.M{"code": strings.ToUpper(in.HospitalCode)}).Decode(&hospital); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "รหัสโรงพยาบาล หรือชื่อผู้ใช้ไม่ถูกต้อง"})
		return
	}

	var user models.User
	if err := a.UsersColl.FindOne(ctx, bson.M{
		"hospitalId": hospital.ID,
		"username":   username,
	}).Decode(&user); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "รหัสโรงพยาบาล หรือชื่อผู้ใช้ไม่ถูกต้อง"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "รหัสผ่านไม่ถูกต้อง"})
		return
	}

	token, err := utils.GenerateToken(a.Cfg.JWTSecret, user.ID.Hex(), hospital.ID.Hex(), user.Role, a.Cfg.JWTExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user.ToPublic(&hospital),
	})
}

func (a *AuthHandler) Me(c *gin.Context) {
	uid, _ := c.Get("userId")
	id, _ := primitive.ObjectIDFromHex(uid.(string))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := a.UsersColl.FindOne(ctx, bson.M{"_id": id}).Decode(&user); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	var hospital models.Hospital
	_ = a.HospitalColl.FindOne(ctx, bson.M{"_id": user.HospitalID}).Decode(&hospital)

	c.JSON(http.StatusOK, user.ToPublic(&hospital))
}
