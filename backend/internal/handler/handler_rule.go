package handler

import (
	"strconv"

	"github.com/cygreenenv/greenhouse-panel/internal/dto"
	apperrors "github.com/cygreenenv/greenhouse-panel/internal/errors"
	"github.com/cygreenenv/greenhouse-panel/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type RuleHandler struct {
	service   *service.RuleService
	validator *validator.Validate
}

func NewRuleHandler(s *service.RuleService, v *validator.Validate) *RuleHandler {
	return &RuleHandler{service: s, validator: v}
}

func (h *RuleHandler) List(c *gin.Context) {
	var greenhouseID uint
	if raw := c.Query("greenhouse_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			Fail(c, apperrors.ErrValidation)
			return
		}
		greenhouseID = uint(value)
	}
	rows, err := h.service.List(greenhouseID)
	if err != nil {
		Fail(c, err)
		return
	}
	Success(c, rows)
}

func (h *RuleHandler) Create(c *gin.Context) {
	var req dto.RuleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil || h.validator.Struct(req) != nil {
		Fail(c, apperrors.ErrValidation)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := h.service.Create(service.CreateRuleInput{
		Name:          req.Name,
		SensorID:      req.SensorID,
		DeviceID:      req.DeviceID,
		TriggerSide:   req.TriggerSide,
		TriggerAction: req.TriggerAction,
		Enabled:       enabled,
	})
	if err != nil {
		Fail(c, err)
		return
	}
	Created(c, row)
}

func (h *RuleHandler) SetStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.RuleStatusRequest
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

func (h *RuleHandler) Remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(id); err != nil {
		Fail(c, err)
		return
	}
	Success(c, gin.H{"deleted": id})
}
