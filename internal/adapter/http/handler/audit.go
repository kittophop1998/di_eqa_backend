package handler

import (
	"strconv"

	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// AuditHandler exposes the audit-log read endpoint (super_admin only).
type AuditHandler struct {
	svc *service.AuditService
}

func NewAuditHandler(svc *service.AuditService) *AuditHandler { return &AuditHandler{svc: svc} }

// List handles GET /api/admin/audit-logs?action=...&page=1&limit=50
func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	out, err := h.svc.List(c.Request.Context(), service.ListAuditLogsInput{
		Action: c.Query("action"), Page: page, Limit: limit,
	})
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}
