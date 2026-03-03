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

/** Authenticated binary download: fetches a protected URL with auth header and returns a Blob */
export async function apiDownload(path: string): Promise<{ blob: Blob; filename?: string }> {
  const token = getToken()
  const url = path.startsWith('http') ? path : `${API_BASE}${path}`
  const res = await fetch(url, {
    headers: token ? { 'Authorization': `Bearer ${token}` } : {},
    credentials: 'include',
  })
  if (res.status === 401) {
    clearToken()
    window.location.href = '/login'
    throw new ApiError(401, 'Unauthorized')
  }
  if (!res.ok) throw new ApiError(res.status, await res.text())
  const blob = await res.blob()
  const cd = res.headers.get('Content-Disposition') ?? ''
  const match = cd.match(/filename[*]?=(?:UTF-8'')?["']?([^"';\r\n]+)["']?/i)
  const filename = match?.[1] ? decodeURIComponent(match[1]) : undefined
  return { blob, filename }
}
