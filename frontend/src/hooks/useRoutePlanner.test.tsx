import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ToastProvider } from '@/lib/toast'
import { useRoutePlanner } from './useRoutePlanner'

// addToastMock must be hoisted so vi.mock factory can reference it
// (vi.mock calls are hoisted above imports by Vitest).
const addToastMock = vi.hoisted(() => vi.fn())

// Mock API layer that useRoutePlanner depends on at module-init time.
vi.mock('@/api/routes', async () => {
  const actual = await vi.importActual<typeof import('@/api/routes')>('@/api/routes')
  return {
    ...actual,
    getCapabilities: vi.fn().mockResolvedValue({ has_badrouter: true }),
    getProfileDefaults: vi.fn().mockResolvedValue({
      profile: 'hiking',
      uphill_cost: 7,
    }),
  }
})

// Mock routingProfiles so we can control getProfileDefaults (cache lookup)
// and cacheServerDefaults without touching the real module-level cache.
vi.mock('@/lib/routingProfiles', async () => {
  const actual = await vi.importActual<typeof import('@/lib/routingProfiles')>('@/lib/routingProfiles')
  return {
    ...actual,
    getProfileDefaults: vi.fn().mockReturnValue(null),
    cacheServerDefaults: vi.fn(),
  }
})

// Capture addToast calls from useToast — we spy on the module so the hook
// picks up our mock. ToastProvider in the wrapper is still real (from importActual)
// for structural completeness, but the hook uses the mocked useToast.
vi.mock('@/lib/toast', async () => {
  const actual = await vi.importActual<typeof import('@/lib/toast')>('@/lib/toast')
  return {
    ...actual,
    useToast: vi.fn().mockReturnValue({ addToast: addToastMock }),
  }
})

describe('useRoutePlanner (P6 null-initial-state)', () => {
  let qc: QueryClient

  beforeEach(async () => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    addToastMock.mockClear()

    // Reset routingProfiles mocks to their default (cache miss → null)
    const { getProfileDefaults, cacheServerDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue(null)
    vi.mocked(cacheServerDefaults).mockReset()

    // Reset API mock to default (resolves successfully)
    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockResolvedValue({
      profile: 'hiking',
      uphill_cost: 7,
    } as any)
  })

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </ToastProvider>
  )

  it('starts with profileConfig === null before server fetch', () => {
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.profileConfig).toBeNull()
  })

  it('populates profileConfig after server fetch resolves', async () => {
    // The mount-fetch resolves → cacheServerDefaults is called → then the
    // setState updater calls getProfileDefaults('hiking') from routingProfiles.
    // Make it return the hiking profile so profileConfig gets populated.
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    const hikingProfile = { profile: 'hiking', uphill_cost: 7 } as any
    // First call is the initial guard check (state.profileConfig === null path):
    // The useEffect early-return reads state.profileConfig via closure, NOT via
    // getProfileDefaults — so getProfileDefaults only needs to return the profile
    // when called inside the setState updater (after fetch resolves).
    vi.mocked(getProfileDefaults).mockReturnValue(hikingProfile)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => {
      expect(result.current.profileConfig).not.toBeNull()
    })
    expect(result.current.profileConfig?.profile).toBe('hiking')
  })

  it('shows error toast when mount-fetch fails; profileConfig stays null', async () => {
    // Override the API mock to reject
    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockRejectedValueOnce(new Error('network'))

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    // Wait for the catch handler to fire — addToast should be called
    await waitFor(() => {
      expect(addToastMock).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          title: expect.stringContaining('Profil-Defaults'),
        })
      )
    })
    expect(result.current.profileConfig).toBeNull()
  })

  it('addWaypoint before mount-fetch resolves does not crash (profileConfig still null)', async () => {
    // Use a controlled promise to keep mount-fetch unresolved
    let resolveMount: (v: any) => void
    const mountPromise = new Promise<any>(r => { resolveMount = r })
    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockImplementationOnce(() => mountPromise)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.profileConfig).toBeNull()

    // addWaypoint while profileConfig is null — must not throw
    await act(async () => {
      result.current.addWaypoint([47.0, 11.0])
    })

    // Waypoint state should still update (user expects it)
    expect(result.current.waypoints.length).toBeGreaterThan(0)

    // Resolve mount-fetch so cleanup is clean
    await act(async () => {
      resolveMount({ profile: 'hiking', uphill_cost: 7 })
      await new Promise(r => setTimeout(r, 0))
    })
  })

  it('mount-fetch does not overwrite profileConfig set by loadRoute', async () => {
    // Control the mount-fetch: it will not resolve until we say so
    let resolveMountFetch!: (v: any) => void
    const mountFetchPromise = new Promise<any>(r => { resolveMountFetch = r })

    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockReturnValueOnce(mountFetchPromise)

    // Make routingProfiles.getProfileDefaults return a cached MTB profile
    // so that loadRoute's synchronous cache-hit path fires.
    const mtbProfileConfig = { profile: 'mtb', uphill_cost: 5 } as any
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockImplementation((name: string) => {
      if (name === 'mtb') return mtbProfileConfig
      return null
    })

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    // At this point profileConfig is null (mount-fetch pending, no loadRoute yet)
    expect(result.current.profileConfig).toBeNull()

    // Simulate loadRoute setting profileConfig synchronously (cache-hit path)
    act(() => {
      result.current.loadRoute({
        id: 'route-1',
        profile: 'mtb',
        sport_type: 'mountain_biking',
        waypoints: [],
        segments: [],
        total_distance_m: 0,
        total_elevation_gain: 0,
        total_elevation_loss: 0,
        name: 'Test Route',
        created_at: '',
        updated_at: '',
      } as any)
    })

    // loadRoute should have set profileConfig to MTB synchronously
    expect(result.current.profileConfig?.profile).toBe('mtb')

    // Now resolve the mount-fetch with hiking data
    await act(async () => {
      resolveMountFetch({ profile: 'hiking', uphill_cost: 7 })
      // Let microtasks + state updates settle
      await new Promise(r => setTimeout(r, 0))
    })

    // profileConfig must still be 'mtb' — the mount-fetch race-guard prevented overwrite
    expect(result.current.profileConfig?.profile).toBe('mtb')
  })
})
