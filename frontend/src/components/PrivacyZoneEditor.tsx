import { useRef, useEffect, useState, useCallback } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { createPrivacyZone, updatePrivacyZone, type PrivacyZone } from '@/api/privacy'
import ColorPicker from '@/components/ColorPicker'
import Toggle from '@/components/Toggle'
import { useToast } from '@/lib/toast'

interface Props {
  zone: PrivacyZone | null
  onClose: () => void
}

type ZoneMode = 'circle' | 'polygon'

function circlePolygon(center: [number, number], radiusM: number, steps = 64): GeoJSON.Feature {
  const coords: [number, number][] = []
  const lat = center[1]
  const lng = center[0]

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

/** Berechne Schwerpunkt eines Polygon-Punktearrays */
function centroid(points: [number, number][]): [number, number] {
  const sum = points.reduce(([ax, ay], [x, y]) => [ax + x, ay + y], [0, 0])
  return [sum[0] / points.length, sum[1] / points.length]
}

/** Haversine distance in metres */
function haversineM(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const R = 6371000
  const p1 = (lat1 * Math.PI) / 180
  const p2 = (lat2 * Math.PI) / 180
  const dp = ((lat2 - lat1) * Math.PI) / 180
  const dl = ((lon2 - lon1) * Math.PI) / 180
  const a = Math.sin(dp / 2) ** 2 + Math.cos(p1) * Math.cos(p2) * Math.sin(dl / 2) ** 2
  return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
}

/** Clamp a point to within radiusM of center */
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
  return [
    clng + (dx * scale) / (111320 * Math.cos((clat * Math.PI) / 180)),
    clat + (dy * scale) / 111320,
  ]
}

/** Ray-casting point-in-polygon */
function pointInPolygon(pt: [number, number], ring: [number, number][]): boolean {
  let inside = false
  const [x, y] = pt
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [xi, yi] = ring[i]
    const [xj, yj] = ring[j]
    const intersect = yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi
    if (intersect) inside = !inside
  }
  return inside
}

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

export default function PrivacyZoneEditor({ zone, onClose }: Props) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)   // circle center or polygon polygon
  const pinMarkerRef = useRef<maplibregl.Marker | null>(null) // offset pin (blue)
  // Polygon-Vertex-Marker-Liste
  const polyMarkersRef = useRef<maplibregl.Marker[]>([])

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

  // Polygon-Punkte: beim Bearbeiten einer Polygon-Zone initial aus GeoJSON laden
  const parseInitialPolygon = (): [number, number][] => {
    if (zone?.zone_type === 'polygon' && zone.polygon) {
      try {
        const parsed = JSON.parse(zone.polygon) as GeoJSON.Polygon
        const ring = parsed.coordinates[0]
        // Letzten Punkt weglassen (gleich erstem bei geschlossenem Polygon)
        return ring.slice(0, -1) as [number, number][]
      } catch {
        return []
      }
    }
    return []
  }

  const [polygonPoints, setPolygonPoints] = useState<[number, number][]>(parseInitialPolygon)
  const [polygonClosed, setPolygonClosed] = useState(zone?.zone_type === 'polygon' && (zone.polygon ?? '').length > 0)

  // Refs für Werte, die im MapLibre-Event-Handler benötigt werden
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
      addToast({ type: 'success', title: 'Zone erstellt' })
      onClose()
    },
    onError: (error) => {
      addToast({ type: 'error', title: 'Zone konnte nicht erstellt werden', message: error instanceof Error ? error.message : 'Unbekannter Fehler' })
    },
  })

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Parameters<typeof updatePrivacyZone>[1] }) =>
      updatePrivacyZone(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['privacy-zones'] })
      addToast({ type: 'success', title: 'Zone aktualisiert' })
      onClose()
    },
    onError: (error) => {
      addToast({ type: 'error', title: 'Zone konnte nicht aktualisiert werden', message: error instanceof Error ? error.message : 'Unbekannter Fehler' })
    },
  })

  function buildPolygonGeoJSON(points: [number, number][]): string {
    // GeoJSON Polygon braucht geschlossenen Ring: erster == letzter Punkt
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
      // Polygon-Modus
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

  // Polygon-Layer-Daten aktualisieren
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

  // Vertex-Marker aktualisieren
  const updateVertexMarkers = useCallback((map: maplibregl.Map, points: [number, number][], c: string) => {
    // Alte Marker entfernen
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

      marker.on('drag', () => {
        const pos = marker.getLngLat()
        setPolygonPoints(prev => {
          const updated = [...prev]
          updated[idx] = [pos.lng, pos.lat]
          polygonPointsRef.current = updated
          // If offset pin is enabled, keep pin inside polygon
          if (offsetPinEnabledRef.current) {
            clampPinToPolygon(map, updated)
          }
          return updated
        })
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
    el.title = 'Genauer Pin (wahre Position)'
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
      // ── Kreis-Layer ──────────────────────────────────────────────────────
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

      // ── Polygon-Layer ─────────────────────────────────────────────────────
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

      // Marker für Kreismodus
      if (hasInitialPosition && zoneModeRef.current === 'circle') {
        const marker = new maplibregl.Marker({ color: colorRef.current, draggable: true })
          .setLngLat([lng, lat])
          .addTo(map)
        marker.on('dragend', () => {
          const pos = marker.getLngLat()
          setLat(pos.lat)
          setLng(pos.lng)
          latRef.current = pos.lat
          lngRef.current = pos.lng
          updateCircle(map, [pos.lng, pos.lat], radiusRef.current, colorRef.current)
          // Clamp pin if offset enabled
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
        })
        markerRef.current = marker
      }

      // Initial-Polygon rendern wenn Bearbeitung einer Polygon-Zone
      if (zoneModeRef.current === 'polygon') {
        const initPts = parseInitialPolygon()
        if (initPts.length >= 3) {
          updatePolygonLayer(map, initPts, true, colorRef.current)
          updateVertexMarkers(map, initPts, colorRef.current)
          // Bounds anpassen
          const bounds = new maplibregl.LngLatBounds()
          initPts.forEach(p => bounds.extend(p))
          map.fitBounds(bounds, { padding: 60, maxZoom: 16, duration: 600 })
        }
      }
    })

    // Klick-Handler: unterschiedlich je nach Modus
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
          marker.on('dragend', () => {
            const pos = marker.getLngLat()
            setLat(pos.lat)
            setLng(pos.lng)
            latRef.current = pos.lat
            lngRef.current = pos.lng
            updateCircle(map, [pos.lng, pos.lat], radiusRef.current, colorRef.current)
            // Clamp pin
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
          })
          markerRef.current = marker
        }
        updateCircle(map, [newLng, newLat], radiusRef.current, colorRef.current)

        // Place pin at same location if offset not yet initialised
        if (offsetPinEnabledRef.current && !pinMarkerRef.current) {
          syncPinMarker(map, [newLng, newLat])
        }
      } else {
        // Polygon-Modus: Punkt nur hinzufügen wenn noch nicht geschlossen
        if (polygonClosedRef.current) return
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

    // Doppelklick schließt Polygon
    map.on('dblclick', (e) => {
      if (zoneModeRef.current !== 'polygon' || polygonClosedRef.current) return
      e.preventDefault() // Zoom verhindern
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

    // Cursor: crosshair im Zeichen-Modus, grabbing nur bei gedrückter Maustaste
    const canvas = map.getCanvas()
    canvas.style.cursor = 'crosshair'
    const onMousedown = () => { canvas.style.cursor = 'grabbing' }
    const onMouseup   = () => { canvas.style.cursor = 'crosshair' }
    canvas.addEventListener('mousedown', onMousedown)
    canvas.addEventListener('mouseup',   onMouseup)

    return () => {
      canvas.removeEventListener('mousedown', onMousedown)
      canvas.removeEventListener('mouseup',   onMouseup)
      map.remove()
      mapRef.current = null
      pinMarkerRef.current = null
      polyMarkersRef.current = []
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Sync zoneMode-Ref und Sichtbarkeit der Layer
  useEffect(() => {
    zoneModeRef.current = zoneMode
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return

    map.getCanvas().style.cursor = 'crosshair'

    const apply = () => {
      const circleVis = zoneMode === 'circle' ? 'visible' : 'none'
      const polyVis = zoneMode === 'polygon' ? 'visible' : 'none'
      if (map.getLayer('zone-circle-fill')) map.setLayoutProperty('zone-circle-fill', 'visibility', circleVis)
      if (map.getLayer('zone-circle-border')) map.setLayoutProperty('zone-circle-border', 'visibility', circleVis)
      if (map.getLayer('zone-polygon-fill')) map.setLayoutProperty('zone-polygon-fill', 'visibility', polyVis)
      if (map.getLayer('zone-polygon-line')) map.setLayoutProperty('zone-polygon-line', 'visibility', polyVis)

      if (zoneMode === 'circle') {
        // Polygon-Marker entfernen
        polyMarkersRef.current.forEach(m => m.remove())
        polyMarkersRef.current = []
      } else {
        // Kreis-Marker entfernen
        if (markerRef.current) { markerRef.current.remove(); markerRef.current = null }
        // Bestehende Polygon-Punkte anzeigen
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
        // Farbe der Vertex-Marker aktualisieren
        updateVertexMarkers(map, polygonPoints, color)
      }
    }
    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [radius, lat, lng, color])

  // Polygon-Punkte aktualisieren → Layer + Marker neu zeichnen
  useEffect(() => {
    const map = mapRef.current
    if (!map || zoneMode !== 'polygon') return

    const apply = () => {
      updatePolygonLayer(map, polygonPoints, polygonClosed, color)
      updateVertexMarkers(map, polygonPoints, color)
    }
    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [polygonPoints, polygonClosed])

  // polygonClosed-Ref synchron halten
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
      {/* Modus-Toggle */}
      <div className="flex gap-2">
        <button
          onClick={() => setZoneMode('circle')}
          className={`flex-1 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer transition-colors border ${
            zoneMode === 'circle'
              ? 'bg-accent-DEFAULT border-accent-DEFAULT text-white'
              : 'border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
          }`}
        >
          Kreis
        </button>
        <button
          onClick={() => setZoneMode('polygon')}
          className={`flex-1 px-4 py-2 rounded-lg text-sm font-medium cursor-pointer transition-colors border ${
            zoneMode === 'polygon'
              ? 'bg-accent-DEFAULT border-accent-DEFAULT text-white'
              : 'border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
          }`}
        >
          Polygon
        </button>
      </div>

      {/* Map */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <div ref={mapContainerRef} className="h-[400px] w-full" />
      </div>

      {/* Hinweis je nach Modus */}
      {zoneMode === 'circle' ? (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Klicke auf die Karte, um das Zentrum der Zone festzulegen. Den Marker kannst du auch verschieben.
        </p>
      ) : (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {polygonClosed
            ? 'Polygon fertig. Eckpunkte lassen sich per Drag-and-Drop verschieben.'
            : polygonPoints.length < 3
            ? `Klicke auf die Karte, um Eckpunkte hinzuzufügen (${polygonPoints.length} von mind. 3).`
            : `${polygonPoints.length} Eckpunkte gesetzt. Klicke auf "Schließen" oder doppelklicke um das Polygon zu schließen.`}
        </p>
      )}

      {/* Polygon-Steuerung */}
      {zoneMode === 'polygon' && (
        <div className="flex gap-2">
          <button
            onClick={handleClosePolygon}
            disabled={polygonPoints.length < 3 || polygonClosed}
            className="px-3 py-1.5 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
          >
            Polygon schliessen
          </button>
          <button
            onClick={handleResetPolygon}
            disabled={polygonPoints.length === 0}
            className="px-3 py-1.5 rounded-lg text-sm font-medium border border-red-200 dark:border-red-900 text-red-600 dark:text-red-400 disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed hover:bg-red-50 dark:hover:bg-red-950 transition-colors"
          >
            Zurücksetzen
          </button>
          <span className="ml-auto self-center text-sm text-neutral-400 tabular-nums">
            {polygonPoints.length} Eckpunkte
          </span>
        </div>
      )}

      {/* Name */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Name
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="z.B. Zuhause, Arbeitsplatz"
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
      </div>

      {/* Radius-Slider (nur im Kreis-Modus) */}
      {zoneMode === 'circle' && (
        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Radius
            </label>
            <span className="text-sm text-neutral-500 tabular-nums">{formatRadius(radius)}</span>
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
            <span>100 m</span>
            <span>1 km</span>
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
            aria-label="Versetzten Pin aktivieren"
          />
          <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Versetzter Pin</span>
        </div>
        {offsetPinEnabled ? (
          <div className="space-y-1">
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              <span className="inline-block w-2.5 h-2.5 rounded-full bg-blue-400 mr-1 relative top-px" />
              <strong>Pin</strong> = genaue Position (für Regeln). Verschiebe den blauen Punkt auf der Karte.
            </p>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              <span className="inline-block w-2.5 h-2.5 rounded-full bg-orange-400 mr-1 relative top-px" />
              <strong>Zone</strong> = Maskierungsbereich (unsichtbar in öffentlicher Heatmap).
            </p>
            {!pinInsideZone && (
              <p className="text-xs text-amber-600 dark:text-amber-400">
                Pin liegt ausserhalb der Zone — beim Speichern wird er automatisch zur Zone-Mitte zurückgesetzt.
              </p>
            )}
            <p className="text-xs text-neutral-400 tabular-nums">
              Pin: {pinLat.toFixed(5)}, {pinLng.toFixed(5)}
            </p>
          </div>
        ) : (
          <p className="text-xs text-neutral-400 dark:text-neutral-500">
            Aktiviere diese Option, um den genauen Standort (Pin) vom Maskierungsbereich (Zone) zu trennen.
          </p>
        )}
      </div>

      {/* Color picker */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          Zonenfarbe
        </label>
        <ColorPicker value={color} onChange={setColor} />
      </div>

      {/* Koordinaten (nur Kreis-Modus, kein Offset) */}
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
          {isSaving ? 'Speichern...' : 'Speichern'}
        </button>
        <button
          onClick={onClose}
          className="text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors cursor-pointer"
        >
          Abbrechen
        </button>
      </div>
    </div>
  )
}
