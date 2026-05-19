package handler

import (
	"errors"

	"github.com/di-eqa/backend/internal/application/service"
	"github.com/di-eqa/backend/internal/utils"
	"github.com/gin-gonic/gin"
)

// AuthHandler is the HTTP driving adapter for auth use cases.
type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// addressInput is the JSON payload for the delivery address fields.
type addressInput struct {
	AddressNo   string `json:"addressNo"`
	Building    string `json:"building"`
	SubDistrict string `json:"subDistrict"`
	District    string `json:"district"`
	Province    string `json:"province"`
	PostalCode  string `json:"postalCode"`
}

// registerInput models the /auth/register JSON payload.
//
// HospitalCode is only required for memberType="internal". For "external"
// users it is ignored. fullName is still accepted as a backward-compatible
// fallback when callers haven't been updated to send firstName/lastName.
type registerInput struct {
	MemberType   string `json:"memberType"`
	HospitalCode string `json:"hospitalCode"`

	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6"`

	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	FullName  string `json:"fullName"`
	Email     string `json:"email"`

	Clinic       string `json:"clinic"`
	LabName      string `json:"labName"`
	HospitalType string `json:"hospitalType"`
	BedSize      string `json:"bedSize"`

	Address addressInput `json:"address"`

	CertificateYear int `json:"certificateYear"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var in registerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.ErrBadRequest(c, err.Error())
		return
	}

	out, err := h.svc.Register(c.Request.Context(), service.RegisterInput{
		MemberType:   in.MemberType,
		HospitalCode: in.HospitalCode,
		Username:     in.Username,
		Password:     in.Password,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		FullName:     in.FullName,
		Email:        in.Email,
		Clinic:       in.Clinic,
		LabName:      in.LabName,
		HospitalType: in.HospitalType,
		BedSize:      in.BedSize,
		Address: service.RegisterAddress{
			AddressNo:   in.Address.AddressNo,
			Building:    in.Address.Building,
			SubDistrict: in.Address.SubDistrict,
			District:    in.Address.District,
			Province:    in.Address.Province,
			PostalCode:  in.Address.PostalCode,
		},
		CertificateYear: in.CertificateYear,
		IP:              c.ClientIP(),
		UserAgent:       c.Request.UserAgent(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrHospitalNotFound):
			utils.ErrBadRequest(c, "ไม่พบรหัสโรงพยาบาลนี้")
		case errors.Is(err, service.ErrUsernameExists):
			utils.ErrConflict(c, "username นี้ถูกใช้งานแล้ว กรุณาเลือก username ใหม่")
		case errors.Is(err, service.ErrInvalidMemberType):
			utils.ErrBadRequest(c, "ประเภทสมาชิกไม่ถูกต้อง")
		default:
			utils.ErrInternalErr(c, err)
		}
		return
	}
	utils.RespondOK(c, gin.H{"token": out.Token, "user": out.User})
}

type loginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var in loginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.ErrBadRequest(c, err.Error())
		return
	}
	out, err := h.svc.Login(c.Request.Context(), service.LoginInput{
		Username: in.Username,
		Password: in.Password,
	})
	if err != nil {
		utils.ErrUnauthorized(c, "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง")
		return
	}
	utils.RespondOK(c, gin.H{"token": out.Token, "user": out.User})
}

func (h *AuthHandler) Me(c *gin.Context) {
	uid, _ := c.Get("userId")
	userIDStr, _ := uid.(string)

	pub, err := h.svc.Me(c.Request.Context(), userIDStr)
	if err != nil {
		utils.ErrUnauthorized(c, "user not found")
		return
	}
	utils.RespondOK(c, pub)
}
