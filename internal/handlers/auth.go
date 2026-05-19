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

type addressInput struct {
	AddressNo   string `json:"addressNo"`
	Building    string `json:"building"`
	SubDistrict string `json:"subDistrict"`
	District    string `json:"district"`
	Province    string `json:"province"`
	PostalCode  string `json:"postalCode"`
}

type registerInput struct {
	MemberType   string `json:"memberType"`
	HospitalCode string `json:"hospitalCode"`

	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6"`

	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	FullName  string `json:"fullName"`
	Email     string `json:"email"`

	Clinic       string `json:"clinic"`
	LabName      string `json:"labName"`
	HospitalType string `json:"hospitalType"`
	BedSize      string `json:"bedSize"`

	Address addressInput `json:"address"`

	CertificateYear int `json:"certificateYear"`
}

func normalizeMemberType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", models.MemberInternal:
		return models.MemberInternal
	case models.MemberExternal:
		return models.MemberExternal
	default:
		return ""
	}
}

func (a *AuthHandler) Register(c *gin.Context) {
	var in registerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	memberType := normalizeMemberType(in.MemberType)
	if memberType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ประเภทสมาชิกไม่ถูกต้อง"})
		return
	}
	if memberType == models.MemberInternal && strings.TrimSpace(in.HospitalCode) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาเลือกโรงพยาบาลสำหรับบุคลากรภายใน"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	username := strings.ToLower(strings.TrimSpace(in.Username))
	fullName := models.ComposeFullName(in.FirstName, in.LastName, in.FullName)

	profile := models.Profile{
		MemberType:   memberType,
		FirstName:    strings.TrimSpace(in.FirstName),
		LastName:     strings.TrimSpace(in.LastName),
		Clinic:       strings.TrimSpace(in.Clinic),
		LabName:      strings.TrimSpace(in.LabName),
		HospitalType: strings.TrimSpace(in.HospitalType),
		BedSize:      strings.TrimSpace(in.BedSize),
		Address: models.Address{
			AddressNo:   strings.TrimSpace(in.Address.AddressNo),
			Building:    strings.TrimSpace(in.Address.Building),
			SubDistrict: strings.TrimSpace(in.Address.SubDistrict),
			District:    strings.TrimSpace(in.Address.District),
			Province:    strings.TrimSpace(in.Address.Province),
			PostalCode:  strings.TrimSpace(in.Address.PostalCode),
		},
		CertificateYear: in.CertificateYear,
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user := models.User{
		Username:  username,
		FullName:  fullName,
		Email:     strings.TrimSpace(in.Email),
		Password:  string(hash),
		Role:      models.RoleUser,
		Profile:   profile,
		CreatedAt: time.Now(),
	}

	var hospital *models.Hospital

	switch memberType {
	case models.MemberInternal:
		var h models.Hospital
		if err := a.HospitalColl.FindOne(ctx, bson.M{"code": strings.ToUpper(in.HospitalCode)}).Decode(&h); err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่พบรหัสโรงพยาบาลนี้"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		hospital = &h

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
		user.HospitalID = hospital.ID

	case models.MemberExternal:
		count, err := a.UsersColl.CountDocuments(ctx, bson.M{
			"username":           username,
			"profile.memberType": models.MemberExternal,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if count > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "username นี้ถูกใช้งานแล้ว"})
			return
		}
	}

	res, err := a.UsersColl.InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	user.ID = res.InsertedID.(primitive.ObjectID)

	hospitalIDHex := ""
	if hospital != nil {
		hospitalIDHex = hospital.ID.Hex()
	}
	token, err := utils.GenerateToken(a.Cfg.JWTSecret, user.ID.Hex(), hospitalIDHex, user.Role, a.Cfg.JWTExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user.ToPublic(hospital),
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

	// ---- Admin / no-hospital login ----
	if in.IsAdmin || in.HospitalCode == "" {
		var user models.User
		if err := a.UsersColl.FindOne(ctx, bson.M{
			"username": username,
			"role":     models.RoleAdmin,
		}).Decode(&user); err == nil {
			if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)); err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "รหัสผ่านไม่ถูกต้อง"})
				return
			}
			token, err := utils.GenerateToken(a.Cfg.JWTSecret, user.ID.Hex(), "", user.Role, a.Cfg.JWTExpiry)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"token": token, "user": user.ToPublic(nil)})
			return
		}

		if in.IsAdmin {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
			return
		}

		// fall through: external user
		var extUser models.User
		if err := a.UsersColl.FindOne(ctx, bson.M{
			"username":           username,
			"profile.memberType": models.MemberExternal,
		}).Decode(&extUser); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(extUser.Password), []byte(in.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "รหัสผ่านไม่ถูกต้อง"})
			return
		}
		token, err := utils.GenerateToken(a.Cfg.JWTSecret, extUser.ID.Hex(), "", extUser.Role, a.Cfg.JWTExpiry)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": token, "user": extUser.ToPublic(nil)})
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

	var hospital *models.Hospital
	if !user.HospitalID.IsZero() {
		var h models.Hospital
		if err := a.HospitalColl.FindOne(ctx, bson.M{"_id": user.HospitalID}).Decode(&h); err == nil {
			hospital = &h
		}
	}

	c.JSON(http.StatusOK, user.ToPublic(hospital))
}
