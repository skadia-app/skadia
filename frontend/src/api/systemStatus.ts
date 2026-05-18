import { apiClient, apiDownload } from './client'

export type SystemStatusState =
  | 'ok'
  | 'degraded'
  | 'unavailable'
  | 'disabled'
  | 'not_configured'
  | 'missing_artifacts'
  | 'blocked'
  | 'unknown'

export interface ComponentStatus {
  key: string
  label: string
  state: SystemStatusState
  reason_code?: string
  summary?: string
  checked_at: string
  latency_ms?: number
  details?: Record<string, string>
}

export interface BuildInfo {
  service: string
  git_commit: string
  commit_time: string
  build_time: string
  image_digest?: string
  config_version?: string
  started_at?: string
  uptime_seconds?: number
}

export interface SystemStatusResponse {
  schema_version: number
  generated_at: string
  overall: SystemStatusState
  build: Record<string, BuildInfo>
  components: Record<string, ComponentStatus>
  feature_matrix?: Record<string, ComponentStatus>
}

export function fetchSystemStatus(): Promise<SystemStatusResponse> {
  return apiClient<SystemStatusResponse>('/admin/system-status')
}

export function downloadSystemStatusExport(): Promise<{ blob: Blob; filename?: string }> {
  return apiDownload('/admin/system-status/export')
}
