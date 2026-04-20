// frontend/src/components/SegmentEditorModal.tsx
// Task 8.4c: MapboxDraw + Draw-Mode-Wiring
import { createPortal } from 'react-dom'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import maplibregl, { type RequestParameters } from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
// @ts-ignore — maplibre-gl-draw has no bundled TS types for default import
import MapboxDraw from 'maplibre-gl-draw'
import type { SegmentMetadata } from './segment-editor/SegmentSidebar'
import type { TrackPoint } from './segment-editor/TrackModeBottomBar'
import { BIDIRECTIONAL_DEFAULT, deriveTrailDifficultyColor, type SegType } from './segment-editor/constants'
import {
  addHeatmapLayers,
  removeHeatmapLayers,
  buildHeatmapTileUrl,
  loadHeatmapColor,
} from '@/lib/mapHeatmap'
import { buildRasterStyle, RASTER_STYLES, type BaseStyle } from '@/lib/mapStyles'
import { apiClient } from '@/api/client'
import { getToken } from '@/lib/auth'

export interface SegmentEditorModalProps {
  mode: 'track' | 'draw' | 'lap'
  trackPoints?: TrackPoint[]
  sourceActivityId?: string
  defaultSportGroup?: string
  onSuccess: () => void
  onClose: () => void
}

export default function SegmentEditorModal(props: SegmentEditorModalProps) {
  const { mode, trackPoints = [], onClose } = props

  // ── Metadata state ────────────────────────────────────────────────────────
  const [meta, setMeta] = useState<SegmentMetadata>(() => ({
    name: '',
    segType: (mode === 'lap' ? 'lap' : 'standard') as SegType,
    sportTypes: null,
    sacScale: null,
    mtbScale: null,
    surface: null,
    difficultyColor: null,
    bidirectional: BIDIRECTIONAL_DEFAULT[(mode === 'lap' ? 'lap' : 'standard') as SegType],
    userEditedBidirectional: false,
    isPublic: true,
  }))

  const patchMeta = useCallback((patch: Partial<SegmentMetadata>) => {
    setMeta(prev => {
      const next = { ...prev, ...patch }
      if (next.segType === 'trail') {
        next.difficultyColor = deriveTrailDifficultyColor(next.sacScale, next.mtbScale)
      }
      return next
    })
  }, [])

  // ── User-ID (for heatmap tile auth) ──────────────────────────────────────
  const { data: meData } = useQuery({
    queryKey: ['me'],
    queryFn: () => apiClient<{ user_id: string }>('/me'),
    staleTime: Infinity,
  })
  const currentUserId = meData?.user_id ?? ''

  // ── Map state ─────────────────────────────────────────────────────────────
  const mapContainerRef = useRef<HTMLDivElement | null>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const trackPointsRef = useRef(trackPoints)
  useEffect(() => { trackPointsRef.current = trackPoints }, [trackPoints])

  const [mapStyle, setMapStyle] = useState<BaseStyle>('osm')
  const [heatmapOn, setHeatmapOn] = useState(false)

  // ── Draw-mode state ───────────────────────────────────────────────────────
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const drawRef = useRef<any>(null)
  const [drawnCoords, setDrawnCoords] = useState<Array<[number, number]>>([])

  // ── Map Init useEffect ────────────────────────────────────────────────────
  useEffect(() => {
    const container = mapContainerRef.current
    if (!container) return

    const pts = trackPointsRef.current
    const initialCenter: [number, number] =
      pts && pts.length > 0 ? [pts[0].lng, pts[0].lat] : [11.0, 48.0]

    const map = new maplibregl.Map({
      container,
      style: buildRasterStyle(mapStyle),
      center: initialCenter,
      zoom: 12,
      transformRequest: (url: string): RequestParameters => {
        if (url.includes('/api/v1/tiles/')) {
          const token = getToken()
          if (token) return { url, headers: { Authorization: `Bearer ${token}` } }
        }
        return { url }
      },
    })
    mapRef.current = map

    map.on('load', () => {
      map.getCanvas().style.cursor = 'crosshair'

      // Fit bounds to track
      const current = trackPointsRef.current
      if (current && current.length > 1) {
        const lngs = current.map(p => p.lng)
        const lats = current.map(p => p.lat)
        map.fitBounds(
          [[Math.min(...lngs), Math.min(...lats)], [Math.max(...lngs), Math.max(...lats)]],
          { padding: 40 },
        )
      }

      // Track-mode: draw activity line
      if (mode === 'track' && current && current.length >= 2) {
        map.addSource('activity-track', {
          type: 'geojson',
          data: {
            type: 'Feature',
            geometry: { type: 'LineString', coordinates: current.map(p => [p.lng, p.lat]) },
            properties: {},
          },
        })
        map.addLayer({
          id: 'activity-track-line',
          type: 'line',
          source: 'activity-track',
          paint: { 'line-color': '#6b7280', 'line-width': 2, 'line-opacity': 0.7 },
        })
      }

      // Draw-mode: attach MapboxDraw
      if (mode === 'draw') {
        const draw = new MapboxDraw({
          displayControlsDefault: false,
          controls: { line_string: false, trash: false },
          defaultMode: 'draw_line_string',
        })
        map.addControl(draw as unknown as maplibregl.IControl)
        drawRef.current = draw

        const refreshCoords = () => {
          const all = draw.getAll()
          const feat = all.features.find(
            (f: GeoJSON.Feature) => f.geometry.type === 'LineString'
          )
          if (feat && feat.geometry.type === 'LineString') {
            setDrawnCoords(feat.geometry.coordinates as Array<[number, number]>)
          } else {
            setDrawnCoords([])
          }
        }
        map.on('draw.create', refreshCoords)
        map.on('draw.update', refreshCoords)
        map.on('draw.delete', refreshCoords)
        // Doppelklick schließt die Linie (MapboxDraw-Default in draw_line_string mode)
      }
    })

    return () => {
      if (drawRef.current) {
        try { map.removeControl(drawRef.current as unknown as maplibregl.IControl) } catch { /* ignore */ }
        drawRef.current = null
      }
      map.remove()
      mapRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mapStyle])

  // ── Heatmap toggle useEffect ──────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    if (heatmapOn && currentUserId) {
      addHeatmapLayers(map, buildHeatmapTileUrl(currentUserId), loadHeatmapColor())
    } else {
      removeHeatmapLayers(map)
    }
  }, [heatmapOn, currentUserId])

  // ── Draw-mode actions ─────────────────────────────────────────────────────
  const drawUndoLast = useCallback(() => {
    const draw = drawRef.current
    if (!draw) return
    const all = draw.getAll()
    const feat = all.features.find((f: GeoJSON.Feature) => f.geometry.type === 'LineString')
    if (!feat || feat.geometry.type !== 'LineString') return
    const coords = feat.geometry.coordinates.slice(0, -1)
    if (coords.length < 2) {
      draw.deleteAll()
      setDrawnCoords([])
      return
    }
    feat.geometry.coordinates = coords
    draw.set({ type: 'FeatureCollection', features: [feat] })
    setDrawnCoords(coords as Array<[number, number]>)
  }, [])

  const drawReverse = useCallback(() => {
    setDrawnCoords(c => {
      const rev = [...c].reverse()
      const draw = drawRef.current
      if (draw) {
        draw.set({
          type: 'FeatureCollection',
          features: [{
            type: 'Feature',
            properties: {},
            geometry: { type: 'LineString', coordinates: rev },
          }],
        })
      }
      return rev
    })
  }, [])

  const drawClear = useCallback(() => {
    drawRef.current?.deleteAll()
    setDrawnCoords([])
  }, [])

  // ── Dirty state + close logic ─────────────────────────────────────────────
  const isDirty = meta.name.length > 0

  const handleClose = useCallback(() => {
    if (isDirty && !window.confirm('Ungespeicherte Änderungen verwerfen?')) return
    onClose()
  }, [isDirty, onClose])

  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape') handleClose() }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [handleClose])

  useEffect(() => {
    if (!isDirty) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [isDirty])

  // Focus-Trap
  const modalRootRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const trap = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return
      const root = modalRootRef.current
      if (!root) return
      const focusable = root.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
      if (focusable.length === 0) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const active = document.activeElement as HTMLElement | null
      if (e.shiftKey && active === first) { e.preventDefault(); last.focus() }
      else if (!e.shiftKey && active === last) { e.preventDefault(); first.focus() }
    }
    window.addEventListener('keydown', trap)
    return () => window.removeEventListener('keydown', trap)
  }, [])

  // Silence unused warnings until later sub-tasks wire them
  void patchMeta; void drawUndoLast; void drawReverse; void drawClear; void drawnCoords

  const content = (
    <div ref={modalRootRef} className="fixed inset-0 z-[200] flex flex-col bg-neutral-950 text-white">
      {/* Top-Bar */}
      <div className="flex items-center gap-2 border-b border-neutral-800 bg-neutral-900 px-3 py-2 text-sm">
        <h2 className="font-semibold">
          {mode === 'track' ? 'Segment aus Track' : mode === 'draw' ? 'Segment zeichnen' : 'Rundkurs definieren'}
        </h2>
        <div className="flex-1" />

        {/* Map-Style toggles */}
        <div className="flex overflow-hidden rounded border border-neutral-700">
          {RASTER_STYLES.map(([key, def]) => (
            <button
              key={key}
              type="button"
              onClick={() => setMapStyle(key)}
              className={`px-2 py-1 text-xs font-medium transition-colors ${
                mapStyle === key
                  ? 'bg-blue-600 text-white'
                  : 'bg-neutral-800 text-neutral-400 hover:bg-neutral-700 hover:text-neutral-100'
              }`}
            >
              {def.label}
            </button>
          ))}
        </div>

        {/* Heatmap toggle */}
        <button
          type="button"
          onClick={() => setHeatmapOn(v => !v)}
          aria-pressed={heatmapOn}
          className={`rounded border px-2 py-1 text-xs transition-colors ${
            heatmapOn
              ? 'border-orange-500/50 bg-orange-600/20 text-orange-400'
              : 'border-neutral-700 bg-neutral-800 text-neutral-400 hover:text-neutral-200'
          }`}
        >
          Heatmap
        </button>

        <button
          type="button"
          onClick={handleClose}
          aria-label="Schließen"
          className="rounded bg-neutral-800 px-2 py-1 text-neutral-300 hover:bg-neutral-700 hover:text-white"
        >
          ✕
        </button>
      </div>

      {/* Middle (sidebar+map) */}
      <div className="flex min-h-0 flex-1">
        <div className="w-80 shrink-0 border-r border-neutral-800 bg-neutral-900 p-3">
          {/* SegmentSidebar placeholder — wired in 8.4e */}
          <p className="text-xs text-neutral-400">Sidebar (TODO 8.4e)</p>
        </div>
        {/* Map container */}
        <div className="relative flex-1">
          <div ref={mapContainerRef} className="absolute inset-0" />
        </div>
      </div>

      {/* Bottom — filled in Tasks 8.4d / 8.4e */}
      <div className="border-t border-neutral-800 bg-neutral-900 p-2 text-xs text-neutral-400">
        Bottom-Bar (TODO 8.4d/e)
      </div>
    </div>
  )

  return createPortal(content, document.body)
}
