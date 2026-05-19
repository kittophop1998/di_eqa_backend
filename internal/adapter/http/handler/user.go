package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// UserHandler is the HTTP adapter for user-management use cases.
type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// List handles GET /api/users?search=...&page=1&limit=20
func (h *UserHandler) List(c *gin.Context) {
	search := c.Query("search")
	page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)

	out, err := h.svc.ListUsers(c.Request.Context(), service.ListUsersInput{
		Search: search,
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

type updateRoleBody struct {
	Role string `json:"role" binding:"required"`
}

// UpdateRole handles PATCH /api/users/:id/role
func (h *UserHandler) UpdateRole(c *gin.Context) {
	var body updateRoleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	callerRole, _ := c.Get("role")
	callerRoleStr, _ := callerRole.(string)
	callerID, _ := c.Get("userId")
	callerIDStr, _ := callerID.(string)

	err := h.svc.UpdateRole(c.Request.Context(), service.UpdateRoleInput{
		UserIDHex:  c.Param("id"),
		CallerID:   callerIDStr,
		CallerRole: callerRoleStr,
		NewRole:    body.Role,
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบผู้ใช้"})
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "ไม่มีสิทธิ์ดำเนินการ"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
