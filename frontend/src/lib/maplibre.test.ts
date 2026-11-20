import { afterEach, describe, expect, it, vi } from 'vitest'

describe('lib/maplibre worker initialization', () => {
  afterEach(() => {
    vi.resetModules()
    vi.doUnmock('maplibre-gl')
  })

  it('calls setWorkerUrl with a non-empty worker URL string on module load', async () => {
    const setWorkerUrl = vi.fn()
    vi.resetModules()
    vi.doMock('maplibre-gl', () => ({
      setWorkerUrl,
      Map: vi.fn(),
    }))

    const mod = await import('@/lib/maplibre')

    expect(setWorkerUrl).toHaveBeenCalledTimes(1)
    const [calledUrl] = setWorkerUrl.mock.calls[0]
    expect(typeof calledUrl).toBe('string')
    expect(calledUrl.length).toBeGreaterThan(0)
    expect(calledUrl).toContain('maplibre-gl-worker')
    expect(mod.Map).toBeDefined()
  })

  it('does not throw when maplibre-gl is partially mocked without setWorkerUrl', async () => {
    vi.resetModules()
    vi.doMock('maplibre-gl', () => ({ default: {} }))

    await expect(import('@/lib/maplibre')).resolves.toBeDefined()
  })
})
