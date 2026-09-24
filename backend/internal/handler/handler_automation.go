package handler

import (
	"github.com/cygreenenv/greenhouse-panel/internal/constants"
	"github.com/cygreenenv/greenhouse-panel/internal/dto"
	apperrors "github.com/cygreenenv/greenhouse-panel/internal/errors"
	"github.com/cygreenenv/greenhouse-panel/internal/model"
	"github.com/cygreenenv/greenhouse-panel/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type AutomationHandler struct {
	service   *service.AutomationService
	validator *validator.Validate
}

func NewAutomationHandler(s *service.AutomationService, v *validator.Validate) *AutomationHandler {
	return &AutomationHandler{service: s, validator: v}
}

func (h *AutomationHandler) List(c *gin.Context) {
	greenhouseID := uint(0)
	if raw := c.Query("greenhouse_id"); raw != "" {
		parsed, ok := queryID(c, "greenhouse_id")
		if !ok {
			return
		}
		greenhouseID = parsed
	}
	rows, err := h.service.List(greenhouseID)
	if err != nil {
		Fail(c, err)
		return
	}
	Success(c, rows)
}

func (h *AutomationHandler) Create(c *gin.Context) {
	var req dto.AutomationRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil || h.validator.Struct(req) != nil || req.AbnormalAction == req.NormalAction {
		Fail(c, apperrors.ErrValidation)
		return
	}
	row := &model.AutomationRule{
		Name:           req.Name,
		GreenhouseID:   req.GreenhouseID,
		SensorID:       req.SensorID,
		DeviceID:       req.DeviceID,
		AbnormalAction: req.AbnormalAction,
		NormalAction:   req.NormalAction,
		Enabled:        true,
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if err := h.service.Create(row); err != nil {
		Fail(c, err)
		return
	}
	Created(c, row)
}

func (h *AutomationHandler) SetEnabled(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.AutomationRuleStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, apperrors.ErrValidation)
		return
	}
	row, err := h.service.SetEnabled(id, req.Enabled)
	if err != nil {
		Fail(c, err)
		return
	}
	Success(c, row)
}

func (h *AutomationHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(id); err != nil {
		Fail(c, err)
		return
	}
	Success(c, gin.H{"id": id, "message": constants.SuccessMessage})
}
