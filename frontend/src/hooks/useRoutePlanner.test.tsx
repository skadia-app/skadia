import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ToastProvider } from '@/lib/toast'
import { useRoutePlanner } from './useRoutePlanner'
import { makeFakeSuggestion } from '@/test-utils/routeFixtures'
import type { PlannedRoute } from '@/api/routes'
import type { ProfileConfig } from '@/lib/routingProfiles'

// addToastMock must be hoisted so vi.mock factory can reference it
// (vi.mock calls are hoisted above imports by Vitest).
const addToastMock = vi.hoisted(() => vi.fn())

// Mock API layer that useRoutePlanner depends on at module-init time.
vi.mock('@/api/routes', async () => {
  const actual = await vi.importActual<typeof import('@/api/routes')>('@/api/routes')
  return {
    ...actual,
    getCapabilities: vi.fn().mockResolvedValue({ routing: true, brouter: false, badrouter: false, offline_map: false, surface_enrichment: false, dem_correction: false, isochrone: false, route_suggestion: false, ml_service: false, import_path_enabled: false }),
    getProfileDefaults: vi.fn().mockResolvedValue({
      profile: 'hiking',
      uphill_cost: 7,
    }),
    updateRoute: vi.fn(async (id: string, payload: Partial<PlannedRoute>) => ({
      id,
      name: 'Gespeicherte Route',
      profile: payload.profile ?? 'hiking',
      sport_type: payload.sport_type ?? 'hiking',
      waypoints: payload.waypoints ?? [],
      segments: payload.segments ?? [],
      elevation_profile: payload.elevation_profile ?? null,
      time_override: false,
      auto_tag: false,
      route_schema_version: payload.route_schema_version ?? 1,
      photos: [],
      tags: [],
      created_at: '2026-04-27T00:00:00Z',
      updated_at: '2026-04-27T00:00:00Z',
    })),
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

  it('starts with profile.status === loading before server fetch', () => {
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.profile.status).toBe('loading')
  })

  it('populates profile to ready after server fetch resolves', async () => {
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
      expect(result.current.profile.status).toBe('ready')
    })
    if (result.current.profile.status === 'ready') {
      expect(result.current.profile.name).toBe('hiking')
    }
  })

  it('shows error toast when mount-fetch fails; profile.status stays loading', async () => {
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
    expect(result.current.profile.status).toBe('loading')
  })

  it('addWaypoint before mount-fetch resolves does not crash (profile still loading)', async () => {
    // Use a controlled promise to keep mount-fetch unresolved
    let resolveMount: (v: any) => void
    const mountPromise = new Promise<any>(r => { resolveMount = r })
    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockImplementationOnce(() => mountPromise)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.profile.status).toBe('loading')

    // addWaypoint while profile is loading — must not throw
    await act(async () => {
      await result.current.addWaypoint([47.0, 11.0])
    })

    // Waypoint state should still update (user expects it)
    expect(result.current.waypoints.length).toBeGreaterThan(0)

    // Resolve mount-fetch so cleanup is clean
    await act(async () => {
      resolveMount({ profile: 'hiking', uphill_cost: 7 })
      await new Promise(r => setTimeout(r, 0))
    })
  })

  it('loadSuggestion setzt 2 Waypoints + 1 Segment mit voller Geometrie', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    const hikingProfile = { profile: 'hiking', uphill_cost: 7 } as any
    vi.mocked(getProfileDefaults).mockReturnValue(hikingProfile)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    await waitFor(() => {
      expect(result.current.profile.status).toBe('ready')
    })

    // Explicit 4-coord geometry: test asserts segment.geometry hat volle Länge
    // durchgereicht — verifiziert die Factory-Override-Mechanik, nicht nur den
    // Default.
    const fakeSuggestion = makeFakeSuggestion({
      geojson: {
        type: 'FeatureCollection',
        features: [{
          type: 'Feature',
          geometry: {
            type: 'LineString',
            coordinates: [
              [12.0, 49.0, 500],
              [12.01, 49.01, 510],
              [12.02, 49.0, 505],
              [12.0, 49.0, 500],
            ],
          },
          properties: {},
        }],
      },
    })

    act(() => {
      result.current.loadSuggestion(fakeSuggestion)
    })

    // 2 Waypoints (Start + End — bei Loop auf derselben Position)
    // latlng is [lat, lon] — coords in GeoJSON are [lon, lat], so:
    // latlng[0] = lat = 49.0, latlng[1] = lon = 12.0
    expect(result.current.waypoints).toHaveLength(2)
    expect(result.current.waypoints[0].latlng[0]).toBeCloseTo(49.0) // lat
    expect(result.current.waypoints[0].latlng[1]).toBeCloseTo(12.0) // lon
    expect(result.current.waypoints[1].latlng[0]).toBeCloseTo(49.0) // lat
    expect(result.current.waypoints[1].latlng[1]).toBeCloseTo(12.0) // lon

    // 1 Segment mit voller Geometrie + Backend-Werten
    expect(result.current.segments).toHaveLength(1)
    expect(result.current.segments[0].distance_m).toBe(11700)
    expect(result.current.segments[0].elevation_gain).toBe(208)
    expect(result.current.segments[0].elevation_loss).toBe(208)
    expect(result.current.segments[0].geometry?.coordinates).toHaveLength(4)
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

    // At this point profile is loading (mount-fetch pending, no loadRoute yet)
    expect(result.current.profile.status).toBe('loading')

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

    // loadRoute should have set profile to ready with MTB synchronously
    expect(result.current.profile.status).toBe('ready')
    if (result.current.profile.status === 'ready') {
      expect(result.current.profile.name).toBe('mtb')
    }

    // Now resolve the mount-fetch with hiking data
    await act(async () => {
      resolveMountFetch({ profile: 'hiking', uphill_cost: 7 })
      // Let microtasks + state updates settle
      await new Promise(r => setTimeout(r, 0))
    })

    // profile must still be 'mtb' — the mount-fetch race-guard prevented overwrite
    expect(result.current.profile.status).toBe('ready')
    if (result.current.profile.status === 'ready') {
      expect(result.current.profile.name).toBe('mtb')
    }
  })

  it('loadSuggestion setzt scalePin auf null zurück (I3-Invariant)', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } satisfies ProfileConfig)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    // Zuerst Scale-Pin setzen (simuliert User-Klick auf SAC-Badge)
    act(() => { result.current.togglePin('sac') })
    expect(result.current.scalePin).toBe('sac')

    // loadSuggestion MUSS Pin zurücksetzen (I3-Invariant: Übernehmen-Flow)
    act(() => { result.current.loadSuggestion(makeFakeSuggestion()) })
    expect(result.current.scalePin).toBeNull()
  })

  it('recalculateAll setzt scalePin auf null zurück', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    // Pin auf MTB setzen
    act(() => { result.current.togglePin('mtb') })
    expect(result.current.scalePin).toBe('mtb')

    // recalculateAll (keine Waypoints → kein Segment-Fetch) → Pin MUSS null sein
    await act(async () => {
      if (result.current.profile.status === 'ready') {
        await result.current.profile.recalculate()
      }
    })
    expect(result.current.scalePin).toBeNull()
  })

  it('loadRoute (cache-hit) setzt scalePin auf null zurück', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    const hikingProfile = { profile: 'hiking', uphill_cost: 7 } as any
    vi.mocked(getProfileDefaults).mockReturnValue(hikingProfile)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => { expect(result.current.profile.status).toBe('ready') })

    // Via Ferrata Pin setzen
    act(() => { result.current.togglePin('vf') })
    expect(result.current.scalePin).toBe('vf')

    // loadRoute cache-hit Pfad (getProfileDefaults gibt Profil zurück)
    act(() => {
      result.current.loadRoute({
        id: 'route-cache-hit',
        profile: 'hiking',
        sport_type: 'hiking',
        waypoints: [],
        segments: [],
        total_distance_m: 0,
        total_elevation_gain: 0,
        total_elevation_loss: 0,
        name: 'Cache-Hit Route',
        created_at: '',
        updated_at: '',
      } as any)
    })
    expect(result.current.scalePin).toBeNull()
  })

  it('loadRoute (cache-miss) setzt scalePin auf null zurück', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    // cache-miss: gibt null zurück, damit loadRoute den else-Pfad (async fetch) nimmt
    vi.mocked(getProfileDefaults).mockReturnValue(null)

    // Aber mount-fetch soll sofort resolven damit der hook stable ist
    const { getProfileDefaults: fetchProfileDefaults } = await import('@/api/routes')
    vi.mocked(fetchProfileDefaults).mockResolvedValue({ profile: 'hiking', uphill_cost: 7 } as any)

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })

    // SAC Pin setzen (noch im loading-Zustand — togglePin ist immer verfügbar)
    act(() => { result.current.togglePin('sac') })
    expect(result.current.scalePin).toBe('sac')

    // loadRoute cache-miss Pfad: getProfileDefaults gibt null → async fetch
    act(() => {
      result.current.loadRoute({
        id: 'route-cache-miss',
        profile: 'hiking',
        sport_type: 'hiking',
        waypoints: [],
        segments: [],
        total_distance_m: 0,
        total_elevation_gain: 0,
        total_elevation_loss: 0,
        name: 'Cache-Miss Route',
        created_at: '',
        updated_at: '',
      } as any)
    })
    // resetScalePin() wird synchron im else-Zweig aufgerufen — sofort prüfbar
    expect(result.current.scalePin).toBeNull()
  })

  it('forceMigrateAndSave saves a loaded pre-T0 route with schema version 1', async () => {
    const { getProfileDefaults } = await import('@/lib/routingProfiles')
    vi.mocked(getProfileDefaults).mockReturnValue({ profile: 'hiking', uphill_cost: 7 } as any)
    const { updateRoute } = await import('@/api/routes')

    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => {
      expect(result.current.profile.status).toBe('ready')
    })

    const route: PlannedRoute = {
      id: 'route-pre-t0',
      name: 'Alte Route',
      profile: 'hiking',
      sport_type: 'hiking',
      waypoints: [
        { id: 'a', latlng: [47, 11] as [number, number], type: 'start' as const },
        { id: 'b', latlng: [47.1, 11.1] as [number, number], type: 'end' as const },
      ],
      segments: [{
        from_waypoint: 'a',
        to_waypoint: 'b',
        mode: 'routed' as const,
        geometry: { type: 'LineString' as const, coordinates: [[11, 47], [11.1, 47.1]] },
        control_points: [],
        smooth: true,
        distance_m: 100,
        elevation_gain: 10,
        elevation_loss: 0,
        elevation_profile: [{ dist_m: 0, ele_m: 100 }, { dist_m: 100, ele_m: 110 }],
      }],
      elevation_profile: [{ dist_m: 0, ele_m: 100 }, { dist_m: 100, ele_m: 110 }],
      time_override: false,
      auto_tag: false,
      route_schema_version: 0,
      photos: [],
      tags: [],
      created_at: '2026-04-27T00:00:00Z',
      updated_at: '2026-04-27T00:00:00Z',
    }

    act(() => {
      result.current.loadRoute(route)
    })
    expect(result.current.routeSchemaVersion).toBe(0)

    await act(async () => {
      await result.current.forceMigrateAndSave()
    })

    expect(updateRoute).toHaveBeenCalledWith('route-pre-t0', expect.objectContaining({
      route_schema_version: 1,
    }))
    await waitFor(() => {
      expect(result.current.routeSchemaVersion).toBe(1)
    })
  })
})
