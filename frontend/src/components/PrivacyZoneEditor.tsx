import { useRef, useEffect, useState, useCallback } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { createPrivacyZone, updatePrivacyZone, type PrivacyZone } from '@/api/privacy'
import ColorPicker from '@/components/ColorPicker'
import Toggle from '@/components/Toggle'
import { useToast } from '@/lib/toast'
import { circlePolygon, clampToCircle, pointInPolygon, centroid, haversineM } from '@/lib/geoUtils'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

interface Props {
  zone: PrivacyZone | null
  onClose: () => void
}

type ZoneMode = 'circle' | 'polygon'

/** Convert a circle (center + radius) to a 64-point polygon ring */
function circleToPolygonPoints(center: [number, number], radiusM: number, steps = 64): [number, number][] {
  const pts: [number, number][] = []
  const [lng, lat] = center
  for (let i = 0; i < steps; i++) {
    const angle = (i / steps) * 2 * Math.PI
    const dx = radiusM * Math.cos(angle)
    const dy = radiusM * Math.sin(angle)
    const dlat = dy / 111320
    const dlng = dx / (111320 * Math.cos((lat * Math.PI) / 180))
    pts.push([lng + dlng, lat + dlat])
  }
  return pts
}

// ── Cursor helpers (CSS classes override MapLibre's inline cursor) ─────────────
function injectPZECursorStyles() {
  if (document.getElementById('pze-cursor-styles')) return
  const style = document.createElement('style')
  style.id = 'pze-cursor-styles'
  style.textContent = [
    '.pze-cursor-crosshair .maplibregl-canvas-container { cursor: crosshair !important; }',
    '.pze-cursor-grabbing  .maplibregl-canvas-container { cursor: grabbing  !important; }',
  ].join('\n')
  document.head.appendChild(style)
}
function setPZECursorClass(el: HTMLElement, cursor: 'crosshair' | 'grabbing' | '') {
  el.classList.remove('pze-cursor-crosshair', 'pze-cursor-grabbing')
  if (cursor) el.classList.add(`pze-cursor-${cursor}`)
}

export default function PrivacyZoneEditor({ zone, onClose }: Props) {
  const { t } = useTranslation()
  const { formatDistanceCompact } = useUnitFormatters()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)   // circle center or polygon polygon
  const pinMarkerRef = useRef<maplibregl.Marker | null>(null) // offset pin (blue)
  // Polygon vertex marker list
  const polyMarkersRef = useRef<maplibregl.Marker[]>([])
  // true while a vertex is being dragged; do not recreate markers in useEffect
  const isDraggingVertexRef = useRef(false)

  const initialMode: ZoneMode = zone?.zone_type === 'polygon' ? 'polygon' : 'circle'

  const [name, setName] = useState(zone?.name ?? '')
  const [lat, setLat] = useState(zone?.lat ?? 51.16)
  const [lng, setLng] = useState(zone?.lng ?? 10.45)
  const [radius, setRadius] = useState(zone?.radius_m ?? 200)
  const [color, setColor] = useState(zone?.color ?? '#F97316')
  const [zoneMode, setZoneMode] = useState<ZoneMode>(initialMode)
  const hasInitialPosition = zone != null

  // Offset-Pin state
  // When offsetPinEnabled=true: pinLat/pinLng = true home location (stored as lat/lng in DB)
  //                              zone center/polygon = visual masking boundary
  // When offsetPinEnabled=false: zone center == pin (legacy behaviour)
  const [offsetPinEnabled, setOffsetPinEnabled] = useState(false)
  // Pin position (true location); defaults to zone center when toggled on
  const [pinLat, setPinLat] = useState(zone?.lat ?? 51.16)
  const [pinLng, setPinLng] = useState(zone?.lng ?? 10.45)

  // Polygon points: load initial values from GeoJSON when editing a polygon zone
  const parseInitialPolygon = (): [number, number][] => {
    if (zone?.zone_type === 'polygon' && zone.polygon) {
      try {
        const parsed = JSON.parse(zone.polygon) as GeoJSON.Polygon
        const ring = parsed.coordinates[0]
        // Omit the last point (same as the first in a closed polygon)
        return ring.slice(0, -1) as [number, number][]
      } catch {
        return []
      }
    }
    return []
  }

  const [polygonPoints, setPolygonPoints] = useState<[number, number][]>(parseInitialPolygon)
  const [polygonClosed, setPolygonClosed] = useState(zone?.zone_type === 'polygon' && (zone.polygon ?? '').length > 0)

  // Refs for values needed by MapLibre event handlers
  const zoneModeRef = useRef<ZoneMode>(initialMode)
  const polygonClosedRef = useRef(polygonClosed)
  const colorRef = useRef(color)
  const offsetPinEnabledRef = useRef(false)
  const pinLatRef = useRef(zone?.lat ?? 51.16)
  const pinLngRef = useRef(zone?.lng ?? 10.45)
  const latRef = useRef(zone?.lat ?? 51.16)
  const lngRef = useRef(zone?.lng ?? 10.45)
  const radiusRef = useRef(zone?.radius_m ?? 200)
  const polygonPointsRef = useRef<[number, number][]>(parseInitialPolygon())

  // Keep refs in sync
  useEffect(() => { colorRef.current = color }, [color])
  useEffect(() => { offsetPinEnabledRef.current = offsetPinEnabled }, [offsetPinEnabled])
  useEffect(() => { pinLatRef.current = pinLat }, [pinLat])
  useEffect(() => { pinLngRef.current = pinLng }, [pinLng])
  useEffect(() => { latRef.current = lat }, [lat])
  useEffect(() => { lngRef.current = lng }, [lng])
  useEffect(() => { radiusRef.current = radius }, [radius])
  useEffect(() => { polygonPointsRef.current = polygonPoints }, [polygonPoints])

  const createMut = useMutation({
    mutationFn: createPrivacyZone,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['privacy-zones'] })
      addToast({ type: 'success', title: t('privacyZones.editor.created') })
      onClose()
    },
    onError: (error) => {
      addToast({ type: 'error', title: t('privacyZones.editor.createFailed'), message: error instanceof Error ? error.message : t('common.unknownError') })
    },
  })

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Parameters<typeof updatePrivacyZone>[1] }) =>
      updatePrivacyZone(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['privacy-zones'] })
      addToast({ type: 'success', title: t('privacyZones.editor.updated') })
      onClose()
    },
    onError: (error) => {
      addToast({ type: 'error', title: t('privacyZones.editor.updateFailed'), message: error instanceof Error ? error.message : t('common.unknownError') })
    },
  })

  function buildPolygonGeoJSON(points: [number, number][]): string {
    // GeoJSON polygons need a closed ring: first point == last point
    const ring = [...points, points[0]]
    return JSON.stringify({ type: 'Polygon', coordinates: [ring] })
  }

  function handleSave() {
    const trimmed = name.trim()
    if (!trimmed) return

    // When offset pin is enabled: lat/lng in DB = pin (true position)
    //                             polygon/zone geometry = masking boundary
    // When offset pin disabled: lat/lng = zone center (legacy)
    const saveLat = offsetPinEnabled ? pinLat : lat
    const saveLng = offsetPinEnabled ? pinLng : lng

    if (zoneMode === 'circle') {
      // When offset is enabled for a circle, convert to polygon so pin and boundary
      // can be stored independently (circle uses center for both lat/lng and geometry)
      if (offsetPinEnabled) {
        // Convert circle to polygon, stored as polygon type
        const circPts = circleToPolygonPoints([lng, lat], radius)
        const polygonGeoJSON = buildPolygonGeoJSON(circPts)
        if (zone) {
          updateMut.mutate({
            id: zone.id,
            data: { name: trimmed, lat: saveLat, lng: saveLng, radius_m: radius, color, zone_type: 'polygon', polygon: polygonGeoJSON },
          })
        } else {
          createMut.mutate({ name: trimmed, lat: saveLat, lng: saveLng, radius_m: radius, color, zone_type: 'polygon', polygon: polygonGeoJSON })
        }
      } else {
        if (zone) {
          updateMut.mutate({
            id: zone.id,
            data: { name: trimmed, lat, lng, radius_m: radius, color, zone_type: 'circle', polygon: undefined },
          })
        } else {
          createMut.mutate({ name: trimmed, lat, lng, radius_m: radius, color, zone_type: 'circle' })
        }
      }
    } else {
      // Polygon mode
      if (polygonPoints.length < 3 || !polygonClosed) return
      const polygonGeoJSON = buildPolygonGeoJSON(polygonPoints)
      // If no offset pin, use centroid as the stored lat/lng
      const [cLng, cLat] = offsetPinEnabled ? [saveLng, saveLat] : (() => { const c = centroid(polygonPoints); return c })()
      const finalLat = offsetPinEnabled ? saveLat : cLat
      const finalLng = offsetPinEnabled ? saveLng : cLng
      if (zone) {
        updateMut.mutate({
          id: zone.id,
          data: { name: trimmed, lat: finalLat, lng: finalLng, radius_m: 200, color, zone_type: 'polygon', polygon: polygonGeoJSON },
        })
      } else {
        createMut.mutate({ name: trimmed, lat: finalLat, lng: finalLng, radius_m: 200, color, zone_type: 'polygon', polygon: polygonGeoJSON })
      }
    }
  }

  // Update circle overlay color and size
  function updateCircle(map: maplibregl.Map, center: [number, number], r: number, c: string) {
    const source = map.getSource('zone-circle') as maplibregl.GeoJSONSource | undefined
    if (source) {
      source.setData(circlePolygon(center, r))
    }
    if (map.getLayer('zone-circle-fill')) {
      map.setPaintProperty('zone-circle-fill', 'fill-color', c)
    }
    if (map.getLayer('zone-circle-border')) {
      map.setPaintProperty('zone-circle-border', 'line-color', c)
    }
  }

  // Update polygon layer data
  const updatePolygonLayer = useCallback((map: maplibregl.Map, points: [number, number][], closed: boolean, c: string) => {
    const source = map.getSource('zone-polygon') as maplibregl.GeoJSONSource | undefined
    if (!source) return

    if (points.length === 0) {
      source.setData({ type: 'FeatureCollection', features: [] })
      return
    }

    const coords = closed ? [...points, points[0]] : points
    const geomType = closed && points.length >= 3 ? 'Polygon' : 'LineString'
    const geometry =
      geomType === 'Polygon'
        ? { type: 'Polygon' as const, coordinates: [coords] }
        : { type: 'LineString' as const, coordinates: coords }

    source.setData({
      type: 'FeatureCollection',
      features: [{ type: 'Feature', properties: {}, geometry }],
    })

    if (map.getLayer('zone-polygon-fill')) {
      map.setPaintProperty('zone-polygon-fill', 'fill-color', c)
    }
    if (map.getLayer('zone-polygon-line')) {
      map.setPaintProperty('zone-polygon-line', 'line-color', c)
    }
  }, [])

  // Update vertex markers
  const updateVertexMarkers = useCallback((map: maplibregl.Map, points: [number, number][], c: string) => {
    // Remove old markers
    polyMarkersRef.current.forEach(m => m.remove())
    polyMarkersRef.current = []

    points.forEach((pt, idx) => {
      const el = document.createElement('div')
      el.style.cssText = `
        width:10px;height:10px;border-radius:50%;
        background:${c};border:2px solid white;
        box-shadow:0 1px 4px rgba(0,0,0,.4);cursor:pointer;
      `
      const marker = new maplibregl.Marker({ element: el, draggable: true })
        .setLngLat(pt)
        .addTo(map)

      marker.on('dragstart', () => {
        isDraggingVertexRef.current = true
      })

      marker.on('drag', () => {
        const pos = marker.getLngLat()
        const updated = [...polygonPointsRef.current]
        updated[idx] = [pos.lng, pos.lat]
        polygonPointsRef.current = updated
        if (map.isStyleLoaded()) {
          updatePolygonLayer(map, updated, polygonClosedRef.current, colorRef.current)
        }
        if (offsetPinEnabledRef.current) {
          clampPinToPolygon(map, updated)
        }
      })

      marker.on('dragend', () => {
        isDraggingVertexRef.current = false
        setPolygonPoints([...polygonPointsRef.current])
      })

      polyMarkersRef.current.push(marker)
    })
  }, [])

  /**
   * Clamp pin to inside polygon (or centroid if outside).
   * Called when polygon vertices move.
   */
  function clampPinToPolygon(_map: maplibregl.Map, pts: [number, number][]) {
    const pin: [number, number] = [pinLngRef.current, pinLatRef.current]
    if (!pointInPolygon(pin, pts)) {
      const c = centroid(pts)
      setPinLat(c[1])
      setPinLng(c[0])
      pinLatRef.current = c[1]
      pinLngRef.current = c[0]
      pinMarkerRef.current?.setLngLat(c)
    }
  }

  /** Create or move the offset pin marker */
  const syncPinMarker = useCallback((map: maplibregl.Map, lngLat: [number, number]) => {
    if (pinMarkerRef.current) {
      pinMarkerRef.current.setLngLat(lngLat)
      return
    }
    const el = document.createElement('div')
    el.style.cssText = `
      width:14px;height:14px;border-radius:50%;
      background:#3B82F6;border:2px solid white;
      box-shadow:0 1px 4px rgba(0,0,0,.5);cursor:grab;
      display:flex;align-items:center;justify-content:center;
    `
    el.title = t('privacyZones.editor.precisePinTitle')
    const marker = new maplibregl.Marker({ element: el, draggable: true })
      .setLngLat(lngLat)
      .addTo(map)

    marker.on('dragend', () => {
      const pos = marker.getLngLat()
      let newLat = pos.lat
      let newLng = pos.lng

      // Clamp pin to stay inside zone
      if (zoneModeRef.current === 'circle') {
        const clamped = clampToCircle(
          [newLng, newLat],
          [lngRef.current, latRef.current],
          radiusRef.current,
        )
        newLng = clamped[0]
        newLat = clamped[1]
        marker.setLngLat([newLng, newLat])
      } else if (zoneModeRef.current === 'polygon' && polygonPointsRef.current.length >= 3) {
        if (!pointInPolygon([newLng, newLat], polygonPointsRef.current)) {
          const c = centroid(polygonPointsRef.current)
          newLng = c[0]
          newLat = c[1]
          marker.setLngLat([newLng, newLat])
        }
      }

      setPinLat(newLat)
      setPinLng(newLng)
      pinLatRef.current = newLat
      pinLngRef.current = newLng
    })

    pinMarkerRef.current = marker
  }, [])

  function handleCenterDragEnd(marker: maplibregl.Marker, map: maplibregl.Map) {
    const pos = marker.getLngLat()
    setLat(pos.lat)
    setLng(pos.lng)
    latRef.current = pos.lat
    lngRef.current = pos.lng
    updateCircle(map, [pos.lng, pos.lat], radiusRef.current, colorRef.current)
    if (offsetPinEnabledRef.current) {
      const clamped = clampToCircle(
        [pinLngRef.current, pinLatRef.current],
        [pos.lng, pos.lat],
        radiusRef.current,
      )
      setPinLat(clamped[1])
      setPinLng(clamped[0])
      pinLatRef.current = clamped[1]
      pinLngRef.current = clamped[0]
      pinMarkerRef.current?.setLngLat(clamped)
    }
  }

  // Initialize map
  useEffect(() => {
    if (!mapContainerRef.current) return

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: {
        version: 8,
        sources: {
          osm: {
            type: 'raster',
            tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
            tileSize: 256,
            attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
          },
        },
        layers: [
          { id: 'osm-tiles', type: 'raster', source: 'osm', minzoom: 0, maxzoom: 19 },
        ],
      },
      center: [lng, lat],
      zoom: hasInitialPosition ? 14 : 6,
      attributionControl: {},
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      // ── Circle layer ─────────────────────────────────────────────────────
      map.addSource('zone-circle', {
        type: 'geojson',
        data: circlePolygon([lng, lat], radius),
      })
      map.addLayer({
        id: 'zone-circle-fill',
        type: 'fill',
        source: 'zone-circle',
        paint: { 'fill-color': colorRef.current, 'fill-opacity': 0.15 },
        layout: { visibility: zoneModeRef.current === 'circle' ? 'visible' : 'none' },
      })
      map.addLayer({
        id: 'zone-circle-border',
        type: 'line',
        source: 'zone-circle',
        paint: { 'line-color': colorRef.current, 'line-width': 2, 'line-opacity': 0.6 },
        layout: { visibility: zoneModeRef.current === 'circle' ? 'visible' : 'none' },
      })

      // ── Polygon layer ────────────────────────────────────────────────────
      map.addSource('zone-polygon', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
      map.addLayer({
        id: 'zone-polygon-fill',
        type: 'fill',
        source: 'zone-polygon',
        paint: { 'fill-color': colorRef.current, 'fill-opacity': 0.15 },
        filter: ['==', '$type', 'Polygon'],
        layout: { visibility: zoneModeRef.current === 'polygon' ? 'visible' : 'none' },
      })
      map.addLayer({
        id: 'zone-polygon-line',
        type: 'line',
        source: 'zone-polygon',
        paint: { 'line-color': colorRef.current, 'line-width': 2, 'line-opacity': 0.7, 'line-dasharray': [3, 2] },
        layout: { visibility: zoneModeRef.current === 'polygon' ? 'visible' : 'none' },
      })

      // Marker for circle mode
      if (hasInitialPosition && zoneModeRef.current === 'circle') {
        const marker = new maplibregl.Marker({ color: colorRef.current, draggable: true })
          .setLngLat([lng, lat])
          .addTo(map)
        marker.on('dragend', () => handleCenterDragEnd(marker, map))
        markerRef.current = marker
      }

      // Render initial polygon when editing a polygon zone
      if (zoneModeRef.current === 'polygon') {
        const initPts = parseInitialPolygon()
        if (initPts.length >= 3) {
          updatePolygonLayer(map, initPts, true, colorRef.current)
          updateVertexMarkers(map, initPts, colorRef.current)
          // Fit bounds
          const bounds = new maplibregl.LngLatBounds()
          initPts.forEach(p => bounds.extend(p))
          map.fitBounds(bounds, { padding: 60, maxZoom: 16, duration: 600 })
        }
      }
    })

    // Click handler: mode-specific behavior
    map.on('click', (e) => {
      if (zoneModeRef.current === 'circle') {
        const newLat = e.lngLat.lat
        const newLng = e.lngLat.lng
        setLat(newLat)
        setLng(newLng)
        latRef.current = newLat
        lngRef.current = newLng

        if (markerRef.current) {
          markerRef.current.setLngLat([newLng, newLat])
        } else {
          const marker = new maplibregl.Marker({ color: colorRef.current, draggable: true })
            .setLngLat([newLng, newLat])
            .addTo(map)
          marker.on('dragend', () => handleCenterDragEnd(marker, map))
          markerRef.current = marker
        }
        updateCircle(map, [newLng, newLat], radiusRef.current, colorRef.current)

        // Place pin at same location if offset not yet initialised
        if (offsetPinEnabledRef.current && !pinMarkerRef.current) {
          syncPinMarker(map, [newLng, newLat])
        }
      } else {
        // Polygon mode: only add points while still open
        if (polygonClosedRef.current) return
        const pts = polygonPointsRef.current
        // Click near the first point (>=3 points) to close the polygon automatically
        if (pts.length >= 3) {
          const firstPx = map.project(pts[0])
          const clickPx = map.project([e.lngLat.lng, e.lngLat.lat])
          const dx = firstPx.x - clickPx.x
          const dy = firstPx.y - clickPx.y
          if (dx * dx + dy * dy <= 15 * 15) {
            polygonClosedRef.current = true
            setPolygonClosed(true)
            if (map.isStyleLoaded()) {
              updatePolygonLayer(map, pts, true, colorRef.current)
            }
            return
          }
        }
        const newPt: [number, number] = [e.lngLat.lng, e.lngLat.lat]
        setPolygonPoints(prev => {
          const updated = [...prev, newPt]
          polygonPointsRef.current = updated
          if (map.isStyleLoaded()) {
            updatePolygonLayer(map, updated, false, colorRef.current)
            updateVertexMarkers(map, updated, colorRef.current)
          }
          return updated
        })
      }
    })

    // Double-click closes the polygon
    map.on('dblclick', (e) => {
      if (zoneModeRef.current !== 'polygon' || polygonClosedRef.current) return
      e.preventDefault() // Prevent zoom
      setPolygonPoints(prev => {
        if (prev.length < 3) return prev
        polygonClosedRef.current = true
        setPolygonClosed(true)
        if (map.isStyleLoaded()) {
          updatePolygonLayer(map, prev, true, colorRef.current)
        }
        // Place pin at centroid if offset mode is enabled
        if (offsetPinEnabledRef.current && !pinMarkerRef.current) {
          const c = centroid(prev)
          setPinLat(c[1])
          setPinLng(c[0])
          pinLatRef.current = c[1]
          pinLngRef.current = c[0]
          syncPinMarker(map, c)
        }
        return prev
      })
    })

    mapRef.current = map

    // Cursor management via CSS classes (MapLibre overrides canvas.style.cursor internally).
    injectPZECursorStyles()
    const container = mapContainerRef.current!
    let isMouseDown = false
    const applyCursor = () => {
      const drawing = zoneModeRef.current === 'polygon' && !polygonClosedRef.current
      setPZECursorClass(container, drawing ? 'crosshair' : '')
    }
    const onMousemove = () => { if (!isMouseDown) applyCursor() }
    const onMousedown = () => { isMouseDown = true;  setPZECursorClass(container, 'grabbing') }
    const onMouseup   = () => { isMouseDown = false; applyCursor() }
    const canvas = map.getCanvas()
    canvas.addEventListener('mousemove', onMousemove)
    canvas.addEventListener('mousedown', onMousedown)
    canvas.addEventListener('mouseup',   onMouseup)

    return () => {
      canvas.removeEventListener('mousemove', onMousemove)
      canvas.removeEventListener('mousedown', onMousedown)
      canvas.removeEventListener('mouseup',   onMouseup)
      setPZECursorClass(container, '')
      map.remove()
      mapRef.current = null
      pinMarkerRef.current = null
      polyMarkersRef.current = []
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Sync zoneMode ref and layer visibility
  useEffect(() => {
    zoneModeRef.current = zoneMode
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return

    setPZECursorClass(mapContainerRef.current!, 'crosshair')

    const apply = () => {
      const circleVis = zoneMode === 'circle' ? 'visible' : 'none'
      const polyVis = zoneMode === 'polygon' ? 'visible' : 'none'
      if (map.getLayer('zone-circle-fill')) map.setLayoutProperty('zone-circle-fill', 'visibility', circleVis)
      if (map.getLayer('zone-circle-border')) map.setLayoutProperty('zone-circle-border', 'visibility', circleVis)
      if (map.getLayer('zone-polygon-fill')) map.setLayoutProperty('zone-polygon-fill', 'visibility', polyVis)
      if (map.getLayer('zone-polygon-line')) map.setLayoutProperty('zone-polygon-line', 'visibility', polyVis)

      if (zoneMode === 'circle') {
        // Remove polygon markers
        polyMarkersRef.current.forEach(m => m.remove())
        polyMarkersRef.current = []
      } else {
        // Remove circle marker
        if (markerRef.current) { markerRef.current.remove(); markerRef.current = null }
        // Show existing polygon points
        updatePolygonLayer(map, polygonPoints, polygonClosed, color)
        updateVertexMarkers(map, polygonPoints, color)
      }
    }

    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [zoneMode])

  // Update circle when radius or color changes
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    colorRef.current = color
    const apply = () => {
      if (zoneMode === 'circle') {
        updateCircle(map, [lng, lat], radius, color)
      } else {
        updatePolygonLayer(map, polygonPoints, polygonClosed, color)
        // Update vertex marker color
        updateVertexMarkers(map, polygonPoints, color)
      }
    }
    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [radius, lat, lng, color])

  // Update polygon points; redraw layer and markers
  useEffect(() => {
    const map = mapRef.current
    if (!map || zoneMode !== 'polygon') return

    const apply = () => {
      updatePolygonLayer(map, polygonPoints, polygonClosed, color)
      // Do not recreate markers while a vertex is being dragged;
      // that would destroy the active drag marker
      if (!isDraggingVertexRef.current) {
        updateVertexMarkers(map, polygonPoints, color)
      }
    }
    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [polygonPoints, polygonClosed])

  // Keep polygonClosed ref in sync
  useEffect(() => { polygonClosedRef.current = polygonClosed }, [polygonClosed])

  // Handle offset-pin toggle
  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    if (offsetPinEnabled) {
      // Show pin at current zone center
      const initLat = zoneMode === 'circle' ? lat : (polygonPoints.length >= 3 ? centroid(polygonPoints)[1] : lat)
      const initLng = zoneMode === 'circle' ? lng : (polygonPoints.length >= 3 ? centroid(polygonPoints)[0] : lng)
      setPinLat(initLat)
      setPinLng(initLng)
      pinLatRef.current = initLat
      pinLngRef.current = initLng
      const apply = () => syncPinMarker(map, [initLng, initLat])
      if (map.isStyleLoaded()) apply()
      else map.once('style.load', apply)
    } else {
      // Remove pin marker
      if (pinMarkerRef.current) {
        pinMarkerRef.current.remove()
        pinMarkerRef.current = null
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [offsetPinEnabled])

  function handleClosePolygon() {
    if (polygonPoints.length < 3 || polygonClosed) return
    setPolygonClosed(true)
    polygonClosedRef.current = true
    const map = mapRef.current
    if (map && map.isStyleLoaded()) {
      updatePolygonLayer(map, polygonPoints, true, color)
    }
    // If offset pin enabled, place pin at centroid
    if (offsetPinEnabled && !pinMarkerRef.current && map) {
      const c = centroid(polygonPoints)
      setPinLat(c[1])
      setPinLng(c[0])
      pinLatRef.current = c[1]
      pinLngRef.current = c[0]
      const apply = () => syncPinMarker(map, c)
      if (map.isStyleLoaded()) apply()
      else map.once('style.load', apply)
    }
  }

  function handleResetPolygon() {
    setPolygonPoints([])
    setPolygonClosed(false)
    polygonClosedRef.current = false
    polygonPointsRef.current = []
    const map = mapRef.current
    if (map && map.isStyleLoaded()) {
      updatePolygonLayer(map, [], false, color)
      polyMarkersRef.current.forEach(m => m.remove())
      polyMarkersRef.current = []
    }
  }

  const isSaving = createMut.isPending || updateMut.isPending

  const canSave =
    name.trim().length > 0 &&
    (zoneMode === 'circle' || (polygonPoints.length >= 3 && polygonClosed))

  // Check if pin is inside zone (for validation hint)
  const pinInsideZone = (() => {
    if (!offsetPinEnabled) return true
    if (zoneMode === 'circle') {
      return haversineM(pinLat, pinLng, lat, lng) <= radius
    } else if (polygonPoints.length >= 3) {
      return pointInPolygon([pinLng, pinLat], polygonPoints)
    }
    return true
  })()

  return (
    <div className="space-y-4">
      {/* Mode toggle */}
      <div className="flex gap-2">
        <button
          onClick={() => setZoneMode('circle')}
          className={`flex-1 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer transition-colors border ${
            zoneMode === 'circle'
              ? 'bg-accent-DEFAULT border-accent-DEFAULT text-white'
              : 'border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
          }`}
        >
          {t('privacyZones.editor.circle')}
        </button>
        <button
          onClick={() => setZoneMode('polygon')}
          className={`flex-1 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer transition-colors border ${
            zoneMode === 'polygon'
              ? 'bg-accent-DEFAULT border-accent-DEFAULT text-white'
              : 'border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
          }`}
        >
          {t('privacyZones.editor.polygon')}
        </button>
      </div>

      {/* Map */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <div ref={mapContainerRef} className="h-[400px] w-full" />
      </div>

      {/* Mode-specific hint */}
      {zoneMode === 'circle' ? (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {t('privacyZones.editor.circleHint')}
        </p>
      ) : (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {polygonClosed
            ? t('privacyZones.editor.polygonDone')
            : polygonPoints.length < 3
            ? t('privacyZones.editor.addVerticesHint', { count: polygonPoints.length })
            : t('privacyZones.editor.closePolygonHint', { count: polygonPoints.length })}
        </p>
      )}

      {/* Polygon controls */}
      {zoneMode === 'polygon' && (
        <div className="flex gap-2">
          <button
            onClick={handleClosePolygon}
            disabled={polygonPoints.length < 3 || polygonClosed}
            className="px-3 py-1.5 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
          >
            {t('privacyZones.editor.closePolygon')}
          </button>
          <button
            onClick={handleResetPolygon}
            disabled={polygonPoints.length === 0}
            className="px-3 py-1.5 rounded-lg text-sm font-medium border border-red-200 dark:border-red-900 text-red-600 dark:text-red-400 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed hover:bg-red-50 dark:hover:bg-red-950 transition-colors"
          >
            {t('privacyZones.editor.reset')}
          </button>
          <span className="ml-auto self-center text-sm text-neutral-400 tabular-nums">
            {t('privacyZones.editor.vertexCount', { count: polygonPoints.length })}
          </span>
        </div>
      )}

      {/* Name */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          {t('privacyZones.editor.name')}
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('privacyZones.editor.namePlaceholder')}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
      </div>

      {/* Radius slider (circle mode only) */}
      {zoneMode === 'circle' && (
        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {t('privacyZones.editor.radius')}
            </label>
            <span className="text-sm text-neutral-500 tabular-nums">{formatDistanceCompact(radius)}</span>
          </div>
          <input
            type="range"
            min={100}
            max={1000}
            step={50}
            value={radius}
            onChange={(e) => setRadius(Number(e.target.value))}
            className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT cursor-pointer"
          />
          <div className="flex justify-between text-xs text-neutral-400 mt-1">
            <span>{formatDistanceCompact(100)}</span>
            <span>{formatDistanceCompact(1000)}</span>
          </div>
        </div>
      )}

      {/* Offset-Pin Toggle */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-3 space-y-2">
        <div className="flex items-center gap-3">
          <Toggle
            checked={offsetPinEnabled}
            onChange={() => setOffsetPinEnabled(v => !v)}
            color="#3B82F6"
            aria-label={t('privacyZones.editor.enableOffsetPin')}
          />
          <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('privacyZones.editor.offsetPin')}</span>
        </div>
        {offsetPinEnabled ? (
          <div className="space-y-1">
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              <span className="inline-block w-2.5 h-2.5 rounded-full bg-blue-400 mr-1 relative top-px" />
              <strong>{t('privacyZones.editor.pin')}</strong> = {t('privacyZones.editor.pinDescription')}
            </p>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              <span className="inline-block w-2.5 h-2.5 rounded-full bg-orange-400 mr-1 relative top-px" />
              <strong>{t('privacyZones.editor.zone')}</strong> = {t('privacyZones.editor.zoneDescription')}
            </p>
            {!pinInsideZone && (
              <p className="text-xs text-amber-600 dark:text-amber-400">
                {t('privacyZones.editor.pinOutside')}
              </p>
            )}
            <p className="text-xs text-neutral-400 tabular-nums">
              Pin: {pinLat.toFixed(5)}, {pinLng.toFixed(5)}
            </p>
          </div>
        ) : (
          <p className="text-xs text-neutral-400 dark:text-neutral-500">
            {t('privacyZones.editor.offsetPinDisabledHint')}
          </p>
        )}
      </div>

      {/* Color picker */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          {t('privacyZones.editor.zoneColor')}
        </label>
        <ColorPicker value={color} onChange={setColor} />
      </div>

      {/* Coordinates (circle mode only, no offset) */}
      {zoneMode === 'circle' && !offsetPinEnabled && (
        <div className="text-xs text-neutral-400 tabular-nums">
          {lat.toFixed(5)}, {lng.toFixed(5)}
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-3">
        <button
          onClick={handleSave}
          disabled={!canSave || isSaving}
          className="bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed text-white rounded-lg px-4 py-2 text-sm font-medium transition-colors"
        >
          {isSaving ? t('common.saving') : t('common.save')}
        </button>
        <button
          onClick={onClose}
          className="text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors cursor-pointer"
        >
          {t('common.cancel')}
        </button>
      </div>
    </div>
  )
}
