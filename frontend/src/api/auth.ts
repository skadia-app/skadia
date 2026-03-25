import { apiFetch } from '@/lib/auth'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

export interface AuthConfig {
  registration_mode: 'invite' | 'open' | 'approval'
  social_providers: string[]
}

export async function getAuthConfig(): Promise<AuthConfig> {
  const res = await fetch(`${API_BASE}/auth/config`)
  if (!res.ok) throw new Error('Failed to fetch auth config')
  return res.json() as Promise<AuthConfig>
}

export async function exchangeAuthCode(code: string): Promise<{ token: string }> {
  const res = await fetch(`${API_BASE}/auth/exchange-code`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include', // for refresh token cookie
    body: JSON.stringify({ code }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Exchange failed')
  }
  return res.json() as Promise<{ token: string }>
}

export async function loginWithEmail(email: string, password: string): Promise<{ token: string }> {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Login failed')
  }
  return res.json() as Promise<{ token: string }>
}

export async function register(email: string, username: string, password: string): Promise<{ status: string }> {
  const res = await fetch(`${API_BASE}/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, username, password }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Registration failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function forgotPassword(email: string): Promise<void> {
  const res = await fetch(`${API_BASE}/auth/forgot-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Request failed')
  }
}

export async function resetPassword(token: string, newPassword: string): Promise<void> {
  const res = await fetch(`${API_BASE}/auth/reset-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, new_password: newPassword }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Reset failed')
  }
}

export async function verifyEmail(token: string): Promise<void> {
  const res = await fetch(`${API_BASE}/auth/verify-email`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Verification failed')
  }
}

export async function logout(): Promise<void> {
  await fetch(`${API_BASE}/auth/logout`, {
    method: 'POST',
    credentials: 'include',
  })
}

export interface MeData {
  user_id: string
  username: string
  display_name?: string
  email?: string
  is_admin: boolean
  email_verified: boolean
  has_password: boolean
  must_change_password: boolean
  registration_source?: string
}

export async function getMe(): Promise<MeData> {
  const res = await apiFetch('/api/v1/me')
  if (!res.ok) throw new Error('Failed to fetch user')
  return res.json() as Promise<MeData>
}

export async function resendVerification(): Promise<{ status: string }> {
  const res = await apiFetch('/api/v1/auth/resend-verification', { method: 'POST' })
  return res.json() as Promise<{ status: string }>
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  const res = await apiFetch(`${API_BASE}/auth/change-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Password change failed')
  }
}

export interface AuditLogEntry {
  id: number
  user_id: string | null
  username: string | null
  event_type: string
  ip_address: string | null
  user_agent: string | null
  details: Record<string, unknown> | null
  created_at: string
}

export interface AuditLogResponse {
  entries: AuditLogEntry[]
  total: number
  limit: number
  offset: number
}

export async function getAuditLog(params: {
  event_type?: string
  user_id?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}): Promise<AuditLogResponse> {
  const sp = new URLSearchParams()
  if (params.event_type) sp.set('event_type', params.event_type)
  if (params.user_id) sp.set('user_id', params.user_id)
  if (params.from) sp.set('from', params.from)
  if (params.to) sp.set('to', params.to)
  sp.set('limit', String(params.limit ?? 50))
  sp.set('offset', String(params.offset ?? 0))
  const res = await apiFetch(`/api/v1/admin/audit-log?${sp}`)
  return res.json() as Promise<AuditLogResponse>
}
