// JWT auth state management
// Token is kept in memory only (not localStorage) for security

let accessToken: string | null = null
let refreshTimer: ReturnType<typeof setTimeout> | null = null
let refreshInFlight: Promise<boolean> | null = null

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'
const DEV_AUTH = import.meta.env.VITE_DEV_AUTH === 'true'

// Dev mock token (valid-looking JWT structure, not a real secret)
const DEV_TOKEN = 'eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJkZXYtdXNlciIsImV4cCI6OTk5OTk5OTk5OX0.dev'

export function getToken(): string | null {
  if (DEV_AUTH && accessToken === null) {
    accessToken = DEV_TOKEN
  }
  return accessToken
}

export function setToken(token: string): void {
  accessToken = token
  scheduleRefresh(token)
}

export function clearToken(): void {
  accessToken = null
  if (refreshTimer) {
    clearTimeout(refreshTimer)
    refreshTimer = null
  }
}

export function isAuthenticated(): boolean {
  return getToken() !== null
}

export function isDevAuth(): boolean {
  return DEV_AUTH
}

/** Parse JWT payload without verification (for expiry check) */
function parseJwtPayload(token: string): { exp?: number; sub?: string } | null {
  try {
    const parts = token.split('.')
    if (parts.length !== 3) return null
    const payload = atob(parts[1].replace(/-/g, '+').replace(/_/g, '/'))
    return JSON.parse(payload) as { exp?: number; sub?: string }
  } catch {
    return null
  }
}

/** Schedule a silent token refresh 60s before expiry */
function scheduleRefresh(token: string): void {
  if (refreshTimer) clearTimeout(refreshTimer)
  if (DEV_AUTH) return

  const payload = parseJwtPayload(token)
  if (!payload?.exp) return

  const nowSec = Math.floor(Date.now() / 1000)
  const refreshInMs = (payload.exp - nowSec - 60) * 1000

  if (refreshInMs <= 0) {
    void refreshToken()
    return
  }

  refreshTimer = setTimeout(() => {
    void refreshToken()
  }, refreshInMs)
}

/** Refresh the access token via HttpOnly cookie */
export async function refreshToken(): Promise<boolean> {
  if (DEV_AUTH) return true

  // Deduplicate concurrent refresh calls — only one in-flight at a time
  if (refreshInFlight) return refreshInFlight

  refreshInFlight = (async () => {
    try {
      const res = await fetch(`${API_BASE}/auth/refresh`, {
        method: 'POST',
        credentials: 'include',
      })
      if (!res.ok) {
        clearToken()
        return false
      }
      const data = (await res.json()) as { token: string }
      setToken(data.token)
      return true
    } catch {
      clearToken()
      return false
    } finally {
      refreshInFlight = null
    }
  })()

  return refreshInFlight
}

/** Try to restore session on app start (silent refresh) */
export async function tryRestoreSession(): Promise<boolean> {
  if (DEV_AUTH) {
    accessToken = DEV_TOKEN
    return true
  }

  return refreshToken()
}

/** Authenticated fetch wrapper — adds Authorization header automatically */
export function apiFetch(input: string, init?: RequestInit): Promise<Response> {
  const token = getToken()
  const headers = new Headers(init?.headers)
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  return fetch(input, { ...init, headers })
}
