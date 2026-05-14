import { apiFetch, getToken, refreshToken } from '@/lib/auth'
import { API_BASE } from '@/api/client'

export interface AuthConfig {
  registration_mode: 'invite' | 'open' | 'approval'
  social_providers: string[]
  smtp_configured: boolean
  bootstrap_mode?: boolean
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

export async function resendVerificationEmail(email: string): Promise<{ status: string }> {
  const res = await fetch(`${API_BASE}/auth/resend-verification-email`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Request failed')
  }
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

// ── Admin User Management ───────────────────────────────────────────────────

export interface AdminUser {
  id: string
  username: string
  display_name: string
  email: string
  is_admin: boolean
  is_active: boolean
  email_verified: boolean
  registration_source: string
  last_login_at: string | null
  created_at: string
  deleted_at: string | null
  hotfolder_enabled: boolean
  pending_approval: boolean
}

export interface AdminUsersResponse {
  users: AdminUser[]
  total: number
  limit: number
  offset: number
}

export interface AdminUserSession {
  family_id: string
  user_agent: string
  ip_address: string
  created_at: string
  expires_at: string
  revoked: boolean
}

export async function getAdminUsers(params: {
  search?: string
  status?: string
  limit?: number
  offset?: number
}): Promise<AdminUsersResponse> {
  const sp = new URLSearchParams()
  if (params.search) sp.set('search', params.search)
  sp.set('status', params.status ?? 'all')
  sp.set('limit', String(params.limit ?? 50))
  sp.set('offset', String(params.offset ?? 0))
  const res = await apiFetch(`/api/v1/admin/users?${sp}`)
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to fetch users')
  }
  return res.json() as Promise<AdminUsersResponse>
}

export async function adminActivateUser(id: string): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/admin/users/${id}/activate`, { method: 'POST' })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Activation failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function adminDeactivateUser(id: string): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/admin/users/${id}/deactivate`, { method: 'POST' })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Deactivation failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function adminResetPassword(
  id: string,
  data: { password?: string; send_email?: boolean },
): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/admin/users/${id}/reset-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Password reset failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function adminCreateUser(data: {
  email: string
  username: string
  password?: string
}): Promise<{ status?: string; invite_sent?: boolean; user?: unknown }> {
  const res = await apiFetch('/api/v1/admin/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'User creation failed')
  }
  return res.json() as Promise<{ status?: string; invite_sent?: boolean; user?: unknown }>
}

export async function getAdminUserSessions(id: string): Promise<{ sessions: AdminUserSession[] }> {
  const res = await apiFetch(`/api/v1/admin/users/${id}/sessions`)
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to fetch sessions')
  }
  return res.json() as Promise<{ sessions: AdminUserSession[] }>
}

export async function adminRevokeUserSessions(id: string): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/admin/users/${id}/revoke-sessions`, { method: 'POST' })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Session revocation failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function adminSetUserAdmin(id: string, isAdmin: boolean): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/admin/users/${id}/set-admin`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ is_admin: isAdmin }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Set admin failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function adminDeleteUser(id: string, confirm: string): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/admin/users/${id}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirm }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'User deletion failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function adminSetUserHotfolder(id: string, enabled: boolean): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/admin/users/${id}/hotfolder`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Hotfolder toggle failed')
  }
  return res.json() as Promise<{ status: string }>
}

// ── User Account Management ─────────────────────────────────────────────────

export interface UserSession {
  family_id: string
  user_agent: string
  ip_address: string
  created_at: string
  expires_at: string
  is_current: boolean
}

export async function getSessions(): Promise<{ sessions: UserSession[] }> {
  const res = await apiFetch('/api/v1/auth/sessions')
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Failed to fetch sessions')
  }
  return res.json() as Promise<{ sessions: UserSession[] }>
}

export async function revokeSession(familyId: string): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/auth/sessions/${familyId}/revoke`, { method: 'POST' })
  if (!res.ok) {
    const data = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error ?? 'Session revocation failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function updateMe(data: { username?: string; display_name?: string }): Promise<{ status: string }> {
  const res = await apiFetch('/api/v1/me', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Update failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function changeEmail(data: { new_email: string; password?: string }): Promise<{ status: string }> {
  const res = await apiFetch('/api/v1/auth/change-email', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Email change failed')
  }
  return res.json() as Promise<{ status: string }>
}

export async function deleteAccount(data: { password?: string; confirmation?: string }): Promise<{ status: string }> {
  const res = await apiFetch('/api/v1/auth/account', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Account deletion failed')
  }
  return res.json() as Promise<{ status: string }>
}

export interface DataExportStatus {
  job_id: string
  status: 'queued' | 'running' | 'completed' | 'failed'
  started_at?: string
  updated_at?: string
  expires_at?: string
  sha256?: string
  size_bytes?: number
  download_url?: string
  download_session_url?: string
  manifest_download_session_url?: string
  artifact_format?: 'single_zip' | 'multipart'
  parts?: DataExportPart[]
  unavailable_reason?: 'artifact_missing' | 'artifact_backend_unavailable'
  partial?: boolean
  error?: string
}

export interface DataExportPart {
  name: string
  role?: string
  sha256?: string
  size_bytes?: number
  download_session_url?: string
  web_import_supported?: boolean
}

export interface DataExportDownloadSession {
  url: string
  expires_at: string
  objects?: DataExportDownloadObject[]
}

export interface DataExportDownloadObject {
  name: string
  role?: string
  url: string
  sha256?: string
  size_bytes?: number
}

export async function startDataExport(): Promise<{ job_id: string; status: string }> {
  const res = await apiFetch('/api/v1/auth/data-export', { method: 'POST' })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Export start failed')
  }
  return res.json()
}

export async function getLatestDataExport(): Promise<DataExportStatus | null> {
  const res = await apiFetch('/api/v1/auth/data-export/latest')
  if (res.status === 404) return null
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Latest export lookup failed')
  }
  return res.json()
}

export async function getDataExportStatus(jobId: string): Promise<DataExportStatus> {
  const res = await apiFetch(`/api/v1/auth/data-export/${jobId}`)
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Export status check failed')
  }
  return res.json()
}

export async function createDataExportDownloadSession(
  jobId: string,
  body: { single?: true; part?: string; all?: boolean },
): Promise<DataExportDownloadSession> {
  const res = await apiFetch(`/api/v1/auth/data-export/${jobId}/download-session`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Download session failed')
  }
  return res.json()
}

// ── Data Import ─────────────────────────────────────────────────────────────

export interface DataImportStatus {
  job_id: string
  status: 'uploading' | 'queued' | 'running' | 'completed' | 'failed'
  error?: string
  progress_current?: number
  progress_total?: number
  phase?: number
  phase_name?: string
  phases_total?: number
  message?: string
  entities_imported?: number
  entities_skipped?: number
  entities_failed?: number
  warnings?: string[]
  import_upload?: DataImportUploadStatus
  metadata?: {
    phase: number
    phase_name: string
    message: string
  }
}

export interface DataImportUploadStatus {
  state: string
  expires_at?: string
  expected_bytes?: number
  received_bytes?: number
  last_part_received_at?: string | null
  parts?: DataImportUploadPart[]
}

export interface DataImportUploadPart {
  name: string
  role?: string
  sha256?: string
  size_bytes?: number
  received?: boolean
  legacy_checksum_missing?: boolean
}

export interface MultipartDataImportSession {
  job_id: string
  status: 'uploading'
  expires_at: string
}

export async function startDataImport(file: File): Promise<{ job_id: string; status: string }> {
  const form = new FormData()
  form.append('file', file)
  const res = await apiFetch('/api/v1/auth/data-import', { method: 'POST', body: form })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Import start failed')
  }
  return res.json()
}

export async function createMultipartDataImport(manifest: unknown): Promise<MultipartDataImportSession> {
  const res = await apiFetch('/api/v1/auth/data-import/multipart', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(manifest),
  })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Multipart import session failed')
  }
  return res.json()
}

export async function uploadMultipartDataImportPart(jobId: string, file: File): Promise<{ status: string }> {
  const path = `/api/v1/auth/data-import/${jobId}/parts/${encodeURIComponent(file.name)}`
  let res = await apiFetch(path, {
    method: 'POST',
    body: file,
  })
  if (res.status === 401 && await refreshToken()) {
    const headers = new Headers()
    const token = getToken()
    if (token) headers.set('Authorization', `Bearer ${token}`)
    res = await fetch(path, {
      method: 'POST',
      body: file,
      headers,
      credentials: 'include',
    })
  }
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Multipart import part upload failed')
  }
  return res.json()
}

export async function startMultipartDataImport(jobId: string): Promise<{ job_id: string; status: string }> {
  const res = await apiFetch(`/api/v1/auth/data-import/${jobId}/start`, { method: 'POST' })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Multipart import start failed')
  }
  return res.json()
}

export async function cancelMultipartDataImport(jobId: string): Promise<{ status: string }> {
  const res = await apiFetch(`/api/v1/auth/data-import/${jobId}/cancel`, { method: 'POST' })
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Multipart import cancel failed')
  }
  return res.json()
}

export async function getDataImportStatus(jobId: string): Promise<DataImportStatus> {
  const res = await apiFetch(`/api/v1/auth/data-import/${jobId}`)
  if (!res.ok) {
    const d = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(d.error ?? 'Import status check failed')
  }
  return res.json()
}
