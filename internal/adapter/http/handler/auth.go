package handler

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/gin-gonic/gin"
)

// AuthHandler is the HTTP driving adapter for auth use cases.
type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler { return &AuthHandler{svc: svc} }

// flexAddress accepts either the structured address object or a plain string
// (stored in addressNo) so both client shapes of `profile.address` work.
type flexAddress struct{ entity.Address }

func (a *flexAddress) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		a.Address = entity.Address{AddressNo: s}
		return nil
	}
	return json.Unmarshal(b, &a.Address)
}

// registerBody models POST /auth/register. role, status and hospitalId are
// not fields here, so a client cannot set them (BR-40).
type registerBody struct {
	Username            string `json:"username"`
	Password            string `json:"password"`
	FirstName           string `json:"firstName"`
	LastName            string `json:"lastName"`
	Email               string `json:"email"`
	RequestedHospitalID string `json:"requestedHospitalId"`
	Profile             *struct {
		Clinic          string      `json:"clinic"`
		LabName         string      `json:"labName"`
		HospitalType    string      `json:"hospitalType"`
		BedSize         string      `json:"bedSize"`
		Address         flexAddress `json:"address"`
		CertificateYear int         `json:"certificateYear"`
	} `json:"profile"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var in registerBody
	if !bind(c, &in, false) {
		return
	}
	req := service.RegisterInput{
		Username: in.Username, Password: in.Password, FirstName: in.FirstName, LastName: in.LastName,
		Email: in.Email, RequestedHospitalID: in.RequestedHospitalID,
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	}
	if p := in.Profile; p != nil {
		req.Profile = service.ProfileInput{
			Clinic: p.Clinic, LabName: p.LabName, HospitalType: p.HospitalType, BedSize: p.BedSize,
			Address: p.Address.Address, CertificateYear: p.CertificateYear,
		}
	}
	if err := h.svc.Register(c.Request.Context(), req); err != nil {
		response.FromError(c, err)
		return
	}
	response.Created(c, gin.H{"status": "pending"})
}

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var in loginBody
	if !bind(c, &in, false) {
		return
	}
	out, err := h.svc.Login(c.Request.Context(), service.LoginInput{Username: in.Username, Password: in.Password})
	if err != nil {
		response.FromError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *AuthHandler) Me(c *gin.Context) {
	p, ok := principal(c)
	if !ok {
		return
	}
	me, err := h.svc.Me(c.Request.Context(), p)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, me)
}
