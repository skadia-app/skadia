import { useEffect, useRef, useImperativeHandle, forwardRef, useState } from 'react'
import maplibregl from 'maplibre-gl'
import type { LngLatLike } from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { useQuery } from '@tanstack/react-query'
import ColorPicker from '@/components/ColorPicker'
import { buildRasterStyle, BASE_STYLES, type BaseStyle } from '@/lib/mapStyles'
import { addHeatmapLayers, removeHeatmapLayers, buildHeatmapTileUrl, loadHeatmapColor, saveHeatmapColor } from '@/lib/mapHeatmap'

export interface TrackPoint {
  lat: number
  lng: number
}

export interface TrackInfo {
  activityId: string
  label: string
  color: string
  stream: TrackPoint[]
}

interface FusionMapProps {
  tracks: TrackInfo[]
}

export interface FusionMapHandle {
  setHoverPoints: (points: Array<{ lat: number; lng: number; color: string }> | null) => void
}

const FusionMap = forwardRef<FusionMapHandle, FusionMapProps>(function FusionMap({ tracks }, ref) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const hoverMarkersRef = useRef<maplibregl.Marker[]>([])
  const pendingCameraRestoreRef = useRef<{ center: LngLatLike; zoom: number } | null>(null)
  const tracksRef = useRef(tracks)

  const [baseStyle, setBaseStyle] = useState<BaseStyle>(() => {
    try {
      const d = JSON.parse(localStorage.getItem('skadia-map-defaults') ?? '{}') as Record<string, unknown>
      return (d.baseStyle as BaseStyle) ?? 'osm'
    } catch { return 'osm' }
  })
  const [showHeatmap, setShowHeatmap] = useState(false)
  const [heatmapColor, setHeatmapColor] = useState<string>(loadHeatmapColor)
  const [styleVersion, setStyleVersion] = useState(0)

  const { data: meData } = useQuery({
    queryKey: ['me'],
    queryFn: () =>
      fetch('/api/v1/me', { credentials: 'include' })
        .then(r => r.json()) as Promise<{ user_id: string }>,
    staleTime: Infinity,
  })
  const userId = meData?.user_id ?? ''

  // Keep tracksRef in sync with prop
  useEffect(() => { tracksRef.current = tracks }, [tracks])

  useImperativeHandle(ref, () => ({
    setHoverPoints(points) {
      hoverMarkersRef.current.forEach(m => m.remove())
      hoverMarkersRef.current = []
      if (!points || points.length === 0) return
      const map = mapRef.current
      if (!map) return
      points.forEach(point => {
        const el = document.createElement('div')
        el.style.width = '10px'
        el.style.height = '10px'
        el.style.borderRadius = '50%'
        el.style.backgroundColor = 'white'
        el.style.border = `2px solid ${point.color}`
        el.style.boxShadow = `0 0 4px ${point.color}`
        const marker = new maplibregl.Marker({ element: el })
          .setLngLat([point.lng, point.lat])
          .addTo(map)
        hoverMarkersRef.current.push(marker)
      })
    },
  }), [])

  const addTrackToMap = (m: maplibregl.Map, track: TrackInfo, idx: number) => {
    const sourceId = `track-${track.activityId}`
    const layerId = `track-line-${track.activityId}`
    if (m.getSource(sourceId)) return // already added
    const coords = track.stream.map(p => [p.lng, p.lat] as [number, number])
    if (coords.length < 2) return
    m.addSource(sourceId, {
      type: 'geojson',
      data: { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: coords } },
    })
    m.addLayer({
      id: layerId,
      type: 'line',
      source: sourceId,
      layout: { 'line-join': 'round', 'line-cap': 'round' },
      paint: {
        'line-color': track.color,
        'line-width': idx === 0 ? 5 : idx === 1 ? 3 : 2,
        'line-opacity': idx === 0 ? 0.9 : idx === 1 ? 0.85 : 0.8,
      },
    })
  }

  useEffect(() => {
    if (!containerRef.current) return

    const map = new maplibregl.Map({
      container: containerRef.current,
      style: buildRasterStyle(baseStyle),
      center: [14, 47],
      zoom: 4,
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')
    mapRef.current = map

    map.on('load', () => {
      const allPoints = tracks.flatMap(t => t.stream)
      if (allPoints.length > 0) {
        const bounds = new maplibregl.LngLatBounds()
        allPoints.forEach(p => bounds.extend([p.lng, p.lat]))
        map.fitBounds(bounds, { padding: 40, maxZoom: 16 })
      }

      tracks.forEach((track, idx) => {
        if (track.stream.length === 0) return
        const sourceId = `track-${track.activityId}`
        const layerId = `track-line-${track.activityId}`

        const lineWidth = idx === 0 ? 5 : idx === 1 ? 3 : 2
        const lineOpacity = idx === 0 ? 0.9 : idx === 1 ? 0.85 : 0.8

        const geojson: GeoJSON.Feature<GeoJSON.LineString> = {
          type: 'Feature',
          properties: { label: track.label },
          geometry: {
            type: 'LineString',
            coordinates: track.stream.map(p => [p.lng, p.lat]),
          },
        }

        map.addSource(sourceId, { type: 'geojson', data: geojson })
        map.addLayer({
          id: layerId,
          type: 'line',
          source: sourceId,
          paint: {
            'line-color': track.color,
            'line-width': lineWidth,
            'line-opacity': lineOpacity,
          },
          layout: { 'line-cap': 'round', 'line-join': 'round' },
        })
      })
    })

    map.on('style.load', () => {
      if (pendingCameraRestoreRef.current) {
        map.jumpTo(pendingCameraRestoreRef.current)
        pendingCameraRestoreRef.current = null
      }
      // Re-add all current tracks after style change
      tracksRef.current.forEach((track, idx) => addTrackToMap(map, track, idx))
      setStyleVersion(v => v + 1)
    })

    return () => {
      hoverMarkersRef.current.forEach(m => m.remove())
      hoverMarkersRef.current = []
      map.remove()
      mapRef.current = null
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Style switcher effect
  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    pendingCameraRestoreRef.current = {
      center: map.getCenter() as LngLatLike,
      zoom: map.getZoom(),
    }
    map.setStyle(buildRasterStyle(baseStyle))
  }, [baseStyle]) // eslint-disable-line react-hooks/exhaustive-deps

  // Heatmap effect
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    removeHeatmapLayers(map)
    if (showHeatmap && userId) {
      // Insert heatmap BEFORE first track layer so track appears on top
      const firstTrackLayerId = tracks.length > 0
        ? `track-line-${tracks[0].activityId}`
        : undefined
      addHeatmapLayers(map, buildHeatmapTileUrl(userId), heatmapColor, firstTrackLayerId)
    }
  }, [showHeatmap, heatmapColor, styleVersion, userId, tracks])

  // Update track data when tracks change
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.loaded()) return

    tracks.forEach((track, idx) => {
      const sourceId = `track-${track.activityId}`
      const layerId = `track-line-${track.activityId}`
      const source = map.getSource(sourceId) as maplibregl.GeoJSONSource | undefined

      if (track.stream.length === 0) return

      const geojson: GeoJSON.Feature<GeoJSON.LineString> = {
        type: 'Feature',
        properties: {},
        geometry: {
          type: 'LineString',
          coordinates: track.stream.map(p => [p.lng, p.lat]),
        },
      }

      if (source) {
        source.setData(geojson)
      } else {
        // New track added after map init — add source + layer
        const lineWidth = idx === 0 ? 5 : idx === 1 ? 3 : 2
        const lineOpacity = idx === 0 ? 0.9 : idx === 1 ? 0.85 : 0.8
        map.addSource(sourceId, { type: 'geojson', data: geojson })
        map.addLayer({
          id: layerId,
          type: 'line',
          source: sourceId,
          paint: {
            'line-color': track.color,
            'line-width': lineWidth,
            'line-opacity': lineOpacity,
          },
          layout: { 'line-cap': 'round', 'line-join': 'round' },
        })
      }
    })

    // Re-fit bounds if we have points
    const allPoints = tracks.flatMap(t => t.stream)
    if (allPoints.length > 0) {
      const bounds = new maplibregl.LngLatBounds()
      allPoints.forEach(p => bounds.extend([p.lng, p.lat]))
      map.fitBounds(bounds, { padding: 40, maxZoom: 16, duration: 800 })
    }
  }, [tracks])

  const handleHeatmapColorChange = (hex: string) => {
    setHeatmapColor(hex)
    saveHeatmapColor(hex)
  }

  return (
    <div className="relative w-full h-full min-h-64 rounded-xl overflow-hidden border border-neutral-200 dark:border-neutral-800">
      <div ref={containerRef} className="w-full h-full" />

      {/* Kartenstil-Tabs + Heatmap */}
      <div className="absolute top-3 left-3 z-10 flex items-center gap-1.5 flex-wrap pointer-events-auto">
        <div className="flex items-center gap-0.5 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md p-0.5">
          {(Object.keys(BASE_STYLES) as BaseStyle[]).map(key => (
            <button
              key={key}
              onClick={() => setBaseStyle(key)}
              className={[
                'px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer',
                baseStyle === key
                  ? 'bg-accent-DEFAULT text-white'
                  : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700',
              ].join(' ')}
            >
              {BASE_STYLES[key].label}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-1 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md px-2 py-1">
          <button
            onClick={() => setShowHeatmap(v => !v)}
            title={showHeatmap ? 'Heatmap ausblenden' : 'Heatmap einblenden'}
            className={[
              'flex items-center gap-1.5 text-xs font-medium rounded-md px-1.5 py-0.5 transition-colors cursor-pointer',
              showHeatmap
                ? 'text-accent-DEFAULT'
                : 'text-neutral-600 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-white',
            ].join(' ')}
          >
            <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2C8.5 6 6 9 6 13a6 6 0 0012 0c0-2.5-1-4.5-2.5-6.5C14.5 8.5 13.5 10 13 12c-.5-1.5-.5-3 0-5.5C12.5 5 12.2 3.5 12 2z"/>
            </svg>
            Heatmap
          </button>
          {showHeatmap && (
            <ColorPicker value={heatmapColor} onChange={handleHeatmapColorChange} />
          )}
        </div>
      </div>

      {tracks.length > 0 && tracks.some(t => t.stream.length > 0) && (
        <div className="absolute bottom-3 left-3 bg-white/90 dark:bg-neutral-900/90 backdrop-blur-sm rounded-lg px-3 py-2 space-y-1 pointer-events-none">
          {tracks.map(t => (
            <div key={t.activityId} className="flex items-center gap-2">
              <div
                className="rounded-full flex-shrink-0"
                style={{ backgroundColor: t.color, width: '16px', height: '3px' }}
              />
              <span className="text-xs text-neutral-700 dark:text-neutral-300">{t.label}</span>
            </div>
          ))}
        </div>
      )}
      {tracks.every(t => t.stream.length === 0) && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <span className="text-xs text-neutral-400 bg-white/80 dark:bg-neutral-900/80 px-3 py-1.5 rounded-lg">
            GPS-Daten werden geladen...
          </span>
        </div>
      )}
    </div>
  )
})

export default FusionMap
