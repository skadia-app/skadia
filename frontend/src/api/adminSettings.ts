import { apiFetch } from '@/lib/auth'

export interface SettingField {
  value: string
  source: 'env' | 'db' | 'default'
  locked?: boolean
  has_value?: boolean
}

export interface SettingsResponseMeta {
  storage_backend?: string
  oauth_encryption_configured?: string
}

export type SettingsResponse = Record<string, Record<string, SettingField>> & {
  _meta?: SettingsResponseMeta
}

export interface EmailTemplate {
  key: string
  subject: string
  html: string
  text: string
  enabled: boolean
  source: 'default' | 'db'
  updated_at?: string
}

export interface RenderedEmailTemplate {
  subject: string
  html: string
  text: string
}

export type EmailTemplateDraft = Pick<EmailTemplate, 'subject' | 'html' | 'text' | 'enabled'>

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

export async function getEmailTemplates(): Promise<EmailTemplate[]> {
  const res = await apiFetch('/api/v1/admin/email/templates')
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to fetch email templates')
  }
  const data = await res.json() as { templates: EmailTemplate[] }
  return data.templates
}

export async function saveEmailTemplate(key: string, template: EmailTemplateDraft): Promise<void> {
  const res = await apiFetch(`/api/v1/admin/email/templates/${key}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(template),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to save email template')
  }
}

export async function previewEmailTemplate(key: string, draft?: EmailTemplateDraft): Promise<RenderedEmailTemplate> {
  const res = await apiFetch(`/api/v1/admin/email/templates/${key}/preview`, {
    method: 'POST',
    headers: draft ? { 'Content-Type': 'application/json' } : undefined,
    body: draft ? JSON.stringify(draft) : undefined,
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to preview email template')
  }
  return res.json() as Promise<RenderedEmailTemplate>
}

export async function sendEmailTemplateTest(key: string): Promise<void> {
  const res = await apiFetch(`/api/v1/admin/email/templates/${key}/test`, { method: 'POST' })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to send test email')
  }
}

export async function resetEmailTemplate(key: string): Promise<void> {
  const res = await apiFetch(`/api/v1/admin/email/templates/${key}`, { method: 'DELETE' })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to reset email template')
  }
}
