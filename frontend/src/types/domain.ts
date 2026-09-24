export type SensorType = 'temperature' | 'humidity' | 'light' | 'co2' | 'soil_moisture';
export interface Threshold { id: number; sensorId: number; minValue: number; maxValue: number; }
export interface Sensor { id: number; greenhouseId: number; name: string; type: SensorType; unit: string; status: string; threshold?: Threshold; }
export interface Greenhouse { id: number; name: string; location: string; area: number; sensors: Sensor[]; devices: Device[]; }
export interface Reading { id: number; sensorId: number; value: number; recordedAt: string; sensor: Sensor; }
export interface Alert { id: number; greenhouseId: number; sensorId: number; level: string; message: string; value: number; status: string; createdAt: string; sensor?: Sensor; }
export interface Device { id: number; greenhouseId: number; name: string; type: string; status: 'on' | 'off'; updatedAt: string; }
export type RuleTriggerSide = 'high' | 'low' | 'both';
export type RuleResult = 'success' | 'skipped' | 'failed' | '';
export interface AutomationRule {
  id: number;
  greenhouseId: number;
  name: string;
  sensorId: number;
  deviceId: number;
  triggerSide: RuleTriggerSide;
  triggerAction: 'on' | 'off';
  recoveryAction: 'on' | 'off';
  enabled: boolean;
  state: 'normal' | 'abnormal';
  lastResult: RuleResult;
  lastMessage: string;
  lastTriggeredAt?: string;
  createdAt: string;
  updatedAt: string;
  sensor?: Sensor;
  device?: Device;
}
export interface EnvironmentReport { greenhouseId: number; range: string; generatedAt: string; alerts: number; metrics: Record<string, { average: number; min: number; max: number; unit: string }> }
export interface ApiResponse<T> { code: number; message: string; data: T; }
