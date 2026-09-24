package repository

import (
	"errors"
	"fmt"
	"github.com/cygreenenv/greenhouse-panel/internal/constants"
	apperrors "github.com/cygreenenv/greenhouse-panel/internal/errors"
	"github.com/cygreenenv/greenhouse-panel/internal/model"
	"gorm.io/gorm"
)

type RuleRepository struct{ db *gorm.DB }

func NewRuleRepository(db *gorm.DB) *RuleRepository { return &RuleRepository{db: db} }

func (r *RuleRepository) List(greenhouseID uint) ([]model.AutomationRule, error) {
	var rows []model.AutomationRule
	q := r.db.Preload("Sensor").Preload("Device").Order("id asc")
	if greenhouseID > 0 {
		q = q.Where("greenhouse_id = ?", greenhouseID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list automation rules: %w", err)
	}
	return rows, nil
}

func (r *RuleRepository) Get(id uint) (*model.AutomationRule, error) {
	var row model.AutomationRule
	err := r.db.First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get automation rule: %w", err)
	}
	return &row, nil
}

func (r *RuleRepository) EnabledBySensor(sensorID uint) ([]model.AutomationRule, error) {
	var rows []model.AutomationRule
	if err := r.db.Where("sensor_id = ? AND enabled = ?", sensorID, true).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list enabled rules by sensor: %w", err)
	}
	return rows, nil
}

func (r *RuleRepository) Create(row *model.AutomationRule) error {
	// Select 显式写入布尔零值，避免 enabled=false 被数据库默认值覆盖。
	if err := r.db.Select("GreenhouseID", "Name", "SensorID", "DeviceID", "TriggerSide", "TriggerAction", "RecoveryAction", "Enabled", "State").Create(row).Error; err != nil {
		return fmt.Errorf("create automation rule: %w", err)
	}
	return nil
}

func (r *RuleRepository) SetEnabled(id uint, enabled bool) (*model.AutomationRule, error) {
	row, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	row.Enabled = enabled
	updates := map[string]any{"enabled": enabled}
	if enabled {
		// 重新启用时把状态机复位为正常，避免停用期间读数变化后误触发恢复动作。
		row.State = constants.RuleStateNormal
		updates["state"] = constants.RuleStateNormal
	}
	if err = r.db.Model(&model.AutomationRule{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update automation rule enabled: %w", err)
	}
	return row, nil
}

func (r *RuleRepository) Delete(id uint) error {
	res := r.db.Delete(&model.AutomationRule{}, id)
	if res.Error != nil {
		return fmt.Errorf("delete automation rule: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.ErrRecordNotFound
	}
	return nil
}

// SaveResult 持久化规则状态机的最新一次执行结果。
func (r *RuleRepository) SaveResult(row *model.AutomationRule) error {
	fields := map[string]any{
		"state":             row.State,
		"last_result":       row.LastResult,
		"last_message":      row.LastMessage,
		"last_triggered_at": row.LastTriggeredAt,
	}
	if err := r.db.Model(&model.AutomationRule{}).Where("id = ?", row.ID).Updates(fields).Error; err != nil {
		return fmt.Errorf("save automation rule result: %w", err)
	}
	return nil
}
