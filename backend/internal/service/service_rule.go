package service

import (
	"fmt"
	"github.com/cygreenenv/greenhouse-panel/internal/constants"
	apperrors "github.com/cygreenenv/greenhouse-panel/internal/errors"
	"github.com/cygreenenv/greenhouse-panel/internal/model"
	"github.com/cygreenenv/greenhouse-panel/internal/repository"
	ws "github.com/cygreenenv/greenhouse-panel/internal/websocket"
	"log/slog"
	"net/http"
	"time"
)

// RuleService 管理阈值联动规则，并在每次读数入库后评估规则状态机。
type RuleService struct {
	ruleRepo   *repository.RuleRepository
	sensorRepo *repository.SensorRepository
	deviceRepo *repository.DeviceRepository
	control    *ControlService
	logger     *slog.Logger
	hub        *ws.Hub
}

func NewRuleService(rr *repository.RuleRepository, sr *repository.SensorRepository, dr *repository.DeviceRepository, cs *ControlService, l *slog.Logger, h *ws.Hub) *RuleService {
	return &RuleService{ruleRepo: rr, sensorRepo: sr, deviceRepo: dr, control: cs, logger: l, hub: h}
}

func (s *RuleService) List(greenhouseID uint) ([]model.AutomationRule, error) {
	return s.ruleRepo.List(greenhouseID)
}

func (s *RuleService) SetEnabled(id uint, enabled bool) (*model.AutomationRule, error) {
	// 停用期间绝不动设备；重新启用时状态机复位为正常，避免误触发恢复动作。
	return s.ruleRepo.SetEnabled(id, enabled)
}

func (s *RuleService) Delete(id uint) error { return s.ruleRepo.Delete(id) }

// Create 校验规则配置完整后落库；新规则状态从 normal 开始，不会追溯历史异常。
func (s *RuleService) Create(req CreateRuleInput) (*model.AutomationRule, error) {
	if !isValidTriggerSide(req.TriggerSide) || !isValidSwitchAction(req.TriggerAction) {
		return nil, apperrors.New(http.StatusBadRequest, "触发条件或动作不合法", http.StatusBadRequest)
	}
	sensor, err := s.sensorRepo.Get(req.SensorID)
	if err != nil {
		return nil, err
	}
	device, err := s.deviceRepo.Get(req.DeviceID)
	if err != nil {
		return nil, err
	}
	if sensor.GreenhouseID != device.GreenhouseID {
		return nil, apperrors.New(http.StatusBadRequest, "传感器与设备必须属于同一温室", http.StatusBadRequest)
	}
	rule := &model.AutomationRule{
		GreenhouseID:   sensor.GreenhouseID,
		Name:           req.Name,
		SensorID:       req.SensorID,
		DeviceID:       req.DeviceID,
		TriggerSide:    req.TriggerSide,
		TriggerAction:  req.TriggerAction,
		RecoveryAction: constants.OppositeAction(req.TriggerAction),
		Enabled:        req.Enabled,
		State:          constants.RuleStateNormal,
	}
	if err = s.ruleRepo.Create(rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// EvaluateReading 在一条读数产生后评估该传感器下所有已启用规则。
// 联动失败不阻断读数/报警主流程，只记录规则结果与日志。
func (s *RuleService) EvaluateReading(sensor *model.Sensor, value float64) {
	rules, err := s.ruleRepo.EnabledBySensor(sensor.ID)
	if err != nil {
		s.logger.Error("list rules for reading evaluation failed", "sensorId", sensor.ID, "error", err)
		return
	}
	for i := range rules {
		s.evaluate(&rules[i], sensor, value)
	}
}

func (s *RuleService) evaluate(rule *model.AutomationRule, sensor *model.Sensor, value float64) {
	if err := s.validateComplete(rule); err != nil {
		s.markSkipped(rule, "规则配置不完整，未执行设备动作")
		return
	}
	abnormal := isBeyondSide(rule.TriggerSide, value, sensor.Threshold.MinValue, sensor.Threshold.MaxValue)
	switch {
	case abnormal && rule.State != constants.RuleStateAbnormal:
		// normal -> abnormal：一次异常只在这个翻转点动作一次。
		s.execute(rule, rule.TriggerAction, value, constants.RuleStateAbnormal, "越限触发")
	case !abnormal && rule.State == constants.RuleStateAbnormal:
		// abnormal -> normal：恢复到正常范围后执行另一侧动作。
		s.execute(rule, rule.RecoveryAction, value, constants.RuleStateNormal, "恢复联动")
	}
}

func (s *RuleService) execute(rule *model.AutomationRule, action string, value float64, nextState, phase string) {
	device, err := s.deviceRepo.Get(rule.DeviceID)
	if err != nil {
		s.markFailed(rule, fmt.Sprintf("%s失败：设备不存在", phase))
		return
	}
	if device.Status == action {
		// 设备已处于目标状态，无需重复开关，但仍按一次异常处理一次记录。
		now := time.Now()
		rule.State = nextState
		rule.LastResult = constants.RuleResultSkipped
		rule.LastMessage = fmt.Sprintf("%s：%s已是%s状态，读数 %.2f，无需重复操作", phase, device.Name, actionLabel(action), value)
		rule.LastTriggeredAt = &now
		s.persist(rule)
		return
	}
	if _, err = s.control.Toggle(rule.DeviceID, action, constants.OperatorAutomation); err != nil {
		s.markFailed(rule, fmt.Sprintf("%s：%s切换失败", phase, device.Name))
		return
	}
	now := time.Now()
	rule.State = nextState
	rule.LastResult = constants.RuleResultSuccess
	rule.LastMessage = fmt.Sprintf("%s：读数 %.2f，已将%s%s", phase, value, device.Name, actionLabel(action))
	rule.LastTriggeredAt = &now
	s.persist(rule)
}

func (s *RuleService) markSkipped(rule *model.AutomationRule, message string) {
	if rule.LastResult == constants.RuleResultSkipped && rule.LastMessage == message {
		return
	}
	rule.LastResult = constants.RuleResultSkipped
	rule.LastMessage = message
	s.persist(rule)
}

func (s *RuleService) markFailed(rule *model.AutomationRule, message string) {
	// 失败不推进状态机，下一条读数会重试，保证异常期间不漏动作。
	rule.LastResult = constants.RuleResultFailed
	rule.LastMessage = message
	now := time.Now()
	rule.LastTriggeredAt = &now
	if err := s.ruleRepo.SaveResult(rule); err != nil {
		s.logger.Error("persist rule failure failed", "ruleId", rule.ID, "error", err)
	}
	s.hub.Broadcast(constants.EventRule, rule)
	s.logger.Warn("automation rule execution failed", "ruleId", rule.ID, "message", message)
}

func (s *RuleService) persist(rule *model.AutomationRule) {
	if err := s.ruleRepo.SaveResult(rule); err != nil {
		s.logger.Error("persist rule result failed", "ruleId", rule.ID, "error", err)
		return
	}
	s.hub.Broadcast(constants.EventRule, rule)
}

// validateComplete 校验规则引用的传感器/设备存在、阈值合法且动作有效；
// 停用或配置不完整的规则绝不动设备。
func (s *RuleService) validateComplete(rule *model.AutomationRule) error {
	if rule.SensorID == 0 || rule.DeviceID == 0 || rule.TriggerSide == "" || rule.TriggerAction == "" || rule.RecoveryAction == "" {
		return fmt.Errorf("incomplete rule %d", rule.ID)
	}
	if !isValidTriggerSide(rule.TriggerSide) || !isValidSwitchAction(rule.TriggerAction) || !isValidSwitchAction(rule.RecoveryAction) {
		return fmt.Errorf("rule %d has invalid trigger configuration", rule.ID)
	}
	sensor, err := s.sensorRepo.Get(rule.SensorID)
	if err != nil {
		return err
	}
	device, err := s.deviceRepo.Get(rule.DeviceID)
	if err != nil {
		return err
	}
	if sensor.GreenhouseID != device.GreenhouseID || sensor.GreenhouseID != rule.GreenhouseID {
		return fmt.Errorf("rule %d references cross-greenhouse resources", rule.ID)
	}
	if sensor.Threshold.MinValue >= sensor.Threshold.MaxValue {
		return fmt.Errorf("rule %d sensor threshold invalid", rule.ID)
	}
	return nil
}

func isBeyondSide(side string, value, min, max float64) bool {
	switch side {
	case constants.TriggerSideLow:
		return value < min
	case constants.TriggerSideBoth:
		return value < min || value > max
	default:
		return value > max
	}
}

func isValidTriggerSide(side string) bool {
	return side == constants.TriggerSideHigh || side == constants.TriggerSideLow || side == constants.TriggerSideBoth
}

func isValidSwitchAction(action string) bool {
	return action == constants.StatusOn || action == constants.StatusOff
}

func actionLabel(action string) string {
	if action == constants.StatusOn {
		return "开启"
	}
	return "关闭"
}

// CreateRuleInput 是新建联动规则的入参，恢复动作由触发动作自动取反。
type CreateRuleInput struct {
	Name          string
	SensorID      uint
	DeviceID      uint
	TriggerSide   string
	TriggerAction string
	Enabled       bool
}
