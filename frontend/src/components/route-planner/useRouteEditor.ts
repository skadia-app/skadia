/**
 * useRouteEditor — MapLibre interaction hook for the route planner.
 *
 * Responsibilities:
 *  - Click on map        → add waypoint
 *  - Drag waypoint marker → re-route
 *  - Click on route line  → insert waypoint at that position
 *  - Right-click waypoint → context menu (delete / set start|end)
 *  - Render route layers  (solid line for routed, dashed for offroad)
 *  - Render waypoint markers (large ● for start/end, medium ● for via)
 *  - Hover interactions on route line and markers
 */

import { useEffect, useRef, useCallback } from 'react'
import maplibregl from 'maplibre-gl'
import type { RouteWaypoint, RouteSegment } from '@/api/routes'
import type { SegmentKey } from '@/hooks/useRoutePlanner'
import { nearestPointOnPolyline } from '@/lib/catmull-rom'
import { SURFACE_GROUPS } from '@/lib/surfaceConstants'
import { filterSubSegmentsByDescriptor } from '@/lib/descriptorFilter'
import type { RouteSubSegment } from '@/lib/routeSurfaceAggregation'

// ── Layer / Source IDs ──────────────────────────────────────────────────────
const SOURCE_ROUTE       = 'route-planner-source'
const SOURCE_WP          = 'route-planner-waypoints-source'
const SOURCE_CP          = 'route-planner-control-points-source'
const SOURCE_APPROACH    = 'route-planner-approach-source'
const SOURCE_CALCULATING = 'route-planner-calculating-source'
const SOURCE_HIGHLIGHT   = 'route-planner-highlight-source'

const LAYER_ROUTED           = 'route-planner-line'
const LAYER_OFFROAD          = 'route-planner-offroad'
const LAYER_APPROACH         = 'route-planner-approach'
const LAYER_WP_CIRCLE        = 'route-planner-waypoints'
const LAYER_WP_LABEL         = 'route-planner-waypoints-label'
const LAYER_CP_CIRCLE        = 'route-planner-control-points'
const LAYER_CALCULATING      = 'route-planner-calculating'
const LAYER_HIGHLIGHT        = 'route-planner-highlight'
const LAYER_CALCULATING_GLOW = 'route-planner-calculating-glow'

// Pixel radius for "close enough to waypoint" snap on click
const CLICK_SNAP_PX = 20

// ── Segment context menu labels ──────────────────────────────────────────────
const SEGMENT_MENU_LABELS = {
  drawOffroad: 'Offroad zeichnen',
  routeOnRoads: 'Auf Wegen routen',
} as const

// ── Minimal planner interface required by this hook ─────────────────────────
// Mirrors what useRoutePlanner will expose (Task 27).
export interface RoutePlannerState {
  waypoints: RouteWaypoint[]
  segments: RouteSegment[]
  isCalculating?: boolean
  highlightedType?:
    | { kind: 'highway'; key: string }
    | { kind: 'surface'; key: string }
    | { kind: 'descriptor'; descriptorKey: 'mtb_technical' | 'alpine_hiking' | 'exposed' | 'low_visibility' }
    | null
  addWaypoint: (latlng: [number, number]) => void
  moveWaypoint: (id: string, latlng: [number, number]) => void
  removeWaypoint: (id: string) => void
  insertWaypoint: (afterId: string, latlng: [number, number]) => string | null
  setWaypointType: (id: string, type: RouteWaypoint['type']) => void
  toggleOffroad?: (key: SegmentKey) => void
  updateControlPoint?: (key: SegmentKey, cpIndex: number, latlng: [number, number]) => void
  insertControlPoint?: (key: SegmentKey, afterCpIndex: number, latlng: [number, number]) => void
  removeControlPoint?: (key: SegmentKey, cpIndex: number) => void
}

// ── Context menu (right-click on waypoint) ──────────────────────────────────
export interface WaypointContextMenuEvent {
  waypointId: string
  screenX: number
  screenY: number
}

export interface UseRouteEditorOptions {
  /** Called when the user right-clicks a waypoint marker. */
  onContextMenu?: (evt: WaypointContextMenuEvent) => void
}

// ── Build GeoJSON for route line ─────────────────────────────────────────────
function buildRouteGeoJSON(
  segments: RouteSegment[],
): GeoJSON.FeatureCollection<GeoJSON.LineString> {
  const features: GeoJSON.Feature<GeoJSON.LineString>[] = []
  for (const seg of segments) {
    if (!seg.geometry || seg.geometry.coordinates.length < 2) continue
    features.push({
      type: 'Feature',
      properties: {
        mode: seg.mode,
        segment_from: seg.from_waypoint,
        segment_to: seg.to_waypoint,
      },
      geometry: seg.geometry,
    })
  }
  return { type: 'FeatureCollection', features }
}

// ── Build GeoJSON for control points ────────────────────────────────────────
function buildControlPointGeoJSON(
  segments: RouteSegment[],
): GeoJSON.FeatureCollection<GeoJSON.Point> {
  const features: GeoJSON.Feature<GeoJSON.Point>[] = []
  for (const seg of segments) {
    if (seg.mode !== 'offroad') continue
    const cps = seg.control_points ?? []
    for (let i = 0; i < cps.length; i++) {
      features.push({
        type: 'Feature',
        properties: {
          segment_from: seg.from_waypoint,
          segment_to: seg.to_waypoint,
          cp_index: i,
        },
        geometry: {
          type: 'Point',
          // GeoJSON coordinates are [lng, lat]; control_points are [lat, lng]
          coordinates: [cps[i][1], cps[i][0]],
        },
      })
    }
  }
  return { type: 'FeatureCollection', features }
}

// ── Build GeoJSON for approach lines ────────────────────────────────────────
function buildApproachGeoJSON(
  segments: RouteSegment[],
): GeoJSON.FeatureCollection<GeoJSON.LineString> {
  const features: GeoJSON.Feature<GeoJSON.LineString>[] = []
  for (const seg of segments) {
    if (seg.approach_from && seg.approach_from.length >= 2) {
      features.push({
        type: 'Feature',
        properties: {},
        geometry: { type: 'LineString', coordinates: seg.approach_from },
      })
    }
    if (seg.approach_to && seg.approach_to.length >= 2) {
      features.push({
        type: 'Feature',
        properties: {},
        geometry: { type: 'LineString', coordinates: seg.approach_to },
      })
    }
  }
  return { type: 'FeatureCollection', features }
}

// ── Build GeoJSON for "calculating" segments (straight lines between unconnected consecutive waypoints) ──
// A pair of consecutive waypoints is "calculating" when no segment connects them yet, OR when an
// existing routed segment has no geometry (calculation still in progress).
function buildCalculatingGeoJSON(
  segments: RouteSegment[],
  waypoints: RouteWaypoint[],
): GeoJSON.FeatureCollection<GeoJSON.LineString> {
  const features: GeoJSON.Feature<GeoJSON.LineString>[] = []
  if (waypoints.length < 2) return { type: 'FeatureCollection', features }

  for (let i = 0; i < waypoints.length - 1; i++) {
    const from = waypoints[i]
    const to   = waypoints[i + 1]

    // Find the segment connecting this pair
    const seg = segments.find(
      s => s.from_waypoint === from.id && s.to_waypoint === to.id,
    )

    // Show calculating line when:
    // - No segment exists yet (just added waypoint, calculation not started)
    // - Segment exists but is routed with no geometry (in-flight calculation)
    const isCalculating =
      !seg ||
      (seg.mode === 'routed' && (!seg.geometry || seg.geometry.coordinates.length < 2))

    if (!isCalculating) continue

    features.push({
      type: 'Feature',
      properties: {},
      geometry: {
        type: 'LineString',
        coordinates: [
          [from.latlng[1], from.latlng[0]], // GeoJSON: [lng, lat]
          [to.latlng[1],   to.latlng[0]],
        ],
      },
    })
  }
  return { type: 'FeatureCollection', features }
}

// ── Build GeoJSON for waypoints ──────────────────────────────────────────────
function buildWaypointGeoJSON(
  waypoints: RouteWaypoint[],
): GeoJSON.FeatureCollection<GeoJSON.Point> {
  const features: GeoJSON.Feature<GeoJSON.Point>[] = waypoints.map((wp, idx) => ({
    type: 'Feature',
    properties: {
      id:    wp.id,
      type:  wp.type,
      index: idx,
      label: wp.type === 'start' ? 'A'
           : wp.type === 'end'   ? 'B'
           : String(idx),
      // via waypoints are smaller; start/end are large
      size:  wp.type === 'via' ? 8 : 14,
    },
    geometry: {
      type: 'Point',
      // GeoJSON coordinates are [lng, lat]
      coordinates: [wp.latlng[1], wp.latlng[0]],
    },
  }))
  return { type: 'FeatureCollection', features }
}

// ── Find closest waypoint to a screen pixel position ────────────────────────
function nearestWaypoint(
  map: maplibregl.Map,
  point: maplibregl.Point,
  waypoints: RouteWaypoint[],
): RouteWaypoint | null {
  let best: RouteWaypoint | null = null
  let bestDist = CLICK_SNAP_PX

  for (const wp of waypoints) {
    const screenPt = map.project([wp.latlng[1], wp.latlng[0]])
    const dx = screenPt.x - point.x
    const dy = screenPt.y - point.y
    const d = Math.sqrt(dx * dx + dy * dy)
    if (d < bestDist) { bestDist = d; best = wp }
  }
  return best
}

// ── Main hook ────────────────────────────────────────────────────────────────
export function useRouteEditor(
  mapRef: React.RefObject<maplibregl.Map | null>,
  planner: RoutePlannerState,
  active: boolean,
  options: UseRouteEditorOptions = {},
) {
  // Keep a stable ref to planner so event handlers don't need to re-register.
  const plannerRef = useRef(planner)
  plannerRef.current = planner

  const optionsRef = useRef(options)
  optionsRef.current = options

  // Track which waypoint is currently being dragged (by its ID).
  const draggingRef = useRef<string | null>(null)
  // Track the latest dragged waypoint position (visual only during drag; committed on mouseup).
  const draggedWpPositionRef = useRef<[number, number] | null>(null)
  // Track which control point is currently being dragged.
  const draggingCpRef = useRef<{ key: SegmentKey; cpIndex: number } | null>(null)
  // Track the latest dragged control point position (visual only during drag; committed on mouseup).
  const draggedCpPositionRef = useRef<[number, number] | null>(null)
  // Track last mouse-down position to distinguish click from drag.
  const mouseDownPtRef = useRef<maplibregl.Point | null>(null)
  // Track segment context menu popup.
  const segmentPopupRef = useRef<maplibregl.Popup | null>(null)

  // Track whether map sources have been added (guards GeoJSON sync).
  const sourcesReadyRef = useRef(false)

  // ── Layer setup / teardown ─────────────────────────────────────────────────
  const addLayers = useCallback((map: maplibregl.Map) => {
    // Route source (routed + offroad segments)
    if (!map.getSource(SOURCE_ROUTE)) {
      map.addSource(SOURCE_ROUTE, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }

    // Waypoints source
    if (!map.getSource(SOURCE_WP)) {
      map.addSource(SOURCE_WP, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }

    // Control points source (offroad segments)
    if (!map.getSource(SOURCE_CP)) {
      map.addSource(SOURCE_CP, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }

    // Approach lines source (snap distance > threshold)
    if (!map.getSource(SOURCE_APPROACH)) {
      map.addSource(SOURCE_APPROACH, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }

    // Calculating segments source (straight placeholder lines while routing is in progress)
    if (!map.getSource(SOURCE_CALCULATING)) {
      map.addSource(SOURCE_CALCULATING, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }

    // Sources are now ready — sync any already-available planner data immediately.
    // This covers the case where the GeoJSON-sync effect fired before sources existed
    // (Bug 1 fix: sources created after the sync effect already ran).
    sourcesReadyRef.current = true
    const { segments, waypoints } = plannerRef.current
    ;(map.getSource(SOURCE_ROUTE) as maplibregl.GeoJSONSource).setData(buildRouteGeoJSON(segments))
    ;(map.getSource(SOURCE_WP) as maplibregl.GeoJSONSource).setData(buildWaypointGeoJSON(waypoints))
    ;(map.getSource(SOURCE_CP) as maplibregl.GeoJSONSource).setData(buildControlPointGeoJSON(segments))
    ;(map.getSource(SOURCE_APPROACH) as maplibregl.GeoJSONSource).setData(buildApproachGeoJSON(segments))
    ;(map.getSource(SOURCE_CALCULATING) as maplibregl.GeoJSONSource).setData(buildCalculatingGeoJSON(segments, waypoints))

    // Calculating segments — glow + dashed line for in-progress route computations
    if (!map.getLayer(LAYER_CALCULATING_GLOW)) {
      map.addLayer({
        id: LAYER_CALCULATING_GLOW,
        type: 'line',
        source: SOURCE_CALCULATING,
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': '#F97316',
          'line-width': 10,
          'line-opacity': 0.15,
          'line-blur': 4,
        },
      })
    }

    if (!map.getLayer(LAYER_CALCULATING)) {
      map.addLayer({
        id: LAYER_CALCULATING,
        type: 'line',
        source: SOURCE_CALCULATING,
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': '#F97316',
          'line-width': 2.5,
          'line-dasharray': [2, 2],
          'line-opacity': 0.6,
        },
      })
    }

    // Routed segments — solid orange line with white casing
    if (!map.getLayer(LAYER_ROUTED)) {
      map.addLayer({
        id: LAYER_ROUTED,
        type: 'line',
        source: SOURCE_ROUTE,
        filter: ['==', ['get', 'mode'], 'routed'],
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': '#F97316',
          'line-width': 4,
          'line-opacity': 0.9,
        },
      })

      // White casing below the orange line
      map.addLayer(
        {
          id: LAYER_ROUTED + '-casing',
          type: 'line',
          source: SOURCE_ROUTE,
          filter: ['==', ['get', 'mode'], 'routed'],
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: {
            'line-color': '#FFFFFF',
            'line-width': 7,
            'line-opacity': 0.7,
          },
        },
        LAYER_ROUTED, // insert below the orange line
      )
    }

    // Offroad segments — dashed purple line
    if (!map.getLayer(LAYER_OFFROAD)) {
      map.addLayer({
        id: LAYER_OFFROAD,
        type: 'line',
        source: SOURCE_ROUTE,
        filter: ['==', ['get', 'mode'], 'offroad'],
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': '#A855F7',
          'line-width': 3,
          'line-dasharray': [4, 3],
          'line-opacity': 0.9,
        },
      })
    }

    // Approach lines — dashed violet, thinner than offroad
    if (!map.getLayer(LAYER_APPROACH)) {
      map.addLayer({
        id: LAYER_APPROACH,
        type: 'line',
        source: SOURCE_APPROACH,
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': '#A855F7',
          'line-width': 2,
          'line-dasharray': [3, 3],
          'line-opacity': 0.7,
        },
      })
    }

    // Invisible wide hit area on route line for inserting waypoints
    if (!map.getLayer(LAYER_ROUTED + '-hit')) {
      map.addLayer({
        id: LAYER_ROUTED + '-hit',
        type: 'line',
        source: SOURCE_ROUTE,
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': 'transparent',
          'line-width': 20,
          'line-opacity': 0,
        },
      })
    }

    // Control point circles (offroad segments — orange, smaller than waypoints)
    if (!map.getLayer(LAYER_CP_CIRCLE)) {
      map.addLayer({
        id: LAYER_CP_CIRCLE,
        type: 'circle',
        source: SOURCE_CP,
        paint: {
          'circle-radius': 5,
          'circle-color': '#F97316',
          'circle-opacity': 0.7,
          'circle-stroke-color': '#FFFFFF',
          'circle-stroke-width': 1.5,
        },
      })
    }

    // Waypoint circles
    if (!map.getLayer(LAYER_WP_CIRCLE)) {
      map.addLayer({
        id: LAYER_WP_CIRCLE,
        type: 'circle',
        source: SOURCE_WP,
        paint: {
          'circle-radius': ['get', 'size'],
          'circle-color': [
            'match', ['get', 'type'],
            'start', '#22C55E',
            'end',   '#EF4444',
            /* via */ '#F97316',
          ],
          'circle-stroke-color': '#FFFFFF',
          'circle-stroke-width': 2,
          'circle-opacity': 1,
        },
      })
    }

    // Highlight overlay (yellow sub-segments on hover/pin — rendered on top of route)
    if (!map.getSource(SOURCE_HIGHLIGHT)) {
      map.addSource(SOURCE_HIGHLIGHT, { type: 'geojson', data: { type: 'FeatureCollection', features: [] } })
    }
    if (!map.getLayer(LAYER_HIGHLIGHT)) {
      map.addLayer({
        id: LAYER_HIGHLIGHT, type: 'line', source: SOURCE_HIGHLIGHT,
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': '#FACC15', 'line-width': 6, 'line-opacity': 0.9 },
      })
    }

    // Waypoint labels (A / B)
    if (!map.getLayer(LAYER_WP_LABEL)) {
      map.addLayer({
        id: LAYER_WP_LABEL,
        type: 'symbol',
        source: SOURCE_WP,
        filter: ['in', ['get', 'type'], ['literal', ['start', 'end']]],
        layout: {
          'text-field': ['get', 'label'],
          'text-size': 11,
          'text-font': ['Noto Sans Regular'],
          'text-anchor': 'center',
          'text-allow-overlap': true,
          'text-ignore-placement': true,
        },
        paint: {
          'text-color': '#FFFFFF',
          'text-halo-color': 'transparent',
          'text-halo-width': 0,
        },
      })
    }
  }, [])

  const removeLayers = useCallback((map: maplibregl.Map) => {
    const layers = [
      LAYER_HIGHLIGHT,
      LAYER_WP_LABEL,
      LAYER_WP_CIRCLE,
      LAYER_CP_CIRCLE,
      LAYER_ROUTED + '-hit',
      LAYER_OFFROAD,
      LAYER_APPROACH,
      LAYER_ROUTED,
      LAYER_ROUTED + '-casing',
      LAYER_CALCULATING,
      LAYER_CALCULATING_GLOW,
    ]
    for (const id of layers) {
      if (map.getLayer(id)) map.removeLayer(id)
    }
    if (map.getSource(SOURCE_ROUTE))       map.removeSource(SOURCE_ROUTE)
    if (map.getSource(SOURCE_WP))          map.removeSource(SOURCE_WP)
    if (map.getSource(SOURCE_CP))          map.removeSource(SOURCE_CP)
    if (map.getSource(SOURCE_APPROACH))    map.removeSource(SOURCE_APPROACH)
    if (map.getSource(SOURCE_CALCULATING)) map.removeSource(SOURCE_CALCULATING)
    if (map.getSource(SOURCE_HIGHLIGHT))  map.removeSource(SOURCE_HIGHLIGHT)
    sourcesReadyRef.current = false
  }, [])

  // ── Sync GeoJSON data whenever planner state changes ──────────────────────
  // Bug 1 fix: only sync when sources actually exist (sourcesReadyRef guards this).
  // The initial data push happens in addLayers() right after sources are created.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !active || !sourcesReadyRef.current) return

    const routeSrc       = map.getSource(SOURCE_ROUTE)       as maplibregl.GeoJSONSource | undefined
    const wpSrc          = map.getSource(SOURCE_WP)          as maplibregl.GeoJSONSource | undefined
    const cpSrc          = map.getSource(SOURCE_CP)          as maplibregl.GeoJSONSource | undefined
    const approachSrc    = map.getSource(SOURCE_APPROACH)    as maplibregl.GeoJSONSource | undefined
    const calcSrc        = map.getSource(SOURCE_CALCULATING) as maplibregl.GeoJSONSource | undefined

    if (routeSrc)    routeSrc.setData(buildRouteGeoJSON(planner.segments))
    if (wpSrc)       wpSrc.setData(buildWaypointGeoJSON(planner.waypoints))
    if (cpSrc)       cpSrc.setData(buildControlPointGeoJSON(planner.segments))
    if (approachSrc) approachSrc.setData(buildApproachGeoJSON(planner.segments))
    if (calcSrc)     calcSrc.setData(buildCalculatingGeoJSON(planner.segments, planner.waypoints))
  // planner.isCalculating is optional — include it so the effect re-runs when
  // calculation starts/stops (clears the placeholder line on completion).
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, planner.segments, planner.waypoints, planner.isCalculating ?? false])

  // ── Highlight overlay: yellow sub-segments on hover/pin ───────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !active || !sourcesReadyRef.current) return
    const src = map.getSource(SOURCE_HIGHLIGHT) as maplibregl.GeoJSONSource | undefined
    if (!src) return

    const ht = planner.highlightedType
    if (!ht) {
      src.setData({ type: 'FeatureCollection', features: [] })
      return
    }

    const features: GeoJSON.Feature<GeoJSON.LineString>[] = []
    for (const seg of planner.segments) {
      if (!seg.sub_segments?.length || !seg.geometry?.coordinates?.length) continue
      const coords = seg.geometry.coordinates
      const subs = seg.sub_segments as RouteSubSegment[]

      let matching: RouteSubSegment[]
      if (ht.kind === 'descriptor') {
        matching = filterSubSegmentsByDescriptor(subs, ht.descriptorKey)
      } else {
        matching = subs.filter(ss => ht.kind === 'highway'
          ? ss.highway === ht.key
          : (SURFACE_GROUPS[ss.surface] ?? ss.surface) === ht.key
        )
      }

      for (const sub of matching) {
        const start = Math.max(0, sub.from_idx)
        const end = Math.min(coords.length - 1, sub.to_idx)
        if (end - start < 1) continue
        features.push({
          type: 'Feature',
          properties: {},
          geometry: { type: 'LineString', coordinates: coords.slice(start, end + 1) },
        })
      }
    }

    const hlMap = mapRef.current
    if (hlMap && hlMap.getLayer(LAYER_HIGHLIGHT)) {
      const DEFAULT_HL_COLOR = '#FACC15'
      let color = DEFAULT_HL_COLOR
      let dash: number[] | null = null
      if (ht.kind === 'descriptor') {
        switch (ht.descriptorKey) {
          case 'mtb_technical':  color = '#F59E0B'; break
          case 'alpine_hiking':  color = '#DC2626'; break
          case 'exposed':        color = '#9333EA'; break
          case 'low_visibility': color = '#6B7280'; dash = [2, 2]; break
        }
      }
      hlMap.setPaintProperty(LAYER_HIGHLIGHT, 'line-color', color)
      // null → unsets property, MapLibre falls back to solid line (NOT [1] which = dotted)
      hlMap.setPaintProperty(LAYER_HIGHLIGHT, 'line-dasharray', dash)
    }

    src.setData({ type: 'FeatureCollection', features })
  }, [active, planner.highlightedType, planner.segments])

  // ── Animation loop: pulse glow on calculating layer (only when calculating) ─
  const isCalculating = planner.isCalculating ?? false
  useEffect(() => {
    const map = mapRef.current
    if (!map || !active || !isCalculating) return

    let animFrame: number | null = null

    const animate = () => {
      if (!map.getLayer(LAYER_CALCULATING_GLOW) || !map.getLayer(LAYER_CALCULATING)) return
      const t = performance.now() / 1000
      // Pulse glow opacity between 0.08 and 0.25
      const glowOpacity = 0.08 + 0.17 * (0.5 + 0.5 * Math.sin(t * Math.PI * 1.5))
      map.setPaintProperty(LAYER_CALCULATING_GLOW, 'line-opacity', glowOpacity)
      // Shift dash pattern to simulate marching ants (cycle through 4 dash-phase variants)
      const phase = Math.floor(t * 6) % 4
      // Each phase shifts the dash array by 1 unit, creating a crawling effect
      const dashArrays: number[][] = [
        [2, 2],
        [1, 2, 1, 0],
        [0, 1, 2, 1],
        [0, 2, 2, 0],
      ]
      map.setPaintProperty(LAYER_CALCULATING, 'line-dasharray', dashArrays[phase])
      animFrame = requestAnimationFrame(animate)
    }

    animFrame = requestAnimationFrame(animate)

    return () => {
      if (animFrame !== null) cancelAnimationFrame(animFrame)
    }
  }, [active, isCalculating])

  // ── Map interactions (click, drag, context menu) ──────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    if (!active) {
      removeLayers(map)
      map.getCanvas().style.cursor = ''
      return
    }

    // Add layers — wait until map style is loaded
    const setupLayers = () => addLayers(map)
    if (map.isStyleLoaded()) {
      setupLayers()
    } else {
      map.once('style.load', setupLayers)
    }

    // Set default cursor to crosshair when planner is active
    map.getCanvas().style.cursor = 'crosshair'

    // ── Hover: route hit area ──────────────────────────────────────────────
    const onRouteEnter = () => { map.getCanvas().style.cursor = 'grab' }
    const onRouteLeave = () => { map.getCanvas().style.cursor = 'crosshair' }
    map.on('mouseenter', LAYER_ROUTED + '-hit', onRouteEnter)
    map.on('mouseleave', LAYER_ROUTED + '-hit', onRouteLeave)

    // ── Hover: waypoint circles ────────────────────────────────────────────
    const onWpEnter = () => { map.getCanvas().style.cursor = 'grab' }
    const onWpLeave = () => { map.getCanvas().style.cursor = 'crosshair' }
    map.on('mouseenter', LAYER_WP_CIRCLE, onWpEnter)
    map.on('mouseleave', LAYER_WP_CIRCLE, onWpLeave)

    // ── Hover: control point circles ───────────────────────────────────────
    const onCpEnter = () => { map.getCanvas().style.cursor = 'grab' }
    const onCpLeave = () => {
      if (!draggingCpRef.current) map.getCanvas().style.cursor = 'crosshair'
    }
    map.on('mouseenter', LAYER_CP_CIRCLE, onCpEnter)
    map.on('mouseleave', LAYER_CP_CIRCLE, onCpLeave)

    // ── Mousedown on route line → insert waypoint + start drag ──────────────
    // Instead of click-to-insert, we do mousedown → insert → drag → mouseup.
    // The new waypoint is inserted immediately and enters drag mode so the user
    // can position it interactively. Routing only starts on mouseup.
    const onRouteMouseDown = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      // Don't handle if a waypoint or control point was clicked
      const { waypoints } = plannerRef.current
      const nearest = nearestWaypoint(map, e.point, waypoints)
      if (nearest) return // too close to existing waypoint

      // Check if click is on a control point
      const cpFeatures = map.queryRenderedFeatures(e.point, { layers: [LAYER_CP_CIRCLE] })
      if (cpFeatures.length > 0) return

      const { segments } = plannerRef.current
      const latlng: [number, number] = [e.lngLat.lat, e.lngLat.lng]

      // Find the segment whose geometry was clicked
      let bestSegIdx = -1
      let bestDist = Infinity
      for (let i = 0; i < segments.length; i++) {
        const seg = segments[i]
        if (!seg.geometry) continue
        for (const coord of seg.geometry.coordinates) {
          const pt = map.project([coord[0], coord[1]])
          const dx = pt.x - e.point.x
          const dy = pt.y - e.point.y
          const d = Math.sqrt(dx * dx + dy * dy)
          if (d < bestDist) { bestDist = d; bestSegIdx = i }
        }
      }

      if (bestSegIdx >= 0) {
        const seg = segments[bestSegIdx]
        if (seg.mode === 'offroad') {
          // F5: Let onOffroadClick handle offroad segment clicks to prevent double insertControlPoint
          return
        }
        // Insert waypoint on routed segment and immediately start dragging it
        e.preventDefault()
        const afterWpId = seg.from_waypoint
        const newId = plannerRef.current.insertWaypoint(afterWpId, latlng)
        if (newId) {
          // Enter drag mode for the newly inserted waypoint
          draggingRef.current = newId
          draggedWpPositionRef.current = latlng
          mouseDownPtRef.current = e.point
          map.getCanvas().style.cursor = 'grabbing'
          map.dragPan.disable()
        }
      }
    }
    map.on('mousedown', LAYER_ROUTED + '-hit', onRouteMouseDown)

    // Also listen for clicks on offroad line (for inserting control points)
    const onOffroadClick = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      // Check if click is on a control point — if so, skip
      const cpFeatures = map.queryRenderedFeatures(e.point, { layers: [LAYER_CP_CIRCLE] })
      if (cpFeatures.length > 0) return

      const features = e.features ?? []
      if (features.length === 0) return
      const props = features[0].properties
      if (!props) return

      const segFrom = props['segment_from'] as string
      const segTo = props['segment_to'] as string
      const { segments } = plannerRef.current
      const seg = segments.find(s => s.from_waypoint === segFrom && s.to_waypoint === segTo)
      if (!seg || seg.mode !== 'offroad') return

      const latlng: [number, number] = [e.lngLat.lat, e.lngLat.lng]
      const cps = seg.control_points ?? []
      if (cps.length >= 2 && plannerRef.current.insertControlPoint) {
        const result = nearestPointOnPolyline(latlng, cps)
        plannerRef.current.insertControlPoint(
          { from: segFrom, to: segTo },
          result.insertAfter,
          latlng,
        )
      }
    }
    map.on('click', LAYER_OFFROAD, onOffroadClick)

    // ── Click on map background → add waypoint ────────────────────────────
    const onMapClick = (e: maplibregl.MapMouseEvent) => {
      // Ignore if click was on route line, waypoint, or control point
      const routeFeatures = map.queryRenderedFeatures(e.point, {
        layers: map.getLayer(LAYER_ROUTED + '-hit') ? [LAYER_ROUTED + '-hit'] : [],
      })
      if (routeFeatures.length > 0) return

      const offroadFeatures = map.queryRenderedFeatures(e.point, {
        layers: map.getLayer(LAYER_OFFROAD) ? [LAYER_OFFROAD] : [],
      })
      if (offroadFeatures.length > 0) return

      const cpFeatures = map.queryRenderedFeatures(e.point, {
        layers: map.getLayer(LAYER_CP_CIRCLE) ? [LAYER_CP_CIRCLE] : [],
      })
      if (cpFeatures.length > 0) return

      const { waypoints } = plannerRef.current
      const nearest = nearestWaypoint(map, e.point, waypoints)
      if (nearest) return

      const latlng: [number, number] = [e.lngLat.lat, e.lngLat.lng]
      plannerRef.current.addWaypoint(latlng)
    }
    map.on('click', onMapClick)

    // ── Right-click on control point → delete ─────────────────────────────
    const onCpContextMenu = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      e.preventDefault()
      const features = e.features ?? map.queryRenderedFeatures(e.point, { layers: [LAYER_CP_CIRCLE] })
      if (features.length === 0) return
      const props = features[0].properties
      if (!props) return

      const segFrom = props['segment_from'] as string
      const segTo = props['segment_to'] as string
      const cpIndex = props['cp_index'] as number

      plannerRef.current.removeControlPoint?.(
        { from: segFrom, to: segTo },
        cpIndex,
      )
    }
    map.on('contextmenu', LAYER_CP_CIRCLE, onCpContextMenu)

    // ── Right-click on route line → segment context menu (toggle offroad) ─
    const onSegmentContextMenu = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      // Don't show if right-click was on a waypoint or control point
      const { waypoints } = plannerRef.current
      const nearest = nearestWaypoint(map, e.point, waypoints)
      if (nearest) return

      const cpFeatures = map.queryRenderedFeatures(e.point, { layers: [LAYER_CP_CIRCLE] })
      if (cpFeatures.length > 0) return

      const features = e.features ?? []
      if (features.length === 0) return
      const props = features[0].properties
      if (!props) return

      const segFrom = props['segment_from'] as string
      const segTo = props['segment_to'] as string
      const mode = props['mode'] as string

      e.preventDefault()

      // Close any existing popup
      segmentPopupRef.current?.remove()

      const label = mode === 'routed' ? SEGMENT_MENU_LABELS.drawOffroad : SEGMENT_MENU_LABELS.routeOnRoads

      const container = document.createElement('div')
      container.className = 'flex flex-col gap-1 p-1'
      const btn = document.createElement('button')
      btn.textContent = label
      btn.className = 'text-sm px-3 py-1.5 rounded hover:bg-neutral-100 dark:hover:bg-neutral-700 text-left whitespace-nowrap cursor-pointer'
      btn.addEventListener('click', () => {
        plannerRef.current.toggleOffroad?.({ from: segFrom, to: segTo })
        segmentPopupRef.current?.remove()
        segmentPopupRef.current = null
      })
      container.appendChild(btn)

      const popup = new maplibregl.Popup({ closeButton: false, closeOnClick: true })
        .setLngLat(e.lngLat)
        .setDOMContent(container)
        .addTo(map)
      segmentPopupRef.current = popup
    }
    map.on('contextmenu', LAYER_ROUTED + '-hit', onSegmentContextMenu)
    map.on('contextmenu', LAYER_OFFROAD, onSegmentContextMenu)

    // ── Right-click on waypoint → context menu ─────────────────────────────
    const onContextMenu = (e: maplibregl.MapMouseEvent) => {
      const { waypoints: wps } = plannerRef.current
      const nearestWp = nearestWaypoint(map, e.point, wps)
      if (!nearestWp) return

      e.preventDefault()
      optionsRef.current.onContextMenu?.({
        waypointId: nearestWp.id,
        screenX: e.originalEvent.clientX,
        screenY: e.originalEvent.clientY,
      })
    }
    map.on('contextmenu', onContextMenu)

    // ── Drag waypoint markers ──────────────────────────────────────────────
    const onWpMouseDown = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      // Use the feature id from MapLibre's event directly instead of the pixel-based
      // nearestWaypoint search. MapLibre already fires this event only when the mouse is
      // on the waypoint layer, so queryRenderedFeatures gives us the exact clicked feature.
      // This avoids the pixel-snap ambiguity at dense waypoint placement where the
      // nearest-pixel search could resolve to a different waypoint than the one clicked.
      const features = e.features ?? map.queryRenderedFeatures(e.point, { layers: [LAYER_WP_CIRCLE] })
      if (features.length === 0) return
      const wpId = features[0].properties?.id as string | undefined
      if (!wpId) return

      e.preventDefault()
      draggingRef.current = wpId
      mouseDownPtRef.current = e.point
      map.getCanvas().style.cursor = 'grabbing'
      map.dragPan.disable()
    }

    // ── Drag control points ────────────────────────────────────────────────
    const onCpMouseDown = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      const features = e.features ?? map.queryRenderedFeatures(e.point, { layers: [LAYER_CP_CIRCLE] })
      if (features.length === 0) return
      const props = features[0].properties
      if (!props) return

      e.preventDefault()
      draggingCpRef.current = {
        key: { from: props['segment_from'] as string, to: props['segment_to'] as string },
        cpIndex: props['cp_index'] as number,
      }
      mouseDownPtRef.current = e.point
      map.getCanvas().style.cursor = 'grabbing'
      map.dragPan.disable()
    }

    const onMouseMove = (e: maplibregl.MapMouseEvent) => {
      const latlng: [number, number] = [e.lngLat.lat, e.lngLat.lng]

      // F1: During waypoint drag, only update visual position via GeoJSON source.
      // The planner state (and routing API) is only updated on mouseup.
      if (draggingRef.current) {
        draggedWpPositionRef.current = latlng
        const { waypoints } = plannerRef.current
        const updated = waypoints.map(wp =>
          wp.id === draggingRef.current ? { ...wp, latlng } : wp,
        )
        const wpSrc = map.getSource(SOURCE_WP) as maplibregl.GeoJSONSource | undefined
        wpSrc?.setData(buildWaypointGeoJSON(updated))
        return
      }

      // F3: During control point drag, only update visual position via GeoJSON source.
      // The planner state (spline rebuild) is only updated on mouseup.
      if (draggingCpRef.current) {
        draggedCpPositionRef.current = latlng
        const { segments } = plannerRef.current
        const { key, cpIndex } = draggingCpRef.current
        const updatedSegments = segments.map(seg => {
          if (seg.from_waypoint !== key.from || seg.to_waypoint !== key.to) return seg
          const cps = [...(seg.control_points ?? [])]
          cps[cpIndex] = latlng
          return { ...seg, control_points: cps }
        })
        const cpSrc = map.getSource(SOURCE_CP) as maplibregl.GeoJSONSource | undefined
        cpSrc?.setData(buildControlPointGeoJSON(updatedSegments))
        return
      }
    }

    const onMouseUp = (e?: maplibregl.MapMouseEvent) => {
      // F1: Commit waypoint position to planner state on mouseup (triggers routing API once).
      if (draggingRef.current) {
        const wpId = draggingRef.current
        // Use the last tracked drag position; fall back to mouseup lngLat if no mousemove occurred.
        const finalPos: [number, number] = draggedWpPositionRef.current
          ?? (e ? [e.lngLat.lat, e.lngLat.lng] : null)
          ?? plannerRef.current.waypoints.find(w => w.id === wpId)?.latlng
          ?? [0, 0]
        draggingRef.current = null
        draggedWpPositionRef.current = null
        mouseDownPtRef.current = null
        map.getCanvas().style.cursor = 'crosshair'
        map.dragPan.enable()
        plannerRef.current.moveWaypoint(wpId, finalPos)
        return
      }

      // F3: Commit control point position to planner state on mouseup (triggers spline rebuild once).
      if (draggingCpRef.current) {
        const { key, cpIndex } = draggingCpRef.current
        // Use the last tracked drag position; fall back to mouseup lngLat if no mousemove occurred.
        const finalPos: [number, number] = draggedCpPositionRef.current
          ?? (e ? [e.lngLat.lat, e.lngLat.lng] : null)
          ?? plannerRef.current.segments
              .find(s => s.from_waypoint === key.from && s.to_waypoint === key.to)
              ?.control_points?.[cpIndex]
          ?? [0, 0]
        draggingCpRef.current = null
        draggedCpPositionRef.current = null
        mouseDownPtRef.current = null
        map.getCanvas().style.cursor = 'crosshair'
        map.dragPan.enable()
        plannerRef.current.updateControlPoint?.(key, cpIndex, finalPos)
        return
      }
    }

    map.on('mousedown', LAYER_WP_CIRCLE, onWpMouseDown)
    map.on('mousedown', LAYER_CP_CIRCLE, onCpMouseDown)
    map.on('mousemove', onMouseMove)
    map.on('mouseup', onMouseUp)

    // Touch support — touchend = finish drag
    const onTouchEnd = () => onMouseUp()
    map.on('touchend', onTouchEnd)

    // Window mouseup fallback — if user releases mouse outside canvas, re-enable dragPan
    const onWindowMouseUp = () => {
      if (draggingRef.current || draggingCpRef.current) {
        onMouseUp()
      }
    }
    window.addEventListener('mouseup', onWindowMouseUp)

    return () => {
      map.off('mouseenter', LAYER_ROUTED + '-hit', onRouteEnter)
      map.off('mouseleave', LAYER_ROUTED + '-hit', onRouteLeave)
      map.off('mouseenter', LAYER_WP_CIRCLE, onWpEnter)
      map.off('mouseleave', LAYER_WP_CIRCLE, onWpLeave)
      map.off('mouseenter', LAYER_CP_CIRCLE, onCpEnter)
      map.off('mouseleave', LAYER_CP_CIRCLE, onCpLeave)
      map.off('mousedown', LAYER_ROUTED + '-hit', onRouteMouseDown)
      map.off('click', LAYER_OFFROAD, onOffroadClick)
      map.off('click', onMapClick)
      map.off('contextmenu', LAYER_CP_CIRCLE, onCpContextMenu)
      map.off('contextmenu', LAYER_ROUTED + '-hit', onSegmentContextMenu)
      map.off('contextmenu', LAYER_OFFROAD, onSegmentContextMenu)
      map.off('contextmenu', onContextMenu)
      map.off('mousedown', LAYER_WP_CIRCLE, onWpMouseDown)
      map.off('mousedown', LAYER_CP_CIRCLE, onCpMouseDown)
      map.off('mousemove', onMouseMove)
      map.off('mouseup', onMouseUp)
      map.off('touchend', onTouchEnd)
      window.removeEventListener('mouseup', onWindowMouseUp)
      segmentPopupRef.current?.remove()
      segmentPopupRef.current = null
      removeLayers(map)
      map.getCanvas().style.cursor = ''
    }
  }, [active, mapRef, addLayers, removeLayers])
}
