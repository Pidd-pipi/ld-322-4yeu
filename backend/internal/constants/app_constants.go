package constants

const (
	APIPrefix               = "/api/v1"
	HealthPath              = "/healthz"
	WebSocketPath           = "/ws"
	DefaultPage             = 1
	DefaultPageSize         = 100
	MaxPageSize             = 500
	StatusOnline            = "online"
	StatusOff               = "off"
	StatusOn                = "on"
	AlertPending            = "pending"
	AlertHandled            = "handled"
	RoleAdmin               = "admin"
	SuccessMessage          = "ok"
	EventAlert              = "alert.created"
	EventDevice             = "device.updated"
	EventAutomationExecuted = "automation.executed"
	EventAutomationUpdated  = "automation.updated"
	RuleStateNormal         = "normal"
	RuleStateAbnormal       = "abnormal"
	RuleResultSuccess       = "success"
	RuleResultFailed        = "failed"
	OperatorAutomation      = "automation-rule"
)
