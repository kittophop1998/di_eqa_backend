package response

import (
	"log"
	"net/http"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// Compatibility shims for the legacy live-session handlers, which are no
// longer routed and are deleted in Epic G. They emit the standard error shape.

// Deprecated: use Error.
func ErrBadRequest(c *gin.Context, _ string) {
	Error(c, http.StatusBadRequest, service.CodeValidation, "ข้อมูลไม่ถูกต้อง", nil)
}

// Deprecated: use Error.
func ErrUnauthorized(c *gin.Context, _ string) {
	Error(c, http.StatusUnauthorized, service.CodeUnauthenticated, "กรุณาเข้าสู่ระบบ", nil)
}

// Deprecated: use Error.
func ErrForbidden(c *gin.Context, _ string) {
	Error(c, http.StatusForbidden, service.CodeForbidden, "คุณไม่มีสิทธิ์ดำเนินการนี้", nil)
}

// Deprecated: use Error.
func ErrNotFound(c *gin.Context, _ string) {
	Error(c, http.StatusNotFound, service.CodeNotFound, "ไม่พบข้อมูลที่ต้องการ", nil)
}

// Deprecated: use Error.
func ErrInternalErr(c *gin.Context, err error) {
	log.Printf("request_id=%s internal error: %v", RequestID(c), err)
	Error(c, http.StatusInternalServerError, service.CodeInternal, "เกิดข้อผิดพลาดภายในระบบ", nil)
}

// Deprecated: use OK.
func RespondOK(c *gin.Context, data any) { OK(c, data) }
