import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest'
import type GeoJSON from 'geojson'
import type { ElevationPoint } from '@/api/routes'

// ---------------------------------------------------------------------------
// Module-level mocks — hoisted before imports
// ---------------------------------------------------------------------------

// Mock @/api/routes so we can control getElevationProfile responses.
vi.mock('@/api/routes', async () => {
  const actual = await vi.importActual<typeof import('@/api/routes')>('@/api/routes')
  return {
    ...actual,
    getElevationProfile: vi.fn(),
  }
})

// Mock @/lib/geoUtils haversineM with a simple Euclidean stand-in so tests
// don't depend on actual geographic math. For unit tests of chunking logic,
// any consistent distance function works.
vi.mock('@/lib/geoUtils', () => ({
  haversineM: (lat1: number, lng1: number, lat2: number, lng2: number): number => {
    const dlat = lat2 - lat1
    const dlng = lng2 - lng1
    return Math.sqrt(dlat * dlat + dlng * dlng) * 111_000 // degrees → metres approximation
  },
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------
import { getElevationProfile } from '@/api/routes'
import {
  MAX_VERTICES_PER_CALL,
  getElevationAtVerticesChunked,
  splitLineStringAtVertices,
  concatProfilesWithDedup,
  computeCumulativeDistancesFromCoords,
  clearDemCoverageWarning,
} from '@/api/elevation-chunked'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Build a straight-line LineString with n evenly-spaced coords. */
function makeLine(n: number, step = 0.001): GeoJSON.LineString {
  return {
    type: 'LineString',
    coordinates: Array.from({ length: n }, (_, i) => [i * step, 47]),
  }
}

/** Build an ElevationPoint[] matching the given coordinates. */
function makeProfile(coords: GeoJSON.Position[], eles: (number | null)[]): ElevationPoint[] {
  const n = coords.length
  return Array.from({ length: n }, (_, i) => ({
    fraction: n > 1 ? i / (n - 1) : 0,
    elevation: eles[i] ?? null,
  }))
}

// ---------------------------------------------------------------------------
// Task B.1 — getElevationProfile signature tests
// ---------------------------------------------------------------------------

describe('getElevationProfile (opts-signature backward-compat)', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ profile: [] }), {
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    // Restore the real getElevationProfile so fetch stubbing takes effect.
    vi.mocked(getElevationProfile).mockRestore?.()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // Re-import the real implementation for these tests.
  test('legacy call sends num_points, no at_vertices', async () => {
    const { getElevationProfile: realGetElevationProfile } = await vi.importActual<
      typeof import('@/api/routes')
    >('@/api/routes')
    const line: GeoJSON.LineString = { type: 'LineString', coordinates: [[11, 47], [11.5, 47.5]] }
    await realGetElevationProfile(line, { numPoints: 200 })
    expect(fetchMock).toHaveBeenCalledOnce()
    const callBody = JSON.parse(fetchMock.mock.calls[0][1].body as string)
    expect(callBody).toMatchObject({ geometry: line, num_points: 200 })
    expect(callBody).not.toHaveProperty('at_vertices')
  })

  test('atVertices: true sends at_vertices=true, omits num_points', async () => {
    const { getElevationProfile: realGetElevationProfile } = await vi.importActual<
      typeof import('@/api/routes')
    >('@/api/routes')
    const line: GeoJSON.LineString = { type: 'LineString', coordinates: [[11, 47], [11.5, 47.5]] }
    await realGetElevationProfile(line, { atVertices: true })
    expect(fetchMock).toHaveBeenCalledOnce()
    const callBody = JSON.parse(fetchMock.mock.calls[0][1].body as string)
    expect(callBody).toMatchObject({ geometry: line, at_vertices: true })
    expect(callBody).not.toHaveProperty('num_points')
  })

  test('default call (no opts) sends num_points 200, no at_vertices', async () => {
    const { getElevationProfile: realGetElevationProfile } = await vi.importActual<
      typeof import('@/api/routes')
    >('@/api/routes')
    const line: GeoJSON.LineString = { type: 'LineString', coordinates: [[11, 47], [11.5, 47.5]] }
    await realGetElevationProfile(line)
    const callBody = JSON.parse(fetchMock.mock.calls[0][1].body as string)
    expect(callBody).toMatchObject({ num_points: 200 })
    expect(callBody).not.toHaveProperty('at_vertices')
  })
})

// ---------------------------------------------------------------------------
// Task B.2 — splitLineStringAtVertices
// ---------------------------------------------------------------------------

describe('splitLineStringAtVertices', () => {
  test('preserves 1-vertex overlap at chunk boundaries', () => {
    const line = makeLine(100)
    const chunks = splitLineStringAtVertices(line, 30)
    expect(chunks).toHaveLength(4) // 100 coords → strides: [0,30), [29,59), [58,88), [87,100)
    for (let i = 0; i < chunks.length - 1; i++) {
      expect(chunks[i].coordinates.at(-1)).toEqual(chunks[i + 1].coordinates[0])
    }
  })

  test('returns single chunk when size <= maxVertices', () => {
    const line = makeLine(10)
    const chunks = splitLineStringAtVertices(line, 10)
    expect(chunks).toHaveLength(1)
    expect(chunks[0].coordinates).toHaveLength(10)
  })

  test('covers all original coordinates without gaps', () => {
    const line = makeLine(100)
    const chunks = splitLineStringAtVertices(line, 30)
    // First coord of chunk[0] = first coord of line
    expect(chunks[0].coordinates[0]).toEqual(line.coordinates[0])
    // Last coord of last chunk = last coord of line
    const last = chunks[chunks.length - 1]
    expect(last.coordinates.at(-1)).toEqual(line.coordinates.at(-1))
  })
})

// ---------------------------------------------------------------------------
// Task B.2 — concatProfilesWithDedup
// ---------------------------------------------------------------------------

describe('concatProfilesWithDedup', () => {
  test('drops first vertex of each non-first chunk and accumulates dist_m', () => {
    // Two chunks sharing one overlap vertex.
    // chunk[0]: coords [0,0] → [1,0] → [2,0] (step = 1 degree lng, haversineM ~ 111000 m each)
    // chunk[1]: coords [2,0] → [3,0] → [4,0] (overlap at [2,0])
    const coords0: GeoJSON.Position[] = [[0, 0], [1, 0], [2, 0]]
    const coords1: GeoJSON.Position[] = [[2, 0], [3, 0], [4, 0]]
    const chunks: GeoJSON.LineString[] = [
      { type: 'LineString', coordinates: coords0 },
      { type: 'LineString', coordinates: coords1 },
    ]
    const profiles: ElevationPoint[][] = [
      makeProfile(coords0, [100, 110, 120]),
      makeProfile(coords1, [120, 130, 140]),
    ]
    const out = concatProfilesWithDedup(profiles, chunks)

    // Expected: 5 points (3 + 3 - 1 overlap dropped)
    expect(out).toHaveLength(5)
    expect(out[0].ele_m).toBe(100)
    expect(out[1].ele_m).toBe(110)
    expect(out[2].ele_m).toBe(120) // last of chunk[0] = overlap vertex kept from chunk[0]
    expect(out[3].ele_m).toBe(130) // first non-overlap of chunk[1]
    expect(out[4].ele_m).toBe(140)

    // dist_m must be strictly increasing
    for (let i = 1; i < out.length; i++) {
      expect(out[i].dist_m).toBeGreaterThan(out[i - 1].dist_m)
    }
    // dist_m[2] == dist_m from chunk[0] accumulation; dist_m[3] must be greater
    expect(out[3].dist_m).toBeGreaterThan(out[2].dist_m)
  })

  test('preserves null ele_m values', () => {
    const coords: GeoJSON.Position[] = [[0, 0], [1, 0]]
    const chunks: GeoJSON.LineString[] = [{ type: 'LineString', coordinates: coords }]
    const profiles: ElevationPoint[][] = [makeProfile(coords, [null, null])]
    const out = concatProfilesWithDedup(profiles, chunks)
    expect(out.every(p => p.ele_m === null)).toBe(true)
  })

  test('empty chunk is skipped without breaking accumulator', () => {
    const coords: GeoJSON.Position[] = [[0, 0], [1, 0]]
    const chunks: GeoJSON.LineString[] = [
      { type: 'LineString', coordinates: coords },
      { type: 'LineString', coordinates: [[1, 0], [2, 0]] },
    ]
    const profiles: ElevationPoint[][] = [makeProfile(coords, [100, 110]), []]
    // Empty second profile should not throw and output only 2 points
    const out = concatProfilesWithDedup(profiles, chunks)
    expect(out).toHaveLength(2)
  })
})

// ---------------------------------------------------------------------------
// Task B.2 — getElevationAtVerticesChunked
// ---------------------------------------------------------------------------

describe('getElevationAtVerticesChunked', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    clearDemCoverageWarning()
  })

  test('single call below cap', async () => {
    const line = makeLine(3)
    const mockProfile = makeProfile(line.coordinates, [100, 200, 300])
    vi.mocked(getElevationProfile).mockResolvedValueOnce({ profile: mockProfile })

    const result = await getElevationAtVerticesChunked(line)

    expect(getElevationProfile).toHaveBeenCalledOnce()
    expect(getElevationProfile).toHaveBeenCalledWith(line, { atVertices: true })
    expect(result).toHaveLength(3)
    expect(result[0].dist_m).toBe(0)
    expect(result[0].ele_m).toBe(100)
    expect(result[2].ele_m).toBe(300)
  })

  test('chunked above cap: sequential calls, result has correct length', async () => {
    // Build a line with MAX_VERTICES_PER_CALL + 2 vertices to force chunking.
    // Two chunks: [0..MAX] and [MAX-1..MAX+1] (1-vertex overlap).
    const n = MAX_VERTICES_PER_CALL + 2
    const line: GeoJSON.LineString = {
      type: 'LineString',
      coordinates: Array.from({ length: n }, (_, i) => [i * 0.000001, 47]),
    }
    // Compute expected chunk sizes via splitLineStringAtVertices.
    const chunks = splitLineStringAtVertices(line, MAX_VERTICES_PER_CALL)
    expect(chunks).toHaveLength(2)

    const profile0 = makeProfile(chunks[0].coordinates, Array(chunks[0].coordinates.length).fill(100) as number[])
    const profile1 = makeProfile(chunks[1].coordinates, Array(chunks[1].coordinates.length).fill(200) as number[])

    vi.mocked(getElevationProfile)
      .mockResolvedValueOnce({ profile: profile0 })
      .mockResolvedValueOnce({ profile: profile1 })

    const result = await getElevationAtVerticesChunked(line, null)

    // Expected length: chunk0.length + chunk1.length - 1 (overlap dedup)
    const expectedLen = chunks[0].coordinates.length + chunks[1].coordinates.length - 1
    expect(getElevationProfile).toHaveBeenCalledTimes(2)
    expect(result).toHaveLength(expectedLen)
    expect(result[0].dist_m).toBe(0)
    // dist_m strictly increasing
    for (let i = 1; i < result.length; i++) {
      expect(result[i].dist_m).toBeGreaterThan(result[i - 1].dist_m)
    }
  })

  test('all-null profile triggers exactly one toast per session', async () => {
    // Reset the module-level warned flag by re-importing with a fresh module.
    // vitest isolates modules per test file by default, but the flag is
    // module-level state. We'll call the function twice and verify toast fires once.
    const line = makeLine(2)
    const nullProfile = makeProfile(line.coordinates, [null, null])
    vi.mocked(getElevationProfile).mockResolvedValue({ profile: nullProfile })

    const addToast = vi.fn()

    // First call: should trigger toast
    await getElevationAtVerticesChunked(line, addToast)
    expect(addToast).toHaveBeenCalledOnce()
    expect(addToast.mock.calls[0][0]).toMatchObject({
      group: 'dem-coverage',
      type: 'info',
    })

    // Second call: flag already set, toast must NOT fire again
    await getElevationAtVerticesChunked(line, addToast)
    expect(addToast).toHaveBeenCalledOnce() // still just one
  })

  test('non-null profile does not trigger toast', async () => {
    const line = makeLine(2)
    vi.mocked(getElevationProfile).mockResolvedValueOnce({
      profile: makeProfile(line.coordinates, [100, 200]),
    })
    const addToast = vi.fn()
    await getElevationAtVerticesChunked(line, addToast)
    expect(addToast).not.toHaveBeenCalled()
  })

  test('null addToast suppresses toast even for all-null profile', async () => {
    const line = makeLine(2)
    vi.mocked(getElevationProfile).mockResolvedValueOnce({
      profile: makeProfile(line.coordinates, [null, null]),
    })
    // Should not throw
    await expect(getElevationAtVerticesChunked(line, null)).resolves.toBeDefined()
  })
})

// ---------------------------------------------------------------------------
// Task B.2 — computeCumulativeDistancesFromCoords (internal helper)
// ---------------------------------------------------------------------------

describe('computeCumulativeDistancesFromCoords', () => {
  test('index 0 is always 0', () => {
    const coords: GeoJSON.Position[] = [[0, 0], [1, 0], [2, 0]]
    expect(computeCumulativeDistancesFromCoords(coords)[0]).toBe(0)
  })

  test('monotonically increasing for distinct points', () => {
    const coords: GeoJSON.Position[] = [[0, 0], [1, 0], [2, 0]]
    const d = computeCumulativeDistancesFromCoords(coords)
    expect(d[1]).toBeGreaterThan(0)
    expect(d[2]).toBeGreaterThan(d[1])
  })

  test('empty array returns empty', () => {
    expect(computeCumulativeDistancesFromCoords([])).toHaveLength(0)
  })
})

// ---------------------------------------------------------------------------
// TestDemCoverageWarning_ClearedOnLogout — multi-user safety
// ---------------------------------------------------------------------------

describe('clearDemCoverageWarning — logout multi-user safety', () => {
  test('TestDemCoverageWarning_ClearedOnLogout: warning flag set → clearDemCoverageWarning() → toast fires again', async () => {
    vi.resetAllMocks()
    clearDemCoverageWarning()

    const line = makeLine(2)
    const nullProfile = makeProfile(line.coordinates, [null, null])
    vi.mocked(getElevationProfile).mockResolvedValue({ profile: nullProfile })

    const addToast = vi.fn()

    // First call: sets the warned flag → toast fires
    await getElevationAtVerticesChunked(line, addToast)
    expect(addToast).toHaveBeenCalledOnce()

    // Simulate logout: clear the warning flag (production path via BroadcastChannel)
    clearDemCoverageWarning()

    // Second call: flag was reset → toast fires again (User B deserves their own warning)
    await getElevationAtVerticesChunked(line, addToast)
    expect(addToast).toHaveBeenCalledTimes(2)
  })
})
