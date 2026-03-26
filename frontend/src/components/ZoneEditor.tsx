/**
 * ZoneEditor — universal circle/polygon zone drawing on a MapLibre map.
 *
 * Handles:
 *  - Circle mode: click to place center, drag marker, radius slider + number input
 *  - Polygon mode: click to add vertices, double-click or button to close,
 *                  draggable vertex markers after closing
 *  - Holes: after main polygon is closed, "Ausschnitt hinzufügen" switches to
 *           hole-drawing mode; additional rings stored in polygon.coordinates[1+]
 *  - Optional pin marker (showPin=true) that must stay within the zone
 *
 * Used by: privacy zones, location tags, automation rules, …
 */

import { useRef, useEffect, useState, useCallback } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import pencilCursorSvg from '@/assets/pencil-cursor.svg'

// ── Types ─────────────────────────────────────────────────────────────────────

export interface ZoneData {
  mode: 'circle' | 'polygon'
  /** [lng, lat] */
  center?: [number, number]
  radiusM?: number
  polygon?: { type: 'Polygon'; coordinates: number[][][] }
  /** [lng, lat] — only relevant when showPin=true */
  pinPosition?: [number, number]
}

export interface ZoneEditorProps {
  value: ZoneData
  onChange: (zone: ZoneData) => void
  /** Minimum radius in metres (default: 50) */
  minRadiusM?: number
  /** Maximum radius in metres (default: 50000) */
  maxRadiusM?: number
  /** Show a separate, draggable pin inside the zone (default: false) */
  showPin?: boolean
  /** Enable hole (cut-out) drawing after outer polygon is closed (default: false) */
  enableHoles?: boolean
  /** Versteckt den internen Kreis/Polygon-Toggle (wenn der Modus extern gesteuert wird) */
  hideModeSwitcher?: boolean
  className?: string
  /** MapLibre style URL or style object; defaults to OSM raster */
  mapStyle?: string
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/** Approximate circle as a 64-vertex GeoJSON Polygon */
function circlePolygon(
  center: [number, number],
  radiusM: number,
  steps = 64,
): GeoJSON.Feature<GeoJSON.Polygon> {
  const coords: [number, number][] = []
  const [lng, lat] = center
  for (let i = 0; i <= steps; i++) {
    const angle = (i / steps) * 2 * Math.PI
    const dx = radiusM * Math.cos(angle)
    const dy = radiusM * Math.sin(angle)
    const dlat = dy / 111320
    const dlng = dx / (111320 * Math.cos((lat * Math.PI) / 180))
    coords.push([lng + dlng, lat + dlat])
  }
  return {
    type: 'Feature',
    properties: {},
    geometry: { type: 'Polygon', coordinates: [coords] },
  }
}

function formatRadius(m: number): string {
  if (m >= 1000) return `${(m / 1000).toFixed(1).replace('.0', '')} km`
  return `${m} m`
}

/** Clamp a point inside a circle */
function clampToCircle(
  point: [number, number],
  center: [number, number],
  radiusM: number,
): [number, number] {
  const [plng, plat] = point
  const [clng, clat] = center
  const dx = (plng - clng) * 111320 * Math.cos((clat * Math.PI) / 180)
  const dy = (plat - clat) * 111320
  const dist = Math.sqrt(dx * dx + dy * dy)
  if (dist <= radiusM) return point
  const scale = (radiusM * 0.95) / dist
  return [clng + (dx * scale) / (111320 * Math.cos((clat * Math.PI) / 180)), clat + (dy * scale) / 111320]
}

/** Rough point-in-polygon (ray casting) */
function pointInPolygon(pt: [number, number], ring: [number, number][]): boolean {
  let inside = false
  const [x, y] = pt
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [xi, yi] = ring[i]
    const [xj, yj] = ring[j]
    const intersect =
      yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi
    if (intersect) inside = !inside
  }
  return inside
}

/** Centroid of a point array */
function centroid(points: [number, number][]): [number, number] {
  const sum = points.reduce(([ax, ay], [x, y]) => [ax + x, ay + y], [0, 0])
  return [sum[0] / points.length, sum[1] / points.length]
}

// The pencil SVG encoded as a data-URI so we can use it as a CSS cursor.
// Vite gives us the file URL; we convert it to a data-URI at runtime once.
let _pencilCursorCSS = `url("${pencilCursorSvg}") 1 15, crosshair`

// ── Cursor helpers ─────────────────────────────────────────────────────────────
// MapLibre setzt canvas.style.cursor intern bei jedem mousemove auf 'grab'.
// Inline-Styles lassen sich nicht zuverlässig überschreiben — daher CSS-Klassen
// mit !important auf dem Container-Div.

function injectCursorStyles() {
  if (document.getElementById('ze-cursor-styles')) return
  const style = document.createElement('style')
  style.id = 'ze-cursor-styles'
  style.textContent = [
    '.ze-cursor-crosshair .maplibregl-canvas { cursor: crosshair !important; }',
    '.ze-cursor-grabbing  .maplibregl-canvas { cursor: grabbing  !important; }',
  ].join('\n')
  document.head.appendChild(style)
}

function setCursorClass(el: HTMLElement, cursor: 'crosshair' | 'grabbing' | '') {
  el.classList.remove('ze-cursor-crosshair', 'ze-cursor-grabbing')
  if (cursor) el.classList.add(`ze-cursor-${cursor}`)
}

// ── Component ─────────────────────────────────────────────────────────────────

const DEFAULT_CENTER: [number, number] = [10.45, 51.16] // Germany
const DEFAULT_RADIUS = 200

type DrawingMode = 'idle' | 'hole'

export default function ZoneEditor({
  value,
  onChange,
  minRadiusM = 50,
  maxRadiusM = 50000,
  showPin = false,
  enableHoles = false,
  hideModeSwitcher = false,
  className,
  mapStyle,
}: ZoneEditorProps) {
  // ── Derive initial state from `value` ──────────────────────────────────────
  const initialMode = value.mode ?? 'circle'
  const initialCenter = value.center ?? DEFAULT_CENTER
  const initialRadius = value.radiusM ?? DEFAULT_RADIUS
  // Outer ring: coordinates[0] (without closing point)
  const initialPolygonPoints: [number, number][] =
    value.polygon?.coordinates[0]
      ? (value.polygon.coordinates[0].slice(0, -1) as [number, number][])
      : []
  const initialPolygonClosed = initialPolygonPoints.length >= 3
  // Holes: coordinates[1..N] (each without closing point)
  const initialHoles: [number, number][][] =
    value.polygon?.coordinates.slice(1).map(ring =>
      ring.slice(0, -1) as [number, number][],
    ) ?? []

  // ── State ──────────────────────────────────────────────────────────────────
  const [mode, setMode] = useState<'circle' | 'polygon'>(initialMode)
  const [center, setCenter] = useState<[number, number]>(initialCenter)
  const [radiusM, setRadiusM] = useState(initialRadius)
  const [radiusInput, setRadiusInput] = useState(String(initialRadius))
  const [polygonPoints, setPolygonPoints] = useState<[number, number][]>(initialPolygonPoints)
  const [polygonClosed, setPolygonClosed] = useState(initialPolygonClosed)
  const [pinPos, setPinPos] = useState<[number, number] | undefined>(value.pinPosition)

  // Holes state
  const [holes, setHoles] = useState<[number, number][][]>(initialHoles)
  const [drawingMode, setDrawingMode] = useState<DrawingMode>('idle')
  const [currentHolePoints, setCurrentHolePoints] = useState<[number, number][]>([])

  // ── Refs (stable across renders, read inside MapLibre handlers) ───────────
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const centerMarkerRef = useRef<maplibregl.Marker | null>(null)
  const pinMarkerRef = useRef<maplibregl.Marker | null>(null)
  const polyMarkersRef = useRef<maplibregl.Marker[]>([])
  const holeMarkersRef = useRef<maplibregl.Marker[][]>([]) // per-hole vertex markers
  const isDraggingVertexRef = useRef(false)
  const isDraggingHoleRef = useRef(false)

  // These refs let MapLibre event handlers (registered once on mount) read
  // the latest React state without re-registering.
  const modeRef = useRef(initialMode)
  const centerRef = useRef(initialCenter)
  const radiusRef = useRef(initialRadius)
  const polygonClosedRef = useRef(initialPolygonClosed)
  const polygonPointsRef = useRef(initialPolygonPoints)
  const pinPosRef = useRef<[number, number] | undefined>(value.pinPosition)
  const drawingModeRef = useRef<DrawingMode>('idle')
  const currentHolePointsRef = useRef<[number, number][]>([])
  const holesRef = useRef<[number, number][][]>(initialHoles)

  // Keep refs in sync with state
  useEffect(() => { modeRef.current = mode }, [mode])
  useEffect(() => { centerRef.current = center }, [center])
  useEffect(() => { radiusRef.current = radiusM }, [radiusM])
  useEffect(() => { polygonClosedRef.current = polygonClosed }, [polygonClosed])
  useEffect(() => { polygonPointsRef.current = polygonPoints }, [polygonPoints])
  useEffect(() => { pinPosRef.current = pinPos }, [pinPos])
  useEffect(() => { drawingModeRef.current = drawingMode }, [drawingMode])
  useEffect(() => { currentHolePointsRef.current = currentHolePoints }, [currentHolePoints])
  useEffect(() => { holesRef.current = holes }, [holes])

  // ── Build full polygon coordinates (outer ring + holes) ────────────────────
  function buildFullCoords(
    outerPts: [number, number][],
    holeList: [number, number][][],
  ): number[][][] {
    const outer = [...outerPts, outerPts[0]] as [number, number][]
    const holeRings = holeList.map(h => [...h, h[0]] as [number, number][])
    return [outer, ...holeRings]
  }

  // ── Emit onChange whenever the zone data changes ───────────────────────────
  useEffect(() => {
    if (mode === 'circle') {
      onChange({
        mode: 'circle',
        center,
        radiusM,
        pinPosition: showPin ? pinPos : undefined,
      })
    } else {
      if (polygonClosed && polygonPoints.length >= 3) {
        const coords = buildFullCoords(polygonPoints, holes)
        onChange({
          mode: 'polygon',
          polygon: { type: 'Polygon', coordinates: coords },
          pinPosition: showPin ? pinPos : undefined,
        })
      } else {
        onChange({
          mode: 'polygon',
          pinPosition: showPin ? pinPos : undefined,
        })
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, center, radiusM, polygonPoints, polygonClosed, pinPos, showPin, holes])

  // ── Map layer helpers ──────────────────────────────────────────────────────

  const updateCircleLayer = useCallback((map: maplibregl.Map, c: [number, number], r: number) => {
    const src = map.getSource('ze-circle') as maplibregl.GeoJSONSource | undefined
    src?.setData(circlePolygon(c, r))
  }, [])

  /** Update the main polygon layer with outer ring + holes (evenodd fill-rule) */
  const updatePolygonLayer = useCallback(
    (map: maplibregl.Map, points: [number, number][], closed: boolean, holeList: [number, number][][]) => {
      const src = map.getSource('ze-polygon') as maplibregl.GeoJSONSource | undefined
      if (!src) return

      if (points.length === 0) {
        src.setData({ type: 'FeatureCollection', features: [] })
        return
      }
      if (closed && points.length >= 3) {
        const coords = buildFullCoords(points, holeList)
        src.setData({
          type: 'FeatureCollection',
          features: [{
            type: 'Feature',
            properties: {},
            geometry: { type: 'Polygon', coordinates: coords },
          }],
        })
      } else {
        // Still drawing — show a LineString
        src.setData({
          type: 'FeatureCollection',
          features: [{
            type: 'Feature',
            properties: {},
            geometry: { type: 'LineString', coordinates: points },
          }],
        })
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  /** Update the current-hole-in-progress preview layer */
  const updateHolePreviewLayer = useCallback(
    (map: maplibregl.Map, pts: [number, number][], closed: boolean) => {
      const src = map.getSource('ze-hole-preview') as maplibregl.GeoJSONSource | undefined
      if (!src) return
      if (pts.length === 0) {
        src.setData({ type: 'FeatureCollection', features: [] })
        return
      }
      const coords = closed && pts.length >= 3 ? [...pts, pts[0]] : pts
      src.setData({
        type: 'FeatureCollection',
        features: [{
          type: 'Feature',
          properties: {},
          geometry: closed && pts.length >= 3
            ? { type: 'Polygon', coordinates: [coords] }
            : { type: 'LineString', coordinates: coords },
        }],
      })
    },
    [],
  )

  const updateVertexMarkers = useCallback(
    (map: maplibregl.Map, points: [number, number][]) => {
      polyMarkersRef.current.forEach(m => m.remove())
      polyMarkersRef.current = []
      points.forEach((pt, idx) => {
        const el = document.createElement('div')
        el.style.cssText = `
          width:10px;height:10px;border-radius:50%;
          background:#F97316;border:2px solid white;
          box-shadow:0 1px 4px rgba(0,0,0,.4);cursor:grab;
        `
        const marker = new maplibregl.Marker({ element: el, draggable: true })
          .setLngLat(pt)
          .addTo(map)
        marker.on('dragstart', () => { isDraggingVertexRef.current = true })
        marker.on('drag', () => {
          const pos = marker.getLngLat()
          const updated = [...polygonPointsRef.current]
          updated[idx] = [pos.lng, pos.lat]
          polygonPointsRef.current = updated
          if (map.isStyleLoaded()) updatePolygonLayer(map, updated, polygonClosedRef.current, holesRef.current)
        })
        marker.on('dragend', () => {
          isDraggingVertexRef.current = false
          setPolygonPoints([...polygonPointsRef.current])
        })
        polyMarkersRef.current.push(marker)
      })
    },
    [],
  )

  /** Re-render vertex markers for all holes */
  const updateAllHoleMarkers = useCallback(
    (map: maplibregl.Map, holeList: [number, number][][]) => {
      // Remove all existing hole markers
      holeMarkersRef.current.forEach(group => group.forEach(m => m.remove()))
      holeMarkersRef.current = []

      holeList.forEach((holePts, holeIdx) => {
        const group: maplibregl.Marker[] = []
        holePts.forEach((pt, vtxIdx) => {
          const el = document.createElement('div')
          el.style.cssText = `
            width:8px;height:8px;border-radius:50%;
            background:#60A5FA;border:2px solid white;
            box-shadow:0 1px 4px rgba(0,0,0,.4);cursor:grab;
          `
          const marker = new maplibregl.Marker({ element: el, draggable: true })
            .setLngLat(pt)
            .addTo(map)
          marker.on('dragstart', () => { isDraggingHoleRef.current = true })
          marker.on('drag', () => {
            const pos = marker.getLngLat()
            const updated = holesRef.current.map(h => [...h]) as [number, number][][]
            updated[holeIdx][vtxIdx] = [pos.lng, pos.lat]
            holesRef.current = updated
            if (map.isStyleLoaded()) updatePolygonLayer(map, polygonPointsRef.current, polygonClosedRef.current, updated)
          })
          marker.on('dragend', () => {
            isDraggingHoleRef.current = false
            setHoles([...holesRef.current])
          })
          group.push(marker)
        })
        holeMarkersRef.current.push(group)
      })
    },
    [updatePolygonLayer],
  )

  /** Place or move the center marker; create it if it doesn't exist yet */
  const syncCenterMarker = useCallback(
    (map: maplibregl.Map, lngLat: [number, number]) => {
      if (centerMarkerRef.current) {
        centerMarkerRef.current.setLngLat(lngLat)
      } else {
        const marker = new maplibregl.Marker({ color: '#F97316', draggable: true })
          .setLngLat(lngLat)
          .addTo(map)
        marker.on('dragend', () => {
          const pos = marker.getLngLat()
          const newCenter: [number, number] = [pos.lng, pos.lat]
          setCenter(newCenter)
          centerRef.current = newCenter
          updateCircleLayer(map, newCenter, radiusRef.current)
          // Clamp pin if needed
          if (showPin && pinPosRef.current) {
            const clamped = clampToCircle(pinPosRef.current, newCenter, radiusRef.current)
            setPinPos(clamped)
            pinPosRef.current = clamped
            pinMarkerRef.current?.setLngLat(clamped)
          }
        })
        centerMarkerRef.current = marker
      }
    },
    [showPin, updateCircleLayer],
  )

  /** Place or move the pin marker */
  const syncPinMarker = useCallback(
    (map: maplibregl.Map, lngLat: [number, number]) => {
      if (pinMarkerRef.current) {
        pinMarkerRef.current.setLngLat(lngLat)
      } else {
        const el = document.createElement('div')
        el.style.cssText = `
          width:14px;height:14px;border-radius:50%;
          background:#3B82F6;border:2px solid white;
          box-shadow:0 1px 4px rgba(0,0,0,.5);cursor:grab;
        `
        const marker = new maplibregl.Marker({ element: el, draggable: true })
          .setLngLat(lngLat)
          .addTo(map)
        marker.on('dragend', () => {
          const pos = marker.getLngLat()
          let newPin: [number, number] = [pos.lng, pos.lat]
          if (modeRef.current === 'circle' && centerRef.current) {
            newPin = clampToCircle(newPin, centerRef.current, radiusRef.current)
          } else if (modeRef.current === 'polygon' && polygonPointsRef.current.length >= 3) {
            if (!pointInPolygon(newPin, polygonPointsRef.current)) {
              newPin = centroid(polygonPointsRef.current)
            }
          }
          setPinPos(newPin)
          pinPosRef.current = newPin
          marker.setLngLat(newPin)
        })
        pinMarkerRef.current = marker
      }
    },
    [],
  )

  // ── Visibility helpers ─────────────────────────────────────────────────────

  function applyModeVisibility(map: maplibregl.Map, m: 'circle' | 'polygon') {
    const circleVis = m === 'circle' ? 'visible' : 'none'
    const polyVis = m === 'polygon' ? 'visible' : 'none'
    if (map.getLayer('ze-circle-fill')) map.setLayoutProperty('ze-circle-fill', 'visibility', circleVis)
    if (map.getLayer('ze-circle-border')) map.setLayoutProperty('ze-circle-border', 'visibility', circleVis)
    if (map.getLayer('ze-polygon-fill')) map.setLayoutProperty('ze-polygon-fill', 'visibility', polyVis)
    if (map.getLayer('ze-polygon-line')) map.setLayoutProperty('ze-polygon-line', 'visibility', polyVis)
    if (map.getLayer('ze-hole-preview-fill')) map.setLayoutProperty('ze-hole-preview-fill', 'visibility', polyVis)
    if (map.getLayer('ze-hole-preview-line')) map.setLayoutProperty('ze-hole-preview-line', 'visibility', polyVis)
  }

  // ── Map initialisation ────────────────────────────────────────────────────

  useEffect(() => {
    if (!mapContainerRef.current) return

    const inlineStyle: maplibregl.StyleSpecification = {
      version: 8,
      sources: {
        osm: {
          type: 'raster',
          tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
          tileSize: 256,
          attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
        },
      },
      layers: [{ id: 'osm-tiles', type: 'raster', source: 'osm', minzoom: 0, maxzoom: 19 }],
    }

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: mapStyle ?? inlineStyle,
      center: initialCenter,
      zoom: initialPolygonClosed || value.center ? 13 : 6,
      attributionControl: {},
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      // ── Circle source + layers ───────────────────────────────────────────
      map.addSource('ze-circle', {
        type: 'geojson',
        data: circlePolygon(initialCenter, initialRadius),
      })
      map.addLayer({
        id: 'ze-circle-fill',
        type: 'fill',
        source: 'ze-circle',
        paint: { 'fill-color': '#F97316', 'fill-opacity': 0.15 },
        layout: { visibility: modeRef.current === 'circle' ? 'visible' : 'none' },
      })
      map.addLayer({
        id: 'ze-circle-border',
        type: 'line',
        source: 'ze-circle',
        paint: { 'line-color': '#F97316', 'line-width': 2, 'line-opacity': 0.7 },
        layout: { visibility: modeRef.current === 'circle' ? 'visible' : 'none' },
      })

      // ── Polygon source + layers (evenodd for holes) ──────────────────────
      map.addSource('ze-polygon', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
      map.addLayer({
        id: 'ze-polygon-fill',
        type: 'fill',
        source: 'ze-polygon',
        paint: {
          'fill-color': '#F97316',
          'fill-opacity': 0.15,
          'fill-antialias': true,
        },
        filter: ['==', '$type', 'Polygon'],
        layout: { visibility: modeRef.current === 'polygon' ? 'visible' : 'none' },
      } as any)
      map.addLayer({
        id: 'ze-polygon-line',
        type: 'line',
        source: 'ze-polygon',
        paint: { 'line-color': '#F97316', 'line-width': 2, 'line-opacity': 0.7, 'line-dasharray': [3, 2] },
        layout: { visibility: modeRef.current === 'polygon' ? 'visible' : 'none' },
      })

      // ── Hole-in-progress preview layer ───────────────────────────────────
      map.addSource('ze-hole-preview', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
      map.addLayer({
        id: 'ze-hole-preview-fill',
        type: 'fill',
        source: 'ze-hole-preview',
        paint: { 'fill-color': '#60A5FA', 'fill-opacity': 0.15 },
        filter: ['==', '$type', 'Polygon'],
        layout: { visibility: modeRef.current === 'polygon' ? 'visible' : 'none' },
      })
      map.addLayer({
        id: 'ze-hole-preview-line',
        type: 'line',
        source: 'ze-hole-preview',
        paint: { 'line-color': '#60A5FA', 'line-width': 2, 'line-opacity': 0.8, 'line-dasharray': [2, 2] },
        layout: { visibility: modeRef.current === 'polygon' ? 'visible' : 'none' },
      })

      // ── Restore initial state ────────────────────────────────────────────
      if (modeRef.current === 'circle' && value.center) {
        syncCenterMarker(map, value.center)
        updateCircleLayer(map, value.center, initialRadius)
      }

      if (modeRef.current === 'polygon' && initialPolygonPoints.length >= 3) {
        updatePolygonLayer(map, initialPolygonPoints, true, initialHoles)
        updateVertexMarkers(map, initialPolygonPoints)
        updateAllHoleMarkers(map, initialHoles)
        const bounds = new maplibregl.LngLatBounds()
        initialPolygonPoints.forEach(p => bounds.extend(p))
        map.fitBounds(bounds, { padding: 60, maxZoom: 16, duration: 600 })
      }

      if (showPin && value.pinPosition) {
        syncPinMarker(map, value.pinPosition)
      }
    })

    // ── Click handler ──────────────────────────────────────────────────────
    map.on('click', (e) => {
      const lngLat: [number, number] = [e.lngLat.lng, e.lngLat.lat]

      if (modeRef.current === 'circle') {
        setCenter(lngLat)
        centerRef.current = lngLat
        syncCenterMarker(map, lngLat)
        updateCircleLayer(map, lngLat, radiusRef.current)
        if (showPin && !pinPosRef.current) {
          setPinPos(lngLat)
          pinPosRef.current = lngLat
          syncPinMarker(map, lngLat)
        }
      } else if (drawingModeRef.current === 'hole') {
        // Hole drawing mode
        setCurrentHolePoints(prev => {
          const updated = [...prev, lngLat]
          currentHolePointsRef.current = updated
          if (map.isStyleLoaded()) {
            updateHolePreviewLayer(map, updated, false)
          }
          return updated
        })
      } else {
        // Polygon outer ring mode — only add vertices if not yet closed
        if (polygonClosedRef.current) return
        const pts = polygonPointsRef.current
        // Klick nahe am ersten Punkt (≥3 Punkte) → Polygon automatisch schließen
        if (pts.length >= 3) {
          const firstPx = map.project(pts[0])
          const clickPx = map.project(lngLat)
          const dx = firstPx.x - clickPx.x
          const dy = firstPx.y - clickPx.y
          if (dx * dx + dy * dy <= 15 * 15) {
            polygonClosedRef.current = true
            setPolygonClosed(true)
            if (map.isStyleLoaded()) {
              updatePolygonLayer(map, pts, true, holesRef.current)
              updateVertexMarkers(map, pts)
            }
            if (showPin && !pinPosRef.current) {
              const c = centroid(pts)
              setPinPos(c)
              pinPosRef.current = c
              syncPinMarker(map, c)
            }
            return
          }
        }
        setPolygonPoints(prev => {
          const updated = [...prev, lngLat]
          polygonPointsRef.current = updated
          if (map.isStyleLoaded()) {
            updatePolygonLayer(map, updated, false, holesRef.current)
            updateVertexMarkers(map, updated)
          }
          return updated
        })
      }
    })

    // ── Double-click closes polygon or hole ────────────────────────────────
    map.on('dblclick', (e) => {
      e.preventDefault()

      if (drawingModeRef.current === 'hole') {
        // Close hole
        const pts = currentHolePointsRef.current
        if (pts.length < 3) return
        const newHoles = [...holesRef.current, pts]
        setHoles(newHoles)
        holesRef.current = newHoles
        setCurrentHolePoints([])
        currentHolePointsRef.current = []
        setDrawingMode('idle')
        drawingModeRef.current = 'idle'
        if (map.isStyleLoaded()) {
          updateHolePreviewLayer(map, [], false)
          updatePolygonLayer(map, polygonPointsRef.current, polygonClosedRef.current, newHoles)
          updateAllHoleMarkers(map, newHoles)
        }
        setCursorClass(mapContainerRef.current!, '')
        return
      }

      if (modeRef.current !== 'polygon' || polygonClosedRef.current) return
      setPolygonPoints(prev => {
        if (prev.length < 3) return prev
        polygonClosedRef.current = true
        setPolygonClosed(true)
        if (map.isStyleLoaded()) {
          updatePolygonLayer(map, prev, true, holesRef.current)
          updateVertexMarkers(map, prev)
        }
        if (showPin && !pinPosRef.current) {
          const c = centroid(prev)
          setPinPos(c)
          pinPosRef.current = c
          syncPinMarker(map, c)
        }
        return prev
      })
    })

    mapRef.current = map

    // Cursor-Management via CSS-Klassen auf dem Container (schlägt MapLibres inline cursor).
    // MapLibre setzt canvas.style.cursor = 'grab' bei jedem internen mousemove — das lässt
    // sich nur mit !important in einer CSS-Klasse auf einem Ancestor zuverlässig überschreiben.
    injectCursorStyles()
    const container = mapContainerRef.current!
    let isMouseDown = false
    const applyCursor = () => {
      if (isMouseDown) return
      if (drawingModeRef.current === 'hole' || (modeRef.current === 'polygon' && !polygonClosedRef.current)) {
        setCursorClass(container, 'crosshair')
      } else {
        setCursorClass(container, '')
      }
    }
    const onMousemove = () => applyCursor()
    const onMousedown = () => { isMouseDown = true;  setCursorClass(container, 'grabbing') }
    const onMouseup   = () => { isMouseDown = false; applyCursor() }
    const canvas = map.getCanvas()
    canvas.addEventListener('mousemove', onMousemove)
    canvas.addEventListener('mousedown', onMousedown)
    canvas.addEventListener('mouseup',   onMouseup)

    return () => {
      canvas.removeEventListener('mousemove', onMousemove)
      canvas.removeEventListener('mousedown', onMousedown)
      canvas.removeEventListener('mouseup',   onMouseup)
      setCursorClass(container, '')
      map.remove()
      mapRef.current = null
      centerMarkerRef.current = null
      pinMarkerRef.current = null
      polyMarkersRef.current = []
      holeMarkersRef.current = []
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── Mode switch effect ─────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const apply = () => {
      applyModeVisibility(map, mode)

      if (mode === 'circle') {
        // Remove vertex markers, restore circle state
        polyMarkersRef.current.forEach(m => m.remove())
        polyMarkersRef.current = []
        holeMarkersRef.current.forEach(g => g.forEach(m => m.remove()))
        holeMarkersRef.current = []
        if (centerRef.current) {
          syncCenterMarker(map, centerRef.current)
          updateCircleLayer(map, centerRef.current, radiusRef.current)
        }
      } else {
        // Remove circle center marker
        centerMarkerRef.current?.remove()
        centerMarkerRef.current = null
        updatePolygonLayer(map, polygonPointsRef.current, polygonClosedRef.current, holesRef.current)
        if (polygonClosedRef.current) {
          updateVertexMarkers(map, polygonPointsRef.current)
          updateAllHoleMarkers(map, holesRef.current)
        }
      }

      // Set drawing cursor for polygon mode when not yet closed
      if (mode === 'polygon' && !polygonClosedRef.current) {
        setCursorClass(mapContainerRef.current!, 'crosshair')
      } else {
        setCursorClass(mapContainerRef.current!, '')
      }
    }

    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode])

  // ── Radius change effect ───────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'circle' || !centerRef.current) return
    const apply = () => updateCircleLayer(map, centerRef.current!, radiusM)
    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [radiusM])

  // ── Polygon points effect (vertex drag) ────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'polygon') return
    const apply = () => {
      updatePolygonLayer(map, polygonPoints, polygonClosed, holes)
      if (polygonClosed && !isDraggingVertexRef.current) updateVertexMarkers(map, polygonPoints)
    }
    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [polygonPoints, polygonClosed])

  // ── Holes effect ───────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || mode !== 'polygon' || !polygonClosed) return
    const apply = () => {
      updatePolygonLayer(map, polygonPoints, polygonClosed, holes)
      if (!isDraggingHoleRef.current) updateAllHoleMarkers(map, holes)
    }
    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [holes])

  // ── Cursor-Klasse aktualisieren wenn Zeichen-Modus wechselt ──────────────────
  useEffect(() => {
    const el = mapContainerRef.current
    if (!el) return
    const drawing = drawingMode === 'hole' || (mode === 'polygon' && !polygonClosed)
    setCursorClass(el, drawing ? 'crosshair' : '')
  }, [mode, polygonClosed, drawingMode])

  // ── UI action handlers ─────────────────────────────────────────────────────

  function handleClosePolygon() {
    if (polygonPoints.length < 3 || polygonClosed) return
    setPolygonClosed(true)
    polygonClosedRef.current = true
    const map = mapRef.current
    if (map?.isStyleLoaded()) {
      updatePolygonLayer(map, polygonPoints, true, holes)
      updateVertexMarkers(map, polygonPoints)
    }
    if (showPin && !pinPos) {
      const c = centroid(polygonPoints)
      setPinPos(c)
      pinPosRef.current = c
      if (map?.isStyleLoaded()) syncPinMarker(map, c)
    }
    setCursorClass(mapContainerRef.current!, '')
  }

  function handleResetPolygon() {
    setPolygonPoints([])
    setPolygonClosed(false)
    polygonClosedRef.current = false
    polygonPointsRef.current = []
    setHoles([])
    holesRef.current = []
    setCurrentHolePoints([])
    currentHolePointsRef.current = []
    setDrawingMode('idle')
    drawingModeRef.current = 'idle'
    const map = mapRef.current
    if (map?.isStyleLoaded()) {
      updatePolygonLayer(map, [], false, [])
      updateHolePreviewLayer(map, [], false)
      polyMarkersRef.current.forEach(m => m.remove())
      polyMarkersRef.current = []
      holeMarkersRef.current.forEach(g => g.forEach(m => m.remove()))
      holeMarkersRef.current = []
    }
    if (map) {
      setCursorClass(mapContainerRef.current!, 'crosshair')
    }
  }

  function handleStartHole() {
    setDrawingMode('hole')
    drawingModeRef.current = 'hole'
    setCurrentHolePoints([])
    currentHolePointsRef.current = []
    const map = mapRef.current
    if (map) setCursorClass(mapContainerRef.current!, 'crosshair')
  }

  function handleCloseHole() {
    if (currentHolePoints.length < 3) return
    const map = mapRef.current
    const newHoles = [...holes, currentHolePoints]
    setHoles(newHoles)
    holesRef.current = newHoles
    setCurrentHolePoints([])
    currentHolePointsRef.current = []
    setDrawingMode('idle')
    drawingModeRef.current = 'idle'
    if (map?.isStyleLoaded()) {
      updateHolePreviewLayer(map, [], false)
      updatePolygonLayer(map, polygonPoints, polygonClosed, newHoles)
      updateAllHoleMarkers(map, newHoles)
    }
    if (map) setCursorClass(mapContainerRef.current!, '')
  }

  function handleCancelHole() {
    setDrawingMode('idle')
    drawingModeRef.current = 'idle'
    setCurrentHolePoints([])
    currentHolePointsRef.current = []
    const map = mapRef.current
    if (map?.isStyleLoaded()) updateHolePreviewLayer(map, [], false)
    if (map) setCursorClass(mapContainerRef.current!, '')
  }

  function handleRemoveHole(holeIdx: number) {
    const newHoles = holes.filter((_, i) => i !== holeIdx)
    setHoles(newHoles)
    holesRef.current = newHoles
    const map = mapRef.current
    if (map?.isStyleLoaded()) {
      updatePolygonLayer(map, polygonPoints, polygonClosed, newHoles)
      updateAllHoleMarkers(map, newHoles)
    }
  }

  function handleRadiusInputBlur() {
    const parsed = parseInt(radiusInput, 10)
    if (!isNaN(parsed)) {
      const clamped = Math.max(minRadiusM, Math.min(maxRadiusM, parsed))
      setRadiusM(clamped)
      setRadiusInput(String(clamped))
    } else {
      setRadiusInput(String(radiusM))
    }
  }

  function handleRadiusInputKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') e.currentTarget.blur()
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  const canClosePolygon = polygonPoints.length >= 3 && !polygonClosed
  const canAddHole = enableHoles && polygonClosed && drawingMode === 'idle'
  const isDrawingHole = drawingMode === 'hole'

  return (
    <div className={`space-y-4 ${className ?? ''}`}>
      {/* Mode toggle — versteckt wenn extern gesteuert */}
      {!hideModeSwitcher && (
        <div className="flex gap-2">
          {(['circle', 'polygon'] as const).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMode(m)}
              className={`flex-1 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer transition-colors border ${
                mode === m
                  ? 'bg-accent-DEFAULT border-accent-DEFAULT text-white'
                  : 'border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
              }`}
            >
              {m === 'circle' ? 'Kreis' : 'Polygon'}
            </button>
          ))}
        </div>
      )}

      {/* Map */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <div ref={mapContainerRef} className="h-[380px] w-full" />
      </div>

      {/* Instruction text */}
      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        {mode === 'circle' ? (
          'Klicke auf die Karte, um das Zentrum zu setzen. Den Marker kannst du verschieben.'
        ) : isDrawingHole ? (
          `Ausschnitt zeichnen: ${currentHolePoints.length} Punkte. Doppelklick oder "Schliessen" zum Beenden.`
        ) : polygonClosed ? (
          'Polygon fertig. Eckpunkte lassen sich per Drag-and-Drop verschieben.'
        ) : polygonPoints.length < 3 ? (
          `Klicke auf die Karte, um Eckpunkte hinzuzufügen (${polygonPoints.length} von mind. 3).`
        ) : (
          `${polygonPoints.length} Eckpunkte gesetzt. Doppelklick oder "Schließen" um das Polygon zu schließen.`
        )}
      </p>

      {/* Polygon controls */}
      {mode === 'polygon' && (
        <div className="flex flex-wrap gap-2 items-center">
          {!polygonClosed && (
            <>
              <button
                type="button"
                onClick={handleClosePolygon}
                disabled={!canClosePolygon}
                className="px-3 py-1.5 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-700
                  text-neutral-700 dark:text-neutral-300 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed
                  hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
              >
                Polygon schliessen
              </button>
              <button
                type="button"
                onClick={handleResetPolygon}
                disabled={polygonPoints.length === 0}
                className="px-3 py-1.5 rounded-lg text-sm font-medium border border-red-200 dark:border-red-900
                  text-red-600 dark:text-red-400 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed
                  hover:bg-red-50 dark:hover:bg-red-950 transition-colors"
              >
                Zurücksetzen
              </button>
            </>
          )}

          {/* Hole controls when outer polygon is closed */}
          {polygonClosed && enableHoles && (
            <>
              {isDrawingHole ? (
                <>
                  <button
                    type="button"
                    onClick={handleCloseHole}
                    disabled={currentHolePoints.length < 3}
                    className="px-3 py-1.5 rounded-lg text-sm font-medium border border-blue-300 dark:border-blue-700
                      text-blue-700 dark:text-blue-300 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed
                      hover:bg-blue-50 dark:hover:bg-blue-950 transition-colors"
                  >
                    Ausschnitt schliessen
                  </button>
                  <button
                    type="button"
                    onClick={handleCancelHole}
                    className="px-3 py-1.5 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-700
                      text-neutral-600 dark:text-neutral-400
                      hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                  >
                    Abbrechen
                  </button>
                  <span className="text-xs text-blue-500 dark:text-blue-400">
                    {currentHolePoints.length} Punkte
                  </span>
                </>
              ) : (
                <button
                  type="button"
                  onClick={handleStartHole}
                  disabled={!canAddHole}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium border border-blue-200 dark:border-blue-800
                    text-blue-600 dark:text-blue-400 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed
                    hover:bg-blue-50 dark:hover:bg-blue-950 transition-colors"
                >
                  + Ausschnitt hinzufügen
                </button>
              )}
            </>
          )}

          {polygonClosed && (
            <button
              type="button"
              onClick={handleResetPolygon}
              className="ml-auto px-3 py-1.5 rounded-lg text-sm font-medium border border-red-200 dark:border-red-900
                text-red-600 dark:text-red-400
                hover:bg-red-50 dark:hover:bg-red-950 transition-colors cursor-pointer"
            >
              Zurücksetzen
            </button>
          )}

          {!isDrawingHole && (
            <span className="ml-auto text-sm text-neutral-400 tabular-nums">
              {polygonPoints.length} Eckpunkte
              {holes.length > 0 && `, ${holes.length} Ausschnitt${holes.length > 1 ? 'e' : ''}`}
            </span>
          )}
        </div>
      )}

      {/* Hole list (remove individual holes) */}
      {mode === 'polygon' && enableHoles && holes.length > 0 && (
        <div className="space-y-1">
          {holes.map((hole, idx) => (
            <div key={idx} className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
              <span className="w-2 h-2 rounded-full bg-blue-400 flex-shrink-0" />
              <span>Ausschnitt {idx + 1} ({hole.length} Punkte)</span>
              <button
                type="button"
                onClick={() => handleRemoveHole(idx)}
                className="ml-auto text-neutral-400 hover:text-red-500 cursor-pointer transition-colors"
                title="Ausschnitt entfernen"
              >
                <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Circle radius */}
      {mode === 'circle' && (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Radius
            </label>
            <div className="flex items-center gap-1.5">
              <input
                type="number"
                value={radiusInput}
                min={minRadiusM}
                max={maxRadiusM}
                onChange={(e) => setRadiusInput(e.target.value)}
                onBlur={handleRadiusInputBlur}
                onKeyDown={handleRadiusInputKeyDown}
                className="w-24 text-right border border-neutral-300 dark:border-neutral-700 rounded px-2 py-0.5
                  text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100
                  focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent tabular-nums"
              />
              <span className="text-sm text-neutral-500">m</span>
              <span className="text-sm text-neutral-400 tabular-nums ml-1">
                ({formatRadius(radiusM)})
              </span>
            </div>
          </div>
          <input
            type="range"
            min={minRadiusM}
            max={maxRadiusM}
            step={radiusM < 1000 ? 50 : 500}
            value={radiusM}
            onChange={(e) => {
              const v = Number(e.target.value)
              setRadiusM(v)
              setRadiusInput(String(v))
            }}
            className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700
              accent-accent-DEFAULT cursor-pointer"
          />
          <div className="flex justify-between text-xs text-neutral-400">
            <span>{formatRadius(minRadiusM)}</span>
            <span>{formatRadius(maxRadiusM)}</span>
          </div>
        </div>
      )}

      {/* Pin info when showPin */}
      {showPin && (
        <p className="text-xs text-neutral-400 dark:text-neutral-500">
          {pinPos
            ? `Pin: ${pinPos[1].toFixed(5)}, ${pinPos[0].toFixed(5)}`
            : 'Pin wird nach dem Setzen des Zentrums platziert.'}
        </p>
      )}

      {/* Coordinates readout (circle mode) */}
      {mode === 'circle' && center && (
        <p className="text-xs text-neutral-400 tabular-nums">
          {center[1].toFixed(5)}, {center[0].toFixed(5)}
        </p>
      )}
    </div>
  )
}
