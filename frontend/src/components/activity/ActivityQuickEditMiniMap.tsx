import { useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import type { StreamPoint } from '@/api/types'
import { buildLayerStyle } from '@/lib/mapStyles'
import { useCapabilities } from '@/hooks/useCapabilities'
import { useMapLayers } from '@/hooks/useMapLayers'
import { lineStringToSvgPoints } from '@/lib/trackSvg'

interface ActivityQuickEditMiniMapProps {
  streams: StreamPoint[]
  loading: boolean
}

export function trackCoordsFromStreams(streams: StreamPoint[]): [number, number][] {
  return streams
    .filter(s => s.lat != null && s.lng != null)
    .map(s => [s.lng!, s.lat!] as [number, number])
}

function fallbackSvgPoints(coords: [number, number][]): string | null {
  if (coords.length < 2) return null
  return lineStringToSvgPoints(JSON.stringify({ type: 'LineString', coordinates: coords }), 200, 12)
}

export default function ActivityQuickEditMiniMap({ streams, loading }: ActivityQuickEditMiniMapProps) {
  const { t } = useTranslation()
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const coords = useMemo(() => trackCoordsFromStreams(streams), [streams])
  const fallbackPoints = useMemo(() => fallbackSvgPoints(coords), [coords])
  const { base } = useMapLayers()
  const { data: caps } = useCapabilities()
  const selectedLayer = base[0]

  useEffect(() => {
    const el = mapContainerRef.current
    if (!el || coords.length < 2 || mapRef.current || !selectedLayer) return

    const initStyle = buildLayerStyle(selectedLayer, caps ?? {})
    if (!initStyle) return

    const bounds = coords.reduce(
      (b, c) => b.extend(c),
      new maplibregl.LngLatBounds(coords[0], coords[0]),
    )

    let fitDone = false
    const map = new maplibregl.Map({
      container: el,
      style: initStyle as import('maplibre-gl').StyleSpecification | string,
      center: coords[0],
      zoom: 12,
      interactive: false,
      attributionControl: false,
    })
    mapRef.current = map
    map.addControl(new maplibregl.AttributionControl({ compact: true }), 'top-left')

    const addTrack = () => {
      if (!map.getSource('quick-edit-track')) {
        map.addSource('quick-edit-track', {
          type: 'geojson',
          data: {
            type: 'Feature',
            properties: {},
            geometry: { type: 'LineString', coordinates: coords },
          },
        })
      }
      if (!map.getLayer('quick-edit-track-line')) {
        map.addLayer({
          id: 'quick-edit-track-line',
          type: 'line',
          source: 'quick-edit-track',
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: { 'line-color': '#F97316', 'line-width': 4, 'line-opacity': 0.9 },
        })
      }
      if (!fitDone) {
        map.fitBounds(bounds, { padding: 28, animate: false })
        fitDone = true
      }
    }

    map.on('style.load', addTrack)
    map.on('load', () => {
      requestAnimationFrame(() => map.resize())
      addTrack()
    })

    return () => {
      map.remove()
      mapRef.current = null
    }
  }, [caps, coords, selectedLayer])

  if (loading) {
    return <div className="flex h-full w-full items-center justify-center text-sm text-neutral-400">{t('activityQuickEditMiniMap.loadingTrack')}</div>
  }

  if (coords.length < 2) {
    return <div className="flex h-full w-full items-center justify-center text-sm text-neutral-400">{t('activityQuickEditMiniMap.noTrack')}</div>
  }

  return (
    <>
      {selectedLayer ? (
        <div ref={mapContainerRef} className="h-full w-full" />
      ) : fallbackPoints ? (
        <svg viewBox="0 0 200 200" className="h-full w-full bg-neutral-100 dark:bg-neutral-800">
          <polyline points={fallbackPoints} fill="none" stroke="currentColor" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" className="text-accent-DEFAULT" />
        </svg>
      ) : null}
    </>
  )
}
