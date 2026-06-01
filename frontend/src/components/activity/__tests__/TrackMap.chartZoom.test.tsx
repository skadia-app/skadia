import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TrackMap from '../TrackMap'
import type { StreamPoint } from '@/api/types'
import type { ZoomRange } from '@/components/ActivityCharts'
import { i18n } from '@/i18n/i18n'

const mapMocks = vi.hoisted(() => {
  const instances: FakeMap[] = []
  const popupSetHtml = vi.fn()
  class FakeBounds {
    extend = vi.fn(() => this)
  }
  class FakeMap {
    sources = new Set<string>()
    layers = new Set<string>()
    loadHandlers: Array<() => void> = []
    onceHandlers = new Map<string, Array<() => void>>()
    addControl = vi.fn()
    remove = vi.fn()
    isStyleLoaded = vi.fn(() => false)
    hasImage = vi.fn(() => false)
    addImage = vi.fn()
    fitBounds = vi.fn()
    setLayoutProperty = vi.fn()
    setPaintProperty = vi.fn()
    setFilter = vi.fn()
    setFeatureState = vi.fn()
    removeFeatureState = vi.fn()
    getStyle = vi.fn(() => ({ version: 8 }))
    getCanvas = vi.fn(() => ({ style: { cursor: '' } }))
    layerHandlers = new Map<string, Array<(event: unknown) => void>>()
    on = vi.fn((event: string, layerOrCallback: string | (() => void), maybeCallback?: (event: unknown) => void) => {
      if (event === 'load' && typeof layerOrCallback === 'function') this.loadHandlers.push(layerOrCallback)
      if (event === 'click' && typeof layerOrCallback === 'string' && maybeCallback) {
        const handlers = this.layerHandlers.get(layerOrCallback) ?? []
        handlers.push(maybeCallback)
        this.layerHandlers.set(layerOrCallback, handlers)
      }
      return this
    })
    once = vi.fn((event: string, callback: () => void) => {
      const handlers = this.onceHandlers.get(event) ?? []
      handlers.push(callback)
      this.onceHandlers.set(event, handlers)
      return this
    })
    off = vi.fn()
    addSource = vi.fn((id: string) => {
      this.sources.add(id)
    })
    getSource = vi.fn((id: string) => {
      if (!this.sources.has(id)) return undefined
      return { setData: vi.fn() }
    })
    addLayer = vi.fn((layer: { id: string }) => {
      this.layers.add(layer.id)
    })
    getLayer = vi.fn((id: string) => this.layers.has(id))
    trigger(event: string) {
      if (event === 'load') {
        for (const callback of this.loadHandlers) callback()
      }
      const onceHandlers = this.onceHandlers.get(event) ?? []
      this.onceHandlers.delete(event)
      for (const callback of onceHandlers) callback()
    }
    triggerLayerClick(layer: string, properties: Record<string, unknown>) {
      const handlers = this.layerHandlers.get(layer) ?? []
      for (const callback of handlers) {
        callback({
          features: [{ properties }],
          lngLat: { lng: 11, lat: 47 },
        })
      }
    }
  }
  return { instances, popupSetHtml, FakeMap, FakeBounds }
})

vi.mock('maplibre-gl', () => ({
  default: {
    Map: class extends mapMocks.FakeMap {
      constructor() {
        super()
        mapMocks.instances.push(this)
      }
    },
    Marker: class {
      setLngLat = vi.fn(() => this)
      addTo = vi.fn(() => this)
      remove = vi.fn()
    },
    Popup: class {
      setLngLat = vi.fn(() => this)
      setHTML = mapMocks.popupSetHtml.mockImplementation(() => this)
      addTo = vi.fn(() => this)
      remove = vi.fn()
    },
    NavigationControl: class {},
    LngLatBounds: mapMocks.FakeBounds,
  },
}))

vi.mock('@/api/activityPhotos', () => ({
  listActivityPhotos: vi.fn(async () => []),
}))

vi.mock('@/api/client', () => ({
  apiDownload: vi.fn(),
}))

vi.mock('@/api/trainingZones', () => ({
  fetchTrainingZones: vi.fn(async () => ({ sports: [] })),
}))

vi.mock('@/api/settings', () => ({
  getHeatmapSettings: vi.fn(async () => ({ teleport_filter_activity: false })),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

const streams: StreamPoint[] = [
  { time: '2026-01-01T00:00:00Z', lat: 47.0, lng: 11.0, distance_m: 0 },
  { time: '2026-01-01T00:01:00Z', lat: 47.1, lng: 11.1, distance_m: 1000 },
  { time: '2026-01-01T00:02:00Z', lat: 47.2, lng: 11.2, distance_m: 2000 },
  { time: '2026-01-01T00:03:00Z', lat: 47.3, lng: 11.3, distance_m: 3000 },
]

function renderTrackMap(zoomRange: ZoomRange | null, hasGps = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <TrackMap
        streams={streams}
        hoverIndex={null}
        sportColor="#f97316"
        sportType="running"
        hasGps={hasGps}
        zoomRange={zoomRange}
        onMaximize={vi.fn()}
      />
    </QueryClientProvider>,
  )
}

describe('TrackMap chart zoom coupling', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('fits the selected chart range even when MapLibre reports isStyleLoaded=false after load', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
    mapMocks.instances.length = 0
    const { rerender } = renderTrackMap(null)
    const map = mapMocks.instances[0]

    await act(async () => {
      map.trigger('load')
    })
    map.fitBounds.mockClear()

    const zoomRange: ZoomRange = {
      startIdx: 1,
      endIdx: 2,
      startKm: 1,
      endKm: 2,
      startPct: 25,
      endPct: 50,
    }

    await act(async () => {
      rerender(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <TrackMap
            streams={streams}
            hoverIndex={null}
            sportColor="#f97316"
            sportType="running"
            hasGps
            zoomRange={zoomRange}
            onMaximize={vi.fn()}
          />
        </QueryClientProvider>,
      )
    })

    expect(map.fitBounds).toHaveBeenCalledTimes(1)
    expect(map.setLayoutProperty).toHaveBeenCalledWith('highlight-line', 'visibility', 'visible')
  })

  it('localizes no-GPS fallback messages in English', async () => {
    await i18n.changeLanguage('en')

    renderTrackMap(null, false)

    expect(screen.getByText('No GPS data')).toBeInTheDocument()
    expect(screen.queryByText('Keine GPS-Daten')).not.toBeInTheDocument()
  })

  it('formats summary POI popup distance and elevation with selected user units', async () => {
    await i18n.changeLanguage('en')
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
    mapMocks.instances.length = 0
    mapMocks.popupSetHtml.mockClear()

    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <TrackMap
          streams={streams}
          hoverIndex={null}
          sportColor="#f97316"
          sportType="running"
          hasGps
          zoomRange={null}
          onMaximize={vi.fn()}
          highlightedPois={[{
            id: 'poi-1',
            category: 'summit',
            name: 'Example summit',
            lat: 47.1,
            lng: 11.1,
            trackPos: null,
            distM: 1609.344,
            eleM: 100,
          }]}
        />
      </QueryClientProvider>,
    )
    const map = mapMocks.instances[0]

    await act(async () => {
      map.trigger('load')
    })

    map.triggerLayerClick('activity-track-summary-summits-layer', {
      name: 'Example summit',
      label: 'Summit',
      distM: 1609.344,
      eleM: 100,
    })

    const popupHtml = mapMocks.popupSetHtml.mock.calls.at(-1)?.[0] as string
    expect(popupHtml).toContain('328 ft')
    expect(popupHtml).toContain('1.0 mi')
    expect(popupHtml).not.toContain('100 m')
    expect(popupHtml).not.toContain('1,609 m')
  })
})
