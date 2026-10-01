package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/di-eqa/backend/internal/adapter/http/middleware"
	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// maxBodyBytes caps JSON request bodies.
const maxBodyBytes = 1 << 20

// bind decodes the JSON body into v. On failure it writes a VALIDATION_ERROR
// and returns false. When allowEmpty is set a missing body is accepted.
func bind(c *gin.Context, v any, allowEmpty bool) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	dec := json.NewDecoder(c.Request.Body)
	err := dec.Decode(v)
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) && allowEmpty {
		return true
	}
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &te):
		field := te.Field
		if field == "" {
			field = "body"
		}
		response.Validation(c, field, "must be of type "+te.Type.String())
	case errors.Is(err, io.EOF):
		response.Validation(c, "body", "request body is required")
	default:
		response.Validation(c, "body", "must be valid JSON")
	}
	return false
}

// actorFrom builds the audit actor from the authenticated request.
func actorFrom(c *gin.Context) service.Actor {
	a := service.Actor{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
	if p := middleware.PrincipalFrom(c); p != nil {
		a.ID, a.Name, a.Role = p.UserID, p.FullName, p.Role
	}
	return a
}

// principal returns the authenticated principal; the Auth middleware always
// runs first, so a missing one is a wiring bug and answers 401.
func principal(c *gin.Context) (*service.Principal, bool) {
	p := middleware.PrincipalFrom(c)
	if p == nil {
		response.Error(c, http.StatusUnauthorized, service.CodeUnauthenticated, "กรุณาเข้าสู่ระบบ", nil)
		return nil, false
	}
	return p, true
}

func queryInt(c *gin.Context, key string) int {
	n, _ := strconv.Atoi(c.Query(key))
	return n
}

// queryBool parses an optional boolean query parameter.
func queryBool(c *gin.Context, key string) (*bool, bool) {
	raw := c.Query(key)
	if raw == "" {
		return nil, true
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		response.Validation(c, key, "must be true or false")
		return nil, false
	}
	return &b, true
}
