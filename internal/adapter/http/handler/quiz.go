package handler

import (
	"errors"
	"strconv"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/di-eqa/backend/internal/utils"
	"github.com/gin-gonic/gin"
)

// QuizHandler is the HTTP driving adapter for quiz/submission use cases.
type QuizHandler struct {
	svc *service.QuizService
}

func NewQuizHandler(svc *service.QuizService) *QuizHandler {
	return &QuizHandler{svc: svc}
}

func (h *QuizHandler) List(c *gin.Context) {
	role, _ := c.Get("role")
	roleStr, _ := role.(string)
	hid, _ := c.Get("hospitalId")
	hidStr, _ := hid.(string)

	out, err := h.svc.List(c.Request.Context(), roleStr, hidStr)
	if err != nil {
		utils.ErrInternalErr(c, err)
		return
	}
	utils.RespondOK(c, out)
}

func (h *QuizHandler) Get(c *gin.Context) {
	uid, _ := c.Get("userId")
	userIDStr, _ := uid.(string)
	role, _ := c.Get("role")
	roleStr, _ := role.(string)
	hid, _ := c.Get("hospitalId")
	hidStr, _ := hid.(string)

	out, err := h.svc.Get(c.Request.Context(),
		c.Param("id"), userIDStr, c.Query("session"), roleStr, hidStr)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrForbidden):
			utils.ErrForbidden(c, err.Error())
		case errors.Is(err, service.ErrSessionNotRunning):
			utils.ErrForbidden(c, "session has not started yet")
		case errors.Is(err, service.ErrNotFound):
			utils.ErrNotFound(c, "quiz not found")
		default:
			utils.ErrBadRequest(c, err.Error())
		}
		return
	}
	utils.RespondOK(c, out)
}

type submitInput struct {
	SessionID   string            `json:"sessionId"`
	DurationSec int               `json:"durationSec"`
	Assignments map[string]string `json:"assignments"`
}

func (h *QuizHandler) Submit(c *gin.Context) {
	var in submitInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.ErrBadRequest(c, err.Error())
		return
	}
	if in.Assignments == nil {
		in.Assignments = map[string]string{}
	}

	uid, _ := c.Get("userId")
	userIDStr, _ := uid.(string)

	sub, err := h.svc.Submit(c.Request.Context(), service.SubmitInput{
		QuizIDHex:   c.Param("id"),
		UserIDHex:   userIDStr,
		SessionID:   in.SessionID,
		DurationSec: in.DurationSec,
		Assignments: in.Assignments,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDuplicateSubmit):
			utils.ErrTooManyRequests(c, "กำลังประมวลผลคำตอบของคุณอยู่ กรุณารอสักครู่")
		case errors.Is(err, service.ErrNotFound):
			utils.ErrNotFound(c, "quiz not found")
		default:
			utils.ErrInternalErr(c, err)
		}
		return
	}
	utils.RespondOK(c, sub)
}

func (h *QuizHandler) MyHistory(c *gin.Context) {
	uid, _ := c.Get("userId")
	userIDStr, _ := uid.(string)

	subs, err := h.svc.MyHistory(c.Request.Context(), userIDStr)
	if err != nil {
		utils.ErrInternalErr(c, err)
		return
	}
	utils.RespondOK(c, subs)
}

func (h *QuizHandler) GetSubmission(c *gin.Context) {
	uid, _ := c.Get("userId")
	userIDStr, _ := uid.(string)

	sub, err := h.svc.GetSubmission(c.Request.Context(), c.Param("id"), userIDStr)
	if err != nil {
		utils.ErrNotFound(c, "not found")
		return
	}
	utils.RespondOK(c, sub)
}

func (h *QuizHandler) Leaderboard(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.ParseInt(limitStr, 10, 64)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	out, err := h.svc.Leaderboard(c.Request.Context(), c.Param("id"), limit)
	if err != nil {
		utils.ErrInternalErr(c, err)
		return
	}
	utils.RespondOK(c, out)
}
