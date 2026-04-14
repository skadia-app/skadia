import { getToken, refreshToken, clearToken, redirectToLogin } from '@/lib/auth'

export const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown
}

/** Authenticated API client with automatic token refresh on 401 */
export async function apiClient<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const response = await doFetch(path, options)

  if (response.status === 401) {
    const refreshed = await refreshToken()
    if (refreshed) {
      const retry = await doFetch(path, options)
      if (!retry.ok) throw await apiErrorFromResponse(retry)
      return retry.json() as Promise<T>
    }
    clearToken()
    redirectToLogin()
    throw new ApiError(401, 'Unauthorized')
  }

  if (!response.ok) {
    throw await apiErrorFromResponse(response)
  }

  if (response.status === 204) return null as unknown as T
  // 202 may or may not have a JSON body; only parse if Content-Type indicates JSON
  if (response.status === 202) {
    const ct = response.headers.get('content-type') ?? ''
    if (ct.includes('application/json')) return response.json() as Promise<T>
    return null as unknown as T
  }
  return response.json() as Promise<T>
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
  ) {
    super(`API error ${status}: ${body}`)
    this.name = 'ApiError'
  }
}

/** Build an ApiError from a non-ok fetch Response, parsing JSON when possible. */
async function apiErrorFromResponse(res: Response): Promise<ApiError> {
  const text = await res.text()
  const ct = res.headers.get('content-type') ?? ''
  if (ct.includes('application/json') && text.length > 0) {
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

/** Build an ApiError from an XHR response, parsing JSON when possible. */
function apiErrorFromXhr(xhr: XMLHttpRequest): ApiError {
  const text = xhr.responseText ?? ''
  const ct = xhr.getResponseHeader('content-type') ?? ''
  if (ct.includes('application/json') && text.length > 0) {
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

/** Authenticated binary download: fetches a protected URL with auth header and returns a Blob */
export async function apiDownload(
  path: string,
  options?: { method?: string; body?: unknown },
): Promise<{ blob: Blob; filename?: string }> {
  const token = getToken()
  const url = path.startsWith('http') ? path : `${API_BASE}${path}`
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
