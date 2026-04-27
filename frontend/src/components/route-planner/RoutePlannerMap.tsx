import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'
import maplibregl, { type RequestParameters } from 'maplibre-gl'
import type { StyleSpecification } from 'maplibre-gl'
import { getToken } from '@/lib/auth'

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
  getViewportSnapshot,
}, ref) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)

  useImperativeHandle(ref, () => ({
    setHoverPoint() {},
    fitBoundsToBBox() {},
    fitBoundsToFullRoute() {},
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
      map.remove()
      mapRef.current = null
      onMapDispose()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return <div ref={containerRef} className={className} />
})
