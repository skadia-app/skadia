import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError, throwOnApiResponse } from '@/api/client'
import { apiFetch, clearToken, setToken } from '@/lib/auth'

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  const headers = new Headers(init.headers)
  if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  return new Response(JSON.stringify(body), { ...init, headers })
}

async function captureApiError(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise
  } catch (err) {
    expect(err).toBeInstanceOf(ApiError)
    return err as ApiError
  }
  throw new Error('expected ApiError')
}

afterEach(() => {
  clearToken()
  vi.restoreAllMocks()
})

describe('throwOnApiResponse', () => {
  it('throws ApiError with parsed json for non-ok apiFetch responses', async () => {
    const error = await captureApiError(
      throwOnApiResponse(jsonResponse(
        { error_code: 'storage_quota_exceeded', code: 'quota_exceeded', message: 'Zu groß' },
        { status: 409 },
      )),
    )

    expect(error.status).toBe(409)
    expect(error.json?.error_code).toBe('storage_quota_exceeded')
    expect(error.json?.code).toBe('quota_exceeded')
  })

  it('throws ApiError with raw body for text and empty non-ok responses', async () => {
    const textError = await captureApiError(
      throwOnApiResponse(new Response('plain failure', {
        status: 500,
        headers: { 'Content-Type': 'text/plain' },
      })),
    )
    expect(textError.status).toBe(500)
    expect(textError.body).toBe('plain failure')
    expect(textError.json).toBeUndefined()

    const emptyError = await captureApiError(
      throwOnApiResponse(new Response(null, { status: 503 })),
    )
    expect(emptyError.status).toBe(503)
    expect(emptyError.body).toBe('')
    expect(emptyError.json).toBeUndefined()
  })

  it('returns null for empty successes and parses json success bodies', async () => {
    await expect(throwOnApiResponse(new Response(null, { status: 204 }))).resolves.toBeNull()
    await expect(throwOnApiResponse(new Response(null, { status: 202 }))).resolves.toBeNull()
    await expect(throwOnApiResponse(new Response('accepted', {
      status: 202,
      headers: { 'Content-Type': 'text/plain' },
    }))).resolves.toBeNull()

    await expect(throwOnApiResponse<{ ok: boolean }>(jsonResponse(
      { ok: true },
      { status: 200 },
    ))).resolves.toEqual({ ok: true })
  })

  it('does not retry non-idempotent POST after a 401 apiFetch response', async () => {
    setToken('old-token')
    const calls: Array<{ url: string; init?: RequestInit }> = []
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
      calls.push({ url, init })
      if (url.endsWith('/auth/refresh')) {
        return jsonResponse({ token: 'new-token' }, { status: 200 })
      }
      return jsonResponse({ error_code: 'token_expired' }, { status: 401 })
    })

    await expect(
      throwOnApiResponse(apiFetch('/api/v1/activities/act-1/lifts/recalculate-all', { method: 'POST' })),
    ).rejects.toMatchObject({ status: 401 })

    const postCalls = calls.filter(call => call.url === '/api/v1/activities/act-1/lifts/recalculate-all')
    expect(postCalls).toHaveLength(1)
    expect(calls.some(call => call.url.endsWith('/auth/refresh'))).toBe(true)
  })
})
