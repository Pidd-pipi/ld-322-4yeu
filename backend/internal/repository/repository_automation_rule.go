package repository

import (
	"errors"
	"fmt"
	apperrors "github.com/cygreenenv/greenhouse-panel/internal/errors"
	"github.com/cygreenenv/greenhouse-panel/internal/model"
	"gorm.io/gorm"
	"time"
)

type AutomationRuleRepository struct{ db *gorm.DB }

type AutomationResultFields struct {
	Status  string
	Message string
	Action  string
	Value   float64
}

func NewAutomationRuleRepository(db *gorm.DB) *AutomationRuleRepository {
	return &AutomationRuleRepository{db: db}
}

func (r *AutomationRuleRepository) WithTx(tx *gorm.DB) *AutomationRuleRepository {
	return &AutomationRuleRepository{db: tx}
}

func (r *AutomationRuleRepository) Create(row *model.AutomationRule) error {
	if err := r.db.Select("*").Create(row).Error; err != nil {
		return fmt.Errorf("create automation rule: %w", err)
	}
	return nil
}

func (r *AutomationRuleRepository) List(greenhouseID uint) ([]model.AutomationRule, error) {
	var rows []model.AutomationRule
	q := r.db.Preload("Sensor.Threshold").Preload("Device").Order("id desc")
	if greenhouseID > 0 {
		q = q.Where("greenhouse_id = ?", greenhouseID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list automation rules: %w", err)
	}
	return rows, nil
}

func (r *AutomationRuleRepository) Get(id uint) (*model.AutomationRule, error) {
	var row model.AutomationRule
	err := r.db.Preload("Sensor.Threshold").Preload("Device").First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get automation rule: %w", err)
	}
	return &row, nil
}

func (r *AutomationRuleRepository) ActiveForSensor(sensorID uint) ([]model.AutomationRule, error) {
	var rows []model.AutomationRule
	err := r.db.Preload("Sensor.Threshold").Preload("Device").
		Where("sensor_id = ? AND enabled = ?", sensorID, true).
		Order("id asc").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list active automation rules: %w", err)
	}
	return rows, nil
}

func (r *AutomationRuleRepository) TryTransition(id uint, fromState, toState string) (bool, error) {
	result := r.db.Model(&model.AutomationRule{}).
		Where("id = ? AND (last_state = ? OR last_state = '' OR last_state IS NULL)", id, fromState).
		Updates(map[string]any{"last_state": toState, "updated_at": time.Now()})
	if result.Error != nil {
		return false, fmt.Errorf("transition automation rule: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (r *AutomationRuleRepository) SaveResult(id uint, state string, fields AutomationResultFields) (*model.AutomationRule, error) {
	now := time.Now()
	err := r.db.Model(&model.AutomationRule{}).Where("id = ?", id).Updates(map[string]any{
		"last_state":             state,
		"last_execution_status":  fields.Status,
		"last_execution_message": fields.Message,
		"last_execution_action":  fields.Action,
		"last_execution_value":   fields.Value,
		"last_executed_at":       now,
		"updated_at":             now,
	}).Error
	if err != nil {
		return nil, fmt.Errorf("save automation result: %w", err)
	}
	return r.Get(id)
}

func (r *AutomationRuleRepository) SetEnabled(id uint, enabled bool) (*model.AutomationRule, error) {
	if _, err := r.Get(id); err != nil {
		return nil, err
	}
	if err := r.db.Model(&model.AutomationRule{}).Where("id = ?", id).
		Updates(map[string]any{"enabled": enabled, "updated_at": time.Now()}).Error; err != nil {
		return nil, fmt.Errorf("update automation rule enabled: %w", err)
	}
	return r.Get(id)
}

func (r *AutomationRuleRepository) Delete(id uint) error {
	result := r.db.Delete(&model.AutomationRule{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete automation rule: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrRecordNotFound
	}
	return nil
}
