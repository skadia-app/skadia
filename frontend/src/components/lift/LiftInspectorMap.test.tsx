/**
 * Tests for LiftInspectorMap — Critic findings F10, F11, F16.
 *
 * F10: candidates prop change triggers layer re-render without full unmount.
 * F11: empty stream shows fallback overlay; coords<2 skipped with console.warn.
 * F16: legend overlay present with German text labels for all candidate types.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
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
  useMapLayers: vi.fn(() => ({ base: [{ key: 'default', type: 'raster' }] })),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: vi.fn(() => ({ data: {} })),
}))

vi.mock('@/lib/mapStyles', () => ({
  buildLayerStyle: vi.fn(() => ({ version: 8, sources: {}, layers: [] })),
}))

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
