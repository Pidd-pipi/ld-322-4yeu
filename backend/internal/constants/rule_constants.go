package constants

const (
	TriggerSideHigh = "high"
	TriggerSideLow  = "low"
	TriggerSideBoth = "both"

	RuleStateNormal   = "normal"
	RuleStateAbnormal = "abnormal"

	RuleResultSuccess = "success"
	RuleResultSkipped = "skipped"
	RuleResultFailed  = "failed"

	OperatorAdmin      = "admin"
	OperatorAutomation = "automation"

	EventRule = "rule.executed"
)

// OppositeAction 返回设备开关动作的另一侧动作。
func OppositeAction(action string) string {
	if action == StatusOn {
		return StatusOff
	}
	return StatusOn
}
