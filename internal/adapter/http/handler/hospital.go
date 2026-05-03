package handler

import (
	"net/http"

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *HospitalHandler) GetByCode(c *gin.Context) {
	hospital, err := h.svc.GetByCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hospital not found"})
		return
	}
	c.JSON(http.StatusOK, hospital)
}
