package handler

import (
	"net/http"
	"strconv"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// AuditHandler is the HTTP driving adapter for audit-log queries.
// Recording is performed implicitly by other services, so this adapter only
// exposes a read endpoint for the super-admin UI.
type AuditHandler struct {
	svc *service.AuditService
}

func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// List handles GET /api/audit-logs?action=...&page=1&limit=50
func (h *AuditHandler) List(c *gin.Context) {
	action := c.Query("action")
	page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)

	out, err := h.svc.List(c.Request.Context(), service.ListAuditLogsInput{
		Action: action,
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}
