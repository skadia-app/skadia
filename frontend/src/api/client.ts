import { getToken, refreshToken, clearToken } from '@/lib/auth'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

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
      if (!retry.ok) throw new ApiError(retry.status, await retry.text())
      return retry.json() as Promise<T>
    }
    clearToken()
    window.location.href = '/login'
    throw new ApiError(401, 'Unauthorized')
  }

  if (!response.ok) {
    throw new ApiError(response.status, await response.text())
  }

  if (response.status === 204) return undefined as T
  // 202 may or may not have a JSON body; only parse if Content-Type indicates JSON
  if (response.status === 202) {
    const ct = response.headers.get('content-type') ?? ''
    if (ct.includes('application/json')) return response.json() as Promise<T>
    return undefined as T
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
        window.location.href = '/login'
        reject(new ApiError(401, 'Unauthorized'))
        return
      }
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new ApiError(xhr.status, xhr.responseText))
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
      if (!retry.ok) throw new ApiError(retry.status, await retry.text())
      return retry.json() as Promise<T>
    }
    clearToken()
    window.location.href = '/login'
    throw new ApiError(401, 'Unauthorized')
  }

  if (!res.ok) throw new ApiError(res.status, await res.text())
  return res.json() as Promise<T>
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public body: string,
  ) {
    super(`API error ${status}: ${body}`)
    this.name = 'ApiError'
  }
}

/** Parse filename from Content-Disposition header. */
export function parseContentDisposition(header: string | null): string | undefined {
  const match = (header ?? '').match(/filename[*]?=(?:UTF-8'')?["']?([^"';\r\n]+)["']?/i)
  return match?.[1] ? decodeURIComponent(match[1]) : undefined
}

/** Authenticated binary download: fetches a protected URL with auth header and returns a Blob */
export async function apiDownload(path: string): Promise<{ blob: Blob; filename?: string }> {
  const token = getToken()
  const url = path.startsWith('http') ? path : `${API_BASE}${path}`
  const res = await fetch(url, {
    headers: token ? { 'Authorization': `Bearer ${token}` } : {},
    credentials: 'include',
    cache: 'no-cache',
  })
  if (res.status === 401) {
    clearToken()
    window.location.href = '/login'
    throw new ApiError(401, 'Unauthorized')
  }
  if (!res.ok) throw new ApiError(res.status, await res.text())
  const blob = await res.blob()
  const filename = parseContentDisposition(res.headers.get('Content-Disposition'))
  return { blob, filename }
}
