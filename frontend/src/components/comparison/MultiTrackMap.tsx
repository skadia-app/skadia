// MultiTrackMap — generic N-track MapLibre map.
// Reads sources from SourcesContext, hover markers from useComparisonStore.
// No fusion/comparison/recording-group specific logic.

import { useEffect, useRef, useState } from 'react'
import maplibregl from 'maplibre-gl'
import type { LngLatLike } from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import ColorPicker from '@/components/ColorPicker'
import { buildRasterStyle, BASE_STYLES, type BaseStyle } from '@/lib/mapStyles'
import { addHeatmapLayers, removeHeatmapLayers, buildHeatmapTileUrl, loadHeatmapColor, saveHeatmapColor } from '@/lib/mapHeatmap'
import { useSources } from './SourcesContext'
import { useComparisonStore } from './useComparisonStore'

// ── Line styles per index ─────────────────────────────────────────────────────

const LINE_STYLES = ['solid', 'dashed', 'dotted'] as const
type LineStyle = typeof LINE_STYLES[number]

function getLineStyle(idx: number): LineStyle {
  return LINE_STYLES[idx % LINE_STYLES.length]
}

// ── Props ─────────────────────────────────────────────────────────────────────

interface MultiTrackMapProps {
  showStyleSwitcher?: boolean
  showHeatmap?: boolean
  height?: string
  className?: string
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function MultiTrackMap({
  showStyleSwitcher = true,
  showHeatmap: showHeatmapProp = true,
  height = '100%',
  className,
}: MultiTrackMapProps) {
  const { visibleSources } = useSources()
  const hoverPoints = useComparisonStore(s => s.hoverPoints)

  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const hoverMarkersRef = useRef<maplibregl.Marker[]>([])
  const pendingCameraRestoreRef = useRef<{ center: LngLatLike; zoom: number } | null>(null)
  const visibleSourcesRef = useRef(visibleSources)
  const [styleVersion, setStyleVersion] = useState(0)

  const [baseStyle, setBaseStyle] = useState<BaseStyle>(() => {
    try {
      const d = JSON.parse(localStorage.getItem('skadia-map-defaults') ?? '{}') as Record<string, unknown>
      return (d.baseStyle as BaseStyle) ?? 'osm'
    } catch { return 'osm' }
  })
  const prevBaseStyleRef = useRef<BaseStyle>(baseStyle)
  const [showHeatmapState, setShowHeatmapState] = useState(false)
  const [heatmapColor, setHeatmapColor] = useState<string>(loadHeatmapColor)

  const { data: meData } = useQuery({
    queryKey: ['me'],
    queryFn: () => apiClient<{ user_id: string }>('/me'),
    staleTime: Infinity,
  })
  const userId = meData?.user_id ?? ''

  // Keep ref in sync
  useEffect(() => { visibleSourcesRef.current = visibleSources }, [visibleSources])

  // ── Map init ────────────────────────────────────────────────────────────────
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
      fitBounds(map, visibleSourcesRef.current)
      visibleSourcesRef.current.forEach((src, idx) => addTrackLayer(map, src.id, src.color, buildCoords(src), idx))
    })

    map.on('style.load', () => {
      if (pendingCameraRestoreRef.current) {
        map.jumpTo(pendingCameraRestoreRef.current)
        pendingCameraRestoreRef.current = null
      }
      visibleSourcesRef.current.forEach((src, idx) => addTrackLayer(map, src.id, src.color, buildCoords(src), idx))
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

  // ── Style switcher ──────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || prevBaseStyleRef.current === baseStyle) return
    prevBaseStyleRef.current = baseStyle
    pendingCameraRestoreRef.current = {
      center: map.getCenter() as LngLatLike,
      zoom: map.getZoom(),
    }
    map.setStyle(buildRasterStyle(baseStyle))
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [baseStyle])

  // ── Heatmap ─────────────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    removeHeatmapLayers(map)
    if (showHeatmapState && userId) {
      const firstLayerId = visibleSourcesRef.current.length > 0
        ? `track-line-${visibleSourcesRef.current[0].id}`
        : undefined
      addHeatmapLayers(map, buildHeatmapTileUrl(userId), heatmapColor, firstLayerId)
    }
  }, [showHeatmapState, heatmapColor, styleVersion, userId])

  // ── Track updates ───────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.loaded()) return

    visibleSources.forEach((src, idx) => {
      const coords = buildCoords(src)
      const sourceId = `track-${src.id}`
      const layerId = `track-line-${src.id}`
      const existing = map.getSource(sourceId) as maplibregl.GeoJSONSource | undefined

      if (coords.length < 2) return

      const geojson: GeoJSON.Feature<GeoJSON.LineString> = {
        type: 'Feature',
        properties: {},
        geometry: { type: 'LineString', coordinates: coords },
      }

      if (existing) {
        existing.setData(geojson)
      } else {
        addTrackLayer(map, src.id, src.color, coords, idx)
      }
    })

    fitBounds(map, visibleSources, { animate: true })
  }, [visibleSources])

  // ── Hover markers ───────────────────────────────────────────────────────────
  useEffect(() => {
    hoverMarkersRef.current.forEach(m => m.remove())
    hoverMarkersRef.current = []
    const map = mapRef.current
    if (!map || !hoverPoints || hoverPoints.length === 0) return
    hoverPoints.forEach(point => {
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
  }, [hoverPoints])

  const handleHeatmapColorChange = (hex: string) => {
    setHeatmapColor(hex)
    saveHeatmapColor(hex)
  }

  const hasData = visibleSources.some(s => (s.streams?.length ?? 0) > 0)

  return (
    <div
      className={`relative w-full min-h-64 rounded-xl overflow-hidden border border-neutral-200 dark:border-neutral-800 ${className ?? ''}`}
      style={{ height }}
    >
      <div ref={containerRef} className="w-full h-full" />

      {/* Style switcher + heatmap */}
      {showStyleSwitcher && (
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
          {showHeatmapProp && (
            <div className="flex items-center gap-1 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md px-2 py-1">
              <button
                onClick={() => setShowHeatmapState(v => !v)}
                title={showHeatmapState ? 'Heatmap ausblenden' : 'Heatmap einblenden'}
                className={[
                  'flex items-center gap-1.5 text-xs font-medium rounded-md px-1.5 py-0.5 transition-colors cursor-pointer',
                  showHeatmapState
                    ? 'text-accent-DEFAULT'
                    : 'text-neutral-600 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-white',
                ].join(' ')}
              >
                <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 2C8.5 6 6 9 6 13a6 6 0 0012 0c0-2.5-1-4.5-2.5-6.5C14.5 8.5 13.5 10 13 12c-.5-1.5-.5-3 0-5.5C12.5 5 12.2 3.5 12 2z"/>
                </svg>
                Heatmap
              </button>
              {showHeatmapState && (
                <ColorPicker value={heatmapColor} onChange={handleHeatmapColorChange} />
              )}
            </div>
          )}
        </div>
      )}

      {/* Legend */}
      {hasData && (
        <div className="absolute bottom-3 left-3 bg-white/90 dark:bg-neutral-900/90 backdrop-blur-sm rounded-lg px-3 py-2 space-y-1 pointer-events-none">
          {visibleSources.map((src, idx) => {
            const lineStyle = getLineStyle(idx)
            return (
              <div key={src.id} className="flex items-center gap-2">
                <LegendLine color={src.color} lineStyle={lineStyle} />
                <span className="text-xs text-neutral-700 dark:text-neutral-300">{src.label}</span>
              </div>
            )
          })}
        </div>
      )}

      {/* Empty state */}
      {!hasData && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <span className="text-xs text-neutral-400 bg-white/80 dark:bg-neutral-900/80 px-3 py-1.5 rounded-lg">
            GPS-Daten werden geladen…
          </span>
        </div>
      )}
    </div>
  )
}

// ── Legend line indicator ─────────────────────────────────────────────────────

function LegendLine({ color, lineStyle }: { color: string; lineStyle: LineStyle }) {
  if (lineStyle === 'solid') {
    return (
      <div className="rounded-full flex-shrink-0" style={{ backgroundColor: color, width: '16px', height: '3px' }} />
    )
  }
  if (lineStyle === 'dashed') {
    return (
      <svg width="16" height="3" viewBox="0 0 16 3" fill="none" className="flex-shrink-0">
        <line x1="0" y1="1.5" x2="16" y2="1.5" stroke={color} strokeWidth="2" strokeDasharray="4 2" />
      </svg>
    )
  }
  return (
    <svg width="16" height="3" viewBox="0 0 16 3" fill="none" className="flex-shrink-0">
      <line x1="0" y1="1.5" x2="16" y2="1.5" stroke={color} strokeWidth="2" strokeDasharray="2 2" />
    </svg>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function buildCoords(src: { streams?: import('@/api/types').StreamPoint[] }): [number, number][] {
  if (!src.streams) return []
  return src.streams
    .filter(p => p.lat != null && p.lng != null)
    .map(p => [p.lng!, p.lat!])
}

function addTrackLayer(
  map: maplibregl.Map,
  id: string,
  color: string,
  coords: [number, number][],
  idx: number,
): void {
  const sourceId = `track-${id}`
  const layerId = `track-line-${id}`
  if (map.getSource(sourceId) || coords.length < 2) return

  const lineType = idx % 3 === 0 ? 'solid' : idx % 3 === 1 ? [4, 2] : [2, 2]
  const lineWidth = idx === 0 ? 4 : idx === 1 ? 3 : 2.5

  map.addSource(sourceId, {
    type: 'geojson',
    data: { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: coords } },
  })
  map.addLayer({
    id: layerId,
    type: 'line',
    source: sourceId,
    layout: { 'line-join': 'round', 'line-cap': 'round' },
    paint: {
      'line-color': color,
      'line-width': lineWidth,
      'line-opacity': 0.88,
      ...(lineType !== 'solid' ? { 'line-dasharray': lineType as number[] } : {}),
    },
  })
}

function fitBounds(
  map: maplibregl.Map,
  sources: { streams?: import('@/api/types').StreamPoint[] }[],
  opts: { animate?: boolean } = {},
): void {
  const allPoints = sources.flatMap(s =>
    (s.streams ?? []).filter(p => p.lat != null && p.lng != null)
  )
  if (allPoints.length === 0) return
  const bounds = new maplibregl.LngLatBounds()
  allPoints.forEach(p => bounds.extend([p.lng!, p.lat!]))
  map.fitBounds(bounds, { padding: 40, maxZoom: 16, duration: opts.animate ? 800 : 0 })
}
