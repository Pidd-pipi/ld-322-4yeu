import client from './client';
import type { ApiResponse, AutomationRule, AutomationRuleRequest } from '../types/domain';

export const getAutomationRules = async (greenhouseId?: number) =>
  (await client.get<ApiResponse<AutomationRule[]>>('/automation-rules', {
    params: greenhouseId ? { greenhouse_id: greenhouseId } : {},
  })).data.data;

export const createAutomationRule = async (payload: AutomationRuleRequest) =>
  (await client.post<ApiResponse<AutomationRule>>('/automation-rules', payload)).data.data;

export const setAutomationRuleEnabled = async (id: number, enabled: boolean) =>
  (await client.patch<ApiResponse<AutomationRule>>(`/automation-rules/${id}/status`, { enabled })).data.data;

export const deleteAutomationRule = async (id: number) =>
  (await client.delete<ApiResponse<{ id: number }>>(`/automation-rules/${id}`)).data.data;
