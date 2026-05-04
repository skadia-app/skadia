import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { useRef, useState, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { listActivityPhotos, type ActivityPhoto } from '@/api/activityPhotos'
import { apiDownload } from '@/api/client'
import type { StreamPoint, GeoTag, ActivityStop } from '@/api/types'
import type { CoordRange } from '@/api/routes'
import type { ZoomRange } from '@/components/ActivityCharts'
import { SURFACE_GROUPS } from '@/lib/surfaceConstants'
import { RESAMPLE_SPACING_M } from '@/lib/trackSummaryConstants'
import { SAC_COLORS, MTB_COLORS, VF_COLORS, VF_UNKNOWN_COLOR } from '@/lib/scaleColors'
import { Mountain } from 'lucide-react'
import { useMetricColoring } from './useMetricColoring'
import MapColorDropdown from './MapColorDropdown'
import MapColorLegend from './MapColorLegend'
import ScaleLegend from '@/components/shared/scale-legend/ScaleLegend'

export type SurfaceHighlightKey = {
  kind: 'highway' | 'surface'
  key: string
  /** Raw OSM types that overflowed into the '__other__' bucket — used to union their ranges. */
  overflowRawTypes?: string[]
} | null

export type HighlightedPoiCategory = 'summit' | 'pass' | 'still_water' | 'flowing_water' | 'settlement'
export type HighlightedPoiHoverKind = 'summit' | 'water' | 'settlement'

export interface HighlightedPoi {
  id: string
  category: HighlightedPoiCategory
  name: string
  lat: number
  lng: number
  trackPos: number | null
  distM: number | null
  eleM: number | null
}

interface TrackMapProps {
  activityId?: string
  streams: StreamPoint[]
  streamsLoading?: boolean
  hoverIndex: number | null
  sportColor: string
  sportType: string
  fitSportId?: number
  hasGps: boolean
  zoomRange: ZoomRange | null
  onMaximize: () => void
  geoTags?: GeoTag[]
  stops?: ActivityStop[]
  highlightedPois?: HighlightedPoi[]
  highlightedPoiHoverKind?: HighlightedPoiHoverKind | null
  /** Active hover/pin from SurfacesSection — drives the map line-highlight overlay. */
  surfaceHighlight?: SurfaceHighlightKey
  /** Map raw OSM surface type → coord-index ranges into the resampled track. */
  surfaceRanges?: Record<string, CoordRange[]> | null
  /** Map raw OSM highway type → coord-index ranges into the resampled track. */
  highwayRanges?: Record<string, CoordRange[]> | null
  locationPin?: { lat: number; lng: number; name: string; color: string }
  /**
   * Resampling interval in metres from the API response (resample_spacing_m field).
   * Falls back to the local RESAMPLE_SPACING_M constant when absent (older backends).
   * Must match the interval used when building surface_ranges / highway_ranges.
   */
  resampleSpacingM?: number | null
  /**
   * Active scale kind from ScaleBadge hover/pin (sac | mtb | vf).
   * When set, all ranges for that scale are rendered in rainbow colours + ScaleLegend.
   */
  scaleHighlight?: 'sac' | 'mtb' | 'vf' | null
  /** Ranges for the active scale kind (sac_ranges / mtb_ranges / vf_ranges from API). */
  scaleRanges?: Record<string, CoordRange[]> | null
  /** Whether vf_unknown_ranges exist — passed to ScaleLegend for K? indicator. */
  hasUnknownVf?: boolean
}

const SUMMARY_SUMMIT_SOURCE_ID = 'activity-track-summary-summits-source'
const SUMMARY_SUMMIT_LAYER_ID = 'activity-track-summary-summits-layer'
const SUMMARY_POI_SOURCE_ID = 'activity-track-summary-pois-source'
const SUMMARY_POI_LAYER_ID = 'activity-track-summary-pois-layer'
const SURFACE_HIGHLIGHT_SOURCE_ID = 'activity-track-summary-surface-highlight-source'
const SURFACE_HIGHLIGHT_HALO_LAYER_ID = 'activity-track-summary-surface-highlight-halo'
const SURFACE_HIGHLIGHT_LINE_LAYER_ID = 'activity-track-summary-surface-highlight-line'

// Scale (SAC/MTB/VF) rainbow highlight — separate source so surface + scale
// highlights can coexist without interference.
const SCALE_HIGHLIGHT_SOURCE_ID = 'activity-scale-highlight-source'
const SCALE_HIGHLIGHT_HALO_LAYER_ID = 'activity-scale-highlight-halo'
const SCALE_HIGHLIGHT_LINE_LAYER_ID = 'activity-scale-highlight-line'

// Per-level color lookup for rainbow rendering.
const SCALE_LEVEL_COLORS: Record<'sac' | 'mtb' | 'vf', Record<string, string>> = {
  sac: SAC_COLORS as Record<string, string>,
  mtb: MTB_COLORS as Record<string, string>,
  vf: VF_COLORS as Record<string, string>,
}

function highlightedPoiKind(category: HighlightedPoiCategory): HighlightedPoiHoverKind {
  if (category === 'summit' || category === 'pass') return 'summit'
  if (category === 'still_water' || category === 'flowing_water') return 'water'
  return 'settlement'
}

function highlightedPoiCategoryLabel(category: HighlightedPoiCategory): string {
  switch (category) {
    case 'summit': return 'Gipfel'
    case 'pass': return 'Pass'
    case 'still_water': return 'Gewässer'
    case 'flowing_water': return 'Bach/Fluss'
    case 'settlement': return 'Ort'
  }
}

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

export default function TrackMap({
  activityId,
  streams,
  streamsLoading,
  hoverIndex,
  sportColor,
  sportType,
  fitSportId,
  hasGps,
  zoomRange,
  onMaximize,
  geoTags,
  stops,
  highlightedPois = [],
  highlightedPoiHoverKind = null,
  surfaceHighlight = null,
  surfaceRanges = null,
  highwayRanges = null,
  locationPin,
  resampleSpacingM,
  scaleHighlight = null,
  scaleRanges = null,
  hasUnknownVf = false,
}: TrackMapProps) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const locationPinMapContainerRef = useRef<HTMLDivElement>(null)
  const locationPinMapRef = useRef<maplibregl.Map | null>(null)
  const photoMarkersRef = useRef<maplibregl.Marker[]>([])
  const photoBlobUrlsRef = useRef<string[]>([])

  const [showPhotos, setShowPhotos] = useState(true)
  const [showSummits, setShowSummits] = useState(true)
  const [mapLoaded, setMapLoaded] = useState(false)
  const [mapInstance, setMapInstance] = useState<maplibregl.Map | null>(null)
  const hasSummarySummitPois = highlightedPois.some(p => p.category === 'summit' || p.category === 'pass')

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

  // Build coordinate list for bounds computation — memoized to avoid recomputation on every hover render
  const { coordinates, hasTrack } = useMemo(() => {
    const coordinates = streams
      .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
      .map(s => [s.lng, s.lat] as [number, number])
    const hasTrack = coordinates.length >= 2
    return { coordinates, hasTrack }
  }, [streams])

  // Resampled coordinates at ~10 m spacing — mirrors the backend's
  // `resampleStreamPoints` (services/api/internal/enrichment/surfaces_graph.go).
  // CoordRange.from / .to from `surface_ranges` / `highway_ranges` index into
  // this array, so we can slice it directly to build highlight LineStrings.
  const resampledCoords = useMemo<[number, number][]>(() => {
    if (coordinates.length < 2) return []
    // Use interval from API response; fall back to local constant for older backends
    // that don't yet include resample_spacing_m in the track-summary response.
    const SPACING_M = resampleSpacingM ?? RESAMPLE_SPACING_M
    // Cumulative distance via equirectangular approximation.
    // Uses per-segment cos(lat) at the *start* of each segment, identical to
    // the backend equirectDistMeters(lat1,lng1,lat2,lng2) in surfaces_graph.go.
    // Using a single cos(lat0) for the whole track drifts by cos(lat1)/cos(lat0)
    // on long N–S routes; per-segment evaluation eliminates that bias.
    const M_PER_DEG_LAT = 111320 // ≈ geo.MetersPerDegreeLat
    const cum = new Array<number>(coordinates.length)
    cum[0] = 0
    for (let i = 1; i < coordinates.length; i++) {
      const lat1Rad = (coordinates[i - 1][1] * Math.PI) / 180
      const cosLat = Math.cos(lat1Rad) // cos of segment start lat — matches backend
      const dLat = (coordinates[i][1] - coordinates[i - 1][1]) * M_PER_DEG_LAT
      const dLng = (coordinates[i][0] - coordinates[i - 1][0]) * cosLat * M_PER_DEG_LAT
      cum[i] = cum[i - 1] + Math.sqrt(dLat * dLat + dLng * dLng)
    }
    const total = cum[cum.length - 1]
    if (total < 1e-6) return []
    const numPts = Math.max(2, Math.floor(total / SPACING_M) + 1)
    const out: [number, number][] = new Array(numPts)
    let j = 0
    for (let i = 0; i < numPts; i++) {
      const target = (i * total) / (numPts - 1)
      if (target >= total) {
        out[i] = coordinates[coordinates.length - 1]
        continue
      }
      while (j < coordinates.length - 2 && cum[j + 1] < target) j++
      const segLen = cum[j + 1] - cum[j]
      const t = segLen > 1e-10 ? (target - cum[j]) / segLen : 0
      out[i] = [
        coordinates[j][0] + t * (coordinates[j + 1][0] - coordinates[j][0]),
        coordinates[j][1] + t * (coordinates[j + 1][1] - coordinates[j][1]),
      ]
    }
    return out
  }, [coordinates, resampleSpacingM])

  const metricColoring = useMetricColoring({
    streams,
    activity: { sport_type: sportType, fit_sport_id: fitSportId },
    hasGps,
    zoomRange,
    map: mapInstance,
  })

  // Initialize map
  useEffect(() => {
    if (!mapContainerRef.current || !hasTrack) return

    // Previous map instance was destroyed in cleanup — any markerRef pointing to it is invalid
    markerRef.current = null

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

      // Full track — populated by useMetricColoring hook (FeatureCollection with per-segment features)
      map.addSource('track', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
      map.addLayer({
        id: 'track-line',
        type: 'line',
        source: 'track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': sportColor,   // overridden by hook immediately
          'line-width': 3,
          'line-opacity': 0.85,       // overridden by hook
        },
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
    setMapInstance(map)
    return () => {
      photoMarkersRef.current.forEach(m => m.remove())
      photoMarkersRef.current = []
      photoBlobUrlsRef.current.forEach(u => URL.revokeObjectURL(u))
      photoBlobUrlsRef.current = []
      if (map.getLayer('heading-arrows')) map.removeLayer('heading-arrows')
      if (map.getSource('heading-arrows')) map.removeSource('heading-arrows')
      setMapInstance(null)
      map.remove()
      mapRef.current = null
    }
  }, [hasTrack]) // eslint-disable-line react-hooks/exhaustive-deps

  // Track-summary POIs use route-planner-style GeoJSON layers instead of DOM markers:
  // cheap to update for hover and visually quieter than photo thumbnails.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !mapLoaded || !hasTrack) return

    if (!map.getSource(SUMMARY_SUMMIT_SOURCE_ID)) {
      map.addSource(SUMMARY_SUMMIT_SOURCE_ID, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }
    if (!map.hasImage('summary-summit-icon')) {
      const sz = 24
      const cvs = document.createElement('canvas')
      cvs.width = sz
      cvs.height = sz
      const ctx = cvs.getContext('2d')
      if (ctx) {
        ctx.beginPath()
        ctx.moveTo(sz / 2, 3)
        ctx.lineTo(3, sz - 3)
        ctx.lineTo(sz - 3, sz - 3)
        ctx.closePath()
        ctx.fillStyle = '#475569'
        ctx.fill()
        ctx.strokeStyle = '#ffffff'
        ctx.lineWidth = 2
        ctx.lineJoin = 'round'
        ctx.stroke()
        ctx.beginPath()
        ctx.moveTo(sz / 2, 3)
        ctx.lineTo(sz / 2 - 4, 11)
        ctx.lineTo(sz / 2 + 4, 11)
        ctx.closePath()
        ctx.fillStyle = '#f8fafc'
        ctx.fill()
        map.addImage('summary-summit-icon', ctx.getImageData(0, 0, sz, sz))
      }
    }
    if (!map.getLayer(SUMMARY_SUMMIT_LAYER_ID) && map.hasImage('summary-summit-icon')) {
      map.addLayer({
        id: SUMMARY_SUMMIT_LAYER_ID,
        type: 'symbol',
        source: SUMMARY_SUMMIT_SOURCE_ID,
        layout: {
          'icon-image': 'summary-summit-icon',
          // Stronger active/dimmed contrast than the quiet POI layer below — pulls
          // the eye to summits/passes when the user hovers the matching pill.
          'icon-size': ['case', ['boolean', ['get', 'active'], false], 1.2, 0.6],
          'icon-allow-overlap': true,
          'icon-ignore-placement': true,
        },
        paint: {
          'icon-opacity': ['case', ['boolean', ['get', 'dimmed'], false], 0.25, 0.9],
        },
      })
    }

    if (!map.getSource(SUMMARY_POI_SOURCE_ID)) {
      map.addSource(SUMMARY_POI_SOURCE_ID, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }
    if (!map.getLayer(SUMMARY_POI_LAYER_ID)) {
      // Water/settlement markers stay visually static — only the popover list
      // reacts to hover. User decision: keep map quiet, only summits pop.
      map.addLayer({
        id: SUMMARY_POI_LAYER_ID,
        type: 'circle',
        source: SUMMARY_POI_SOURCE_ID,
        paint: {
          'circle-radius': 4,
          'circle-color': [
            'match',
            ['get', 'category'],
            'still_water', '#0ea5e9',
            'flowing_water', '#38bdf8',
            'settlement', '#64748b',
            '#64748b',
          ],
          'circle-opacity': 0.68,
          'circle-stroke-color': '#ffffff',
          'circle-stroke-width': 1.5,
        },
      })
    }

    // Surface/highway hover highlight — yellow line over the track. Inserted
    // BEFORE the summit/POI layers so the markers stay on top.
    if (!map.getSource(SURFACE_HIGHLIGHT_SOURCE_ID)) {
      map.addSource(SURFACE_HIGHLIGHT_SOURCE_ID, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }
    if (!map.getLayer(SURFACE_HIGHLIGHT_HALO_LAYER_ID)) {
      map.addLayer(
        {
          id: SURFACE_HIGHLIGHT_HALO_LAYER_ID,
          type: 'line',
          source: SURFACE_HIGHLIGHT_SOURCE_ID,
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: { 'line-color': '#ffffff', 'line-width': 9, 'line-opacity': 0.7 },
        },
        SUMMARY_SUMMIT_LAYER_ID,
      )
    }
    if (!map.getLayer(SURFACE_HIGHLIGHT_LINE_LAYER_ID)) {
      map.addLayer(
        {
          id: SURFACE_HIGHLIGHT_LINE_LAYER_ID,
          type: 'line',
          source: SURFACE_HIGHLIGHT_SOURCE_ID,
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: { 'line-color': '#FACC15', 'line-width': 5, 'line-opacity': 0.95 },
        },
        SUMMARY_SUMMIT_LAYER_ID,
      )
    }

    // Scale (SAC/MTB/VF) rainbow highlight — per-feature color via 'line-color' data expression.
    if (!map.getSource(SCALE_HIGHLIGHT_SOURCE_ID)) {
      map.addSource(SCALE_HIGHLIGHT_SOURCE_ID, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
    }
    if (!map.getLayer(SCALE_HIGHLIGHT_HALO_LAYER_ID)) {
      map.addLayer(
        {
          id: SCALE_HIGHLIGHT_HALO_LAYER_ID,
          type: 'line',
          source: SCALE_HIGHLIGHT_SOURCE_ID,
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: { 'line-color': '#ffffff', 'line-width': 9, 'line-opacity': 0.55 },
        },
        SUMMARY_SUMMIT_LAYER_ID,
      )
    }
    if (!map.getLayer(SCALE_HIGHLIGHT_LINE_LAYER_ID)) {
      map.addLayer(
        {
          id: SCALE_HIGHLIGHT_LINE_LAYER_ID,
          type: 'line',
          source: SCALE_HIGHLIGHT_SOURCE_ID,
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          // Color comes from per-feature 'color' property set in the update effect below.
          paint: {
            'line-color': ['coalesce', ['get', 'color'], '#FACC15'],
            'line-width': 5,
            'line-opacity': 0.95,
          },
        },
        SUMMARY_SUMMIT_LAYER_ID,
      )
    }

    const summitFeatures = highlightedPois
      .filter(p => p.category === 'summit' || p.category === 'pass')
      .map(p => ({
        type: 'Feature' as const,
        geometry: { type: 'Point' as const, coordinates: [p.lng, p.lat] },
        properties: {
          ...p,
          kind: highlightedPoiKind(p.category),
          label: highlightedPoiCategoryLabel(p.category),
          active: highlightedPoiHoverKind === highlightedPoiKind(p.category),
          dimmed: highlightedPoiHoverKind != null && highlightedPoiHoverKind !== highlightedPoiKind(p.category),
        },
      }))
    // Water features carry no active/dimmed flags — paint is static.
    // Settlements (`category: 'settlement'`) are intentionally dropped from the
    // map: they're areas, not points, so a single dot at the geocoded centre
    // is misleading. The settlement count still surfaces via the StatsRow
    // pill / popover list.
    const quietPoiFeatures = highlightedPois
      .filter(p => p.category !== 'summit' && p.category !== 'pass' && p.category !== 'settlement')
      .map(p => ({
        type: 'Feature' as const,
        geometry: { type: 'Point' as const, coordinates: [p.lng, p.lat] },
        properties: {
          ...p,
          kind: highlightedPoiKind(p.category),
          label: highlightedPoiCategoryLabel(p.category),
        },
      }))

    ;(map.getSource(SUMMARY_SUMMIT_SOURCE_ID) as maplibregl.GeoJSONSource | undefined)?.setData({
      type: 'FeatureCollection',
      features: summitFeatures,
    })
    ;(map.getSource(SUMMARY_POI_SOURCE_ID) as maplibregl.GeoJSONSource | undefined)?.setData({
      type: 'FeatureCollection',
      features: quietPoiFeatures,
    })

    const popup = new maplibregl.Popup({ closeButton: false, closeOnClick: true, offset: 10 })
    const showPopup = (e: maplibregl.MapMouseEvent & { features?: maplibregl.MapGeoJSONFeature[] }) => {
      const props = e.features?.[0]?.properties
      if (!props) return
      const name = escapeHtml(String(props.name || props.label || 'POI'))
      const label = escapeHtml(String(props.label || 'POI'))
      const ele = props.eleM != null ? ` · ${Math.round(Number(props.eleM)).toLocaleString('de-DE')} m` : ''
      const dist = props.distM != null ? ` · ${Math.round(Number(props.distM)).toLocaleString('de-DE')} m` : ''
      popup
        .setLngLat(e.lngLat)
        .setHTML(`<span style="font-size:12px;font-weight:600">${name}</span><br><span style="font-size:11px;color:#6b7280">${label}${ele}${dist}</span>`)
        .addTo(map)
    }
    const setPointer = () => { map.getCanvas().style.cursor = 'pointer' }
    const clearPointer = () => { map.getCanvas().style.cursor = '' }

    map.on('click', SUMMARY_SUMMIT_LAYER_ID, showPopup)
    map.on('click', SUMMARY_POI_LAYER_ID, showPopup)
    map.on('mouseenter', SUMMARY_SUMMIT_LAYER_ID, setPointer)
    map.on('mouseenter', SUMMARY_POI_LAYER_ID, setPointer)
    map.on('mouseleave', SUMMARY_SUMMIT_LAYER_ID, clearPointer)
    map.on('mouseleave', SUMMARY_POI_LAYER_ID, clearPointer)

    return () => {
      popup.remove()
      if (!map.getStyle()) return
      map.off('click', SUMMARY_SUMMIT_LAYER_ID, showPopup)
      map.off('click', SUMMARY_POI_LAYER_ID, showPopup)
      map.off('mouseenter', SUMMARY_SUMMIT_LAYER_ID, setPointer)
      map.off('mouseenter', SUMMARY_POI_LAYER_ID, setPointer)
      map.off('mouseleave', SUMMARY_SUMMIT_LAYER_ID, clearPointer)
      map.off('mouseleave', SUMMARY_POI_LAYER_ID, clearPointer)
    }
  }, [hasTrack, hasSummarySummitPois, highlightedPoiHoverKind, highlightedPois, mapLoaded])

  // Surface/highway highlight overlay — yellow line over the matching parts
  // of the resampled track. Mirrors the route-planner highlight semantics
  // (`useRouteEditor.ts`): for surfaces, we union all raw OSM types whose
  // SURFACE_GROUPS bucket matches the active key; for highways, exact match
  // on the raw key.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !mapLoaded || !hasTrack) return
    const src = map.getSource(SURFACE_HIGHLIGHT_SOURCE_ID) as maplibregl.GeoJSONSource | undefined
    if (!src) return

    const empty: GeoJSON.FeatureCollection<GeoJSON.LineString> = { type: 'FeatureCollection', features: [] }

    if (!surfaceHighlight || resampledCoords.length < 2) {
      src.setData(empty)
      return
    }

    let ranges: CoordRange[] = []
    if (surfaceHighlight.kind === 'highway') {
      if (surfaceHighlight.key === '__other__' && surfaceHighlight.overflowRawTypes) {
        // "Sonstige" highway bucket: union ranges of all overflow raw highway types.
        for (const rawType of surfaceHighlight.overflowRawTypes) {
          ranges = ranges.concat(highwayRanges?.[rawType] ?? [])
        }
      } else {
        ranges = (highwayRanges?.[surfaceHighlight.key]) ?? []
      }
    } else {
      // surface key is the GROUP label (e.g. "Naturweg") — union all raw
      // types whose SURFACE_GROUPS bucket matches.
      // For '__other__', overflowRawTypes carries the exact display-group names
      // that overflowed; look up each group's contributing raw OSM types.
      const map_ = surfaceRanges ?? {}
      if (surfaceHighlight.key === '__other__' && surfaceHighlight.overflowRawTypes) {
        const overflowGroups = new Set(surfaceHighlight.overflowRawTypes)
        for (const rawType of Object.keys(map_)) {
          const group = SURFACE_GROUPS[rawType] ?? rawType
          if (overflowGroups.has(group)) ranges = ranges.concat(map_[rawType] ?? [])
        }
      } else {
        for (const rawType of Object.keys(map_)) {
          const group = SURFACE_GROUPS[rawType] ?? rawType
          if (group === surfaceHighlight.key) ranges = ranges.concat(map_[rawType] ?? [])
        }
      }
    }

    if (ranges.length === 0) {
      src.setData(empty)
      return
    }

    const features: GeoJSON.Feature<GeoJSON.LineString>[] = []
    const last = resampledCoords.length - 1
    for (const r of ranges) {
      const start = Math.max(0, Math.min(last, r.from))
      const end = Math.max(0, Math.min(last, r.to))
      if (end - start < 1) continue
      // CoordRange.to is exclusive in spec, but slicing with end+1 over-shoots
      // by one; coords are dense so the visual delta is sub-pixel either way.
      // Matching route-planner: we use slice(start, end+1) to keep the line closed.
      const slice = resampledCoords.slice(start, end + 1)
      if (slice.length < 2) continue
      features.push({
        type: 'Feature',
        properties: { kind: surfaceHighlight.kind, key: surfaceHighlight.key },
        geometry: { type: 'LineString', coordinates: slice },
      })
    }
    src.setData({ type: 'FeatureCollection', features })
  }, [mapLoaded, hasTrack, surfaceHighlight, surfaceRanges, highwayRanges, resampledCoords])

  // Scale (SAC/MTB/VF) rainbow highlight — renders ALL levels of the active scale
  // simultaneously, each in its own rainbow colour. Mirrors route-planner behaviour:
  // "Hover/Klick auf SAC-Pille = ALLE SAC-Levels in jeweiliger Farbe gleichzeitig."
  useEffect(() => {
    const map = mapRef.current
    if (!map || !mapLoaded || !hasTrack) return
    const src = map.getSource(SCALE_HIGHLIGHT_SOURCE_ID) as maplibregl.GeoJSONSource | undefined
    if (!src) return

    const empty: GeoJSON.FeatureCollection<GeoJSON.LineString> = { type: 'FeatureCollection', features: [] }

    if (!scaleHighlight || !scaleRanges || resampledCoords.length < 2) {
      src.setData(empty)
      return
    }

    const colorMap = SCALE_LEVEL_COLORS[scaleHighlight]
    const features: GeoJSON.Feature<GeoJSON.LineString>[] = []
    const last = resampledCoords.length - 1

    for (const [level, levelRanges] of Object.entries(scaleRanges)) {
      const color = colorMap[level] ?? (scaleHighlight === 'vf' ? VF_UNKNOWN_COLOR : '#FACC15')
      for (const r of levelRanges) {
        const start = Math.max(0, Math.min(last, r.from))
        const end = Math.max(0, Math.min(last, r.to))
        if (end - start < 1) continue
        const slice = resampledCoords.slice(start, end + 1)
        if (slice.length < 2) continue
        features.push({
          type: 'Feature',
          properties: { level, color },
          geometry: { type: 'LineString', coordinates: slice },
        })
      }
    }

    src.setData({ type: 'FeatureCollection', features })
  }, [mapLoaded, hasTrack, scaleHighlight, scaleRanges, resampledCoords])

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

  // Summit layer visibility — toggleable via toolbar button.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !mapLoaded) return
    const visibility = showSummits ? 'visible' : 'none'
    if (map.getLayer(SUMMARY_SUMMIT_LAYER_ID)) {
      map.setLayoutProperty(SUMMARY_SUMMIT_LAYER_ID, 'visibility', visibility)
    }
  }, [showSummits, mapLoaded])

  // Chart brush → highlight via fitBounds (programmaticFit) — line-opacity modulation is handled by useMetricColoring
  useEffect(() => {
    const map = mapRef.current
    if (!map || !hasTrack) return

    const apply = () => {
      if (zoomRange) {
        const segCoords = streams
          .slice(zoomRange.startIdx, zoomRange.endIdx + 1)
          .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
          .map(s => [s.lng, s.lat] as [number, number])

        if (segCoords.length >= 2) {
          const bounds = new maplibregl.LngLatBounds()
          for (const c of segCoords) bounds.extend(c)
          metricColoring.programmaticFit(bounds)
        }
      } else {
        const bounds = new maplibregl.LngLatBounds()
        for (const c of coordinates) bounds.extend(c)
        if (coordinates.length >= 2) {
          metricColoring.programmaticFit(bounds)
        }
      }
    }

    if (map.isStyleLoaded()) {
      apply()
    } else {
      map.once('load', apply)
    }
    // NOTE: line-color and line-opacity are handled by useMetricColoring via the `inside` feature property.
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

  if (!hasTrack) {
    if (streamsLoading !== false) {
      return (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 flex items-center justify-center h-48">
          <span className="text-sm text-neutral-400">Lade Kartendaten...</span>
        </div>
      )
    }
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 flex flex-col items-center justify-center h-48 gap-1">
        <span className="text-sm text-neutral-400">GPS-Daten nicht verfügbar</span>
        <span className="text-xs text-neutral-400">„Neu berechnen" kann helfen</span>
      </div>
    )
  }

  const btnBase = 'flex items-center justify-center w-8 h-8 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md cursor-pointer transition-colors'
  const btnActive = 'text-neutral-600 dark:text-neutral-300 hover:bg-white dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-neutral-100'
  const btnDimmed = 'text-neutral-400 dark:text-neutral-600 hover:bg-white dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-neutral-100'

  return (
    <div className="relative rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={mapContainerRef} className="h-[400px] w-full" />

      {/* Toolbar top-left */}
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

        {/* Summit toggle — only shown when track-summary surfaced any summits/passes. */}
        {hasSummarySummitPois && (
          <button
            onClick={() => setShowSummits(p => !p)}
            className={`${btnBase} ${showSummits ? btnActive : btnDimmed}`}
            title={showSummits ? 'Gipfel ausblenden' : 'Gipfel einblenden'}
          >
            <Mountain className="w-4 h-4" aria-hidden="true" />
          </button>
        )}

        {/* Metric colorization dropdown */}
        {hasGps && (
          <MapColorDropdown
            streams={streams}
            sportType={sportType}
            metric={metricColoring.metric}
            onChange={metricColoring.setMetric}
            modeByMetric={metricColoring.modeByMetric}
            setMode={metricColoring.setMode}
            zonesAvailable={metricColoring.zonesAvailable}
            effectiveMode={metricColoring.effectiveMode}
          />
        )}
      </div>

      {/* Legend top-right (below NavigationControl) */}
      {hasGps && <MapColorLegend scale={metricColoring.scale} />}

      {/* Scale legend — shown when a scale pin is active. Positioned bottom-right
          (matching route-planner convention from ScaleLegend.tsx). */}
      <ScaleLegend
        kind={scaleHighlight ?? null}
        hasUnknownVf={hasUnknownVf}
      />
    </div>
  )
}
