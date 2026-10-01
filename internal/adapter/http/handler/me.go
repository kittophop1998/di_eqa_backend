package handler

import (
	"net/http"

	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// MeHandler serves the user-facing quiz, attempt and certificate endpoints.
// Everything is scoped to the authenticated principal; no handler accepts a
// hospitalId or userId from the client (BR-04).
type MeHandler struct {
	quizzes  *service.MyQuizService
	attempts *service.AttemptService
	certs    *service.CertificateService
}

func NewMeHandler(q *service.MyQuizService, a *service.AttemptService, c *service.CertificateService) *MeHandler {
	return &MeHandler{quizzes: q, attempts: a, certs: c}
}

// ListQuizzes handles GET /api/me/quizzes.
func (h *MeHandler) ListQuizzes(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	out, err := h.quizzes.List(c.Request.Context(), p)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// GetQuiz handles GET /api/me/quizzes/:quizId.
func (h *MeHandler) GetQuiz(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	out, err := h.quizzes.Get(c.Request.Context(), p, c.Param("quizId"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// StartAttempt handles POST /api/me/quizzes/:quizId/attempts: 201 new, 200 resumed.
func (h *MeHandler) StartAttempt(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	view, created, err := h.attempts.Start(c.Request.Context(), p, c.Param("quizId"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, view)
}

// ListAttempts handles GET /api/me/attempts.
func (h *MeHandler) ListAttempts(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	out, err := h.attempts.History(c.Request.Context(), p, c.Query("quizId"), queryInt(c, "page"), queryInt(c, "pageSize"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// GetAttempt handles GET /api/me/attempts/:attemptId: AttemptView while in
// progress, AttemptResult afterwards.
func (h *MeHandler) GetAttempt(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	st, err := h.attempts.Get(c.Request.Context(), p, c.Param("attemptId"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	if st.View != nil {
		response.OK(c, st.View)
		return
	}
	response.OK(c, st.Result)
}

type answersBody struct {
	Answers map[string]*string `json:"answers"`
}

// SaveAnswers handles PUT /api/me/attempts/:attemptId/answers.
func (h *MeHandler) SaveAnswers(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	var body answersBody
	if !bind(c, &body, false) {
		return
	}
	if body.Answers == nil {
		response.Validation(c, "answers", "is required")
		return
	}
	out, err := h.attempts.SaveAnswers(c.Request.Context(), p, c.Param("attemptId"), body.Answers)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// Submit handles POST /api/me/attempts/:attemptId/submit.
func (h *MeHandler) Submit(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	var body answersBody
	if !bind(c, &body, true) {
		return
	}
	out, err := h.attempts.Submit(c.Request.Context(), p, c.Param("attemptId"), body.Answers)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// Result handles GET /api/me/attempts/:attemptId/result.
func (h *MeHandler) Result(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	out, err := h.attempts.Result(c.Request.Context(), p, c.Param("attemptId"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// Image handles GET /api/me/attempts/:attemptId/questions/:questionId/image.
// The body is streamed by the API; the response carries no filename, storage
// path or cell type (BR-14).
func (h *MeHandler) Image(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	img, err := h.attempts.Image(c.Request.Context(), p, c.Param("attemptId"), c.Param("questionId"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	c.Header("Cache-Control", "private, max-age=0, must-revalidate")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, img.ContentType, img.Data)
}

// ListCertificates handles GET /api/me/certificates.
func (h *MeHandler) ListCertificates(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	items, err := h.certs.ListMine(c.Request.Context(), p)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// GetCertificate handles GET /api/me/certificates/:id.
func (h *MeHandler) GetCertificate(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	out, err := h.certs.GetMine(c.Request.Context(), p, c.Param("id"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}
