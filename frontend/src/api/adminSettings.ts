import { apiFetch } from '@/lib/auth'

export interface SettingField {
  value: string
  source: 'env' | 'db' | 'default'
  locked?: boolean
}

export interface SettingsResponseMeta {
  storage_backend?: string
}

export type SettingsResponse = Record<string, Record<string, SettingField>> & {
  _meta?: SettingsResponseMeta
}

export async function getAdminSettings(): Promise<SettingsResponse> {
  const res = await apiFetch('/api/v1/admin/settings')
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to fetch admin settings')
  }
  return res.json() as Promise<SettingsResponse>
}

export async function putAdminSettings(
  updates: Record<string, string | null>,
): Promise<{ status: string; changed: string[] }> {
  const res = await apiFetch('/api/v1/admin/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(updates),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to update admin settings')
  }
  return res.json() as Promise<{ status: string; changed: string[] }>
}

export async function sendTestEmail(): Promise<{ status: string }> {
  const res = await apiFetch('/api/v1/admin/settings/test-email', { method: 'POST' })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Test email failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function reloadSettings(): Promise<void> {
  const res = await apiFetch('/api/v1/admin/settings/reload', { method: 'POST' })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Settings reload failed')
  }
}
