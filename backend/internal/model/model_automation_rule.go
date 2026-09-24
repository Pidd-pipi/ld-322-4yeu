package model

import "time"

// AutomationRule links one sensor threshold state to a target device action.
type AutomationRule struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	Name                 string     `gorm:"type:varchar(100);not null" json:"name"`
	GreenhouseID         uint       `gorm:"index;not null" json:"greenhouseId"`
	SensorID             uint       `gorm:"index;not null" json:"sensorId"`
	DeviceID             uint       `gorm:"index;not null" json:"deviceId"`
	AbnormalAction       string     `gorm:"type:varchar(32);not null" json:"abnormalAction"`
	NormalAction         string     `gorm:"type:varchar(32);not null" json:"normalAction"`
	Enabled              bool       `gorm:"not null;index" json:"enabled"`
	LastState            string     `gorm:"type:varchar(32);default:normal" json:"lastState"`
	LastExecutionStatus  string     `gorm:"type:varchar(32)" json:"lastExecutionStatus"`
	LastExecutionMessage string     `gorm:"type:varchar(500)" json:"lastExecutionMessage"`
	LastExecutionAction  string     `gorm:"type:varchar(32)" json:"lastExecutionAction"`
	LastExecutionValue   float64    `json:"lastExecutionValue"`
	LastExecutedAt       *time.Time `json:"lastExecutedAt,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	Sensor               Sensor     `json:"sensor,omitempty"`
	Device               Device     `json:"device,omitempty"`
}
