package handler

import (
	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// HospitalHandler serves the public hospital list and admin hospital management.
type HospitalHandler struct {
	svc *service.HospitalService
}

func NewHospitalHandler(svc *service.HospitalService) *HospitalHandler {
	return &HospitalHandler{svc: svc}
}

// PublicList handles GET /api/public/hospitals: {id,name} of active hospitals only.
func (h *HospitalHandler) PublicList(c *gin.Context) {
	items, err := h.svc.ListPublic(c.Request.Context())
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// AdminList handles GET /api/admin/hospitals.
func (h *HospitalHandler) AdminList(c *gin.Context) {
	active, ok := queryBool(c, "active")
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), service.ListHospitalsInput{
		Query: c.Query("q"), Active: active, Page: queryInt(c, "page"), PageSize: queryInt(c, "pageSize"),
	})
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

type hospitalBody struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Logo        string `json:"logo"`
	Province    string `json:"province"`
	District    string `json:"district"`
	SubDistrict string `json:"subDistrict"`
	PostalCode  string `json:"postalCode"`
	Active      *bool  `json:"active"`
}

func (b hospitalBody) fields() service.HospitalFields {
	return service.HospitalFields{
		Code: b.Code, Name: b.Name, Logo: b.Logo, Province: b.Province, District: b.District,
		SubDistrict: b.SubDistrict, PostalCode: b.PostalCode, Active: b.Active,
	}
}

// Create handles POST /api/admin/hospitals.
func (h *HospitalHandler) Create(c *gin.Context) {
	var body hospitalBody
	if !bind(c, &body, false) {
		return
	}
	out, err := h.svc.Create(c.Request.Context(), body.fields(), actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Created(c, out)
}

// Update handles PUT /api/admin/hospitals/:id.
func (h *HospitalHandler) Update(c *gin.Context) {
	var body hospitalBody
	if !bind(c, &body, false) {
		return
	}
	out, err := h.svc.Update(c.Request.Context(), c.Param("id"), body.fields(), actorFrom(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.OK(c, out)
}

// Delete handles DELETE /api/admin/hospitals/:id (soft delete).
func (h *HospitalHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), actorFrom(c)); err != nil {
		response.FromError(c, err)
		return
	}
	response.NoContent(c)
}
