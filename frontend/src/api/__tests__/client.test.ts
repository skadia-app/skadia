import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient, clearETagCache, getCachedETag, storeETagForPath } from '@/api/client'

// ────────────────────────────────────────────────────────────────────────────
// apiClient ETag-cache + If-Match auto-injection.
//
// These tests pin the optimistic-concurrency layer that lives entirely in the
// client: GET responses with an ETag header populate a per-resource cache,
// PATCH/PUT calls inherit it as If-Match, and a 412 response refreshes the
// cache before re-throwing as ApiError so the next user-driven retry uses
// fresh state.
// ────────────────────────────────────────────────────────────────────────────

interface FetchCall {
  url: string
  init?: RequestInit
}

function makeFetchSpy(responder: (call: FetchCall) => Response) {
  return vi
    .spyOn(globalThis, 'fetch')
    .mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
      return Promise.resolve(responder({ url, init }))
    })
}

beforeEach(() => {
  clearETagCache()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('apiClient ETag cache', () => {
  it('stores the ETag from a successful GET against the path', async () => {
    makeFetchSpy(({ url, init }) => {
      const method = (init?.method ?? 'GET').toUpperCase()
      expect(method).toBe('GET')
      expect(url).toContain('/routes/r1')
      return new Response(JSON.stringify({ id: 'r1' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json', ETag: '"7"' },
      })
    })

    await apiClient('/routes/r1')
    expect(getCachedETag('/routes/r1')).toBe('"7"')
  })

  it('strips the query string when keying the cache', async () => {
    makeFetchSpy(() =>
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { 'Content-Type': 'application/json', ETag: '"3"' },
      }),
    )

    // ?sort=foo and ?sort=bar must NOT each get their own cache slot — the
    // resource identity is the path, not the listing variant.
    await apiClient('/routes/r1?sort=foo')
    expect(getCachedETag('/routes/r1?sort=bar')).toBe('"3"')
    expect(getCachedETag('/routes/r1')).toBe('"3"')
  })

  it('auto-injects If-Match on a follow-up PATCH', async () => {
    let observedIfMatch: string | undefined
    let callIdx = 0
    makeFetchSpy(({ init }) => {
      callIdx += 1
      if (callIdx === 1) {
        return new Response(JSON.stringify({ id: 'r1', version: 2 }), {
          status: 200,
          headers: { 'Content-Type': 'application/json', ETag: '"2"' },
        })
      }
      observedIfMatch = (init?.headers as Record<string, string> | undefined)?.['If-Match']
      return new Response(JSON.stringify({ id: 'r1', version: 3 }), {
        status: 200,
        headers: { 'Content-Type': 'application/json', ETag: '"3"' },
      })
    })

    await apiClient('/routes/r1')
    await apiClient('/routes/r1', { method: 'PATCH', body: { name: 'x' } })
    expect(observedIfMatch).toBe('"2"')
    // Cache refreshed from PATCH response so a subsequent PATCH carries v=3.
    expect(getCachedETag('/routes/r1')).toBe('"3"')
  })

  it('honours a caller-supplied If-Match over the cache', async () => {
    let observedIfMatch: string | undefined
    let callIdx = 0
    makeFetchSpy(({ init }) => {
      callIdx += 1
      if (callIdx === 1) {
        return new Response('{}', {
          status: 200,
          headers: { 'Content-Type': 'application/json', ETag: '"2"' },
        })
      }
      observedIfMatch = (init?.headers as Record<string, string> | undefined)?.['If-Match']
      return new Response('{}', {
        status: 200,
        headers: { 'Content-Type': 'application/json', ETag: '"3"' },
      })
    })

    await apiClient('/routes/r1')
    await apiClient('/routes/r1', {
      method: 'PATCH',
      body: { name: 'x' },
      headers: { 'If-Match': '"99"' },
    })
    expect(observedIfMatch).toBe('"99"')
  })

  it('does not inject If-Match on GET / DELETE', async () => {
    let getIfMatch: string | undefined
    let deleteIfMatch: string | undefined
    let callIdx = 0
    makeFetchSpy(({ init }) => {
      callIdx += 1
      const method = (init?.method ?? 'GET').toUpperCase()
      const ifMatch = (init?.headers as Record<string, string> | undefined)?.['If-Match']
      if (method === 'GET') getIfMatch = ifMatch
      if (method === 'DELETE') deleteIfMatch = ifMatch
      const status = method === 'DELETE' ? 204 : 200
      return new Response(method === 'DELETE' ? null : '{}', {
        status,
        headers: status === 204 ? {} : { 'Content-Type': 'application/json', ETag: '"5"' },
      })
    })

    await apiClient('/routes/r1') // populates cache
    await apiClient('/routes/r1') // GET again
    await apiClient('/routes/r1', { method: 'DELETE' })
    expect(callIdx).toBe(3)
    expect(getIfMatch).toBeUndefined()
    expect(deleteIfMatch).toBeUndefined()
  })

  it('does NOT update the cache from a 412 response — throws with serverETag on the error', async () => {
    let callIdx = 0
    makeFetchSpy(() => {
      callIdx += 1
      if (callIdx === 1) {
        return new Response(JSON.stringify({ id: 'r1', version: 1 }), {
          status: 200,
          headers: { 'Content-Type': 'application/json', ETag: '"1"' },
        })
      }
      // PATCH returns 412 with the current state and a fresh ETag.
      return new Response(JSON.stringify({ id: 'r1', version: 5 }), {
        status: 412,
        headers: { 'Content-Type': 'application/json', ETag: '"5"' },
      })
    })

    await apiClient('/routes/r1')
    let conflictBody: { version?: number } | undefined
    let conflictStatus: number | undefined
    let conflictServerETag: string | undefined
    try {
      await apiClient('/routes/r1', { method: 'PATCH', body: { name: 'x' } })
    } catch (err) {
      const e = err as { status?: number; json?: Record<string, unknown>; serverETag?: string }
      conflictStatus = e.status
      conflictBody = e.json as { version?: number }
      conflictServerETag = e.serverETag
    }

    expect(conflictStatus).toBe(412)
    expect(conflictBody?.version).toBe(5)
    // The server's current ETag is available on the error for explicit opt-in.
    expect(conflictServerETag).toBe('"5"')
    // Cache must still hold the original ETag — NOT the server's fresh value.
    expect(getCachedETag('/routes/r1')).toBe('"1"')
  })

  it('second save after 412 carries the stale ETag and gets 412 again', async () => {
    let patchCount = 0
    let lastIfMatch: string | undefined
    makeFetchSpy(({ init }) => {
      const method = (init?.method ?? 'GET').toUpperCase()
      if (method === 'GET') {
        return new Response(JSON.stringify({ id: 'r1', version: 1 }), {
          status: 200,
          headers: { 'Content-Type': 'application/json', ETag: '"1"' },
        })
      }
      patchCount += 1
      lastIfMatch = (init?.headers as Record<string, string> | undefined)?.['If-Match']
      return new Response(JSON.stringify({ id: 'r1', version: 5 }), {
        status: 412,
        headers: { 'Content-Type': 'application/json', ETag: '"5"' },
      })
    })

    await apiClient('/routes/r1') // GET → cache '"1"'

    // First PATCH → 412, cache stays at '"1"'
    await expect(
      apiClient('/routes/r1', { method: 'PATCH', body: { name: 'x' } }),
    ).rejects.toMatchObject({ status: 412 })
    expect(getCachedETag('/routes/r1')).toBe('"1"')

    // Second PATCH (user hits Save again without reloading) → still sends '"1"' → 412 again
    await expect(
      apiClient('/routes/r1', { method: 'PATCH', body: { name: 'x' } }),
    ).rejects.toMatchObject({ status: 412 })
    expect(patchCount).toBe(2)
    expect(lastIfMatch).toBe('"1"') // stale ETag, not '"5"'
  })

  it('storeETagForPath + save succeeds after explicit reload acceptance', async () => {
    let patchIfMatch: string | undefined
    let callIdx = 0
    makeFetchSpy(({ init }) => {
      callIdx += 1
      const method = (init?.method ?? 'GET').toUpperCase()
      if (method === 'GET') {
        return new Response(JSON.stringify({ id: 'r1', version: 1 }), {
          status: 200,
          headers: { 'Content-Type': 'application/json', ETag: '"1"' },
        })
      }
      if (callIdx === 2) {
        // First PATCH → 412
        return new Response(JSON.stringify({ id: 'r1', version: 5 }), {
          status: 412,
          headers: { 'Content-Type': 'application/json', ETag: '"5"' },
        })
      }
      // Third call → PATCH after reload acceptance → 200
      patchIfMatch = (init?.headers as Record<string, string> | undefined)?.['If-Match']
      return new Response(JSON.stringify({ id: 'r1', version: 6 }), {
        status: 200,
        headers: { 'Content-Type': 'application/json', ETag: '"6"' },
      })
    })

    await apiClient('/routes/r1') // GET → '"1"'

    let serverETag: string | undefined
    try {
      await apiClient('/routes/r1', { method: 'PATCH', body: { name: 'x' } })
    } catch (err) {
      const e = err as { serverETag?: string }
      serverETag = e.serverETag
    }

    // Simulate user clicking "Neu laden": caller explicitly accepts server state
    expect(serverETag).toBe('"5"')
    storeETagForPath('/routes/r1', serverETag!)

    // Now save again → must send '"5"' and succeed
    await apiClient('/routes/r1', { method: 'PATCH', body: { name: 'y' } })
    expect(patchIfMatch).toBe('"5"')
    expect(getCachedETag('/routes/r1')).toBe('"6"')
  })

  it('clearETagCache() drops every entry', async () => {
    makeFetchSpy(() =>
      new Response('{}', {
        status: 200,
        headers: { 'Content-Type': 'application/json', ETag: '"2"' },
      }),
    )

    await apiClient('/routes/r1')
    await apiClient('/routes/r2')
    expect(getCachedETag('/routes/r1')).toBe('"2"')
    expect(getCachedETag('/routes/r2')).toBe('"2"')
    clearETagCache()
    expect(getCachedETag('/routes/r1')).toBeUndefined()
    expect(getCachedETag('/routes/r2')).toBeUndefined()
  })
})
