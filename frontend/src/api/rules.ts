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

export type ApplyResult = {
  status?: string
  job_id: string | null
  applied?: number
  affected?: number
}

export async function applyRules(): Promise<ApplyResult> {
  return apiClient('/automation-rules/apply', { method: 'POST' })
}

export interface DryRunRuleMatch {
  rule_name: string
  action: string
}

export interface DryRunAllMatch {
  activity_id: string
  activity_title: string
  rules: DryRunRuleMatch[]
}

export interface DryRunAllResult {
  matches: DryRunAllMatch[]
  total_matches: number
  truncated: boolean
}

export interface DryRunSingleMatch {
  activity_id: string
  activity_title: string
}

export interface DryRunSingleResult {
  matches: DryRunSingleMatch[]
  total_matches: number
  truncated: boolean
}

export async function dryRunAllRules(): Promise<DryRunAllResult> {
  return apiClient<DryRunAllResult>('/automation-rules/apply?dry_run=true', { method: 'POST' })
}

export async function dryRunSingleRule(ruleId: string): Promise<DryRunSingleResult> {
  return apiClient<DryRunSingleResult>(`/automation-rules/${ruleId}/apply?dry_run=true`, { method: 'POST' })
}

export async function applySingleRule(ruleId: string): Promise<{ job_id: string }> {
  return apiClient<{ job_id: string }>(`/automation-rules/${ruleId}/apply`, { method: 'POST' })
}

export interface CommuteRuleResult {
  rule_id: string
  created: boolean
  message: string
}

export async function createCommuteRule(
  activityId: string,
  dest?: { lat: number; lon: number } | { zoneId: string },
): Promise<CommuteRuleResult> {
  const body: Record<string, unknown> = { activity_id: activityId }
  if (dest && 'zoneId' in dest) {
    body.dest_zone_id = dest.zoneId
  } else if (dest && 'lat' in dest) {
    body.dest_lat = dest.lat
    body.dest_lon = dest.lon
  }
  return apiClient<CommuteRuleResult>('/automation-rules/commute', {
    method: 'POST',
    body,
  })
}

export async function createCommuteRuleFromZones(startZoneId: string, endZoneId: string): Promise<CommuteRuleResult> {
  return apiClient<CommuteRuleResult>('/automation-rules/commute', {
    method: 'POST',
    body: { start_zone_id: startZoneId, end_zone_id: endZoneId },
  })
}

