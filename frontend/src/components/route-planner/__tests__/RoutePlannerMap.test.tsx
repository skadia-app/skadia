import { createRef } from 'react'
import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { RoutePlannerMapHandle } from '../RoutePlannerMap'

const maplibreMock = vi.hoisted(() => {
  class LngLatBoundsMock {
    points: [number, number][] = []

    constructor(sw?: [number, number], ne?: [number, number]) {
      if (sw) this.points.push(sw)
      if (ne) this.points.push(ne)
    }

    extend(coord: [number, number]) {
      this.points.push(coord)
      return this
    }

    isEmpty() {
      return this.points.length === 0
    }

    getSouthWest() {
      const lngs = this.points.map(p => p[0])
      const lats = this.points.map(p => p[1])
      return { lng: Math.min(...lngs), lat: Math.min(...lats) }
    }

    getNorthEast() {
      const lngs = this.points.map(p => p[0])
      const lats = this.points.map(p => p[1])
      return { lng: Math.max(...lngs), lat: Math.max(...lats) }
    }
  }

  class MapMock {
    fitBounds = vi.fn()
    addControl = vi.fn()
    on = vi.fn()
    remove = vi.fn()
    resize = vi.fn()
    getCenter = vi.fn(() => ({ lng: 11, lat: 47 }))
    getZoom = vi.fn(() => 9)

    constructor(public options: unknown) {
      maplibreMock.maps.push(this)
    }
  }

  class MarkerMock {
    setLngLat = vi.fn(() => this)
    addTo = vi.fn(() => this)
    remove = vi.fn()

    constructor(public options: unknown) {
      maplibreMock.markers.push(this)
    }
  }

  return {
    maps: [] as MapMock[],
    markers: [] as MarkerMock[],
    LngLatBoundsMock,
    MapMock,
    MarkerMock,
    NavigationControlMock: class {},
  }
})

vi.mock('maplibre-gl', () => ({
  default: {
    Map: maplibreMock.MapMock,
    Marker: maplibreMock.MarkerMock,
    LngLatBounds: maplibreMock.LngLatBoundsMock,
    NavigationControl: maplibreMock.NavigationControlMock,
  },
  Map: maplibreMock.MapMock,
  Marker: maplibreMock.MarkerMock,
  LngLatBounds: maplibreMock.LngLatBoundsMock,
  NavigationControl: maplibreMock.NavigationControlMock,
}))

import { RoutePlannerMap } from '../RoutePlannerMap'

function renderRoutePlannerMap(fullRouteCoords: [number, number][] = []) {
  const ref = createRef<RoutePlannerMapHandle>()
  render(
    <RoutePlannerMap
      ref={ref}
      initialStyle={{ version: 8, sources: {}, layers: [] }}
      initialCenter={[11, 47]}
      initialZoom={9}
      onMapReady={() => {}}
      onMapDispose={() => {}}
      onStyleLoad={() => {}}
      onTilesLoadingChange={() => {}}
      fullRouteCoords={fullRouteCoords}
      getViewportSnapshot={() => ({
        center: [11, 47],
        zoom: 9,
        viewMode: 'tracks',
        baseStyle: 'osm',
      })}
    />,
  )
  const map = maplibreMock.maps.at(-1)
  if (!map) throw new Error('map mock was not constructed')
  if (!ref.current) throw new Error('RoutePlannerMapHandle was not attached')
  return { ref, map }
}

describe('RoutePlannerMapHandle', () => {
  beforeEach(() => {
    maplibreMock.maps.length = 0
    maplibreMock.markers.length = 0
    vi.stubGlobal('ResizeObserver', class {
      observe = vi.fn()
      disconnect = vi.fn()
    })
  })

  it('RoutePlannerMapHandle.setHoverPoint creates+positions marker', () => {
    const { ref, map } = renderRoutePlannerMap()

    act(() => {
      ref.current!.setHoverPoint({ lat: 47.1, lng: 11.2 })
    })

    expect(maplibreMock.markers).toHaveLength(1)
    expect(maplibreMock.markers[0].setLngLat).toHaveBeenCalledWith([11.2, 47.1])
    expect(maplibreMock.markers[0].addTo).toHaveBeenCalledWith(map)
  })

  it('RoutePlannerMapHandle.setHoverPoint(null) removes marker', () => {
    const { ref } = renderRoutePlannerMap()

    act(() => {
      ref.current!.setHoverPoint({ lat: 47.1, lng: 11.2 })
      ref.current!.setHoverPoint(null)
    })

    expect(maplibreMock.markers[0].remove).toHaveBeenCalledTimes(1)
  })

  it('RoutePlannerMapHandle.fitBoundsToBBox calls map.fitBounds with padding 48', () => {
    const { ref, map } = renderRoutePlannerMap()

    act(() => {
      ref.current!.fitBoundsToBBox([11, 47, 12, 48])
    })

    expect(map.fitBounds).toHaveBeenCalledTimes(1)
    expect(map.fitBounds).toHaveBeenCalledWith(expect.any(maplibreMock.LngLatBoundsMock), {
      padding: 48,
      animate: undefined,
      maxZoom: undefined,
    })
  })

  it('RoutePlannerMapHandle.fitBoundsToFullRoute uses cached full-route bounds', () => {
    const { ref, map } = renderRoutePlannerMap([
      [11, 47],
      [11.5, 47.5],
      [12, 48],
    ])

    act(() => {
      ref.current!.fitBoundsToFullRoute()
    })

    expect(map.fitBounds).toHaveBeenCalledTimes(1)
    expect(map.fitBounds).toHaveBeenCalledWith(expect.any(maplibreMock.LngLatBoundsMock), {
      padding: 80,
      maxZoom: expect.any(Number),
    })
  })
})
