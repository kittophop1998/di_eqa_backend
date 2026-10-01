package handler

import (
	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// AdminHandler serves admin users, cell library and quizzes. Authorization is
// enforced once by the /api/admin route group; handlers only map HTTP.
type AdminHandler struct {
	users   *service.AdminUserService
	cells   *service.CellLibraryService
	quizzes *service.QuizAdminService
}

func NewAdminHandler(u *service.AdminUserService, c *service.CellLibraryService, q *service.QuizAdminService) *AdminHandler {
	return &AdminHandler{users: u, cells: c, quizzes: q}
}

// ------------------------------------------------------------- users

// ListUsers handles GET /api/admin/users.
func (h *AdminHandler) ListUsers(c *gin.Context) {
	out, err := h.users.List(c.Request.Context(), service.ListUsersInput{
		Status: c.Query("status"), HospitalID: c.Query("hospitalId"), Role: c.Query("role"),
		Query: c.Query("q"), Page: queryInt(c, "page"), PageSize: queryInt(c, "pageSize"),
	})
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

type approveBody struct {
	HospitalID string `json:"hospitalId"`
}

// ApproveUser handles POST /api/admin/users/:id/approve.
func (h *AdminHandler) ApproveUser(c *gin.Context) {
	var body approveBody
	if !bind(c, &body, true) {
		return
	}
	out, err := h.users.Approve(c.Request.Context(), c.Param("id"), body.HospitalID, actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

type rejectBody struct {
	Reason string `json:"reason"`
}

// RejectUser handles POST /api/admin/users/:id/reject.
func (h *AdminHandler) RejectUser(c *gin.Context) {
	var body rejectBody
	if !bind(c, &body, true) {
		return
	}
	out, err := h.users.Reject(c.Request.Context(), c.Param("id"), body.Reason, actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

type patchUserBody struct {
	HospitalID *string `json:"hospitalId"`
	Status     *string `json:"status"`
	FullName   *string `json:"fullName"`
	Email      *string `json:"email"`
}

// PatchUser handles PATCH /api/admin/users/:id. Role is not patchable here.
func (h *AdminHandler) PatchUser(c *gin.Context) {
	var body patchUserBody
	if !bind(c, &body, false) {
		return
	}
	out, err := h.users.Patch(c.Request.Context(), c.Param("id"), service.PatchUserInput{
		HospitalID: body.HospitalID, Status: body.Status, FullName: body.FullName, Email: body.Email,
	}, actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

type roleBody struct {
	Role string `json:"role"`
}

// SetUserRole handles PATCH /api/admin/users/:id/role (super_admin only).
func (h *AdminHandler) SetUserRole(c *gin.Context) {
	var body roleBody
	if !bind(c, &body, false) {
		return
	}
	out, err := h.users.SetRole(c.Request.Context(), c.Param("id"), body.Role, actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// ------------------------------------------------------------- cell library

// ListCellTypes handles GET /api/admin/cell-types.
func (h *AdminHandler) ListCellTypes(c *gin.Context) {
	out, err := h.cells.ListTypes(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// ListCellImages handles GET /api/admin/cell-images.
func (h *AdminHandler) ListCellImages(c *gin.Context) {
	active, ok := queryBool(c, "active")
	if !ok {
		return
	}
	out, err := h.cells.ListImages(c.Request.Context(), service.ListImagesInput{
		TypeKey: c.Query("typeKey"), Active: active, Page: queryInt(c, "page"), PageSize: queryInt(c, "pageSize"),
	})
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// ------------------------------------------------------------- quizzes

type quizBody struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	QuestionCount *int     `json:"questionCount"`
	PassPercent   int      `json:"passPercent"`
	DurationSec   int      `json:"durationSec"`
	MaxAttempts   int      `json:"maxAttempts"`
	OpensAt       string   `json:"opensAt"`
	ClosesAt      string   `json:"closesAt"`
	HospitalIDs   []string `json:"hospitalIds"`
}

func (b quizBody) input() service.QuizInput {
	return service.QuizInput{
		Title: b.Title, Description: b.Description, QuestionCount: b.QuestionCount,
		PassPercent: b.PassPercent, DurationSec: b.DurationSec, MaxAttempts: b.MaxAttempts,
		OpensAt: b.OpensAt, ClosesAt: b.ClosesAt, HospitalIDs: b.HospitalIDs,
	}
}

// ListQuizzes handles GET /api/admin/quizzes.
func (h *AdminHandler) ListQuizzes(c *gin.Context) {
	out, err := h.quizzes.List(c.Request.Context(), service.ListQuizzesInput{
		Status: c.Query("status"), Query: c.Query("q"), Page: queryInt(c, "page"), PageSize: queryInt(c, "pageSize"),
	})
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// CreateQuiz handles POST /api/admin/quizzes.
func (h *AdminHandler) CreateQuiz(c *gin.Context) {
	var body quizBody
	if !bind(c, &body, false) {
		return
	}
	out, err := h.quizzes.Create(c.Request.Context(), body.input(), actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Created(c, out)
}

// GetQuiz handles GET /api/admin/quizzes/:id.
func (h *AdminHandler) GetQuiz(c *gin.Context) {
	out, err := h.quizzes.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// UpdateQuiz handles PUT /api/admin/quizzes/:id.
func (h *AdminHandler) UpdateQuiz(c *gin.Context) {
	var body quizBody
	if !bind(c, &body, false) {
		return
	}
	out, err := h.quizzes.Update(c.Request.Context(), c.Param("id"), body.input(), actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

type hospitalsBody struct {
	HospitalIDs []string `json:"hospitalIds"`
}

// SetQuizHospitals handles PUT /api/admin/quizzes/:id/hospitals.
func (h *AdminHandler) SetQuizHospitals(c *gin.Context) {
	var body hospitalsBody
	if !bind(c, &body, false) {
		return
	}
	if body.HospitalIDs == nil {
		response.Validation(c, "hospitalIds", "is required")
		return
	}
	out, err := h.quizzes.SetHospitals(c.Request.Context(), c.Param("id"), body.HospitalIDs, actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// PublishQuiz handles POST /api/admin/quizzes/:id/publish.
func (h *AdminHandler) PublishQuiz(c *gin.Context) {
	out, err := h.quizzes.Publish(c.Request.Context(), c.Param("id"), actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// ArchiveQuiz handles POST /api/admin/quizzes/:id/archive.
func (h *AdminHandler) ArchiveQuiz(c *gin.Context) {
	out, err := h.quizzes.Archive(c.Request.Context(), c.Param("id"), actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// DeleteQuiz handles DELETE /api/admin/quizzes/:id.
func (h *AdminHandler) DeleteQuiz(c *gin.Context) {
	if err := h.quizzes.Delete(c.Request.Context(), c.Param("id"), actorFrom(c)); err != nil {
		response.FromError(c, err)
		return
	}
	response.NoContent(c)
}

// OpenAssignment handles POST /api/admin/quizzes/:id/hospitals/:hospitalId/open.
func (h *AdminHandler) OpenAssignment(c *gin.Context) {
	out, err := h.quizzes.OpenAssignment(c.Request.Context(), c.Param("id"), c.Param("hospitalId"), actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// CloseAssignment handles POST /api/admin/quizzes/:id/hospitals/:hospitalId/close.
func (h *AdminHandler) CloseAssignment(c *gin.Context) {
	out, err := h.quizzes.CloseAssignment(c.Request.Context(), c.Param("id"), c.Param("hospitalId"), actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}
