package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cygreenenv/greenhouse-panel/internal/constants"
	apperrors "github.com/cygreenenv/greenhouse-panel/internal/errors"
	"github.com/cygreenenv/greenhouse-panel/internal/model"
	"github.com/cygreenenv/greenhouse-panel/internal/repository"
	ws "github.com/cygreenenv/greenhouse-panel/internal/websocket"
	"gorm.io/gorm"
)

type AutomationService struct {
	db             *gorm.DB
	greenhouseRepo *repository.GreenhouseRepository
	ruleRepo       *repository.AutomationRuleRepository
	sensorRepo     *repository.SensorRepository
	deviceRepo     *repository.DeviceRepository
	logger         *slog.Logger
	hub            *ws.Hub
}

func NewAutomationService(
	db *gorm.DB,
	greenhouseRepo *repository.GreenhouseRepository,
	ruleRepo *repository.AutomationRuleRepository,
	sensorRepo *repository.SensorRepository,
	deviceRepo *repository.DeviceRepository,
	logger *slog.Logger,
	hub *ws.Hub,
) *AutomationService {
	return &AutomationService{
		db: db, greenhouseRepo: greenhouseRepo, ruleRepo: ruleRepo, sensorRepo: sensorRepo,
		deviceRepo: deviceRepo, logger: logger, hub: hub,
	}
}

func (s *AutomationService) List(greenhouseID uint) ([]model.AutomationRule, error) {
	return s.ruleRepo.List(greenhouseID)
}

func (s *AutomationService) Create(row *model.AutomationRule) error {
	if err := s.validate(row); err != nil {
		return err
	}
	now := time.Now()
	row.LastState = constants.RuleStateNormal
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := s.ruleRepo.Create(row); err != nil {
		return err
	}
	created, err := s.ruleRepo.Get(row.ID)
	if err != nil {
		s.logger.Error("load created automation rule failed", "ruleId", row.ID, "error", err)
		return nil
	}
	s.hub.Broadcast(constants.EventAutomationUpdated, created)
	return nil
}

func (s *AutomationService) SetEnabled(id uint, enabled bool) (*model.AutomationRule, error) {
	row, err := s.ruleRepo.SetEnabled(id, enabled)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(constants.EventAutomationUpdated, row)
	return row, nil
}

func (s *AutomationService) Delete(id uint) error {
	if err := s.ruleRepo.Delete(id); err != nil {
		return err
	}
	s.hub.Broadcast(constants.EventAutomationUpdated, deletePayload(id))
	return nil
}

func (s *AutomationService) Evaluate(sensorID uint, value float64) {
	rules, err := s.ruleRepo.ActiveForSensor(sensorID)
	if err != nil {
		s.logger.Error("load automation rules failed", "sensorId", sensorID, "error", err)
		return
	}
	for _, rule := range rules {
		s.evaluateRule(rule, value)
	}
}

func (s *AutomationService) validate(row *model.AutomationRule) error {
	if row.Name == "" || row.SensorID == 0 || row.DeviceID == 0 {
		return apperrors.ErrValidation
	}
	if row.AbnormalAction != constants.StatusOn && row.AbnormalAction != constants.StatusOff {
		return apperrors.ErrValidation
	}
	if row.NormalAction != constants.StatusOn && row.NormalAction != constants.StatusOff {
		return apperrors.ErrValidation
	}
	if row.AbnormalAction == row.NormalAction {
		return apperrors.ErrValidation
	}
	if _, err := s.greenhouseRepo.Get(row.GreenhouseID); err != nil {
		return mapRuleReferenceError(err)
	}
	sensor, err := s.sensorRepo.Get(row.SensorID)
	if err != nil {
		return mapRuleReferenceError(err)
	}
	device, err := s.deviceRepo.Get(row.DeviceID)
	if err != nil {
		return mapRuleReferenceError(err)
	}
	if sensor.GreenhouseID != row.GreenhouseID || device.GreenhouseID != row.GreenhouseID {
		return apperrors.ErrValidation
	}
	if sensor.Threshold.ID == 0 || sensor.Threshold.MinValue >= sensor.Threshold.MaxValue {
		return apperrors.ErrValidation
	}
	return nil
}

func mapRuleReferenceError(err error) error {
	if errors.Is(err, apperrors.ErrRecordNotFound) {
		return apperrors.ErrValidation
	}
	return err
}

func (s *AutomationService) evaluateRule(rule model.AutomationRule, value float64) {
	if !ruleConfigured(rule) {
		return
	}
	targetState := constants.RuleStateNormal
	action := rule.NormalAction
	if value < rule.Sensor.Threshold.MinValue || value > rule.Sensor.Threshold.MaxValue {
		targetState = constants.RuleStateAbnormal
		action = rule.AbnormalAction
	}
	if rule.LastState == targetState {
		return
	}
	fromState := constants.RuleStateNormal
	if targetState == constants.RuleStateNormal {
		fromState = constants.RuleStateAbnormal
	}

	updated, changedDevice, err := s.executeTransition(rule.ID, fromState, targetState, rule.DeviceID, action, value)
	if err != nil {
		s.logger.Error("automation action failed", "ruleId", rule.ID, "deviceId", rule.DeviceID, "error", err)
		failed, saveErr := s.ruleRepo.SaveResult(rule.ID, targetState, repository.AutomationResultFields{
			Status:  constants.RuleResultFailed,
			Message: fmt.Sprintf("联动失败：%v", err),
			Action:  action,
			Value:   value,
		})
		if saveErr != nil {
			s.logger.Error("save automation failure failed", "ruleId", rule.ID, "error", saveErr)
			return
		}
		s.hub.Broadcast(constants.EventAutomationExecuted, failed)
	}
	if updated != nil {
		s.hub.Broadcast(constants.EventAutomationExecuted, updated)
	}
	if changedDevice != nil {
		s.hub.Broadcast(constants.EventDevice, changedDevice)
	}
}

func (s *AutomationService) executeTransition(ruleID uint, fromState, targetState string, deviceID uint, action string, value float64) (*model.AutomationRule, *model.Device, error) {
	var updated *model.AutomationRule
	var changedDevice *model.Device
	err := s.db.Transaction(func(tx *gorm.DB) error {
		ruleRepo := s.ruleRepo.WithTx(tx)
		deviceRepo := s.deviceRepo.WithTx(tx)
		moved, err := ruleRepo.TryTransition(ruleID, fromState, targetState)
		if err != nil {
			return err
		}
		if !moved {
			return nil
		}
		device, changed, err := deviceRepo.EnsureStatus(deviceID, action, constants.OperatorAutomation)
		if err != nil {
			return err
		}
		message := fmt.Sprintf("读数 %.2f，已执行设备%s", value, actionLabel(action))
		if !changed {
			message = fmt.Sprintf("读数 %.2f，设备已处于%s状态，无需重复操作", value, actionLabel(action))
		} else {
			changedDevice = device
		}
		updated, err = ruleRepo.SaveResult(ruleID, targetState, repository.AutomationResultFields{
			Status:  constants.RuleResultSuccess,
			Message: message,
			Action:  action,
			Value:   value,
		})
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return updated, changedDevice, nil
}

func ruleConfigured(rule model.AutomationRule) bool {
	return rule.SensorID != 0 && rule.DeviceID != 0 &&
		rule.Sensor.ID != 0 && rule.Sensor.Threshold.ID != 0 &&
		rule.Sensor.Threshold.MinValue < rule.Sensor.Threshold.MaxValue &&
		rule.Device.ID != 0 && rule.Sensor.GreenhouseID == rule.GreenhouseID &&
		rule.Device.GreenhouseID == rule.GreenhouseID &&
		isDeviceAction(rule.AbnormalAction) && isDeviceAction(rule.NormalAction) &&
		rule.AbnormalAction != rule.NormalAction
}

func isDeviceAction(action string) bool {
	return action == constants.StatusOn || action == constants.StatusOff
}

func actionLabel(action string) string {
	if action == constants.StatusOn {
		return "开启"
	}
	return "关闭"
}

func deletePayload(id uint) map[string]any {
	return map[string]any{"id": id, "deleted": true}
}
