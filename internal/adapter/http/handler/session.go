package handler

import (
	"errors"
	"net/http"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// SessionHandler is the HTTP driving adapter for session use cases.
type SessionHandler struct {
	svc *service.SessionService
}

func NewSessionHandler(svc *service.SessionService) *SessionHandler {
	return &SessionHandler{svc: svc}
}

type createSessionInput struct {
	QuizID       string `json:"quizId"       binding:"required"`
	HospitalID   string `json:"hospitalId"`
	HospitalCode string `json:"hospitalCode"`
}

func (h *SessionHandler) Create(c *gin.Context) {
	var in createSessionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, _ := c.Get("userId")
	hid, _ := c.Get("hospitalId")
	role, _ := c.Get("role")

	sess, err := h.svc.Create(c.Request.Context(), service.CreateSessionInput{
		QuizIDHex:    in.QuizID,
		HospitalCode: in.HospitalCode,
		HospitalID:   in.HospitalID,
		HostIDHex:    uid.(string),
		Role:         role.(string),
		CallerHospID: hid.(string),
	})
	if err != nil {
		if service.IsBadRequest(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *SessionHandler) Start(c *gin.Context) {
	sess, err := h.svc.Start(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *SessionHandler) End(c *gin.Context) {
	sess, err := h.svc.End(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *SessionHandler) Get(c *gin.Context) {
	sess, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *SessionHandler) GetByCode(c *gin.Context) {
	sess, err := h.svc.GetByCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *SessionHandler) ListActive(c *gin.Context) {
	role, _ := c.Get("role")
	roleStr, _ := role.(string)
	hid, _ := c.Get("hospitalId")
	hidStr, _ := hid.(string)

	list, err := h.svc.ListActive(c.Request.Context(), roleStr, hidStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// ensure errors package is used
var _ = errors.New
