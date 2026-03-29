// JWT auth state management
// Token is kept in memory only (not localStorage) for security.
// Cross-tab sync via BroadcastChannel ensures all tabs share the same token
// without persisting it to storage.

let accessToken: string | null = null
let refreshTimer: ReturnType<typeof setTimeout> | null = null
let refreshInFlight: Promise<boolean> | null = null
let authChannel: BroadcastChannel | null = null
let suppressBroadcast = false

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'
const DEV_AUTH = import.meta.env.VITE_DEV_AUTH === 'true'

// Dev mock token (valid-looking JWT structure, not a real secret)
const DEV_TOKEN = 'eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJkZXYtdXNlciIsImV4cCI6OTk5OTk5OTk5OX0.dev'

if (DEV_AUTH && !import.meta.env.DEV) {
  console.error(
    '[auth] CRITICAL: VITE_DEV_AUTH is enabled in a non-dev build. ' +
    'This bypasses all authentication. Remove VITE_DEV_AUTH from your production environment.',
  )
}

export function getToken(): string | null {
  if (DEV_AUTH && accessToken === null) {
    accessToken = DEV_TOKEN
  }
  return accessToken
}

export function setToken(token: string): void {
  accessToken = token
  scheduleRefresh(token)
  if (!suppressBroadcast) {
    authChannel?.postMessage({ type: 'token-update', token })
  }
}

export function clearToken(): void {
  accessToken = null
  if (refreshTimer) {
    clearTimeout(refreshTimer)
    refreshTimer = null
  }
  if (!suppressBroadcast) {
    authChannel?.postMessage({ type: 'logout' })
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
    void refreshToken().then(ok => {
      if (!ok) window.location.href = '/login'
    })
    return
  }

  refreshTimer = setTimeout(() => {
    void refreshToken().then(ok => {
      if (!ok) window.location.href = '/login'
    })
  }, refreshInMs)
}

/** Refresh the access token via HttpOnly cookie */
export async function refreshToken(): Promise<boolean> {
  if (DEV_AUTH) return true

  // Deduplicate concurrent refresh calls — only one in-flight at a time
  if (refreshInFlight) return refreshInFlight

  refreshInFlight = (async () => {
    const ctrl = new AbortController()
    const timeoutId = setTimeout(() => ctrl.abort(), 10_000)
    try {
      const res = await fetch(`${API_BASE}/auth/refresh`, {
        method: 'POST',
        credentials: 'include',
        signal: ctrl.signal,
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
      clearTimeout(timeoutId)
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

  // Token may already be available via cross-tab BroadcastChannel sync
  // (initAuthSync requested it from other tabs). Skip the network refresh.
  if (accessToken) return true

  return refreshToken()
}

/** Authenticated fetch wrapper — adds Authorization header, retries once on 401 */
export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
  const token = getToken()
  const headers = new Headers(init?.headers)
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const res = await fetch(input, { ...init, headers, credentials: 'include' })

  // On 401, try refreshing the token.
  // Only retry on safe/idempotent methods to avoid double-submission (e.g. POST).
  if (res.status === 401 && !DEV_AUTH) {
    const method = (init?.method ?? 'GET').toUpperCase()
    const isIdempotent = method === 'GET' || method === 'HEAD' || method === 'PUT' || method === 'DELETE'

    const refreshed = await refreshToken()
    if (refreshed && isIdempotent) {
      const retryToken = getToken()
      const retryHeaders = new Headers(init?.headers)
      if (retryToken) {
        retryHeaders.set('Authorization', `Bearer ${retryToken}`)
      }
      return fetch(input, { ...init, headers: retryHeaders, credentials: 'include' })
    }
    if (!refreshed) {
      // Refresh failed — redirect to login
      clearToken()
      window.location.href = '/login'
    }
    // Non-idempotent: token refreshed for future calls, but return 401 to caller
  }

  return res
}

const PUBLIC_PATHS = [
  '/login', '/register', '/forgot-password', '/reset-password',
  '/verify-email', '/callback', '/confirm-email-change', '/cancel-deletion',
]

/**
 * Registers a visibilitychange listener that proactively refreshes the access
 * token when the user returns to the tab after a long absence. Prevents the
 * "eternal spinner" scenario where the token expired while the tab was frozen.
 * Call once at app startup (main.tsx).
 */
export function initVisibilityRefresh(): void {
  if (DEV_AUTH) return

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState !== 'visible') return

    const token = accessToken
    if (!token) {
      // Token was cleared while the tab was backgrounded (e.g. scheduled refresh
      // failed). If we're on a protected page, attempt a silent restore — or
      // redirect to login so the user isn't stuck in a zombie state.
      const isPublicPath = PUBLIC_PATHS.some(p => window.location.pathname.startsWith(p))
      if (!isPublicPath) {
        void tryRestoreSession().then(ok => {
          if (!ok) window.location.href = '/login'
        })
      }
      return
    }

    const payload = parseJwtPayload(token)
    if (!payload?.exp) return

    const nowSec = Math.floor(Date.now() / 1000)
    // Refresh if the token has expired or will expire within the next 60 seconds
    if (payload.exp - nowSec <= 60) {
      void refreshToken().then(ok => {
        if (!ok) window.location.href = '/login'
      })
    }
  })
}

/**
 * Cross-tab auth token synchronization via BroadcastChannel.
 *
 * Ensures that when one tab logs in, refreshes, or logs out, all other tabs
 * immediately receive the updated auth state. Without this, tabs that make
 * few API calls (SSE followers with 120s safety-net polling) would not
 * discover token updates until their next request.
 *
 * Also handles the F5 (page reload) case: a reloaded tab requests the current
 * token from other open tabs before falling back to a network refresh.
 *
 * Call once at app startup (main.tsx), before tryRestoreSession().
 */
export function initAuthSync(): void {
  if (DEV_AUTH || typeof BroadcastChannel === 'undefined') return

  authChannel = new BroadcastChannel('skadia-auth')

  authChannel.onmessage = (e: MessageEvent) => {
    const msg = e.data
    if (!msg || typeof msg !== 'object' || typeof msg.type !== 'string') return

    if (msg.type === 'token-update' && typeof msg.token === 'string') {
      // Another tab got a fresh token (login or refresh) — accept it
      suppressBroadcast = true
      setToken(msg.token)
      suppressBroadcast = false
    } else if (msg.type === 'logout') {
      // Another tab logged out — clear local state and redirect
      suppressBroadcast = true
      clearToken()
      suppressBroadcast = false
      const isPublic = PUBLIC_PATHS.some(p => window.location.pathname.startsWith(p))
      if (!isPublic) window.location.href = '/login'
    } else if (msg.type === 'token-request') {
      // Another tab (e.g., after F5 reload) is asking for the current token
      const token = getToken()
      if (token) {
        authChannel?.postMessage({ type: 'token-update', token })
      }
    }
  }

  // If we don't have a token yet (page reload), ask other tabs
  if (!accessToken) {
    authChannel.postMessage({ type: 'token-request' })
  }
}
