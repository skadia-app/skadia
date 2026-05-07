import { getToken, refreshToken, clearToken, redirectToLogin } from '@/lib/auth'

export const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown
}

// ────────────────────────────────────────────────────────────────────────────
// ETag cache — strong validators emitted by GET /routes/{id} (and any future
// resource that opts in) are stashed here per cache-key. PATCH / PUT calls to
// the same key automatically inherit `If-Match`, giving the backend an
// optimistic-concurrency token without each call site having to track it.
//
// The cache is in-memory and module-scoped — no persistence across reloads.
// That is intentional: a stale ETag would cause a 412 false-positive after a
// reload, and re-fetching the resource on mount yields a fresh token anyway.
// ────────────────────────────────────────────────────────────────────────────

const etagCache = new Map<string, string>()

/** Cache key collapses the query string so `?sort=foo` GETs do not pollute the
 *  resource's PATCH precondition. Resource identity lives in the path. */
function etagCacheKey(path: string): string {
  const qs = path.indexOf('?')
  return qs >= 0 ? path.slice(0, qs) : path
}

/** Visible for tests / logout flows: drop all cached ETags. */
export function clearETagCache(): void {
  etagCache.clear()
}

/** Visible for tests: read the cached ETag for a path, if any. */
export function getCachedETag(path: string): string | undefined {
  return etagCache.get(etagCacheKey(path))
}

/**
 * Explicitly store an ETag for a path. Called by higher-level code (e.g.
 * useRoutePlanner) when the user explicitly accepts a conflict-response body
 * and the cache must be advanced to the server's current version before the
 * next write.
 */
export function storeETagForPath(path: string, etag: string): void {
  etagCache.set(etagCacheKey(path), etag)
}

/** Authenticated API client with automatic token refresh on 401 */
export async function apiClient<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const response = await doFetch(path, options)

  if (response.status === 401) {
    const refreshed = await refreshToken()
    if (refreshed) {
      const retry = await doFetch(path, options)
      if (!retry.ok) throw await apiErrorFromResponse(retry)
      maybeStoreETag(path, retry, options)
      return retry.json() as Promise<T>
    }
    clearToken()
    redirectToLogin()
    throw new ApiError(401, 'Unauthorized')
  }

  if (!response.ok) {
    // 412: do NOT update the ETag cache. Updating it here would silently allow
    // a second save attempt (with the stale local edits still in state) to pass
    // the precondition check, overwriting the concurrent change. The caller
    // must explicitly accept the conflict body and advance the cache via
    // storeETagForPath() before the next write is permitted.
    throw await apiErrorFrom412OrResponse(response)
  }

  maybeStoreETag(path, response, options)

  if (response.status === 204) return null as unknown as T
  // 202 may or may not have a JSON body; only parse if Content-Type indicates JSON
  if (response.status === 202) {
    const ct = response.headers.get('content-type') ?? ''
    if (ct.includes('application/json')) return response.json() as Promise<T>
    return null as unknown as T
  }
  return response.json() as Promise<T>
}

/** Store a server-issued ETag against the resource path so a subsequent PATCH /
 *  PUT can use it as `If-Match`. Only successful GET / PATCH / PUT / POST
 *  responses with an ETag header populate the cache — DELETE 204s and other
 *  empty responses are no-ops. */
function maybeStoreETag(path: string, response: Response, options: RequestOptions): void {
  const etag = response.headers.get('ETag')
  if (!etag) return
  const method = (options.method ?? 'GET').toUpperCase()
  if (method !== 'GET' && method !== 'PATCH' && method !== 'PUT' && method !== 'POST') return
  etagCache.set(etagCacheKey(path), etag)
}

function doFetch(path: string, options: RequestOptions): Promise<Response> {
  const token = getToken()
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> | undefined),
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  if (options.body !== undefined && options.body !== null) {
    headers['Content-Type'] = 'application/json'
  }

  // Inject If-Match for write methods that target a previously-cached
  // resource. Caller-supplied If-Match always wins (covers explicit overrides
  // and tests). The wildcard "*" cache value disables auto-injection without
  // removing the entry — useful for forced overwrites.
  const method = (options.method ?? 'GET').toUpperCase()
  if ((method === 'PATCH' || method === 'PUT') && headers['If-Match'] === undefined) {
    const cached = etagCache.get(etagCacheKey(path))
    if (cached) headers['If-Match'] = cached
  }

  const url = path.startsWith('http') ? path : `${API_BASE}${path}`

  return fetch(url, {
    ...options,
    headers,
    credentials: 'include',
    body: options.body != null ? JSON.stringify(options.body) : undefined,
  })
}

/**
 * Upload FormData via XHR with optional callback when all bytes are sent (before server response).
 * Useful to distinguish "uploading" and "processing/scanning" phases in the UI.
 */
export function apiUploadWithProgress<T>(
  path: string,
  formData: FormData,
  onUploaded?: () => void,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const url = path.startsWith('http') ? path : `${API_BASE}${path}`
    const token = getToken()
    const xhr = new XMLHttpRequest()
    xhr.open('POST', url)
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    xhr.withCredentials = true

    if (onUploaded) {
      xhr.upload.addEventListener('load', onUploaded)
    }

    xhr.addEventListener('load', () => {
      if (xhr.status === 401) {
        clearToken()
        redirectToLogin()
        reject(new ApiError(401, 'Unauthorized'))
        return
      }
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(apiErrorFromXhr(xhr))
        return
      }
      try {
        resolve(JSON.parse(xhr.responseText) as T)
      } catch {
        reject(new Error('Invalid JSON response'))
      }
    })
    xhr.addEventListener('error', () => reject(new Error('Network error')))
    xhr.send(formData)
  })
}

/** Upload FormData with automatic token refresh on 401 */
export async function apiUpload<T>(path: string, formData: FormData): Promise<T> {
  const url = path.startsWith('http') ? path : `${API_BASE}${path}`
  const token = getToken()
  const headers: Record<string, string> = {}
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(url, {
    method: 'POST',
    headers,
    credentials: 'include',
    body: formData,
  })

  if (res.status === 401) {
    const refreshed = await refreshToken()
    if (refreshed) {
      const retryToken = getToken()
      const retryHeaders: Record<string, string> = {}
      if (retryToken) retryHeaders['Authorization'] = `Bearer ${retryToken}`
      const retry = await fetch(url, {
        method: 'POST',
        headers: retryHeaders,
        credentials: 'include',
        body: formData,
      })
      if (!retry.ok) throw await apiErrorFromResponse(retry)
      return retry.json() as Promise<T>
    }
    clearToken()
    redirectToLogin()
    throw new ApiError(401, 'Unauthorized')
  }

  if (!res.ok) throw await apiErrorFromResponse(res)
  return res.json() as Promise<T>
}

/**
 * ApiError carries the HTTP status plus the server-provided error payload.
 *
 * `body` is the raw response text (always present, possibly empty). When the
 * server answered with `content-type: application/json` and the payload
 * parses, `json` holds the decoded object — callers should discriminate on
 * `json?.error_code` to react to well-known, i18n-stable conditions without
 * matching brittle regex on `message`.
 *
 * Shape of structured JSON bodies produced by the Go backend (non-exhaustive):
 *   { error_code: 'provider_disabled', message: string }
 *   { error_code: 'already_running',   job_id: string, message?: string }
 */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: string,
    public readonly json?: Record<string, unknown>,
    /**
     * For 412 responses only: the ETag the server returned alongside the
     * current resource state. Callers that accept the conflict body must pass
     * this to storeETagForPath() before the next write to advance the cache.
     */
    public readonly serverETag?: string,
  ) {
    super(`API error ${status}: ${body}`)
    this.name = 'ApiError'
  }
}

/** Build an ApiError from a non-ok fetch Response, parsing JSON when possible. */
async function apiErrorFromResponse(res: Response): Promise<ApiError> {
  const text = await res.text()
  const ct = res.headers.get('content-type') ?? ''
  // Match both `application/json` and RFC 7807 `application/problem+json`.
  if (/\bjson\b/.test(ct) && text.length > 0) {
    try {
      const parsed = JSON.parse(text) as unknown
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return new ApiError(res.status, text, parsed as Record<string, unknown>)
      }
    } catch {
      /* fall through to raw-text ApiError */
    }
  }
  return new ApiError(res.status, text)
}

/**
 * Build an ApiError for a non-ok response, attaching the server's ETag header
 * value for 412 responses. The ETag is NOT written to the cache here — the
 * caller must explicitly call storeETagForPath() after accepting the conflict
 * body, ensuring the cache only advances when the user deliberately reloads.
 */
async function apiErrorFrom412OrResponse(res: Response): Promise<ApiError> {
  const base = await apiErrorFromResponse(res)
  if (res.status !== 412) return base
  const etag = res.headers.get('ETag') ?? undefined
  return new ApiError(base.status, base.body, base.json, etag)
}

/** Build an ApiError from an XHR response, parsing JSON when possible. */
function apiErrorFromXhr(xhr: XMLHttpRequest): ApiError {
  const text = xhr.responseText ?? ''
  const ct = xhr.getResponseHeader('content-type') ?? ''
  // Match both `application/json` and RFC 7807 `application/problem+json`.
  if (/\bjson\b/.test(ct) && text.length > 0) {
    try {
      const parsed = JSON.parse(text) as unknown
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return new ApiError(xhr.status, text, parsed as Record<string, unknown>)
      }
    } catch {
      /* fall through to raw-text ApiError */
    }
  }
  return new ApiError(xhr.status, text)
}

/** Parse filename from Content-Disposition header. */
export function parseContentDisposition(header: string | null): string | undefined {
  const match = (header ?? '').match(/filename[*]?=(?:UTF-8'')?["']?([^"';\r\n]+)["']?/i)
  return match?.[1] ? decodeURIComponent(match[1]) : undefined
}

function resolveDownloadUrl(path: string): string {
  const url = path.startsWith('http') || path.startsWith('/api/') ? path : `${API_BASE}${path}`
  const browserOrigin = window.location.origin
  const candidate = new URL(url, browserOrigin)
  const apiBase = new URL(API_BASE, browserOrigin)
  const apiBasePath = apiBase.pathname.endsWith('/') ? apiBase.pathname : `${apiBase.pathname}/`

  const sameOriginApiPath = candidate.origin === browserOrigin && candidate.pathname.startsWith('/api/')
  const configuredApiPath =
    candidate.origin === apiBase.origin &&
    (candidate.pathname === apiBase.pathname || candidate.pathname.startsWith(apiBasePath))

  if (!sameOriginApiPath && !configuredApiPath) {
    throw new ApiError(400, 'Invalid download URL')
  }
  return url
}

/** Authenticated binary download: fetches a protected URL with auth header and returns a Blob */
export async function apiDownload(
  path: string,
  options?: { method?: string; body?: unknown },
): Promise<{ blob: Blob; filename?: string }> {
  const token = getToken()
  const url = resolveDownloadUrl(path)
  const method = options?.method ?? 'GET'
  const headers: Record<string, string> = {}
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (method === 'POST' && options?.body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  const res = await fetch(url, {
    method,
    headers,
    credentials: 'include',
    cache: 'no-cache',
    body: method === 'POST' && options?.body !== undefined ? JSON.stringify(options.body) : undefined,
  })
  if (res.status === 401) {
    clearToken()
    redirectToLogin()
    throw new ApiError(401, 'Unauthorized')
  }
  if (!res.ok) throw new ApiError(res.status, await res.text())
  const blob = await res.blob()
  const filename = parseContentDisposition(res.headers.get('Content-Disposition'))
  return { blob, filename }
}
