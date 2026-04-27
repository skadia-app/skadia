import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'
import maplibregl, { type RequestParameters } from 'maplibre-gl'
import type { StyleSpecification } from 'maplibre-gl'
import { getToken } from '@/lib/auth'
import { pickMaxZoomForBounds } from '@/lib/geoUtils'

export interface RoutePlannerMapHandle {
  setHoverPoint(p: { lat: number; lng: number } | null): void
  fitBoundsToBBox(
    bbox: [number, number, number, number],
    opts?: { padding?: number; animate?: boolean; maxZoom?: number },
  ): void
  fitBoundsToFullRoute(): void
}

interface RoutePlannerMapProps {
  className?: string
  initialStyle: StyleSpecification | string
  initialCenter: [number, number]
  initialZoom: number
  onMapReady: (map: maplibregl.Map) => void
  onMapDispose: () => void
  onStyleLoad: (map: maplibregl.Map) => void
  onTilesLoadingChange: (loading: boolean) => void
  fullRouteCoords?: [number, number][]
  getViewportSnapshot: () => {
    center: [number, number]
    zoom: number
    viewMode: string
    baseStyle: string
  }
}

export const RoutePlannerMap = forwardRef<RoutePlannerMapHandle, RoutePlannerMapProps>(
function RoutePlannerMap({
  className = 'w-full h-full',
  initialStyle,
  initialCenter,
  initialZoom,
  onMapReady,
  onMapDispose,
  onStyleLoad,
  onTilesLoadingChange,
  fullRouteCoords = [],
  getViewportSnapshot,
}, ref) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const fullRouteCoordsRef = useRef<[number, number][]>([])
  const fullRouteBoundsRef = useRef<maplibregl.LngLatBounds | null>(null)
  const hoverMarkerRef = useRef<maplibregl.Marker | null>(null)

  useEffect(() => {
    fullRouteCoordsRef.current = fullRouteCoords
    if (fullRouteCoords.length === 0) {
      fullRouteBoundsRef.current = null
      return
    }
    const bounds = new maplibregl.LngLatBounds(fullRouteCoords[0], fullRouteCoords[0])
    for (const coord of fullRouteCoords) bounds.extend(coord)
    fullRouteBoundsRef.current = bounds
  }, [fullRouteCoords])

  useImperativeHandle(ref, () => ({
    setHoverPoint(point) {
      hoverMarkerRef.current?.remove()
      hoverMarkerRef.current = null
      if (!point) return
      const map = mapRef.current
      if (!map) return
      const el = document.createElement('div')
      el.style.width = '10px'
      el.style.height = '10px'
      el.style.borderRadius = '50%'
      el.style.backgroundColor = 'white'
      el.style.border = '2px solid #f97316'
      el.style.boxShadow = '0 0 4px #f97316'
      hoverMarkerRef.current = new maplibregl.Marker({ element: el })
        .setLngLat([point.lng, point.lat])
        .addTo(map)
    },
    fitBoundsToBBox() {},
    fitBoundsToFullRoute() {
      const map = mapRef.current
      const bounds = fullRouteBoundsRef.current
      if (!map || !bounds) return
      const sw = bounds.getSouthWest()
      const ne = bounds.getNorthEast()
      map.fitBounds(bounds, { padding: 80, maxZoom: pickMaxZoomForBounds(sw.lat, sw.lng, ne.lat, ne.lng) })
    },
  }), [])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const map = new maplibregl.Map({
      container,
      style: initialStyle,
      center: initialCenter,
      zoom: initialZoom,
      attributionControl: { compact: false },
      transformRequest: (url: string): RequestParameters => {
        if (url.includes('/api/v1/tiles/')) {
          const token = getToken()
          if (token) {
            return { url, headers: { Authorization: `Bearer ${token}` } }
          }
        }
        return { url }
      },
    })

    mapRef.current = map
    onMapReady(map)

    map.addControl(new maplibregl.NavigationControl(), 'top-left')

    map.on('style.load', () => {
      onStyleLoad(map)
    })

    let viewportSaveTimer: ReturnType<typeof setTimeout> | null = null
    function scheduleViewportSave() {
      if (viewportSaveTimer) clearTimeout(viewportSaveTimer)
      viewportSaveTimer = setTimeout(() => {
        const c = map.getCenter()
        const snapshot = getViewportSnapshot()
        localStorage.setItem('skadia-map-viewport', JSON.stringify({
          ...snapshot,
          center: [c.lng, c.lat],
          zoom: map.getZoom(),
        }))
      }, 500)
    }
    map.on('moveend', scheduleViewportSave)
    map.on('zoomend', scheduleViewportSave)

    map.on('sourcedataloading', (e) => {
      if (e.sourceId === 'heatmap') onTilesLoadingChange(true)
    })
    map.on('idle', () => onTilesLoadingChange(false))

    const ro = new ResizeObserver(() => map.resize())
    ro.observe(container)

    return () => {
      if (viewportSaveTimer) clearTimeout(viewportSaveTimer)
      ro.disconnect()
      hoverMarkerRef.current?.remove()
      hoverMarkerRef.current = null
      map.remove()
      mapRef.current = null
      onMapDispose()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return <div ref={containerRef} className={className} />
})
