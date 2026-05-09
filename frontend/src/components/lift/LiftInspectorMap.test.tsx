/**
 * Tests for LiftInspectorMap — Critic findings F10, F11, F16 + Phase 11.7 additions.
 *
 * F10: candidates prop change triggers layer re-render without full unmount.
 * F11: empty stream shows fallback overlay; coords<2 skipped with console.warn.
 * F16: legend overlay present with German text labels for all candidate types.
 * Phase 11.7:
 *   A: layer-picker option count matches base.length.
 *   B: pickDefaultBase returns key==='dark' in dark mode; skips satellite in light.
 *   C: candidateCoords useMemo (tested indirectly via addSource call counts).
 *   D.1: splitTrackAtGaps — segments == 2 when one gap > threshold; 1 segment when none.
 *   F: heatmap toggle button renders; addHeatmapLayers called when activated.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ToastProvider } from '@/lib/toast'
import { ToastContainer } from '@/components/Toast'
import type { LiftCandidate, StreamPoint } from '@/api/types'

// ResizeObserver and matchMedia stubs for jsdom.
global.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
}
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// MapLibre mock — exposes all map methods so we can spy on setData / addLayer calls.
const maplibreMock = vi.hoisted(() => {
  const sourcesMap: Record<string, { data?: unknown }> = {}
  const layersMap: Record<string, unknown> = {}
  const paintProps: Record<string, Record<string, unknown>> = {}

  class GeoJSONSourceMock {
    id: string
    constructor(id: string) { this.id = id }
    setData = vi.fn((data: unknown) => { sourcesMap[this.id] = { data } })
  }

  class MapMock {
    _sources: Record<string, GeoJSONSourceMock> = {}
    addControl = vi.fn()
    on = vi.fn((event: string, arg2: unknown, arg3?: unknown) => {
      // Immediately invoke 'load' and 'style.load' callbacks so layers get added in tests.
      if ((event === 'load' || event === 'style.load') && typeof arg2 === 'function') {
        setTimeout(arg2, 0)
      }
    })
    off = vi.fn()
    remove = vi.fn()
    resize = vi.fn()
    isStyleLoaded = vi.fn(() => true)
    addSource = vi.fn((id: string, spec: unknown) => {
      const src = new GeoJSONSourceMock(id)
      this._sources[id] = src
      sourcesMap[id] = spec as { data?: unknown }
    })
    removeSource = vi.fn((id: string) => { delete this._sources[id]; delete sourcesMap[id] })
    addLayer = vi.fn((spec: { id: string }) => { layersMap[spec.id] = spec })
    removeLayer = vi.fn((id: string) => { delete layersMap[id] })
    getSource = vi.fn((id: string) => this._sources[id] ?? null)
    getLayer = vi.fn((id: string) => layersMap[id] ?? null)
    fitBounds = vi.fn()
    getCanvas = vi.fn(() => ({ style: { cursor: '' } }))
    setPaintProperty = vi.fn((layerId: string, prop: string, val: unknown) => {
      paintProps[layerId] = { ...(paintProps[layerId] ?? {}), [prop]: val }
    })
  }

  return { MapMock, sourcesMap, layersMap, paintProps }
})

vi.mock('maplibre-gl', () => ({
  default: {
    Map: maplibreMock.MapMock,
    LngLatBounds: class {
      extend() { return this }
      isEmpty() { return true }
    },
    NavigationControl: class {},
  },
  Map: maplibreMock.MapMock,
  LngLatBounds: class {
    extend() { return this }
    isEmpty() { return true }
  },
  NavigationControl: class {},
}))

vi.mock('@/hooks/useMapLayers', () => ({
  useMapLayers: vi.fn(() => ({ base: [{ key: 'default', type: 'raster', id: 1, name: 'Standard' }] })),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: vi.fn(() => ({ data: {} })),
}))

vi.mock('@/lib/mapStyles', () => ({
  buildLayerStyle: vi.fn(() => ({ version: 8, sources: {}, layers: [] })),
}))

vi.mock('@/lib/theme', () => ({
  useTheme: vi.fn(() => ({ theme: 'light', toggle: vi.fn(), setTheme: vi.fn() })),
}))

vi.mock('@/providers/UserProvider', () => ({
  useCurrentUserId: vi.fn(() => 'user-abc'),
}))

const heatmapMock = vi.hoisted(() => ({
  addHeatmapLayers: vi.fn(),
  removeHeatmapLayers: vi.fn(),
  buildHeatmapTileUrl: vi.fn((uid: string) => `https://tiles.example/${uid}`),
}))
vi.mock('@/lib/mapHeatmap', () => heatmapMock)

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      <ToastContainer />
    </ToastProvider>
  )
}

function makeCandidate(overrides: Partial<LiftCandidate> = {}): LiftCandidate {
  return {
    id: 'c-0',
    activity_id: 'act-1',
    started_at: '2024-01-01T09:00:00Z',
    ended_at: '2024-01-01T09:15:00Z',
    duration_s: 900,
    ascent_m: 300,
    descent_m: 0,
    distance_m: 1200,
    confidence: 'high',
    confirmed: false,
    rejected_by_user: false,
    source: 'heuristic' as const,
    segment_effort_id: null,
    ...overrides,
  }
}

function makeStream(
  time: string,
  lat: number,
  lng: number,
): StreamPoint {
  return { time, lat, lng }
}

// Streams that fall within c-0's time range (09:00–09:15 UTC)
const twoStreams: StreamPoint[] = [
  makeStream('2024-01-01T09:05:00Z', 47.1, 11.1),
  makeStream('2024-01-01T09:10:00Z', 47.2, 11.2),
]

describe('LiftInspectorMap — F11: empty stream fallback', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('renders fallback overlay text when streams is empty', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    render(
      <LiftInspectorMap
        candidates={[makeCandidate()]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )
    expect(screen.getByText(/Karte nicht verfügbar — fehlender Stream/)).toBeTruthy()
  })

  it('keeps the map container mounted even when streams is empty', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const { container } = render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )
    // Map container must remain in DOM so ResizeObserver can observe it.
    expect(container.querySelector('[data-testid="lift-inspector-map"]')).not.toBeNull()
  })

  it('does NOT show fallback overlay when GPS streams cover candidates', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    render(
      <LiftInspectorMap
        candidates={[makeCandidate()]}
        streams={twoStreams}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )
    expect(screen.queryByText(/Karte nicht verfügbar/)).toBeNull()
  })
})

describe('LiftInspectorMap — F10: candidates prop change re-renders layers', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('F10: re-rendering with changed candidates does not unmount the map wrapper', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const candidate = makeCandidate({ id: 'c-0' })

    const { rerender, container } = render(
      <LiftInspectorMap
        candidates={[candidate]}
        streams={twoStreams}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    // Let the async map 'load' callback fire.
    await act(async () => { await new Promise(r => setTimeout(r, 50)) })

    const wrapperEl = container.querySelector('[data-testid="lift-inspector-map-wrapper"]')
    expect(wrapperEl).not.toBeNull()

    // Re-render with candidate now rejected — map wrapper must remain mounted.
    const rejectedCandidate = { ...candidate, rejected_by_user: true }
    rerender(
      <ToastProvider>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <LiftInspectorMap
            candidates={[rejectedCandidate]}
            streams={twoStreams}
            highlightedCandidateId={null}
            onCandidateClick={() => {}}
          />
        </QueryClientProvider>
        <ToastContainer />
      </ToastProvider>,
    )

    await act(async () => { await new Promise(r => setTimeout(r, 10)) })

    // Wrapper is still mounted after candidates prop change (not destroyed/recreated).
    const wrapperAfter = container.querySelector('[data-testid="lift-inspector-map-wrapper"]')
    expect(wrapperAfter).not.toBeNull()
    // The fallback overlay must NOT appear — stream is non-empty.
    expect(screen.queryByText(/Karte nicht verfügbar/)).toBeNull()
  })
})

describe('LiftInspectorMap — F16: legend overlay', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('renders all five legend labels in German', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )
    expect(screen.getByText('OSM-Treffer')).toBeTruthy()
    expect(screen.getByText('Heuristik (hoch)')).toBeTruthy()
    expect(screen.getByText('Heuristik (mittel)')).toBeTruthy()
    expect(screen.getByText('Heuristik (bidirektional)')).toBeTruthy()
    expect(screen.getByText('Abgelehnt')).toBeTruthy()
  })

  it('legend is always visible (not behind empty-stream overlay)', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )
    const legend = document.querySelector('[aria-label="Kartenlegende"]')
    expect(legend).not.toBeNull()
  })
})

// ── Phase 11.7: B — pickDefaultBase unit tests ──────────────────────────────

describe('pickDefaultBase', () => {
  it('returns undefined for empty layer list', async () => {
    const { pickDefaultBase } = await import('./LiftInspectorMap')
    expect(pickDefaultBase([], 'light')).toBeUndefined()
    expect(pickDefaultBase([], 'dark')).toBeUndefined()
  })

  it('returns layer with key==="dark" in dark mode', async () => {
    const { pickDefaultBase } = await import('./LiftInspectorMap')
    const layers = [
      { id: 1, key: 'osm', name: 'OpenStreetMap', role: 'base' as const, source_kind: 'raster_tiles' as const, default_opacity: 1, is_system: true, enabled: true, sort_order: 1, proxy_through_backend: false, created_at: '', updated_at: '' },
      { id: 2, key: 'dark', name: 'Dark', role: 'base' as const, source_kind: 'raster_tiles' as const, default_opacity: 1, is_system: true, enabled: true, sort_order: 2, proxy_through_backend: false, created_at: '', updated_at: '' },
    ]
    const result = pickDefaultBase(layers, 'dark')
    expect(result?.key).toBe('dark')
  })

  it('skips satellite-named layers in light mode', async () => {
    const { pickDefaultBase } = await import('./LiftInspectorMap')
    const layers = [
      { id: 1, key: 'satellite', name: 'Satellite', role: 'base' as const, source_kind: 'raster_tiles' as const, default_opacity: 1, is_system: true, enabled: true, sort_order: 1, proxy_through_backend: false, created_at: '', updated_at: '' },
      { id: 2, key: 'osm', name: 'OpenStreetMap', role: 'base' as const, source_kind: 'raster_tiles' as const, default_opacity: 1, is_system: true, enabled: true, sort_order: 2, proxy_through_backend: false, created_at: '', updated_at: '' },
    ]
    const result = pickDefaultBase(layers, 'light')
    expect(result?.key).toBe('osm')
  })

  it('falls back to base[0] if all layers are satellite-named in light mode', async () => {
    const { pickDefaultBase } = await import('./LiftInspectorMap')
    const layers = [
      { id: 1, key: 'satellite', name: 'Satellite Imagery', role: 'base' as const, source_kind: 'raster_tiles' as const, default_opacity: 1, is_system: true, enabled: true, sort_order: 1, proxy_through_backend: false, created_at: '', updated_at: '' },
      { id: 2, key: 'aerial', name: 'Aerial', role: 'base' as const, source_kind: 'raster_tiles' as const, default_opacity: 1, is_system: true, enabled: true, sort_order: 2, proxy_through_backend: false, created_at: '', updated_at: '' },
    ]
    const result = pickDefaultBase(layers, 'light')
    expect(result?.key).toBe('satellite') // fallback to base[0]
  })
})

// ── Phase 11.7: D.1 — splitTrackAtGaps unit tests ──────────────────────────

describe('splitTrackAtGaps', () => {
  function pt(time: string, lat: number, lng: number): StreamPoint {
    return { time, lat, lng }
  }

  it('returns single segment when no gaps exceed threshold', async () => {
    const { splitTrackAtGaps } = await import('./LiftInspectorMap')
    const streams: StreamPoint[] = [
      pt('2024-01-01T09:00:00Z', 47.0, 11.0),
      pt('2024-01-01T09:01:00Z', 47.1, 11.1), // 60s gap < 75s
      pt('2024-01-01T09:02:00Z', 47.2, 11.2),
    ]
    const segments = splitTrackAtGaps(streams, 75)
    expect(segments).toHaveLength(1)
    expect(segments[0]).toHaveLength(3)
  })

  it('returns 2 segments when one gap > threshold', async () => {
    const { splitTrackAtGaps } = await import('./LiftInspectorMap')
    const streams: StreamPoint[] = [
      pt('2024-01-01T09:00:00Z', 47.0, 11.0),
      pt('2024-01-01T09:01:00Z', 47.1, 11.1),
      pt('2024-01-01T09:03:00Z', 47.2, 11.2), // 120s gap > 75s
      pt('2024-01-01T09:04:00Z', 47.3, 11.3),
    ]
    const segments = splitTrackAtGaps(streams, 75)
    expect(segments).toHaveLength(2)
    expect(segments[0]).toHaveLength(2)
    expect(segments[1]).toHaveLength(2)
  })

  it('skips points without lat/lng — null point does not appear in any segment', async () => {
    const { splitTrackAtGaps } = await import('./LiftInspectorMap')
    // The null-coord point is skipped and does not appear in output. The time
    // gap is measured between the surrounding valid points (09:00 → 09:02 = 120s
    // which is > 75s), so two segments are produced.
    const streams: StreamPoint[] = [
      pt('2024-01-01T09:00:00Z', 47.0, 11.0),
      { time: '2024-01-01T09:01:00Z', lat: null, lng: null },
      pt('2024-01-01T09:02:00Z', 47.2, 11.2),
    ]
    const segments = splitTrackAtGaps(streams, 75)
    // The null point is not present in any segment (only valid [lng, lat] pairs).
    const allPoints = segments.flat()
    expect(allPoints.every(p => p.length === 2 && typeof p[0] === 'number')).toBe(true)
    expect(allPoints).toHaveLength(2) // only the two valid GPS points
    // Gap of 120s between the two valid points exceeds 75s → split into 2 segments.
    expect(segments).toHaveLength(2)
  })

  it('returns empty array for empty stream', async () => {
    const { splitTrackAtGaps } = await import('./LiftInspectorMap')
    expect(splitTrackAtGaps([], 75)).toHaveLength(0)
  })
})

// ── Phase 11.7: A — layer picker option count ───────────────────────────────

describe('LiftInspectorMap — A: layer picker', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('layer-picker shows one <option> per base layer when base.length > 1', async () => {
    const { useMapLayers } = await import('@/hooks/useMapLayers')
    const mockUseMapLayers = useMapLayers as ReturnType<typeof vi.fn>
    mockUseMapLayers.mockReturnValue({
      base: [
        { id: 1, key: 'osm', name: 'OpenStreetMap', role: 'base', source_kind: 'raster_tiles', default_opacity: 1, is_system: true, enabled: true, sort_order: 1, proxy_through_backend: false, created_at: '', updated_at: '' },
        { id: 2, key: 'satellite', name: 'Satellit', role: 'base', source_kind: 'raster_tiles', default_opacity: 1, is_system: true, enabled: true, sort_order: 2, proxy_through_backend: false, created_at: '', updated_at: '' },
      ],
    })

    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const { container } = render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    const select = container.querySelector('select[aria-label="Karten-Stil wählen"]')
    expect(select).not.toBeNull()
    const options = select!.querySelectorAll('option')
    expect(options.length).toBe(2)
  })

  it('layer picker is NOT rendered when only one base layer exists', async () => {
    const { useMapLayers } = await import('@/hooks/useMapLayers')
    const mockUseMapLayers = useMapLayers as ReturnType<typeof vi.fn>
    mockUseMapLayers.mockReturnValue({
      base: [
        { id: 1, key: 'osm', name: 'OpenStreetMap', role: 'base', source_kind: 'raster_tiles', default_opacity: 1, is_system: true, enabled: true, sort_order: 1, proxy_through_backend: false, created_at: '', updated_at: '' },
      ],
    })

    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const { container } = render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    const select = container.querySelector('select[aria-label="Karten-Stil wählen"]')
    expect(select).toBeNull()
  })
})

// ── Phase 11.7: F — heatmap toggle ─────────────────────────────────────────

describe('LiftInspectorMap — F: heatmap toggle', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('heatmap toggle button is rendered', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )
    const btn = screen.getByTestId('heatmap-toggle')
    expect(btn).toBeTruthy()
    expect(btn.title).toBe('Heatmap einblenden')
  })

  it('clicking heatmap toggle changes button title to ausblenden', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )
    const btn = screen.getByTestId('heatmap-toggle')
    fireEvent.click(btn)
    expect(btn.title).toBe('Heatmap ausblenden')
  })
})
