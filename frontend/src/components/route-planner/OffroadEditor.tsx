import { useRef, useEffect, useState, useCallback } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import type { RouteSegment } from '@/api/routes'

// ─── Catmull-Rom Spline ───────────────────────────────────────────────────────

/**
 * Compute a dense Catmull-Rom spline through the given control points.
 * Returns approximately `samplesPerSegment` interpolated points per span.
 * Phantom points are added at both ends to ensure the spline passes through
 * the first and last control point.
 */
function catmullRom(
  points: [number, number][],
  tension = 0.5,
  samplesPerSegment = 20,
): [number, number][] {
  if (points.length < 2) return [...points]
  if (points.length === 2) {
    // Straight line — no curve needed
    const result: [number, number][] = []
    for (let t = 0; t <= 1; t += 1 / samplesPerSegment) {
      result.push([
        points[0][0] + (points[1][0] - points[0][0]) * t,
        points[0][1] + (points[1][1] - points[0][1]) * t,
      ])
    }
    result.push(points[points.length - 1])
    return result
  }

  // Extend with phantom points
  const pts: [number, number][] = [
    [2 * points[0][0] - points[1][0], 2 * points[0][1] - points[1][1]],
    ...points,
    [
      2 * points[points.length - 1][0] - points[points.length - 2][0],
      2 * points[points.length - 1][1] - points[points.length - 2][1],
    ],
  ]

  const result: [number, number][] = []
  const alpha = tension

  for (let i = 1; i < pts.length - 2; i++) {
    const p0 = pts[i - 1]
    const p1 = pts[i]
    const p2 = pts[i + 1]
    const p3 = pts[i + 2]

    for (let s = 0; s <= samplesPerSegment; s++) {
      const t = s / samplesPerSegment
      const t2 = t * t
      const t3 = t2 * t

      const x =
        alpha * (
          (2 * p1[0]) +
          (-p0[0] + p2[0]) * t +
          (2 * p0[0] - 5 * p1[0] + 4 * p2[0] - p3[0]) * t2 +
          (-p0[0] + 3 * p1[0] - 3 * p2[0] + p3[0]) * t3
        )
      const y =
        alpha * (
          (2 * p1[1]) +
          (-p0[1] + p2[1]) * t +
          (2 * p0[1] - 5 * p1[1] + 4 * p2[1] - p3[1]) * t2 +
          (-p0[1] + 3 * p1[1] - 3 * p2[1] + p3[1]) * t3
        )

      if (s === 0 && result.length > 0) continue // avoid duplicate join points
      result.push([x, y])
    }
  }

  return result
}

// ─── Nearest-point-on-segment helper ─────────────────────────────────────────

/** Find the nearest [lng, lat] point on a polyline (projected coords). */
function nearestPointOnPolyline(
  target: [number, number],
  points: [number, number][],
): { insertAfter: number; t: number } {
  let bestDist = Infinity
  let bestIdx = 0
  let bestT = 0

  for (let i = 0; i < points.length - 1; i++) {
    const ax = points[i][0], ay = points[i][1]
    const bx = points[i + 1][0], by = points[i + 1][1]
    const dx = bx - ax, dy = by - ay
    const lenSq = dx * dx + dy * dy
    let t = lenSq === 0 ? 0 : ((target[0] - ax) * dx + (target[1] - ay) * dy) / lenSq
    t = Math.max(0, Math.min(1, t))
    const px = ax + t * dx - target[0]
    const py = ay + t * dy - target[1]
    const dist = px * px + py * py
    if (dist < bestDist) {
      bestDist = dist
      bestIdx = i
      bestT = t
    }
  }

  return { insertAfter: bestIdx, t: bestT }
}

// ─── Component ────────────────────────────────────────────────────────────────

export interface OffroadEditorProps {
  /** The segment being edited. Must have mode === 'offroad'. */
  segment: RouteSegment
  /** Called whenever control points or smooth flag change. */
  onChange: (updated: Pick<RouteSegment, 'control_points' | 'smooth' | 'geometry'>) => void
  /** Optional height CSS class (default: h-96) */
  className?: string
}

const OFFROAD_LINE_COLOR = '#f97316'      // orange-500
const OFFROAD_LINE_DASHED_COLOR = '#fb923c' // orange-400
const CONTROL_POINT_COLOR = '#f97316'
const HOVER_SEGMENT_COLOR = '#fbbf24'     // amber-400

/**
 * OffroadEditor — interactive MapLibre panel for editing offroad segment
 * control points with optional Catmull-Rom smoothing.
 *
 * Interaction model:
 *  - Click on dashed offroad line  → insert new control point at click position
 *  - Drag control point marker     → update position
 *  - Right-click / dblclick marker → remove control point
 *  - Toggle "Smooth" button        → switch Polyline ↔ Catmull-Rom spline
 */
export default function OffroadEditor({
  segment,
  onChange,
  className = 'h-96',
}: OffroadEditorProps) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markersRef = useRef<maplibregl.Marker[]>([])

  // Mirror state into refs so MapLibre event handlers always see fresh values
  const controlPointsRef = useRef<[number, number][]>(segment.control_points ?? [])
  const smoothRef = useRef<boolean>(segment.smooth ?? false)

  const [controlPoints, setControlPointsState] = useState<[number, number][]>(
    segment.control_points ?? [],
  )
  const [smooth, setSmoothState] = useState<boolean>(segment.smooth ?? false)
  const [hoverInfo, setHoverInfo] = useState<string | null>(null)

  // ── Derived geometry ────────────────────────────────────────────────────────

  /** Build the rendered LineString coordinates from current control points + smooth flag */
  const buildLineCoords = useCallback(
    (pts: [number, number][], isSmooth: boolean): [number, number][] => {
      if (pts.length < 2) return pts
      return isSmooth ? catmullRom(pts) : pts
    },
    [],
  )

  /** Emit onChange with recomputed geometry */
  const emitChange = useCallback(
    (pts: [number, number][], isSmooth: boolean) => {
      const lineCoords = buildLineCoords(pts, isSmooth)
      onChange({
        control_points: pts,
        smooth: isSmooth,
        geometry:
          lineCoords.length >= 2
            ? { type: 'LineString', coordinates: lineCoords }
            : undefined,
      })
    },
    [buildLineCoords, onChange],
  )

  // ── Setters that keep ref + state in sync ───────────────────────────────────

  const setControlPoints = useCallback(
    (pts: [number, number][]) => {
      controlPointsRef.current = pts
      setControlPointsState(pts)
    },
    [],
  )

  const setSmooth = useCallback(
    (val: boolean) => {
      smoothRef.current = val
      setSmoothState(val)
    },
    [],
  )

  // ── MapLibre source helpers ─────────────────────────────────────────────────

  const updateLineSource = useCallback(
    (map: maplibregl.Map, pts: [number, number][], isSmooth: boolean) => {
      if (!map.isStyleLoaded()) return
      const lineCoords = buildLineCoords(pts, isSmooth)

      const lineSource = map.getSource('offroad-line') as maplibregl.GeoJSONSource | undefined
      lineSource?.setData({
        type: 'Feature',
        properties: {},
        geometry:
          lineCoords.length >= 2
            ? { type: 'LineString', coordinates: lineCoords }
            : { type: 'LineString', coordinates: [] },
      })

      const cpSource = map.getSource('offroad-cp') as maplibregl.GeoJSONSource | undefined
      cpSource?.setData({
        type: 'FeatureCollection',
        features: pts.map((pt, idx) => ({
          type: 'Feature' as const,
          properties: { idx },
          geometry: { type: 'Point' as const, coordinates: pt },
        })),
      })
    },
    [buildLineCoords],
  )

  // ── Marker management ───────────────────────────────────────────────────────

  const rebuildMarkers = useCallback(
    (map: maplibregl.Map, pts: [number, number][]) => {
      // Remove old markers
      markersRef.current.forEach(m => m.remove())
      markersRef.current = []

      pts.forEach((pt, idx) => {
        const el = document.createElement('div')
        el.style.cssText = [
          'width:12px',
          'height:12px',
          'border-radius:50%',
          `background:${CONTROL_POINT_COLOR}`,
          'border:2px solid white',
          'box-shadow:0 1px 4px rgba(0,0,0,.45)',
          'cursor:grab',
          'transition:transform 0.1s',
        ].join(';')
        el.title = `Kontrollpunkt ${idx + 1} — Rechtsklick zum Löschen`

        // Hover effect
        el.addEventListener('mouseenter', () => {
          el.style.transform = 'scale(1.4)'
          setHoverInfo(`Punkt ${idx + 1} von ${pts.length}`)
        })
        el.addEventListener('mouseleave', () => {
          el.style.transform = 'scale(1)'
          setHoverInfo(null)
        })

        const marker = new maplibregl.Marker({ element: el, draggable: true })
          .setLngLat(pt)
          .addTo(map)

        // Drag: update line live
        marker.on('drag', () => {
          const pos = marker.getLngLat()
          const updated = [...controlPointsRef.current]
          updated[idx] = [pos.lng, pos.lat]
          controlPointsRef.current = updated
          updateLineSource(map, updated, smoothRef.current)
        })

        // Dragend: commit
        marker.on('dragend', () => {
          const pos = marker.getLngLat()
          const updated = [...controlPointsRef.current]
          updated[idx] = [pos.lng, pos.lat]
          setControlPoints(updated)
          emitChange(updated, smoothRef.current)
        })

        // Right-click: delete
        el.addEventListener('contextmenu', (e) => {
          e.preventDefault()
          e.stopPropagation()
          const updated = controlPointsRef.current.filter((_, i) => i !== idx)
          setControlPoints(updated)
          updateLineSource(map, updated, smoothRef.current)
          rebuildMarkers(map, updated)
          emitChange(updated, smoothRef.current)
        })

        // Double-click: delete (touch-friendly alternative)
        el.addEventListener('dblclick', (e) => {
          e.stopPropagation()
          const updated = controlPointsRef.current.filter((_, i) => i !== idx)
          setControlPoints(updated)
          updateLineSource(map, updated, smoothRef.current)
          rebuildMarkers(map, updated)
          emitChange(updated, smoothRef.current)
        })

        markersRef.current.push(marker)
      })
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [emitChange, setControlPoints, updateLineSource],
  )

  // ── Map initialization ──────────────────────────────────────────────────────

  useEffect(() => {
    if (!mapContainerRef.current) return

    const initPts = controlPointsRef.current

    // Determine initial map center/bounds
    let centerLng = 11.0
    let centerLat = 47.5
    let zoom = 12

    if (initPts.length >= 2) {
      let minLng = Infinity, maxLng = -Infinity
      let minLat = Infinity, maxLat = -Infinity
      for (const [lng, lat] of initPts) {
        if (lng < minLng) minLng = lng
        if (lng > maxLng) maxLng = lng
        if (lat < minLat) minLat = lat
        if (lat > maxLat) maxLat = lat
      }
      centerLng = (minLng + maxLng) / 2
      centerLat = (minLat + maxLat) / 2
      zoom = 13
    } else if (initPts.length === 1) {
      centerLng = initPts[0][0]
      centerLat = initPts[0][1]
    }

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: {
        version: 8,
        sources: {
          osm: {
            type: 'raster',
            tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
            tileSize: 256,
            attribution: '&copy; OpenStreetMap-Mitwirkende',
          },
        },
        layers: [
          { id: 'osm-tiles', type: 'raster', source: 'osm', minzoom: 0, maxzoom: 19 },
        ],
      },
      center: [centerLng, centerLat],
      zoom,
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      const initCoords = buildLineCoords(controlPointsRef.current, smoothRef.current)

      // ── Offroad line (dashed) ─────────────────────────────────────────────
      map.addSource('offroad-line', {
        type: 'geojson',
        data: {
          type: 'Feature',
          properties: {},
          geometry:
            initCoords.length >= 2
              ? { type: 'LineString', coordinates: initCoords }
              : { type: 'LineString', coordinates: [] },
        },
      })

      // Glow / shadow layer (thicker, low opacity)
      map.addLayer({
        id: 'offroad-line-glow',
        type: 'line',
        source: 'offroad-line',
        paint: {
          'line-color': OFFROAD_LINE_COLOR,
          'line-width': 10,
          'line-opacity': 0.15,
          'line-blur': 4,
        },
      })

      // Main dashed line
      map.addLayer({
        id: 'offroad-line-dash',
        type: 'line',
        source: 'offroad-line',
        paint: {
          'line-color': OFFROAD_LINE_DASHED_COLOR,
          'line-width': 3,
          'line-dasharray': [4, 3],
          'line-opacity': 0.9,
        },
      })

      // Hover highlight line (same source, thicker)
      map.addLayer({
        id: 'offroad-line-hover',
        type: 'line',
        source: 'offroad-line',
        paint: {
          'line-color': HOVER_SEGMENT_COLOR,
          'line-width': 6,
          'line-opacity': 0,
        },
      })

      // ── Control points source (circles in GL — visual backup) ─────────────
      map.addSource('offroad-cp', {
        type: 'geojson',
        data: {
          type: 'FeatureCollection',
          features: controlPointsRef.current.map((pt, idx) => ({
            type: 'Feature' as const,
            properties: { idx },
            geometry: { type: 'Point' as const, coordinates: pt },
          })),
        },
      })

      // Control point circles (behind markers, for hit area visual)
      map.addLayer({
        id: 'offroad-cp-circle',
        type: 'circle',
        source: 'offroad-cp',
        paint: {
          'circle-radius': 8,
          'circle-color': CONTROL_POINT_COLOR,
          'circle-stroke-width': 2,
          'circle-stroke-color': '#fff',
          'circle-opacity': 0,  // actual drag is via Marker elements
        },
      })

      // ── Hover on offroad line ─────────────────────────────────────────────
      map.on('mouseenter', 'offroad-line-dash', () => {
        map.getCanvas().style.cursor = 'crosshair'
        map.setPaintProperty('offroad-line-hover', 'line-opacity', 0.35)
      })
      map.on('mouseleave', 'offroad-line-dash', () => {
        map.getCanvas().style.cursor = ''
        map.setPaintProperty('offroad-line-hover', 'line-opacity', 0)
      })

      // ── Click on line → insert control point ─────────────────────────────
      map.on('click', 'offroad-line-dash', (e) => {
        e.preventDefault()
        const clickedLngLat: [number, number] = [e.lngLat.lng, e.lngLat.lat]
        const current = controlPointsRef.current

        let newPts: [number, number][]
        if (current.length < 2) {
          newPts = [...current, clickedLngLat]
        } else {
          // Insert at the nearest position along the polyline
          const { insertAfter } = nearestPointOnPolyline(clickedLngLat, current)
          newPts = [
            ...current.slice(0, insertAfter + 1),
            clickedLngLat,
            ...current.slice(insertAfter + 1),
          ]
        }

        setControlPoints(newPts)
        updateLineSource(map, newPts, smoothRef.current)
        rebuildMarkers(map, newPts)
        emitChange(newPts, smoothRef.current)
      })

      // ── Click on empty map → append control point ────────────────────────
      map.on('click', (e) => {
        // Ignore if clicking on the line layer (handled above)
        const features = map.queryRenderedFeatures(e.point, {
          layers: ['offroad-line-dash'],
        })
        if (features.length > 0) return

        const clickedLngLat: [number, number] = [e.lngLat.lng, e.lngLat.lat]
        const current = controlPointsRef.current
        const newPts = [...current, clickedLngLat]

        setControlPoints(newPts)
        updateLineSource(map, newPts, smoothRef.current)
        rebuildMarkers(map, newPts)
        emitChange(newPts, smoothRef.current)
      })

      // ── Initial markers ───────────────────────────────────────────────────
      rebuildMarkers(map, controlPointsRef.current)

      // Fit to existing points
      if (initPts.length >= 2) {
        const bounds = new maplibregl.LngLatBounds()
        for (const pt of initPts) bounds.extend(pt as maplibregl.LngLatLike)
        map.fitBounds(bounds, { padding: 60, maxZoom: 16, duration: 600 })
      }
    })

    mapRef.current = map
    return () => {
      markersRef.current.forEach(m => m.remove())
      markersRef.current = []
      map.remove()
      mapRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── React to smooth toggle ──────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map?.isStyleLoaded()) return
    updateLineSource(map, controlPointsRef.current, smooth)
  }, [smooth, updateLineSource])

  // ── Handlers ───────────────────────────────────────────────────────────────

  const handleToggleSmooth = () => {
    const next = !smoothRef.current
    setSmooth(next)
    emitChange(controlPointsRef.current, next)
  }

  const handleClearAll = () => {
    setControlPoints([])
    const map = mapRef.current
    if (map?.isStyleLoaded()) {
      updateLineSource(map, [], smoothRef.current)
      rebuildMarkers(map, [])
    }
    emitChange([], smoothRef.current)
  }

  const handleUndoLast = () => {
    const current = controlPointsRef.current
    if (current.length === 0) return
    const updated = current.slice(0, -1)
    setControlPoints(updated)
    const map = mapRef.current
    if (map?.isStyleLoaded()) {
      updateLineSource(map, updated, smoothRef.current)
      rebuildMarkers(map, updated)
    }
    emitChange(updated, smoothRef.current)
  }

  // ─────────────────────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-3">
      {/* Toolbar */}
      <div className="flex items-center gap-2 flex-wrap">
        {/* Smooth toggle */}
        <button
          type="button"
          role="switch"
          aria-checked={smooth}
          onClick={handleToggleSmooth}
          className={[
            'flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium border transition-colors cursor-pointer',
            smooth
              ? 'bg-orange-500 border-orange-500 text-white'
              : 'border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800',
          ].join(' ')}
        >
          <svg
            className="w-4 h-4 flex-shrink-0"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
          >
            {/* Simple spline icon */}
            <path
              d="M2 12 C4 4, 8 4, 8 8 S12 12, 14 4"
              strokeLinecap="round"
              fill="none"
            />
          </svg>
          Catmull-Rom
        </button>

        {/* Mode label */}
        <span className="text-xs text-neutral-500 dark:text-neutral-400 px-2 py-1 rounded-md bg-neutral-100 dark:bg-neutral-800 font-mono">
          {smooth ? 'Spline' : 'Polyline'}
        </span>

        <div className="flex-1" />

        {/* Undo last */}
        <button
          type="button"
          onClick={handleUndoLast}
          disabled={controlPoints.length === 0}
          title="Letzten Punkt entfernen"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm border border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed transition-colors"
        >
          <svg className="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M3 8 L7 4 L7 6 C10 6 13 8 13 12" strokeLinecap="round" strokeLinejoin="round" />
            <path d="M7 6 L7 10" strokeLinecap="round" opacity="0" />
          </svg>
          Rückgängig
        </button>

        {/* Clear all */}
        <button
          type="button"
          onClick={handleClearAll}
          disabled={controlPoints.length === 0}
          title="Alle Punkte löschen"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm border border-red-200 dark:border-red-900 text-red-500 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed transition-colors"
        >
          <svg className="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M4 4 L12 12 M12 4 L4 12" strokeLinecap="round" />
          </svg>
          Alles löschen
        </button>
      </div>

      {/* Map */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden relative">
        <div ref={mapContainerRef} className={`w-full ${className}`} />

        {/* Hover status overlay */}
        {hoverInfo && (
          <div className="absolute bottom-2 left-2 bg-black/60 text-white text-xs px-2 py-1 rounded-md pointer-events-none">
            {hoverInfo}
          </div>
        )}

        {/* Instruction overlay (only when no points) */}
        {controlPoints.length === 0 && (
          <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
            <div className="bg-black/50 text-white text-sm px-4 py-2.5 rounded-xl text-center max-w-xs">
              Klicke auf die Karte, um den ersten Kontrollpunkt zu setzen
            </div>
          </div>
        )}
      </div>

      {/* Info bar */}
      <div className="flex items-center gap-4 text-xs text-neutral-500 dark:text-neutral-400">
        <span>
          <strong className="text-neutral-700 dark:text-neutral-300 tabular-nums">
            {controlPoints.length}
          </strong>{' '}
          Kontrollpunkt{controlPoints.length !== 1 ? 'e' : ''}
        </span>

        {controlPoints.length > 0 && (
          <>
            <span className="text-neutral-300 dark:text-neutral-700">|</span>
            <span>Klick auf Linie: Punkt einfügen</span>
            <span className="text-neutral-300 dark:text-neutral-700">|</span>
            <span>Rechtsklick / Doppelklick auf Punkt: löschen</span>
            <span className="text-neutral-300 dark:text-neutral-700">|</span>
            <span>Ziehen: verschieben</span>
          </>
        )}
      </div>

      {/* Visual legend */}
      <div className="flex items-center gap-4 text-xs text-neutral-500 dark:text-neutral-400">
        <span className="flex items-center gap-1.5">
          <span
            className="inline-block w-6 h-0.5 rounded"
            style={{
              background: `repeating-linear-gradient(90deg, ${OFFROAD_LINE_DASHED_COLOR} 0 8px, transparent 8px 14px)`,
            }}
          />
          Offroad-Linie
        </span>
        <span className="flex items-center gap-1.5">
          <span
            className="inline-block w-3 h-3 rounded-full border-2 border-white flex-shrink-0"
            style={{ background: CONTROL_POINT_COLOR, boxShadow: '0 1px 3px rgba(0,0,0,.3)' }}
          />
          Kontrollpunkt
        </span>
        {smooth && (
          <span className="flex items-center gap-1.5 text-orange-500 font-medium">
            ~ Catmull-Rom aktiv
          </span>
        )}
      </div>
    </div>
  )
}
