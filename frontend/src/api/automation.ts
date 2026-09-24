import client from './client';
import type { ApiResponse, AutomationRule, RuleTriggerSide } from '../types/domain';

export interface RuleCreatePayload {
  name: string;
  sensorId: number;
  deviceId: number;
  triggerSide: RuleTriggerSide;
  triggerAction: 'on' | 'off';
  enabled: boolean;
}

const base = '/automation-rules';

export const getRules = async (greenhouseId: number) =>
  (await client.get<ApiResponse<AutomationRule[]>>(base, { params: { greenhouse_id: greenhouseId } })).data.data;

export const createRule = async (payload: RuleCreatePayload) =>
  (await client.post<ApiResponse<AutomationRule>>(base, payload)).data.data;

export const setRuleEnabled = async (id: number, enabled: boolean) =>
  (await client.patch<ApiResponse<AutomationRule>>(`${base}/${id}/status`, { enabled })).data.data;

export const deleteRule = async (id: number) =>
  (await client.delete<ApiResponse<{ deleted: number }>>(`${base}/${id}`)).data.data;
