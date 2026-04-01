import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { useRef, useState, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getHeatmapSettings } from '@/api/settings'
import { listActivityPhotos, type ActivityPhoto } from '@/api/activityPhotos'
import { apiDownload } from '@/api/client'
import { haversineM } from '@/lib/geoUtils'
import type { StreamPoint, GeoTag, ActivityStop } from '@/api/types'
import type { ZoomRange } from '@/components/ActivityCharts'

interface TrackMapProps {
  activityId?: string
  streams: StreamPoint[]
  hoverIndex: number | null
  sportColor: string
  hasGps: boolean
  zoomRange: ZoomRange | null
  onMaximize: () => void
  geoTags?: GeoTag[]
  stops?: ActivityStop[]
  locationPin?: { lat: number; lng: number; name: string; color: string }
}

const SUMMIT_ICON_SVG = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">' +
  '<path d="M2 20 L8 8 L12 14 L16 8 L22 20 Z" fill="#f97316" stroke="white" stroke-width="1.5"/>' +
  '</svg>'
)}`

/** Teilt Koordinaten bei Sprüngen > thresholdM in separate Segmente auf */
function splitAtGaps(coords: [number, number][], thresholdM: number): [number, number][][] {
  if (coords.length < 2 || thresholdM <= 0) return [coords]
  const segments: [number, number][][] = []
  let current: [number, number][] = [coords[0]]
  for (let i = 1; i < coords.length; i++) {
    if (haversineM(coords[i-1][1], coords[i-1][0], coords[i][1], coords[i][0]) > thresholdM) {
      if (current.length >= 2) segments.push(current)
      current = [coords[i]]
    } else {
      current.push(coords[i])
    }
  }
  if (current.length >= 2) segments.push(current)
  return segments.length > 0 ? segments : [coords]
}

export default function TrackMap({ activityId, streams, hoverIndex, sportColor, hasGps, zoomRange, onMaximize, geoTags, stops, locationPin }: TrackMapProps) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const locationPinMapContainerRef = useRef<HTMLDivElement>(null)
  const locationPinMapRef = useRef<maplibregl.Map | null>(null)
  const photoMarkersRef = useRef<maplibregl.Marker[]>([])
  const photoBlobUrlsRef = useRef<string[]>([])

  const [showPhotos, setShowPhotos] = useState(true)
  const [mapLoaded, setMapLoaded] = useState(false)

  const { data: allPhotos = [] } = useQuery({
    queryKey: ['activity-photos', activityId],
    queryFn: () => listActivityPhotos(activityId!),
    enabled: !!activityId,
    staleTime: 1000 * 60 * 5,
  })

  const positionedPhotos = useMemo(
    () => allPhotos.filter((p): p is ActivityPhoto & { lat: number; lng: number } => p.lat != null && p.lng != null),
    [allPhotos],
  )

  const { data: heatmapSettings } = useQuery({
    queryKey: ['heatmap-settings'],
    queryFn: getHeatmapSettings,
    staleTime: Infinity,
  })

  // Build GeoJSON from stream lat/lng — memoized to avoid recomputation on every hover render
  const { coordinates, trackSegments, trackGeometry, hasTrack } = useMemo(() => {
    const coordinates = streams
      .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
      .map(s => [s.lng, s.lat] as [number, number])
    const hasTrack = coordinates.length >= 2
    const teleportThresholdM = heatmapSettings?.teleport_filter_activity
      ? (heatmapSettings.teleport_threshold_m ?? 2000)
      : 0
    const trackSegments = splitAtGaps(coordinates, teleportThresholdM)
    const trackGeometry = trackSegments.length === 1
      ? { type: 'LineString' as const, coordinates: trackSegments[0] }
      : { type: 'MultiLineString' as const, coordinates: trackSegments }
    return { coordinates, trackSegments, trackGeometry, hasTrack }
  }, [streams, heatmapSettings])

  // Initialize map
  useEffect(() => {
    if (!mapContainerRef.current || !hasTrack) return

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
          {
            id: 'osm-tiles',
            type: 'raster',
            source: 'osm',
            minzoom: 0,
            maxzoom: 19,
          },
        ],
      },
      attributionControl: {},
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      // Create heading arrow icon (simple triangle pointing up = 0°)
      // MapLibre requires ImageData (not HTMLCanvasElement) for reliable dimension detection.
      // Wrapped in try/catch so a failure here never blocks track/source loading below.
      const arrowSize = 16
      const arrowCanvas = document.createElement('canvas')
      arrowCanvas.width = arrowSize
      arrowCanvas.height = arrowSize
      const ctx2d = arrowCanvas.getContext('2d')
      if (ctx2d) {
        ctx2d.fillStyle = '#ffffff' // SDF: weiß = opaque, transparent = unsichtbar
        ctx2d.beginPath()
        ctx2d.moveTo(arrowSize / 2, 0)
        ctx2d.lineTo(arrowSize, arrowSize)
        ctx2d.lineTo(arrowSize / 2, arrowSize * 0.7)
        ctx2d.lineTo(0, arrowSize)
        ctx2d.closePath()
        ctx2d.fill()
        try {
          map.addImage('heading-arrow', ctx2d.getImageData(0, 0, arrowSize, arrowSize), { sdf: true })
        } catch {
          // heading arrows not available in this MapLibre version/context
        }
      }

      // Full track — greyed out when a segment is highlighted
      map.addSource('track', {
        type: 'geojson',
        data: {
          type: 'Feature',
          properties: {},
          geometry: trackGeometry,
        },
      })
      map.addLayer({
        id: 'track-line',
        type: 'line',
        source: 'track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': sportColor, 'line-width': 3, 'line-opacity': 0.85 },
      })

      // Highlight segment — shown on top in sport color when a zoom range is active
      map.addSource('highlight', {
        type: 'geojson',
        data: { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: [] } },
      })
      map.addLayer({
        id: 'highlight-line',
        type: 'line',
        source: 'highlight',
        layout: { 'line-join': 'round', 'line-cap': 'round', visibility: 'none' },
        paint: { 'line-color': sportColor, 'line-width': 4, 'line-opacity': 1 },
      })

      // Heading arrows: sample every ~50 points with valid heading
      const headingFeatures: GeoJSON.Feature[] = []
      const step = Math.max(1, Math.floor(streams.length / 40))
      for (let i = 0; i < streams.length; i += step) {
        const s = streams[i]
        if (s.heading != null && s.lat != null && s.lng != null) {
          headingFeatures.push({
            type: 'Feature',
            geometry: { type: 'Point', coordinates: [s.lng, s.lat] },
            properties: { heading: s.heading },
          })
        }
      }

      if (headingFeatures.length > 0 && map.hasImage('heading-arrow')) {
        map.addSource('heading-arrows', {
          type: 'geojson',
          data: { type: 'FeatureCollection', features: headingFeatures },
        })
        map.addLayer({
          id: 'heading-arrows',
          type: 'symbol',
          source: 'heading-arrows',
          layout: {
            'icon-image': 'heading-arrow',
            'icon-size': 0.6,
            'icon-rotate': ['get', 'heading'],
            'icon-rotation-alignment': 'map',
            'icon-allow-overlap': true,
          },
          paint: {
            'icon-color': '#dc2626', // red-600 — gut sichtbar auf OSM-Raster-Karten
          },
        })
      }

      // Gipfel-Pins aus GeoTags (category: summit/pass)
      const summitTags = (geoTags ?? []).filter(
        (t) => (t.category === 'summit' || t.category === 'pass') && t.lat != null
      )
      if (summitTags.length > 0) {
        const summitGeoJSON = {
          type: 'FeatureCollection' as const,
          features: summitTags.map((t) => ({
            type: 'Feature' as const,
            geometry: { type: 'Point' as const, coordinates: [t.lng!, t.lat!] },
            properties: { name: t.name ?? '', ele: t.ele_m ?? t.ele ?? 0 },
          })),
        }
        map.addSource('activity-summits', { type: 'geojson', data: summitGeoJSON })
        const addSummitLayers = () => {
          map.addLayer({
            id: 'activity-summits-layer',
            type: 'symbol',
            source: 'activity-summits',
            layout: {
              'icon-image': 'summit-icon',
              'icon-size': 0.9,
              'icon-allow-overlap': true,
              'text-field': ['concat', ['get', 'name'], '\n', ['to-string', ['get', 'ele']], ' m'] as maplibregl.ExpressionSpecification,
              'text-font': ['Open Sans Regular'],
              'text-size': 11,
              'text-offset': [0, 1.4],
              'text-anchor': 'top',
              'text-optional': true,
            },
            paint: { 'text-color': '#374151', 'text-halo-color': 'white', 'text-halo-width': 1 },
          })
        }
        if (map.hasImage('summit-icon')) {
          addSummitLayers()
        } else {
          const img = new Image(24, 24)
          img.onload = () => { map.addImage('summit-icon', img); addSummitLayers() }
          img.onerror = () => {
            // Fallback auf circle wenn Icon nicht ladbar
            map.addLayer({
              id: 'activity-summits-layer',
              type: 'circle',
              source: 'activity-summits',
              paint: { 'circle-radius': 6, 'circle-color': '#f97316', 'circle-stroke-color': 'white', 'circle-stroke-width': 2 },
            })
          }
          img.src = SUMMIT_ICON_SVG
        }
      }

      // Restaurant-Pins aus Stops (duration >= 900s) — Fallback ohne GeoTags
      const poiStops = (stops ?? []).filter(
        (s) => s.name && (s.duration_s ?? 0) >= 900 && s.lat != null
      )
      if (poiStops.length > 0) {
        const poiGeoJSON = {
          type: 'FeatureCollection' as const,
          features: poiStops.map((s) => ({
            type: 'Feature' as const,
            geometry: { type: 'Point' as const, coordinates: [s.lng!, s.lat!] },
            properties: {
              name: s.poi_name ?? s.name!,
              duration_min: Math.round((s.duration_s ?? 0) / 60),
              poi_type: s.poi_type ?? '',
            },
          })),
        }
        map.addSource('activity-pois', { type: 'geojson', data: poiGeoJSON })
        map.addLayer({
          id: 'activity-pois-layer',
          type: 'circle',
          source: 'activity-pois',
          paint: {
            'circle-radius': 8,
            'circle-color': '#3b82f6',
            'circle-stroke-color': 'white',
            'circle-stroke-width': 2,
          },
        })
        map.on('click', 'activity-pois-layer', (e) => {
          const props = e.features?.[0]?.properties
          if (!props) return
          new maplibregl.Popup()
            .setLngLat(e.lngLat)
            .setHTML(`<strong>${props.name}</strong><br>${props.duration_min} min`)
            .addTo(map)
        })
      }

      // Gastronomy-Pins aus GeoTags (category: gastronomy)
      const gastronomyTags = (geoTags ?? []).filter(
        (t) => t.category === 'gastronomy' && t.map_point_lat != null && t.map_point_lon != null
      )
      if (gastronomyTags.length > 0) {
        const gastronomyGeoJSON = {
          type: 'FeatureCollection' as const,
          features: gastronomyTags.map((t) => ({
            type: 'Feature' as const,
            geometry: { type: 'Point' as const, coordinates: [t.map_point_lon!, t.map_point_lat!] },
            properties: {
              name: t.name,
              duration_min: t.duration_min != null ? Math.round(t.duration_min) : null,
              poi_type: (t.meta?.poi_type as string | undefined) ?? '',
            },
          })),
        }
        map.addSource('activity-gastronomy-source', { type: 'geojson', data: gastronomyGeoJSON })
        map.addLayer({
          id: 'activity-gastronomy-layer',
          type: 'circle',
          source: 'activity-gastronomy-source',
          paint: {
            'circle-color': '#f97316',
            'circle-radius': 7,
            'circle-stroke-width': 2,
            'circle-stroke-color': '#fff',
          },
        })
        map.on('click', 'activity-gastronomy-layer', (e) => {
          const props = e.features?.[0]?.properties
          if (!props) return
          const durationStr = props.duration_min != null ? `<br>${props.duration_min} min` : ''
          const typeStr = props.poi_type ? `<br><span style="color:#6b7280">${props.poi_type}</span>` : ''
          new maplibregl.Popup()
            .setLngLat(e.lngLat)
            .setHTML(`<strong>${props.name}</strong>${typeStr}${durationStr}`)
            .addTo(map)
        })
      }

      const bounds = new maplibregl.LngLatBounds()
      for (const c of coordinates) bounds.extend(c)
      map.fitBounds(bounds, { padding: 40, maxZoom: 15 })

      setMapLoaded(true)
    })

    mapRef.current = map
    return () => {
      photoMarkersRef.current.forEach(m => m.remove())
      photoMarkersRef.current = []
      photoBlobUrlsRef.current.forEach(u => URL.revokeObjectURL(u))
      photoBlobUrlsRef.current = []
      if (map.getLayer('heading-arrows')) map.removeLayer('heading-arrows')
      if (map.getSource('heading-arrows')) map.removeSource('heading-arrows')
      map.remove()
    }
  }, [hasTrack]) // eslint-disable-line react-hooks/exhaustive-deps

  // Location-Pin-Karte für Indoor-Aktivitäten ohne GPS
  useEffect(() => {
    if (!locationPinMapContainerRef.current || !locationPin) return

    const map = new maplibregl.Map({
      container: locationPinMapContainerRef.current,
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
        layers: [{ id: 'osm-tiles', type: 'raster', source: 'osm', minzoom: 0, maxzoom: 19 }],
      },
      center: [locationPin.lng, locationPin.lat],
      zoom: 15,
      attributionControl: {},
    })

    const el = document.createElement('div')
    el.style.cssText = `width:18px;height:18px;border-radius:50%;background:${locationPin.color};border:3px solid white;box-shadow:0 2px 8px rgba(0,0,0,0.4);`
    new maplibregl.Marker({ element: el })
      .setLngLat([locationPin.lng, locationPin.lat])
      .setPopup(new maplibregl.Popup({ offset: 25 }).setHTML(`<strong>${locationPin.name}</strong>`))
      .addTo(map)

    locationPinMapRef.current = map
    return () => { map.remove() }
  }, [locationPin?.lat, locationPin?.lng, locationPin?.name, locationPin?.color]) // eslint-disable-line react-hooks/exhaustive-deps

  // Hover-Marker: Index bezieht sich auf das volle streams-Array,
  // nicht auf das gefilterte coordinates-Array (Index-Mismatch vermeiden)
  useEffect(() => {
    const map = mapRef.current
    if (!map || !hasTrack) return

    const point = hoverIndex != null ? streams[hoverIndex] : null
    const hasPos = point?.lat != null && point?.lng != null

    if (hasPos) {
      const lngLat: [number, number] = [point.lng!, point.lat!]
      if (!markerRef.current) {
        const el = document.createElement('div')
        el.className = 'track-hover-marker'
        el.style.cssText = 'width:12px;height:12px;border-radius:50%;background:#fff;border:2px solid #f97316;box-shadow:0 0 4px rgba(0,0,0,0.3);'
        markerRef.current = new maplibregl.Marker({ element: el }).setLngLat(lngLat).addTo(map)
      } else {
        markerRef.current.setLngLat(lngLat)
      }
    } else if (markerRef.current) {
      markerRef.current.remove()
      markerRef.current = null
    }
  }, [hoverIndex, streams, hasTrack])

  // Photo thumbnail markers — sync with positionedPhotos + showPhotos toggle
  useEffect(() => {
    const map = mapRef.current
    if (!map || !mapLoaded) return

    // Remove all existing photo markers + revoke blob URLs
    photoMarkersRef.current.forEach(m => m.remove())
    photoMarkersRef.current = []
    photoBlobUrlsRef.current.forEach(u => URL.revokeObjectURL(u))
    photoBlobUrlsRef.current = []

    if (!showPhotos || positionedPhotos.length === 0) return

    for (const photo of positionedPhotos) {
      const el = document.createElement('div')
      el.style.cssText = 'cursor:pointer;filter:drop-shadow(0 1px 4px rgba(0,0,0,0.5));'

      const imgBox = document.createElement('div')
      imgBox.style.cssText =
        'width:40px;height:30px;border-radius:3px;border:2px solid white;overflow:hidden;background:#444;'

      const triangle = document.createElement('div')
      triangle.style.cssText =
        'width:0;height:0;border-left:5px solid transparent;border-right:5px solid transparent;border-top:6px solid white;margin:0 auto;'

      el.appendChild(imgBox)
      el.appendChild(triangle)

      const marker = new maplibregl.Marker({ element: el, anchor: 'bottom' })
        .setLngLat([photo.lng, photo.lat])
        .addTo(map)

      photoMarkersRef.current.push(marker)

      // Load thumbnail with auth
      apiDownload(photo.thumb_url)
        .then(({ blob }) => {
          const blobUrl = URL.createObjectURL(blob)
          photoBlobUrlsRef.current.push(blobUrl)
          const img = document.createElement('img')
          img.src = blobUrl
          img.style.cssText = 'width:100%;height:100%;object-fit:cover;'
          imgBox.innerHTML = ''
          imgBox.appendChild(img)
        })
        .catch(() => {})
    }
  }, [positionedPhotos, mapLoaded, showPhotos]) // eslint-disable-line react-hooks/exhaustive-deps

  // Chart brush selection → grey out full track, highlight selected segment, fit bounds
  useEffect(() => {
    const map = mapRef.current
    if (!map || !hasTrack) return

    const applyZoom = () => {
      if (zoomRange) {
        // Use exact stream indices from the chart's X-axis lookup.
        // This is accurate regardless of GPS point density distribution.
        const segCoords = streams
          .slice(zoomRange.startIdx, zoomRange.endIdx + 1)
          .filter((s): s is StreamPoint & { lat: number; lng: number } =>
            s.lat != null && s.lng != null)
          .map(s => [s.lng, s.lat] as [number, number])

        // Grey out the full track
        map.setPaintProperty('track-line', 'line-color', '#999999')
        map.setPaintProperty('track-line', 'line-opacity', 0.3)

        // Update and show the highlight segment
        ;(map.getSource('highlight') as maplibregl.GeoJSONSource | undefined)?.setData({
          type: 'Feature',
          properties: {},
          geometry: { type: 'LineString', coordinates: segCoords },
        })
        map.setLayoutProperty('highlight-line', 'visibility', 'visible')

        // Fit the map to the highlighted segment
        if (segCoords.length >= 2) {
          const bounds = new maplibregl.LngLatBounds()
          for (const c of segCoords) bounds.extend(c)
          map.fitBounds(bounds, { padding: 50, maxZoom: 18, duration: 600 })
        }
      } else {
        // Restore full track
        map.setPaintProperty('track-line', 'line-color', sportColor)
        map.setPaintProperty('track-line', 'line-opacity', 0.85)
        map.setLayoutProperty('highlight-line', 'visibility', 'none')
        ;(map.getSource('highlight') as maplibregl.GeoJSONSource | undefined)?.setData({
          type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: [] },
        })

        const bounds = new maplibregl.LngLatBounds()
        for (const c of coordinates) bounds.extend(c)
        map.fitBounds(bounds, { padding: 40, maxZoom: 15, duration: 600 })
      }
    }

    if (map.isStyleLoaded()) {
      applyZoom()
    } else {
      map.once('load', applyZoom)
    }
  }, [zoomRange]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!hasGps) {
    if (locationPin) {
      return (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden h-48 relative">
          <div ref={locationPinMapContainerRef} className="w-full h-full" />
          <div className="absolute top-2 left-2 flex items-center gap-1.5 bg-white/90 dark:bg-neutral-900/90 rounded-full px-2.5 py-1 text-xs font-medium shadow-sm pointer-events-none">
            <svg className="w-3 h-3 shrink-0" style={{ color: locationPin.color }} viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
            </svg>
            <span className="text-neutral-700 dark:text-neutral-300">{locationPin.name}</span>
          </div>
        </div>
      )
    }
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 flex items-center justify-center h-48">
        <span className="text-sm text-neutral-400">Keine GPS-Daten</span>
      </div>
    )
  }

  if (!hasTrack && streams.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 flex items-center justify-center h-48">
        <span className="text-sm text-neutral-400">Lade Kartendaten...</span>
      </div>
    )
  }

  if (!hasTrack) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 flex items-center justify-center h-48">
        <span className="text-sm text-neutral-400">Keine GPS-Daten</span>
      </div>
    )
  }

  const btnBase = 'flex items-center justify-center w-8 h-8 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md cursor-pointer transition-colors'
  const btnActive = 'text-neutral-600 dark:text-neutral-300 hover:bg-white dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-neutral-100'
  const btnDimmed = 'text-neutral-400 dark:text-neutral-600 hover:bg-white dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-neutral-100'

  return (
    <div className="relative rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={mapContainerRef} className="h-[400px] w-full" />
      <div className="absolute top-3 left-3 z-10 flex gap-2">
        {/* Maximize */}
        <button
          onClick={onMaximize}
          className={`${btnBase} ${btnActive}`}
          title="Karte maximieren"
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
            <path d="M13.28 7.78l3.22-3.22v2.69a.75.75 0 001.5 0v-4.5a.75.75 0 00-.75-.75h-4.5a.75.75 0 000 1.5h2.69l-3.22 3.22a.75.75 0 001.06 1.06zM2 17.25v-4.5a.75.75 0 011.5 0v2.69l3.22-3.22a.75.75 0 011.06 1.06L4.56 16.5h2.69a.75.75 0 010 1.5h-4.5a.75.75 0 01-.75-.75z" />
          </svg>
        </button>
        {/* Photo toggle — only shown when activity has positioned photos */}
        {positionedPhotos.length > 0 && (
          <button
            onClick={() => setShowPhotos(p => !p)}
            className={`${btnBase} ${showPhotos ? btnActive : btnDimmed}`}
            title={showPhotos ? 'Fotos ausblenden' : 'Fotos einblenden'}
          >
            <svg viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
              <path fillRule="evenodd" d="M1 8a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 018.07 3h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0016.07 6H17a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V8zm13.5 3a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0zM10 14a3 3 0 100-6 3 3 0 000 6z" clipRule="evenodd" />
            </svg>
          </button>
        )}
      </div>
    </div>
  )
}
