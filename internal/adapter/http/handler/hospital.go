package handler

import (
	"errors"

	utils "github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// HospitalHandler is the HTTP driving adapter for hospital use cases.
type HospitalHandler struct {
	svc *service.HospitalService
}

func NewHospitalHandler(svc *service.HospitalService) *HospitalHandler {
	return &HospitalHandler{svc: svc}
}

func (h *HospitalHandler) List(c *gin.Context) {
	q := c.Query("q")
	list, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		utils.ErrInternalErr(c, err)
		return
	}
	utils.RespondOK(c, list)
}

func (h *HospitalHandler) GetByCode(c *gin.Context) {
	hospital, err := h.svc.GetByCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		utils.ErrNotFound(c, "hospital not found")
		return
	}
	utils.RespondOK(c, hospital)
}

// hospitalBody is the JSON payload accepted by Create and Update.
type hospitalBody struct {
	Code        string `json:"code"        binding:"required"`
	Name        string `json:"name"        binding:"required"`
	Logo        string `json:"logo"`
	Province    string `json:"province"`
	District    string `json:"district"`
	SubDistrict string `json:"subDistrict"`
	PostalCode  string `json:"postalCode"`
}

func (b hospitalBody) toFields() service.HospitalFields {
	return service.HospitalFields{
		Code:        b.Code,
		Name:        b.Name,
		Logo:        b.Logo,
		Province:    b.Province,
		District:    b.District,
		SubDistrict: b.SubDistrict,
		PostalCode:  b.PostalCode,
	}
}

// actorFrom builds the audit actor from the authenticated request context.
func actorFrom(c *gin.Context) service.Actor {
	id, _ := c.Get("userId")
	idStr, _ := id.(string)
	role, _ := c.Get("role")
	roleStr, _ := role.(string)
	return service.Actor{
		ID:        idStr,
		Role:      roleStr,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
}

// respondHospitalErr maps application errors onto HTTP status codes.
func respondHospitalErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		utils.ErrNotFound(c, "ไม่พบโรงพยาบาล")
	case errors.Is(err, service.ErrHospitalCodeExists):
		utils.ErrConflict(c, "รหัสโรงพยาบาลนี้ถูกใช้แล้ว")
	case errors.Is(err, service.ErrHospitalInUse):
		utils.ErrConflict(c, "ลบไม่ได้ — ยังมีผู้ใช้สังกัดโรงพยาบาลนี้อยู่")
	case errors.Is(err, service.ErrInvalidPostalCode):
		utils.ErrBadRequest(c, "รหัสไปรษณีย์ต้องเป็นตัวเลข 5 หลัก")
	case errors.Is(err, service.ErrBadInput):
		utils.ErrBadRequest(c, "กรุณากรอกรหัสและชื่อโรงพยาบาล")
	default:
		utils.ErrInternalErr(c, err)
	}
}

// Create handles POST /api/hospitals
func (h *HospitalHandler) Create(c *gin.Context) {
	var body hospitalBody
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.ErrBadRequest(c, err.Error())
		return
	}
	hospital, err := h.svc.Create(c.Request.Context(), body.toFields(), actorFrom(c))
	if err != nil {
		respondHospitalErr(c, err)
		return
	}
	utils.RespondCreated(c, hospital)
}

// Update handles PUT /api/hospitals/:id
func (h *HospitalHandler) Update(c *gin.Context) {
	var body hospitalBody
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.ErrBadRequest(c, err.Error())
		return
	}
	hospital, err := h.svc.Update(c.Request.Context(), c.Param("id"), body.toFields(), actorFrom(c))
	if err != nil {
		respondHospitalErr(c, err)
		return
	}
	utils.RespondOK(c, hospital)
}

// Delete handles DELETE /api/hospitals/:id
func (h *HospitalHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), actorFrom(c)); err != nil {
		respondHospitalErr(c, err)
		return
	}
	utils.RespondOK(c, gin.H{"ok": true})
}
