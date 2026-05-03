package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// --------------------------------------------------------------------------
// Standard envelope types
// --------------------------------------------------------------------------

// ErrResponse is the canonical error body returned to callers.
type ErrResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// OKResponse is a generic success envelope for simple ack responses.
type OKResponse struct {
	Message string `json:"message"`
}

// --------------------------------------------------------------------------
// Success helpers
// --------------------------------------------------------------------------

// RespondOK writes HTTP 200 with arbitrary data as the body.
func RespondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// RespondCreated writes HTTP 201 with arbitrary data as the body.
func RespondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

// RespondMessage writes HTTP 200 with a plain message string.
func RespondMessage(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, OKResponse{Message: msg})
}

// --------------------------------------------------------------------------
// Error helpers  (all abort after writing so downstream handlers are skipped)
// --------------------------------------------------------------------------

// ErrBadRequest writes 400 and aborts.
func ErrBadRequest(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusBadRequest, ErrResponse{
		Code:    http.StatusBadRequest,
		Message: msg,
	})
}

// ErrUnauthorized writes 401 and aborts.
func ErrUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, ErrResponse{
		Code:    http.StatusUnauthorized,
		Message: msg,
	})
}

// ErrForbidden writes 403 and aborts.
func ErrForbidden(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusForbidden, ErrResponse{
		Code:    http.StatusForbidden,
		Message: msg,
	})
}

// ErrNotFound writes 404 and aborts.
func ErrNotFound(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusNotFound, ErrResponse{
		Code:    http.StatusNotFound,
		Message: msg,
	})
}

// ErrConflict writes 409 and aborts.
func ErrConflict(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusConflict, ErrResponse{
		Code:    http.StatusConflict,
		Message: msg,
	})
}

// ErrTooManyRequests writes 429 and aborts.
func ErrTooManyRequests(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrResponse{
		Code:    http.StatusTooManyRequests,
		Message: msg,
	})
}

// ErrInternal writes 500 and aborts.
// Prefer passing a short, user-safe message; log the raw error separately.
func ErrInternal(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, ErrResponse{
		Code:    http.StatusInternalServerError,
		Message: msg,
	})
}

// ErrInternalErr is a convenience wrapper that uses err.Error() as the message.
// Only use this during development; in production you may want to hide the
// original error from the client.
func ErrInternalErr(c *gin.Context, err error) {
	ErrInternal(c, err.Error())
}
