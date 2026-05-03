package handler

import (
	"errors"
	"net/http"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// AuthHandler is the HTTP driving adapter for auth use cases.
type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type registerInput struct {
	HospitalCode string `json:"hospitalCode" binding:"required"`
	Username     string `json:"username"     binding:"required,min=3,max=32"`
	FullName     string `json:"fullName"     binding:"required"`
	Email        string `json:"email"`
	Password     string `json:"password"     binding:"required,min=6"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var in registerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := h.svc.Register(c.Request.Context(), service.RegisterInput{
		HospitalCode: in.HospitalCode,
		Username:     in.Username,
		FullName:     in.FullName,
		Email:        in.Email,
		Password:     in.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrHospitalNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่พบรหัสโรงพยาบาลนี้"})
		case errors.Is(err, service.ErrUsernameExists):
			c.JSON(http.StatusConflict, gin.H{"error": "username นี้มีอยู่แล้วในโรงพยาบาลของคุณ"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": out.Token, "user": out.User})
}

type loginInput struct {
	HospitalCode string `json:"hospitalCode"`
	Username     string `json:"username"  binding:"required"`
	Password     string `json:"password"  binding:"required"`
	IsAdmin      bool   `json:"isAdmin"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var in loginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := h.svc.Login(c.Request.Context(), service.LoginInput{
		HospitalCode: in.HospitalCode,
		Username:     in.Username,
		Password:     in.Password,
		IsAdmin:      in.IsAdmin,
	})
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": out.Token, "user": out.User})
}

func (h *AuthHandler) Me(c *gin.Context) {
	uid, _ := c.Get("userId")
	userIDStr, _ := uid.(string)

	pub, err := h.svc.Me(c.Request.Context(), userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, pub)
}
