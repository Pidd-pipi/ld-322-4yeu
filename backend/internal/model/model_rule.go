package model

import "time"

// AutomationRule 阈值联动规则：某传感器读数超出阈值时自动开关指定设备，
// 读数恢复到正常范围后再执行相反动作。State 记录当前是否处于异常区间，
// 保证一次异常只在越限/恢复的状态翻转点动作一次。
type AutomationRule struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	GreenhouseID    uint       `gorm:"index" json:"greenhouseId"`
	Name            string     `gorm:"type:varchar(100)" json:"name"`
	SensorID        uint       `gorm:"index" json:"sensorId"`
	DeviceID        uint       `gorm:"index" json:"deviceId"`
	TriggerSide     string     `gorm:"type:varchar(16)" json:"triggerSide"`
	TriggerAction   string     `gorm:"type:varchar(16)" json:"triggerAction"`
	RecoveryAction  string     `gorm:"type:varchar(16)" json:"recoveryAction"`
	Enabled         bool       `json:"enabled"`
	State           string     `gorm:"type:varchar(16);default:normal" json:"state"`
	LastResult      string     `gorm:"type:varchar(16)" json:"lastResult"`
	LastMessage     string     `gorm:"type:varchar(500)" json:"lastMessage"`
	LastTriggeredAt *time.Time `json:"lastTriggeredAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	Sensor          Sensor     `json:"sensor,omitempty"`
	Device          Device     `json:"device,omitempty"`
}
