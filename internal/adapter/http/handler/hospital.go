package handler

import (
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
