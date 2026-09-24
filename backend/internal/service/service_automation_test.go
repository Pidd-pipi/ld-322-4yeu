package service

import (
	"github.com/cygreenenv/greenhouse-panel/internal/constants"
	"github.com/cygreenenv/greenhouse-panel/internal/model"
	"github.com/cygreenenv/greenhouse-panel/internal/repository"
	ws "github.com/cygreenenv/greenhouse-panel/internal/websocket"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"testing"
)

type automationFixture struct {
	db     *gorm.DB
	sensor *model.Sensor
	device *model.Device
}

func newAutomationFixture(t *testing.T, dsn string) automationFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(model.All()...); err != nil {
		t.Fatal(err)
	}
	g := model.Greenhouse{Name: "联动温室"}
	if err = db.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	sensor := model.Sensor{GreenhouseID: g.ID, Name: "温度", Type: "temperature", Unit: "°C", Status: constants.StatusOnline}
	if err = db.Create(&sensor).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.Threshold{SensorID: sensor.ID, MinValue: 10, MaxValue: 30}).Error; err != nil {
		t.Fatal(err)
	}
	device := model.Device{GreenhouseID: g.ID, Name: "风机", Type: "fan", Status: constants.StatusOff}
	if err = db.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	return automationFixture{db: db, sensor: &sensor, device: &device}
}

func newAutomationService(db *gorm.DB) *AutomationService {
	return NewAutomationService(
		db,
		repository.NewGreenhouseRepository(db),
		repository.NewAutomationRuleRepository(db),
		repository.NewSensorRepository(db),
		repository.NewDeviceRepository(db),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		ws.NewHub(),
	)
}

func TestAutomationRuleTriggersOnceThenRecovers(t *testing.T) {
	fixture := newAutomationFixture(t, "file:automation_lifecycle_test?mode=memory&cache=shared")
	svc := newAutomationService(fixture.db)
	rule := &model.AutomationRule{
		Name: "高温开风机", GreenhouseID: fixture.sensor.GreenhouseID,
		SensorID: fixture.sensor.ID, DeviceID: fixture.device.ID,
		AbnormalAction: constants.StatusOn, NormalAction: constants.StatusOff, Enabled: true,
	}
	if err := svc.Create(rule); err != nil {
		t.Fatal(err)
	}

	svc.Evaluate(fixture.sensor.ID, 35)
	deviceRepo := repository.NewDeviceRepository(fixture.db)
	device, err := deviceRepo.Get(fixture.device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if device.Status != constants.StatusOn {
		t.Fatalf("want device on after abnormal reading, got %q", device.Status)
	}

	svc.Evaluate(fixture.sensor.ID, 36)
	var actionCount int64
	if err = fixture.db.Model(&model.DeviceAction{}).Where("device_id = ? AND operator = ?", fixture.device.ID, constants.OperatorAutomation).Count(&actionCount).Error; err != nil {
		t.Fatal(err)
	}
	if actionCount != 1 {
		t.Fatalf("want exactly one abnormal action, got %d", actionCount)
	}

	svc.Evaluate(fixture.sensor.ID, 25)
	device, err = deviceRepo.Get(fixture.device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if device.Status != constants.StatusOff {
		t.Fatalf("want device off after recovery, got %q", device.Status)
	}
	stored, err := repository.NewAutomationRuleRepository(fixture.db).Get(rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastState != constants.RuleStateNormal || stored.LastExecutionStatus != constants.RuleResultSuccess {
		t.Fatalf("unexpected stored rule after recovery: %#v", stored)
	}
}

func TestAutomationSkipsDisabledAndIncompleteRules(t *testing.T) {
	fixture := newAutomationFixture(t, "file:automation_skip_test?mode=memory&cache=shared")
	disabled := model.AutomationRule{
		Name: "停用规则", GreenhouseID: fixture.sensor.GreenhouseID, SensorID: fixture.sensor.ID,
		DeviceID: fixture.device.ID, AbnormalAction: constants.StatusOn, NormalAction: constants.StatusOff,
		Enabled: false, LastState: constants.RuleStateNormal,
	}
	incomplete := model.AutomationRule{
		Name: "不完整规则", GreenhouseID: fixture.sensor.GreenhouseID, SensorID: fixture.sensor.ID,
		DeviceID: fixture.device.ID, AbnormalAction: constants.StatusOn, NormalAction: constants.StatusOn,
		Enabled: true, LastState: constants.RuleStateNormal,
	}
	if err := fixture.db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&incomplete).Error; err != nil {
		t.Fatal(err)
	}

	newAutomationService(fixture.db).Evaluate(fixture.sensor.ID, 35)
	device, err := repository.NewDeviceRepository(fixture.db).Get(fixture.device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if device.Status != constants.StatusOff {
		t.Fatalf("disabled or incomplete rules must not change device, got %q", device.Status)
	}
}
