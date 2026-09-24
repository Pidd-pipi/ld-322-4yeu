package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cygreenenv/greenhouse-panel/internal/constants"
	"github.com/cygreenenv/greenhouse-panel/internal/model"
	"github.com/cygreenenv/greenhouse-panel/internal/repository"
	"github.com/cygreenenv/greenhouse-panel/internal/service"
	ws "github.com/cygreenenv/greenhouse-panel/internal/websocket"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var routerDBSuffix atomic.Uint64

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type testRequest func(method, path string, body any) (apiEnvelope, int)

func buildTestServer(t *testing.T) (*gin.Engine, string, testRequest) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:router_rule_%d?mode=memory&cache=shared", routerDBSuffix.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(model.All()...); err != nil {
		t.Fatal(err)
	}
	g := model.Greenhouse{Name: "联动联调温室", Location: "测试区", Area: 100}
	if err = db.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	sensor := model.Sensor{GreenhouseID: g.ID, Name: "温度传感器", Type: "temperature", Unit: "°C", Status: "online"}
	if err = db.Create(&sensor).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.Threshold{SensorID: sensor.ID, MinValue: 15, MaxValue: 30}).Error; err != nil {
		t.Fatal(err)
	}
	device := model.Device{GreenhouseID: g.ID, Name: "循环风机", Type: "fan", Status: "off"}
	if err = db.Create(&device).Error; err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := ws.NewHub()
	greenhouseRepo := repository.NewGreenhouseRepository(db)
	sensorRepo := repository.NewSensorRepository(db)
	alertRepo := repository.NewAlertRepository(db)
	deviceRepo := repository.NewDeviceRepository(db)
	ruleRepo := repository.NewRuleRepository(db)
	authSvc := service.NewAuthService("test-secret")
	control := service.NewControlService(deviceRepo, logger, hub)
	ruleSvc := service.NewRuleService(ruleRepo, sensorRepo, deviceRepo, control, logger, hub)
	monitoring := service.NewMonitoringService(greenhouseRepo, sensorRepo, alertRepo, ruleSvc, logger, hub)
	engine := New(Dependencies{Logger: logger, Auth: authSvc, Monitoring: monitoring, Alerts: service.NewAlertService(alertRepo, logger), Control: control, Rules: ruleSvc, Reports: service.NewReportService(sensorRepo, alertRepo), Hub: hub})

	token, err := authSvc.Login("admin", "admin123")
	if err != nil {
		t.Fatal(err)
	}
	doJSON := func(method, path string, body any) (apiEnvelope, int) {
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		var env apiEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return env, rec.Code
	}
	return engine, sensorAndDevicePath(sensor.ID, device.ID, g.ID), doJSON
}

func sensorAndDevicePath(sensorID, deviceID, greenhouseID uint) string {
	return fmt.Sprintf("s=%d&d=%d&g=%d", sensorID, deviceID, greenhouseID)
}

func TestAutomationRuleHTTPLifecycle(t *testing.T) {
	_, ids, doJSON := buildTestServer(t)
	var sensorID, deviceID, greenhouseID uint
	if _, err := fmt.Sscanf(ids, "s=%d&d=%d&g=%d", &sensorID, &deviceID, &greenhouseID); err != nil {
		t.Fatal(err)
	}

	// 越上限 -> 开风机；恢复 -> 关风机。
	body := map[string]any{
		"name": "温度超限开风机", "sensorId": sensorID, "deviceId": deviceID,
		"triggerSide": "high", "triggerAction": "on", "enabled": true,
	}
	env, status := doJSON(http.MethodPost, "/api/v1/automation-rules", body)
	if status != http.StatusCreated || env.Code != 0 {
		t.Fatalf("create rule: status=%d env=%s", status, env.Message)
	}
	var rule model.AutomationRule
	if err := json.Unmarshal(env.Data, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.RecoveryAction != constants.StatusOff {
		t.Fatalf("recovery action should derive to off, got %s", rule.RecoveryAction)
	}

	// 写入越限读数，应自动开启设备。
	if _, code := doJSON(http.MethodPost, "/api/v1/readings", map[string]any{"sensorId": sensorID, "value": 36}); code != http.StatusCreated {
		t.Fatalf("ingest high reading: %d", code)
	}
	env, code := doJSON(http.MethodGet, fmt.Sprintf("/api/v1/devices?greenhouse_id=%d", greenhouseID), nil)
	if code != http.StatusOK {
		t.Fatalf("list devices: %d", code)
	}
	var devices []model.Device
	_ = json.Unmarshal(env.Data, &devices)
	if len(devices) != 1 || devices[0].Status != constants.StatusOn {
		t.Fatalf("device should be on after trigger, got %+v", devices)
	}

	// 再次越限读数，不应重复写操作记录（一次异常只处理一次）。
	if _, code = doJSON(http.MethodPost, "/api/v1/readings", map[string]any{"sensorId": sensorID, "value": 38}); code != http.StatusCreated {
		t.Fatalf("ingest second high reading: %d", code)
	}

	// 恢复读数，应自动关闭设备。
	if _, code = doJSON(http.MethodPost, "/api/v1/readings", map[string]any{"sensorId": sensorID, "value": 22}); code != http.StatusCreated {
		t.Fatalf("ingest recovery reading: %d", code)
	}
	env, _ = doJSON(http.MethodGet, fmt.Sprintf("/api/v1/devices?greenhouse_id=%d", greenhouseID), nil)
	_ = json.Unmarshal(env.Data, &devices)
	if devices[0].Status != constants.StatusOff {
		t.Fatalf("device should be off after recovery, got %s", devices[0].Status)
	}

	// 列表应带回最近一次执行结果。
	env, _ = doJSON(http.MethodGet, fmt.Sprintf("/api/v1/automation-rules?greenhouse_id=%d", greenhouseID), nil)
	var rules []model.AutomationRule
	_ = json.Unmarshal(env.Data, &rules)
	if len(rules) != 1 || rules[0].LastResult != constants.RuleResultSuccess || rules[0].State != constants.RuleStateNormal {
		t.Fatalf("unexpected rule row: %+v", rules)
	}

	// 停用后再越限，设备不应变化。
	if _, code = doJSON(http.MethodPatch, fmt.Sprintf("/api/v1/automation-rules/%d/status", rule.ID), map[string]any{"enabled": false}); code != http.StatusOK {
		t.Fatalf("disable rule: %d", code)
	}
	if _, code = doJSON(http.MethodPost, "/api/v1/readings", map[string]any{"sensorId": sensorID, "value": 40}); code != http.StatusCreated {
		t.Fatalf("ingest while disabled: %d", code)
	}
	env, _ = doJSON(http.MethodGet, fmt.Sprintf("/api/v1/devices?greenhouse_id=%d", greenhouseID), nil)
	_ = json.Unmarshal(env.Data, &devices)
	if devices[0].Status != constants.StatusOff {
		t.Fatalf("disabled rule must not touch device, got %s", devices[0].Status)
	}

	// 移除规则。
	if _, code = doJSON(http.MethodDelete, fmt.Sprintf("/api/v1/automation-rules/%d", rule.ID), nil); code != http.StatusOK {
		t.Fatalf("delete rule: %d", code)
	}
	env, _ = doJSON(http.MethodGet, fmt.Sprintf("/api/v1/automation-rules?greenhouse_id=%d", greenhouseID), nil)
	_ = json.Unmarshal(env.Data, &rules)
	if len(rules) != 0 {
		t.Fatalf("rule should be removed, got %d", len(rules))
	}
}

func TestCreateRuleRejectsInvalidPayload(t *testing.T) {
	_, ids, doJSON := buildTestServer(t)
	var sensorID, deviceID uint
	if _, err := fmt.Sscanf(ids, "s=%d&d=%d&g=%d", &sensorID, &deviceID, new(uint)); err != nil {
		t.Fatal(err)
	}
	// 缺少 triggerSide，应被 DTO 校验拒绝且不动设备。
	env, status := doJSON(http.MethodPost, "/api/v1/automation-rules", map[string]any{
		"name": "ab", "sensorId": sensorID, "deviceId": deviceID, "triggerAction": "on",
	})
	if status != http.StatusBadRequest || env.Code == 0 {
		t.Fatalf("expected validation error, status=%d", status)
	}
}
