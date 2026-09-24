package service

import (
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/cygreenenv/greenhouse-panel/internal/constants"
	"github.com/cygreenenv/greenhouse-panel/internal/model"
	"github.com/cygreenenv/greenhouse-panel/internal/repository"
	ws "github.com/cygreenenv/greenhouse-panel/internal/websocket"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var ruleDBSuffix atomic.Uint64

type ruleFixture struct {
	db       *gorm.DB
	svc      *RuleService
	ctrl     *ControlService
	device   *model.Device
	sensor   *model.Sensor
	actions  []model.DeviceAction
	devRepo  *repository.DeviceRepository
	ruleRepo *repository.RuleRepository
}

func newRuleFixture(t *testing.T) ruleFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:rule_test_%d?mode=memory&cache=shared", ruleDBSuffix.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(model.All()...); err != nil {
		t.Fatal(err)
	}
	g := model.Greenhouse{Name: "联动测试温室"}
	if err = db.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	sensor := &model.Sensor{GreenhouseID: g.ID, Name: "温度", Type: "temperature", Unit: "°C", Status: "online"}
	if err = db.Create(sensor).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.Threshold{SensorID: sensor.ID, MinValue: 15, MaxValue: 30}).Error; err != nil {
		t.Fatal(err)
	}
	device := &model.Device{GreenhouseID: g.ID, Name: "循环风机", Type: "fan", Status: constants.StatusOff}
	if err = db.Create(device).Error; err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := ws.NewHub()
	sensorRepo := repository.NewSensorRepository(db)
	devRepo := repository.NewDeviceRepository(db)
	ruleRepo := repository.NewRuleRepository(db)
	ctrl := NewControlService(devRepo, logger, hub)
	svc := NewRuleService(ruleRepo, sensorRepo, devRepo, ctrl, logger, hub)
	return ruleFixture{db: db, svc: svc, ctrl: ctrl, device: device, sensor: sensor, devRepo: devRepo, ruleRepo: ruleRepo}
}

func (f *ruleFixture) reloadDevice(t *testing.T) *model.Device {
	t.Helper()
	d, err := f.devRepo.Get(f.device.ID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *ruleFixture) actionCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := f.db.Model(&model.DeviceAction{}).Where("device_id = ? AND operator = ?", f.device.ID, constants.OperatorAutomation).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *ruleFixture) getRule(t *testing.T, id uint) *model.AutomationRule {
	t.Helper()
	r, err := f.ruleRepo.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRuleStateMachine(t *testing.T) {
	tests := []struct {
		name         string
		side         string
		action       string
		enabled      bool
		readings     []float64
		wantStatuses []string
		wantActions  int64
		wantState    string
		wantResult   string
	}{
		{
			name:         "超上限触发一次，持续异常不重复动作，恢复后执行相反动作",
			side:         constants.TriggerSideHigh,
			action:       constants.StatusOn,
			enabled:      true,
			readings:     []float64{35, 36, 37, 25},
			wantStatuses: []string{constants.StatusOff, constants.StatusOn, constants.StatusOn, constants.StatusOn, constants.StatusOff},
			wantActions:  2,
			wantState:    constants.RuleStateNormal,
			wantResult:   constants.RuleResultSuccess,
		},
		{
			name:         "低于下限触发开启，恢复后关闭",
			side:         constants.TriggerSideLow,
			action:       constants.StatusOn,
			enabled:      true,
			readings:     []float64{10, 20},
			wantStatuses: []string{constants.StatusOff, constants.StatusOn, constants.StatusOff},
			wantActions:  2,
			wantState:    constants.RuleStateNormal,
			wantResult:   constants.RuleResultSuccess,
		},
		{
			name:         "任一侧越限均触发，来回越限分别处理并恢复",
			side:         constants.TriggerSideBoth,
			action:       constants.StatusOn,
			enabled:      true,
			readings:     []float64{10, 20, 35, 25},
			wantStatuses: []string{constants.StatusOff, constants.StatusOn, constants.StatusOff, constants.StatusOn, constants.StatusOff},
			wantActions:  4,
			wantState:    constants.RuleStateNormal,
			wantResult:   constants.RuleResultSuccess,
		},
		{
			name:         "读数一直在正常范围内不动作",
			side:         constants.TriggerSideHigh,
			action:       constants.StatusOn,
			enabled:      true,
			readings:     []float64{20, 22, 24},
			wantStatuses: []string{constants.StatusOff, constants.StatusOff, constants.StatusOff, constants.StatusOff},
			wantActions:  0,
			wantState:    constants.RuleStateNormal,
			wantResult:   "",
		},
		{
			name:         "停用的规则异常和恢复都不动设备",
			side:         constants.TriggerSideHigh,
			action:       constants.StatusOn,
			enabled:      false,
			readings:     []float64{35, 25},
			wantStatuses: []string{constants.StatusOff, constants.StatusOff, constants.StatusOff},
			wantActions:  0,
			wantState:    constants.RuleStateNormal,
			wantResult:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRuleFixture(t)
			rule, err := f.svc.Create(CreateRuleInput{
				Name:          "温度联动风机",
				SensorID:      f.sensor.ID,
				DeviceID:      f.device.ID,
				TriggerSide:   tt.side,
				TriggerAction: tt.action,
				Enabled:       tt.enabled,
			})
			if err != nil {
				t.Fatal(err)
			}
			sensor, err := repository.NewSensorRepository(f.db).Get(f.sensor.ID)
			if err != nil {
				t.Fatal(err)
			}
			for i, value := range tt.readings {
				f.svc.EvaluateReading(sensor, value)
				if got := f.reloadDevice(t).Status; got != tt.wantStatuses[i+1] {
					t.Fatalf("reading %v: want device %s, got %s", value, tt.wantStatuses[i+1], got)
				}
			}
			if got := f.actionCount(t); got != tt.wantActions {
				t.Fatalf("want %d automation actions, got %d", tt.wantActions, got)
			}
			if got := f.getRule(t, rule.ID); got.State != tt.wantState || got.LastResult != tt.wantResult {
				t.Fatalf("want state=%s result=%q, got state=%s result=%q", tt.wantState, tt.wantResult, got.State, got.LastResult)
			}
		})
	}
}

func TestRuleAlreadyInTargetStateSkipsDeviceWrite(t *testing.T) {
	f := newRuleFixture(t)
	if err := f.db.Model(&model.Device{}).Where("id = ?", f.device.ID).Update("status", constants.StatusOn).Error; err != nil {
		t.Fatal(err)
	}
	rule, err := f.svc.Create(CreateRuleInput{
		Name: "温度联动风机", SensorID: f.sensor.ID, DeviceID: f.device.ID,
		TriggerSide: constants.TriggerSideHigh, TriggerAction: constants.StatusOn, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sensor, _ := repository.NewSensorRepository(f.db).Get(f.sensor.ID)
	f.svc.EvaluateReading(sensor, 40)
	if got := f.actionCount(t); got != 0 {
		t.Fatalf("want no action row when device already on, got %d", got)
	}
	got := f.getRule(t, rule.ID)
	if got.State != constants.RuleStateAbnormal || got.LastResult != constants.RuleResultSkipped {
		t.Fatalf("want abnormal/skipped, got %s/%s", got.State, got.LastResult)
	}
	// 恢复后仍应执行相反动作（关闭）。
	f.svc.EvaluateReading(sensor, 25)
	if f.reloadDevice(t).Status != constants.StatusOff {
		t.Fatal("expected recovery to turn the device off")
	}
}

func TestRuleIncompleteConfigurationDoesNotTouchDevice(t *testing.T) {
	f := newRuleFixture(t)
	// 规则关联了正确的传感器/设备，但触发动作缺失，属于运行期不完整配置。
	rule := &model.AutomationRule{
		GreenhouseID: f.sensor.GreenhouseID, Name: "残缺规则", SensorID: f.sensor.ID, DeviceID: f.device.ID,
		TriggerSide: constants.TriggerSideHigh, TriggerAction: "", RecoveryAction: "",
		Enabled: true, State: constants.RuleStateNormal,
	}
	if err := f.ruleRepo.Create(rule); err != nil {
		t.Fatal(err)
	}
	sensor, _ := repository.NewSensorRepository(f.db).Get(f.sensor.ID)
	f.svc.EvaluateReading(sensor, 40)
	if f.actionCount(t) != 0 {
		t.Fatal("incomplete rule must not operate the device")
	}
	got := f.getRule(t, rule.ID)
	if got.State != constants.RuleStateNormal {
		t.Fatalf("incomplete rule must not advance state, got %s", got.State)
	}
	if got.LastResult != constants.RuleResultSkipped {
		t.Fatalf("want skipped result, got %s", got.LastResult)
	}
}

func TestRuleCreateRejectsCrossGreenhouseResources(t *testing.T) {
	f := newRuleFixture(t)
	other := model.Greenhouse{Name: "另一座温室", Location: "西区", Area: 10}
	if err := f.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	foreign := model.Device{GreenhouseID: other.ID, Name: "外来泵", Type: "pump", Status: constants.StatusOff}
	if err := f.db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.Create(CreateRuleInput{
		Name: "跨温室规则", SensorID: f.sensor.ID, DeviceID: foreign.ID,
		TriggerSide: constants.TriggerSideHigh, TriggerAction: constants.StatusOn, Enabled: true,
	})
	if err == nil {
		t.Fatal("expected cross-greenhouse rule to be rejected")
	}
}
