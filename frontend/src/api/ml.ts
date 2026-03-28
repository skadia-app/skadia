import { apiClient } from './client'

// ─── ML Status ───────────────────────────────────────────────────────────────

export interface MLStatus {
  available: boolean
}

export async function getMLStatus(): Promise<MLStatus> {
  return apiClient<MLStatus>('/ml/status')
}

// ─── Nightly Jobs (admin) ─────────────────────────────────────────────────────

export interface NightlyJobSettings {
  enabled: boolean
  time: string
  description: string
}

export type NightlyJobKey =
  | 'nightly_feature_engineering'
  | 'nightly_tsfm_analysis'
  | 'nightly_changepoint_detection'

export type NightlyJobsSettings = Record<NightlyJobKey, NightlyJobSettings>

export async function getMLSystemSettings(): Promise<NightlyJobsSettings> {
  return apiClient<NightlyJobsSettings>('/admin/settings/ml')
}

export async function updateNightlyJob(
  key: NightlyJobKey,
  settings: NightlyJobSettings,
): Promise<void> {
  await apiClient<void>(`/admin/settings/ml/${key}`, {
    method: 'PUT',
    body: settings,
  })
}

// ─── Notification / User Settings ────────────────────────────────────────────

export type NotifyKey =
  | 'notify_enrichment'
  | 'notify_anomaly'
  | 'notify_gear'
  | 'notify_chat'

export type UserMLSettings = Record<NotifyKey, boolean>

export async function getUserMLSettings(): Promise<UserMLSettings> {
  return apiClient<UserMLSettings>('/settings/ml/user')
}

export async function updateUserMLSetting(key: NotifyKey, value: boolean): Promise<void> {
  await apiClient<void>(`/settings/ml/user/${key}`, {
    method: 'PUT',
    body: value,
  })
}

// ─── Devices ─────────────────────────────────────────────────────────────────

export interface Device {
  id: string
  platform: string
  device_name?: string | null
  created_at: string
}

export async function getDevices(): Promise<Device[]> {
  return apiClient<Device[]>('/devices')
}

export async function removeDevice(id: string): Promise<void> {
  await apiClient<void>(`/devices/${id}`, { method: 'DELETE' })
}
