// Package response owns JSON response formatting for the HTTP adapter. Every
// error, from handlers, middleware, rate limiting and panic recovery, uses the
// single shape of backlog §5.2.
package response

import (
	"errors"
	"log"
	"net/http"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// RequestIDKey is the gin context key holding the request id.
const RequestIDKey = "requestId"

// RequestIDHeader is the response (and accepted request) header.
const RequestIDHeader = "X-Request-Id"

// ErrBody is the canonical error body.
type ErrBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"requestId"`
}

// statusByCode maps application error codes to HTTP statuses.
var statusByCode = map[string]int{
	service.CodeValidation:         http.StatusBadRequest,
	service.CodeUnauthenticated:    http.StatusUnauthorized,
	service.CodeTokenExpired:       http.StatusUnauthorized,
	service.CodeInvalidCredentials: http.StatusUnauthorized,
	service.CodeAccountPending:     http.StatusForbidden,
	service.CodeAccountRejected:    http.StatusForbidden,
	service.CodeAccountDisabled:    http.StatusForbidden,
	service.CodeForbidden:          http.StatusForbidden,
	service.CodeNotFound:           http.StatusNotFound,
	service.CodeUsernameTaken:      http.StatusConflict,
	service.CodeInvalidState:       http.StatusConflict,
	service.CodeQuizNotOpen:        http.StatusConflict,
	service.CodeMaxAttempts:        http.StatusConflict,
	service.CodeAlreadyPassed:      http.StatusConflict,
	service.CodeAttemptNotInProg:   http.StatusConflict,
	service.CodeAttemptExpired:     http.StatusConflict,
	service.CodeAttemptNotSubmit:   http.StatusConflict,
	service.CodeQuizNotPublished:   http.StatusConflict,
	service.CodeQuizWindowEnded:    http.StatusConflict,
	service.CodePoolTooSmall:       http.StatusConflict,
	service.CodeAssignmentAttempts: http.StatusConflict,
	service.CodeHospitalInUse:      http.StatusConflict,
	service.CodeLastSuperAdmin:     http.StatusConflict,
	service.CodeDuplicate:          http.StatusConflict,
	service.CodeInternal:           http.StatusInternalServerError,
}

// StatusOf returns the HTTP status for an application error code.
func StatusOf(code string) int {
	if s, ok := statusByCode[code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// RequestID returns the id of the current request ("" if none).
func RequestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Error writes an error body and aborts the chain.
func Error(c *gin.Context, status int, code, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, ErrBody{Code: code, Message: message, Details: details, RequestID: RequestID(c)})
}

// FromError writes the response for any error returned by a service. Unknown
// errors and INTERNAL errors produce a generic body; the real cause is logged
// with the request id and never sent to the client (BR-52).
func FromError(c *gin.Context, err error) {
	var ae *service.Error
	if !errors.As(err, &ae) {
		log.Printf("request_id=%s unexpected error: %v", RequestID(c), err)
		Error(c, http.StatusInternalServerError, service.CodeInternal, "เกิดข้อผิดพลาดภายในระบบ", nil)
		return
	}
	if ae.Code == service.CodeInternal {
		log.Printf("request_id=%s internal error: %v", RequestID(c), errors.Unwrap(ae))
		Error(c, http.StatusInternalServerError, service.CodeInternal, "เกิดข้อผิดพลาดภายในระบบ", nil)
		return
	}
	Error(c, StatusOf(ae.Code), ae.Code, ae.Message, ae.Details)
}

// Validation writes a 400 VALIDATION_ERROR with one field issue.
func Validation(c *gin.Context, field, issue string) {
	Error(c, http.StatusBadRequest, service.CodeValidation, "ข้อมูลไม่ถูกต้อง", map[string]any{
		"fields": []map[string]string{{"field": field, "issue": issue}},
	})
}

// OK writes 200 with data.
func OK(c *gin.Context, data any) { c.JSON(http.StatusOK, data) }

// Created writes 201 with data.
func Created(c *gin.Context, data any) { c.JSON(http.StatusCreated, data) }

// NoContent writes 204.
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }
