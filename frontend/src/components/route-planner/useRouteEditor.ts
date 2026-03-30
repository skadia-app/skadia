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

// ── Layer / Source IDs ──────────────────────────────────────────────────────
const SOURCE_ROUTE   = 'route-planner-source'
const SOURCE_WP      = 'route-planner-waypoints-source'

const LAYER_ROUTED   = 'route-planner-line'
const LAYER_OFFROAD  = 'route-planner-offroad'
const LAYER_WP_CIRCLE = 'route-planner-waypoints'
const LAYER_WP_LABEL  = 'route-planner-waypoints-label'

// Pixel radius for "close enough to waypoint" snap on click
const CLICK_SNAP_PX = 20

// ── Minimal planner interface required by this hook ─────────────────────────
// Mirrors what useRoutePlanner will expose (Task 27).
export interface RoutePlannerState {
  waypoints: RouteWaypoint[]
  segments: RouteSegment[]
  addWaypoint: (latlng: [number, number]) => void
  moveWaypoint: (id: string, latlng: [number, number]) => void
  removeWaypoint: (id: string) => void
  insertWaypoint: (afterId: string, latlng: [number, number]) => void
  setWaypointType: (id: string, type: RouteWaypoint['type']) => void
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
      properties: { mode: seg.mode },
      geometry: seg.geometry,
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
  // Track last mouse-down position to distinguish click from drag.
  const mouseDownPtRef = useRef<maplibregl.Point | null>(null)

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

    // Sources are now ready — sync any already-available planner data immediately.
    // This covers the case where the GeoJSON-sync effect fired before sources existed
    // (Bug 1 fix: sources created after the sync effect already ran).
    sourcesReadyRef.current = true
    const { segments, waypoints } = plannerRef.current
    ;(map.getSource(SOURCE_ROUTE) as maplibregl.GeoJSONSource).setData(buildRouteGeoJSON(segments))
    ;(map.getSource(SOURCE_WP) as maplibregl.GeoJSONSource).setData(buildWaypointGeoJSON(waypoints))

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
      LAYER_WP_LABEL,
      LAYER_WP_CIRCLE,
      LAYER_ROUTED + '-hit',
      LAYER_OFFROAD,
      LAYER_ROUTED,
      LAYER_ROUTED + '-casing',
    ]
    for (const id of layers) {
      if (map.getLayer(id)) map.removeLayer(id)
    }
    if (map.getSource(SOURCE_ROUTE)) map.removeSource(SOURCE_ROUTE)
    if (map.getSource(SOURCE_WP))    map.removeSource(SOURCE_WP)
    sourcesReadyRef.current = false
  }, [])

  // ── Sync GeoJSON data whenever planner state changes ──────────────────────
  // Bug 1 fix: only sync when sources actually exist (sourcesReadyRef guards this).
  // The initial data push happens in addLayers() right after sources are created.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !active || !sourcesReadyRef.current) return

    const routeSrc = map.getSource(SOURCE_ROUTE) as maplibregl.GeoJSONSource | undefined
    const wpSrc    = map.getSource(SOURCE_WP)    as maplibregl.GeoJSONSource | undefined

    if (routeSrc) routeSrc.setData(buildRouteGeoJSON(planner.segments))
    if (wpSrc)    wpSrc.setData(buildWaypointGeoJSON(planner.waypoints))
  }, [active, planner.segments, planner.waypoints])

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
    const onRouteEnter = () => { map.getCanvas().style.cursor = 'crosshair' }
    const onRouteLeave = () => { map.getCanvas().style.cursor = 'crosshair' }
    map.on('mouseenter', LAYER_ROUTED + '-hit', onRouteEnter)
    map.on('mouseleave', LAYER_ROUTED + '-hit', onRouteLeave)

    // ── Hover: waypoint circles ────────────────────────────────────────────
    const onWpEnter = () => { map.getCanvas().style.cursor = 'grab' }
    const onWpLeave = () => { map.getCanvas().style.cursor = 'crosshair' }
    map.on('mouseenter', LAYER_WP_CIRCLE, onWpEnter)
    map.on('mouseleave', LAYER_WP_CIRCLE, onWpLeave)

    // ── Click on route line → insert waypoint ─────────────────────────────
    const onRouteClick = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      // Don't handle if a waypoint was clicked (handled by onWpClick)
      const { waypoints } = plannerRef.current
      const nearest = nearestWaypoint(map, e.point, waypoints)
      if (nearest) return // too close to existing waypoint

      // Find the segment whose geometry was clicked and determine insertion point
      const { segments } = plannerRef.current
      const latlng: [number, number] = [e.lngLat.lat, e.lngLat.lng]

      // Insert after the waypoint whose segment was clicked.
      // For simplicity we find the segment closest to the click point.
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
        const afterWpId = segments[bestSegIdx].from_waypoint
        plannerRef.current.insertWaypoint(afterWpId, latlng)
      }
    }
    map.on('click', LAYER_ROUTED + '-hit', onRouteClick)

    // ── Click on map background → add waypoint ────────────────────────────
    const onMapClick = (e: maplibregl.MapMouseEvent) => {
      // Ignore if click was on route line or waypoint
      const routeFeatures = map.queryRenderedFeatures(e.point, {
        layers: [LAYER_ROUTED + '-hit'],
      })
      if (routeFeatures.length > 0) return

      const { waypoints } = plannerRef.current
      const nearest = nearestWaypoint(map, e.point, waypoints)
      if (nearest) return

      const latlng: [number, number] = [e.lngLat.lat, e.lngLat.lng]
      plannerRef.current.addWaypoint(latlng)
    }
    map.on('click', onMapClick)

    // ── Right-click on waypoint → context menu ─────────────────────────────
    const onContextMenu = (e: maplibregl.MapMouseEvent) => {
      const { waypoints } = plannerRef.current
      const nearest = nearestWaypoint(map, e.point, waypoints)
      if (!nearest) return

      e.preventDefault()
      optionsRef.current.onContextMenu?.({
        waypointId: nearest.id,
        screenX: e.originalEvent.clientX,
        screenY: e.originalEvent.clientY,
      })
    }
    map.on('contextmenu', onContextMenu)

    // ── Drag waypoint markers ──────────────────────────────────────────────
    const onWpMouseDown = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      const { waypoints } = plannerRef.current
      const nearest = nearestWaypoint(map, e.point, waypoints)
      if (!nearest) return

      e.preventDefault()
      draggingRef.current = nearest.id
      mouseDownPtRef.current = e.point
      map.getCanvas().style.cursor = 'grabbing'
      map.dragPan.disable()
    }

    const onMouseMove = (e: maplibregl.MapMouseEvent) => {
      if (!draggingRef.current) return
      const latlng: [number, number] = [e.lngLat.lat, e.lngLat.lng]
      plannerRef.current.moveWaypoint(draggingRef.current, latlng)
    }

    const onMouseUp = () => {
      if (!draggingRef.current) return
      draggingRef.current = null
      mouseDownPtRef.current = null
      map.getCanvas().style.cursor = 'crosshair'
      map.dragPan.enable()
    }

    map.on('mousedown', LAYER_WP_CIRCLE, onWpMouseDown)
    map.on('mousemove', onMouseMove)
    map.on('mouseup', onMouseUp)

    // Touch support — touchend = finish drag
    const onTouchEnd = () => onMouseUp()
    map.on('touchend', onTouchEnd)

    return () => {
      map.off('mouseenter', LAYER_ROUTED + '-hit', onRouteEnter)
      map.off('mouseleave', LAYER_ROUTED + '-hit', onRouteLeave)
      map.off('mouseenter', LAYER_WP_CIRCLE, onWpEnter)
      map.off('mouseleave', LAYER_WP_CIRCLE, onWpLeave)
      map.off('click', LAYER_ROUTED + '-hit', onRouteClick)
      map.off('click', onMapClick)
      map.off('contextmenu', onContextMenu)
      map.off('mousedown', LAYER_WP_CIRCLE, onWpMouseDown)
      map.off('mousemove', onMouseMove)
      map.off('mouseup', onMouseUp)
      map.off('touchend', onTouchEnd)
      removeLayers(map)
      map.getCanvas().style.cursor = ''
    }
  }, [active, mapRef, addLayers, removeLayers])
}
