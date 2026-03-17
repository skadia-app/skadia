import { apiClient } from './client'
import type { AutomationRule } from './types'

export async function fetchRules(): Promise<AutomationRule[]> {
  return apiClient<AutomationRule[]>('/automation-rules')
}

export async function createRule(data: Partial<AutomationRule>): Promise<AutomationRule> {
  return apiClient<AutomationRule>('/automation-rules', { method: 'POST', body: data })
}

export async function updateRule(id: string, data: Partial<AutomationRule>): Promise<AutomationRule> {
  return apiClient<AutomationRule>(`/automation-rules/${id}`, { method: 'PUT', body: data })
}

export async function deleteRule(id: string): Promise<void> {
  return apiClient(`/automation-rules/${id}`, { method: 'DELETE' })
}

export async function applyRules(): Promise<{ applied: number; errors: number }> {
  return apiClient('/automation-rules/apply', { method: 'POST' })
}

