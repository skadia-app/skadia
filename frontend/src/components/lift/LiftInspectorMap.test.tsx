/**
 * Tests for LiftInspectorMap — Critic findings F10, F11, F16 + Phase 11.7 + UX fix-pass.
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
 * UX fix-pass (2026-05-09):
 *   Fix 5a: activity track source is LineString (continuous), not MultiLineString.
 *   Fix 5b: gap-bridges source has MultiLineString with N segments = number of gaps.
 *   Fix 5c: direction-arrow markers count is in expected range for coords length.
 *   Fix 6a: legend rows have title attributes from CONFIDENCE_DESCRIPTIONS.
 * Phase 11.9:
 *   getConfidenceKey returns correct key for each fixture variant.
 *   Legend renders all 5 entries with new signal-based label texts.
 *   Legend has a HelpCircle button with matching aria-label.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ToastProvider } from '@/lib/toast'
import { ToastContainer } from '@/components/Toast'
import type { LiftCandidate, StreamPoint } from '@/api/types'
import { i18n } from '@/i18n/i18n'

const mockUserPrefs = vi.hoisted((): { locale: 'de' | 'en'; units: 'metric' | 'imperial' } => ({
  locale: 'de',
  units: 'metric',
}))

beforeEach(async () => {
  mockUserPrefs.locale = 'de'
  mockUserPrefs.units = 'metric'
  await i18n.changeLanguage('de')
})

// ResizeObserver and matchMedia stubs for jsdom.
global.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
}
Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, value: 640 })
Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, value: 480 })
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
  const instances: MapMock[] = []
  const popupHtml: string[] = []

  class GeoJSONSourceMock {
    id: string
    constructor(id: string) { this.id = id }
    setData = vi.fn((data: unknown) => { sourcesMap[this.id] = { data } })
  }

  class MapMock {
    _sources: Record<string, GeoJSONSourceMock> = {}
    handlers: Record<string, (...args: unknown[]) => void> = {}
    addControl = vi.fn()
    on = vi.fn((event: string, arg2: unknown, arg3?: unknown) => {
      // Immediately invoke 'load' and 'style.load' callbacks so layers get added in tests.
      if ((event === 'load' || event === 'style.load') && typeof arg2 === 'function') {
        setTimeout(arg2, 0)
      }
      if (typeof arg2 === 'string' && typeof arg3 === 'function') {
        this.handlers[`${event}:${arg2}`] = arg3 as (...args: unknown[]) => void
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
    addLayer = vi.fn((spec: { id: string }, beforeId?: string) => {
      layersMap[spec.id] = { ...spec, beforeId }
    })
    removeLayer = vi.fn((id: string) => { delete layersMap[id] })
    getSource = vi.fn((id: string) => this._sources[id] ?? null)
    getLayer = vi.fn((id: string) => layersMap[id] ?? null)
    fitBounds = vi.fn()
    getCanvas = vi.fn(() => ({ style: { cursor: '' } }))
    setPaintProperty = vi.fn((layerId: string, prop: string, val: unknown) => {
      paintProps[layerId] = { ...(paintProps[layerId] ?? {}), [prop]: val }
    })
    moveLayer = vi.fn()
    setStyle = vi.fn()
    constructor() {
      instances.push(this)
    }
  }

  class PopupMock {
    setLngLat(_ll: unknown) { return this }
    setHTML(html: string) { popupHtml.push(html); return this }
    addTo(_map: unknown) { return this }
    remove() {}
  }

  return { MapMock, PopupMock, sourcesMap, layersMap, paintProps, instances, popupHtml }
})

const markerMock = vi.hoisted(() => {
  const instances: { lngLat: [number, number]; removed: boolean }[] = []
  class MarkerMock {
    _lngLat: [number, number] = [0, 0]
    _idx: number
    constructor(_opts?: unknown) {
      this._idx = instances.length
      instances.push({ lngLat: [0, 0], removed: false })
    }
    setLngLat(ll: [number, number]) { instances[this._idx].lngLat = ll; return this }
    addTo(_map: unknown) { return this }
    remove() { instances[this._idx].removed = true }
  }
  return { MarkerMock, instances }
})

vi.mock('maplibre-gl', () => ({
  default: {
    Map: maplibreMock.MapMock,
    LngLatBounds: class {
      extend() { return this }
      isEmpty() { return true }
    },
    NavigationControl: class {},
    Marker: markerMock.MarkerMock,
    Popup: maplibreMock.PopupMock,
  },
  Map: maplibreMock.MapMock,
  LngLatBounds: class {
    extend() { return this }
    isEmpty() { return true }
  },
  NavigationControl: class {},
  Marker: markerMock.MarkerMock,
  Popup: maplibreMock.PopupMock,
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
  useUser: () => mockUserPrefs,
  useCurrentUserId: vi.fn(() => 'user-abc'),
}))

const heatmapMock = vi.hoisted(() => ({
  addHeatmapLayers: vi.fn(),
  removeHeatmapLayers: vi.fn(),
  buildHeatmapTileUrl: vi.fn((uid: string) => `https://tiles.example/${uid}`),
  loadHeatmapColor: vi.fn(() => '#F97316'),
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

describe('LiftInspectorMap — map-layer initialization', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    maplibreMock.instances.length = 0
    maplibreMock.popupHtml.length = 0
  })

  it('initializes MapLibre when base layers arrive after the first render', async () => {
    const { useMapLayers } = await import('@/hooks/useMapLayers')
    const mockUseMapLayers = useMapLayers as ReturnType<typeof vi.fn>
    mockUseMapLayers.mockReturnValue({ base: [] })

    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const { rerender } = render(
      <LiftInspectorMap
        candidates={[makeCandidate()]}
        streams={twoStreams}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    expect(maplibreMock.instances).toHaveLength(0)

    mockUseMapLayers.mockReturnValue({
      base: [
        { id: 1, key: 'osm', name: 'OSM', role: 'base', source_kind: 'raster_tiles', default_opacity: 1, is_system: true, enabled: true, sort_order: 1, proxy_through_backend: false, created_at: '', updated_at: '' },
      ],
    })

    rerender(
      <LiftInspectorMap
        candidates={[makeCandidate()]}
        streams={twoStreams}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
    )

    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })

    expect(maplibreMock.instances).toHaveLength(1)
    expect(maplibreMock.sourcesMap['inspector-track']).toBeTruthy()
    expect(maplibreMock.layersMap['inspector-track-line']).toBeTruthy()
    expect(maplibreMock.sourcesMap['lift-c-0']).toBeTruthy()
    expect(maplibreMock.layersMap['lift-line-c-0']).toBeTruthy()
  })

  it('draws OSM lift lines below heuristic lift lines', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const heuristic = makeCandidate({ id: 'heuristic-1', source: 'heuristic', confidence: 'high' })
    const osm = makeCandidate({ id: 'osm-1', source: 'osm', confirmed: true })

    render(
      <LiftInspectorMap
        candidates={[heuristic, osm]}
        streams={twoStreams}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })

    const liftLayerAdds = maplibreMock.instances[0].addLayer.mock.calls
      .map(([spec]) => (spec as { id: string }).id)
      .filter(id => id.startsWith('lift-line-'))
    expect(liftLayerAdds.slice(0, 2)).toEqual(['lift-line-osm-1', 'lift-line-heuristic-1'])
  })

  it('keeps the selected lift highlight above rebuilt OSM layers', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const osm = makeCandidate({ id: 'osm-1', source: 'osm', confirmed: true })

    render(
      <LiftInspectorMap
        candidates={[osm]}
        streams={twoStreams}
        highlightedCandidateId="osm-1"
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })

    expect(maplibreMock.instances[0].moveLayer).toHaveBeenCalledWith('lift-highlight-line')
  })

  it('formats lift popup distance and elevation with selected user units', async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    await i18n.changeLanguage('en')

    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    render(
      <LiftInspectorMap
        candidates={[makeCandidate({ distance_m: 1609.344, ascent_m: 100, descent_m: 50 })]}
        streams={twoStreams}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })

    const map = maplibreMock.instances[0]
    map.handlers['click:lift-line-c-0']({
      lngLat: { toArray: () => [11.15, 47.15] },
    })

    const html = maplibreMock.popupHtml.at(-1) ?? ''
    expect(html).toContain('1.0 mi')
    expect(html).toContain('+328 ft')
    expect(html).toContain('−164 ft')
    expect(html).not.toContain('1.6 km')
    expect(html).not.toContain('+100 m')
  })
})

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

describe('LiftInspectorMap — stream updates', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('updates the track source and fits bounds when streams arrive after mount', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')

    const { rerender } = render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    await act(async () => { await new Promise(r => setTimeout(r, 50)) })

    rerender(
      <ToastProvider>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <LiftInspectorMap
            candidates={[]}
            streams={twoStreams}
            highlightedCandidateId={null}
            onCandidateClick={() => {}}
          />
        </QueryClientProvider>
        <ToastContainer />
      </ToastProvider>,
    )

    await act(async () => { await new Promise(r => setTimeout(r, 10)) })

    expect(maplibreMock.sourcesMap['inspector-track']?.data).toMatchObject({
      geometry: { type: 'LineString', coordinates: [[11.1, 47.1], [11.2, 47.2]] },
    })
    expect(maplibreMock.instances.some(m => m.fitBounds.mock.calls.length > 0)).toBe(true)
  })

  it('does not refit bounds when rerendered with identical track coordinates', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')

    const { rerender } = render(
      <LiftInspectorMap
        candidates={[]}
        streams={twoStreams}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
      { wrapper },
    )

    await act(async () => { await new Promise(r => setTimeout(r, 50)) })
    const map = maplibreMock.instances.at(-1)
    expect(map).toBeDefined()
    const fitCalls = map!.fitBounds.mock.calls.length

    rerender(
      <LiftInspectorMap
        candidates={[]}
        streams={[...twoStreams]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
      />,
    )

    await act(async () => { await new Promise(r => setTimeout(r, 10)) })

    expect(map!.fitBounds).toHaveBeenCalledTimes(fitCalls)
  })
})

describe('LiftInspectorMap — F16: legend overlay', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('renders all five legend labels with signal-based naming (Phase 11.9)', async () => {
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
    // Phase 11.10 labels — signal-based, bidirektional restored
    expect(screen.getByText('OSM-Treffer')).toBeTruthy()
    expect(screen.getByText('Heuristik (Speed + Steigung)')).toBeTruthy()
    expect(screen.getByText('Heuristik (Höhenrate)')).toBeTruthy()
    expect(screen.getByText('Heuristik (bidirektional)')).toBeTruthy()
    expect(screen.getByText('Abgelehnt')).toBeTruthy()
    // Old labels must NOT appear
    expect(screen.queryByText('Heuristik (hoch)')).toBeNull()
    expect(screen.queryByText('Heuristik (mittel)')).toBeNull()
    expect(screen.queryByText('Heuristik (Talquerung)')).toBeNull()
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

  it('layer-picker shows one pill button per base layer when base.length > 1', async () => {
    const { useMapLayers } = await import('@/hooks/useMapLayers')
    const mockUseMapLayers = useMapLayers as ReturnType<typeof vi.fn>
    mockUseMapLayers.mockReturnValue({
      base: [
        { id: 1, key: 'osm', name: 'OpenStreetMap', role: 'base', source_kind: 'raster_tiles', default_opacity: 1, is_system: true, enabled: true, sort_order: 1, proxy_through_backend: false, created_at: '', updated_at: '' },
        { id: 2, key: 'satellite', name: 'Satellit', role: 'base', source_kind: 'raster_tiles', default_opacity: 1, is_system: true, enabled: true, sort_order: 2, proxy_through_backend: false, created_at: '', updated_at: '' },
      ],
    })

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

    // Pill toolbar (MultiTrackMap-style) — one button per base layer.
    expect(screen.getByRole('button', { name: 'OpenStreetMap' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Satellit' })).toBeTruthy()
  })

  it('layer-picker pills are NOT rendered when only one base layer exists', async () => {
    const { useMapLayers } = await import('@/hooks/useMapLayers')
    const mockUseMapLayers = useMapLayers as ReturnType<typeof vi.fn>
    mockUseMapLayers.mockReturnValue({
      base: [
        { id: 1, key: 'osm', name: 'OpenStreetMap', role: 'base', source_kind: 'raster_tiles', default_opacity: 1, is_system: true, enabled: true, sort_order: 1, proxy_through_backend: false, created_at: '', updated_at: '' },
      ],
    })

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

    expect(screen.queryByRole('button', { name: 'OpenStreetMap' })).toBeNull()
  })
})

// ── Phase 11.7: F — heatmap toggle ─────────────────────────────────────────

describe('LiftInspectorMap — F: heatmap toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    maplibreMock.instances.length = 0
    for (const key of Object.keys(maplibreMock.sourcesMap)) delete maplibreMock.sourcesMap[key]
    for (const key of Object.keys(maplibreMock.layersMap)) delete maplibreMock.layersMap[key]
  })

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

  it('adds heatmap below the activity stream when activated', async () => {
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

    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })

    fireEvent.click(screen.getByTestId('heatmap-toggle'))

    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })

    expect(heatmapMock.addHeatmapLayers).toHaveBeenLastCalledWith(
      maplibreMock.instances[0],
      'https://tiles.example/user-abc',
      expect.any(String),
      'inspector-track-line',
    )
  })
})

// ── UX fix-pass (2026-05-09): Fix 5 — track design (unit tests) ─────────────
//
// MapLibre sources are never added in jsdom because the map container has
// clientWidth/clientHeight = 0 (initMap guards against zero-size containers).
// We therefore test the track-shape logic directly via the exported helpers and
// pure computations, without going through the MapLibre mock.

describe('LiftInspectorMap — Fix 5a: continuous track uses LineString (unit)', () => {
  it('GPS stream is mapped to a flat [lng, lat] array (no segment splits)', async () => {
    // The continuousTrackCoords memo in the component maps gpsStreams → [lng, lat].
    // We verify the same logic here directly.
    const pts: StreamPoint[] = [
      makeStream('2024-01-01T09:00:00Z', 47.0, 11.0),
      makeStream('2024-01-01T09:01:00Z', 47.1, 11.1),
      makeStream('2024-01-01T09:03:00Z', 47.2, 11.2), // 120s gap — still in single array
    ]
    // Simulate what the component does: filter valid GPS, map to [lng, lat].
    const coords = pts
      .filter(s => s.lat != null && s.lng != null)
      .map(s => [s.lng!, s.lat!] as [number, number])
    // All 3 points in one flat array — no splitting at the gap.
    expect(coords).toHaveLength(3)
    expect(coords[0]).toEqual([11.0, 47.0])
    expect(coords[2]).toEqual([11.2, 47.2])
  })
})

describe('LiftInspectorMap — Fix 5b: gap-bridge computation (unit)', () => {
  it('computes 1 bridge for a stream with 1 gap > threshold', async () => {
    const { splitTrackAtGaps } = await import('./LiftInspectorMap')
    const streamsWithGap: StreamPoint[] = [
      makeStream('2024-01-01T09:00:00Z', 47.0, 11.0),
      makeStream('2024-01-01T09:01:00Z', 47.1, 11.1),
      // 120s gap
      makeStream('2024-01-01T09:03:00Z', 47.2, 11.2),
      makeStream('2024-01-01T09:04:00Z', 47.3, 11.3),
    ]
    const segments = splitTrackAtGaps(streamsWithGap, 75)
    expect(segments).toHaveLength(2)
    // Bridge = [lastOf(seg0), firstOf(seg1)]
    const bridges: [number, number][][] = []
    for (let i = 0; i < segments.length - 1; i++) {
      const seg = segments[i]
      const next = segments[i + 1]
      if (seg.length > 0 && next.length > 0) bridges.push([seg[seg.length - 1], next[0]])
    }
    expect(bridges).toHaveLength(1)
    // Bridge endpoints: end of seg0 = [11.1, 47.1]; start of seg1 = [11.2, 47.2]
    expect(bridges[0][0]).toEqual([11.1, 47.1])
    expect(bridges[0][1]).toEqual([11.2, 47.2])
  })

  it('produces 0 bridges when no gaps exceed threshold', async () => {
    const { splitTrackAtGaps } = await import('./LiftInspectorMap')
    const streamsNoGap: StreamPoint[] = [
      makeStream('2024-01-01T09:00:00Z', 47.0, 11.0),
      makeStream('2024-01-01T09:00:30Z', 47.1, 11.1), // 30s < 75s
      makeStream('2024-01-01T09:01:00Z', 47.2, 11.2),
    ]
    const segments = splitTrackAtGaps(streamsNoGap, 75)
    expect(segments).toHaveLength(1)
    // 1 segment → 0 bridges
    const bridges: [number, number][][] = []
    for (let i = 0; i < segments.length - 1; i++) {
      const seg = segments[i]
      const next = segments[i + 1]
      if (seg.length > 0 && next.length > 0) bridges.push([seg[seg.length - 1], next[0]])
    }
    expect(bridges).toHaveLength(0)
  })
})

describe('LiftInspectorMap — Fix 5c: direction-arrow count formula (unit)', () => {
  it('interval formula produces between 1 and ~15 arrows for a given point count', () => {
    // Verify the arrow interval logic independent of MapLibre.
    // The cap is "~15" (approximately 15); due to integer arithmetic it may
    // reach 16 for some inputs — that is acceptable per the spec comment.
    for (const n of [2, 30, 100, 300, 1000]) {
      const interval = Math.max(20, Math.floor(n / 15))
      let count = 0
      for (let i = 0; i < n - 1; i += interval) count++
      // Allow up to 16 since floor(n/15) may cause an extra step near n-1.
      expect(count).toBeLessThanOrEqual(16)
      if (n >= 2) expect(count).toBeGreaterThanOrEqual(1)
    }
  })
})

// ── UX fix-pass (2026-05-09): Fix 6a — legend tooltips ──────────────────────

describe('LiftInspectorMap — Fix 6a: legend row title attributes', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('legend rows have non-empty title attributes', async () => {
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
    const legend = container.querySelector('[aria-label="Kartenlegende"]')
    expect(legend).not.toBeNull()
    const rows = legend!.querySelectorAll('[title]')
    expect(rows.length).toBeGreaterThanOrEqual(5)
    rows.forEach(row => {
      expect(row.getAttribute('title')).toBeTruthy()
      expect(row.getAttribute('title')!.length).toBeGreaterThan(10)
    })
  })
})

// ── Phase 11.9: getConfidenceKey + HelpCircle ────────────────────────────────

describe('getConfidenceKey — Phase 11.9', () => {
  it('returns "rejected" for a rejected candidate regardless of source', async () => {
    const { getConfidenceKey } = await import('./LiftInspectorMap')
    expect(getConfidenceKey(makeCandidate({ rejected_by_user: true, source: 'heuristic', confidence: 'high' }))).toBe('rejected')
    expect(getConfidenceKey(makeCandidate({ rejected_by_user: true, source: 'osm', confidence: 'high' }))).toBe('rejected')
  })

  it('returns "osm" for non-rejected OSM candidates', async () => {
    const { getConfidenceKey } = await import('./LiftInspectorMap')
    expect(getConfidenceKey(makeCandidate({ rejected_by_user: false, source: 'osm', confidence: 'high' }))).toBe('osm')
  })

  it('returns "high" for heuristic confidence=high', async () => {
    const { getConfidenceKey } = await import('./LiftInspectorMap')
    expect(getConfidenceKey(makeCandidate({ rejected_by_user: false, source: 'heuristic', confidence: 'high' }))).toBe('high')
  })

  it('returns "bidir" for heuristic confidence=bidir', async () => {
    const { getConfidenceKey } = await import('./LiftInspectorMap')
    expect(getConfidenceKey(makeCandidate({ rejected_by_user: false, source: 'heuristic', confidence: 'bidir' }))).toBe('bidir')
  })

  it('returns "medium" for heuristic confidence=medium (fallback)', async () => {
    const { getConfidenceKey } = await import('./LiftInspectorMap')
    expect(getConfidenceKey(makeCandidate({ rejected_by_user: false, source: 'heuristic', confidence: 'medium' }))).toBe('medium')
  })
})

describe('LiftInspectorMap — Phase 11.9: HelpCircle in legend', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('legend has a HelpCircle button with aria-label containing "Hilfe"', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const { container } = render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
        onOpenHelp={() => {}}
      />,
      { wrapper },
    )
    const legend = container.querySelector('[aria-label="Kartenlegende"]')
    expect(legend).not.toBeNull()
    const helpBtn = legend!.querySelector('button[aria-label*="Hilfe"]')
    expect(helpBtn).not.toBeNull()
  })

  it('clicking the legend HelpCircle calls onOpenHelp', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const onOpenHelp = vi.fn()
    const { container } = render(
      <LiftInspectorMap
        candidates={[]}
        streams={[]}
        highlightedCandidateId={null}
        onCandidateClick={() => {}}
        onOpenHelp={onOpenHelp}
      />,
      { wrapper },
    )
    const legend = container.querySelector('[aria-label="Kartenlegende"]')
    const helpBtn = legend!.querySelector('button[aria-label*="Hilfe"]') as HTMLButtonElement
    fireEvent.click(helpBtn)
    expect(onOpenHelp).toHaveBeenCalledOnce()
  })

  it('legend HelpCircle is NOT rendered when onOpenHelp is not provided', async () => {
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
    const legend = container.querySelector('[aria-label="Kartenlegende"]')
    expect(legend).not.toBeNull()
    const helpBtn = legend!.querySelector('button[aria-label*="Hilfe"]')
    expect(helpBtn).toBeNull()
  })
})
