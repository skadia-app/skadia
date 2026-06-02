import { useRef, useEffect, useMemo, useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMetricColoring } from './activity/useMetricColoring'
import MapColorDropdown from './activity/MapColorDropdown'
import MapColorLegend from './activity/MapColorLegend'
import { buildTrackMapFitTarget } from './activity/trackMapZoom'
import { useEscapeKey } from '@/hooks/useEscapeKey'
import maplibregl, { type RequestParameters } from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { getToken } from '@/lib/auth'
import { useQuery } from '@tanstack/react-query'
import { listActivityPhotos, type ActivityPhoto } from '@/api/activityPhotos'
import { apiDownload } from '@/api/client'
import * as echarts from 'echarts/core'
import { useECharts } from '@/hooks/useECharts'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
  BrushComponent,
  ToolboxComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { LngLatLike } from 'maplibre-gl'
import type { StreamPoint, GeoTag } from '@/api/types'
import { Mountain } from 'lucide-react'
import ColorPicker from '@/components/ColorPicker'
import { buildLayerStyle } from '@/lib/mapStyles'
import { useMapLayers } from '@/hooks/useMapLayers'
import { useCapabilities } from '@/hooks/useCapabilities'
import { apiClient } from '@/api/client'
import { addHeatmapLayers, removeHeatmapLayers, buildHeatmapTileUrl, loadHeatmapColor, saveHeatmapColor } from '@/lib/mapHeatmap'
import { findClosestIdx } from '@/lib/utils'
import type { ZoomRange } from '@/components/ActivityCharts'
import { motionProfileForSport, type MotionProfile } from '@/lib/colorScaleRanges'
import {
  distanceInputUnit,
  elevationInputUnit,
  elevationToDisplayValue,
  speedDisplayUnit,
  speedToDisplayValue,
  temperatureDisplayUnit,
  temperatureToDisplayValue,
} from '@/lib/units'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  AxisPointerComponent,
  BrushComponent,
  ToolboxComponent,
  LegendComponent,
  CanvasRenderer,
])

// ── Types ─────────────────────────────────────────────────────────────────────

type XMode = 'distance' | 'time'

type MetricFieldKey = 'altitude' | 'heartRate' | 'power' | 'cadence' | 'speed' | 'temperature'

type ChartMetrics = Record<MetricFieldKey, number[]>

interface ChartXAxis {
  distanceKm: number[]
  elapsedMin: number[]
  maxDist: number
  maxElapsed: number
}

type ChartData = ChartMetrics & ChartXAxis

interface StreamDef {
  key: MetricFieldKey
  label: string
  unit: string
  color: string
  yAxisIndex: number
}

interface Props {
  streams: StreamPoint[]
  sportColor: string
  sportType: string
  subType?: string | null
  fitSportId?: number
  hasGps: boolean
  onClose: () => void
  activityId?: string
  /** Geo tags for summit/pass markers — same source as TrackMap. */
  geoTags?: GeoTag[]
}

// ── Stream definitions ────────────────────────────────────────────────────────

interface StreamLabels {
  cadenceFoot: string
  cadencePaddle: string
  cadenceDefault: string
  altitude: string
  heartRate: string
  power: string
  speed: string
  temperature: string
}

export function buildAllStreams(motionProfile: MotionProfile, labels: StreamLabels, units: { elevation: string; speed: string; temperature: string }): StreamDef[] {
  const cadenceLabel =
    motionProfile.cadence.startsWith('foot_') ? labels.cadenceFoot :
    motionProfile.cadence === 'paddle' ? labels.cadencePaddle :
    labels.cadenceDefault
  const cadenceUnit =
    motionProfile.cadence.startsWith('foot_') ? 'spm' :
    motionProfile.cadence === 'paddle' ? 'spm' :
    'rpm'

  return [
    { key: 'altitude', label: labels.altitude, unit: units.elevation, color: '#22c55e', yAxisIndex: 0 },
    { key: 'heartRate', label: labels.heartRate, unit: 'bpm', color: '#ef4444', yAxisIndex: 1 },
    { key: 'power', label: labels.power, unit: 'W', color: '#a855f7', yAxisIndex: 2 },
    { key: 'cadence', label: cadenceLabel, unit: cadenceUnit, color: '#f97316', yAxisIndex: 3 },
    { key: 'speed', label: labels.speed, unit: units.speed, color: '#3b82f6', yAxisIndex: 4 },
    { key: 'temperature', label: labels.temperature, unit: units.temperature, color: '#06b6d4', yAxisIndex: 5 },
  ]
}

// ── Data processing (mirrors ActivityCharts) ──────────────────────────────────

export function buildChartData(streams: StreamPoint[]): ChartData {
  const distanceKm: number[] = []
  const elapsedMin: number[] = []
  const altitude: number[] = []
  const heartRate: number[] = []
  const power: number[] = []
  const cadence: number[] = []
  const speed: number[] = []
  const temperature: number[] = []

  const t0 = streams.length > 0 ? new Date(streams[0].time).getTime() : 0

  // Dedup: consecutive samples with identical distance_m collapse. Last-non-NaN
  // wins per metric; all-NaN stays NaN so connectNulls:false keeps gaps.
  let lastDist: number | null = null
  for (const s of streams) {
    const dist: number = s.distance_m != null ? s.distance_m / 1000 : (lastDist ?? 0)
    if (lastDist !== null && dist === lastDist) {
      const idx = distanceKm.length - 1
      // Also advance elapsedMin so maxElapsed reflects the full recording duration
      // even for stationary recordings where all distance_m=0.
      elapsedMin[idx] = (new Date(s.time).getTime() - t0) / 60000
      if (s.altitude_m != null) altitude[idx] = s.altitude_m
      if (s.heart_rate != null) heartRate[idx] = s.heart_rate
      if (s.power_w != null) power[idx] = s.power_w
      if (s.cadence != null) cadence[idx] = s.cadence
      if (s.speed_ms != null) speed[idx] = s.speed_ms * 3.6
      if (s.temperature_c != null) temperature[idx] = s.temperature_c
      continue
    }
    distanceKm.push(dist)
    elapsedMin.push((new Date(s.time).getTime() - t0) / 60000)
    altitude.push(s.altitude_m ?? NaN)
    heartRate.push(s.heart_rate ?? NaN)
    power.push(s.power_w ?? NaN)
    cadence.push(s.cadence ?? NaN)
    speed.push(s.speed_ms != null ? s.speed_ms * 3.6 : NaN)
    temperature.push(s.temperature_c ?? NaN)
    lastDist = dist
  }

  const validDist = distanceKm.filter(v => !isNaN(v) && isFinite(v))
  const rawMax = validDist.length > 0 ? Math.max(...validDist) : 1
  const maxDist = Math.ceil(rawMax * 10) / 10
  const maxElapsed = elapsedMin.length > 0 ? Math.ceil(elapsedMin[elapsedMin.length - 1]) : 0

  return { distanceKm, elapsedMin, altitude, heartRate, power, cadence, speed, temperature, maxDist, maxElapsed }
}

function hasValidData(arr: number[]): boolean {
  return arr.some(v => !isNaN(v))
}

function formatElapsed(minutes: number, maxMinutes: number): string {
  if (maxMinutes < 60) return `${Math.round(minutes)}\u202fmin`
  const h = Math.floor(minutes / 60)
  const m = Math.round(minutes % 60)
  if (h === 0) return `${m}\u202fmin`
  return `${h}h${m.toString().padStart(2, '0')}`
}

// ── GPS gap filter for base-layer track ──────────────────────────────────────

const BASE_LAYER_GAP_THRESHOLD_M = 100

/**
 * Split a sequence of [lng, lat] points into segments at GPS teleport gaps.
 * A gap is detected when the distance_m delta between consecutive points
 * exceeds `threshold`. Returns an array of coordinate segments suitable for
 * a GeoJSON MultiLineString geometry.
 */
function splitTrackAtGaps(
  points: [number, number][],
  distancesM: number[],
  threshold = BASE_LAYER_GAP_THRESHOLD_M,
): [number, number][][] {
  const segments: [number, number][][] = []
  let current: [number, number][] = []
  for (let i = 0; i < points.length; i++) {
    if (i > 0 && Math.abs((distancesM[i] ?? 0) - (distancesM[i - 1] ?? 0)) > threshold) {
      if (current.length > 0) segments.push(current)
      current = []
    }
    current.push(points[i])
  }
  if (current.length > 0) segments.push(current)
  return segments
}

/**
 * Binary-search the nearest index in a sorted numeric array.
 * Used to map chart coordRange values back to stream indices for useMetricColoring.
 */
function findNearestIdx(arr: number[], value: number): number {
  let lo = 0, hi = arr.length - 1
  while (lo < hi) {
    const mid = (lo + hi) >>> 1
    if (arr[mid] < value) lo = mid + 1
    else hi = mid
  }
  return lo
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function MapFullscreen({ streams, sportColor, sportType, subType, fitSportId, hasGps, onClose, activityId, geoTags }: Props) {
  const { t } = useTranslation()
  const { options: unitOptions, formatDistance, formatElevation } = useUnitFormatters()
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const chartInstanceRef = useRef<echarts.ECharts | null>(null)
  const photoMarkersRef = useRef<maplibregl.Marker[]>([])
  const photoBlobUrlsRef = useRef<string[]>([])

  const [xMode, setXMode] = useState<XMode>('distance')
  const [isZoomed, setIsZoomed] = useState(false)
  const [zoomRange, setZoomRange] = useState<ZoomRange | null>(null)
  const [enabledStreams, setEnabledStreams] = useState<Set<MetricFieldKey>>(() => {
    // Preselect every MetricFieldKey; ECharts automatically hides series without data.
    return new Set<MetricFieldKey>(['altitude', 'heartRate', 'power', 'cadence', 'speed', 'temperature'])
  })

  const [showPhotos, setShowPhotos] = useState(true)
  const [showSummits, setShowSummits] = useState(true)
  const [mapLoaded, setMapLoaded] = useState(false)
  const [mapInstance, setMapInstance] = useState<maplibregl.Map | null>(null)

  const { base } = useMapLayers()
  const { data: caps } = useCapabilities()
  const [baseStyle, setBaseStyle] = useState<string>(() => {
    try {
      const d = JSON.parse(localStorage.getItem('skadia-map-defaults') ?? '{}') as Record<string, unknown>
      return (d.baseStyle as string) ?? 'osm'
    } catch { return 'osm' }
  })

  // Sync baseStyle to DB reality: if the persisted key is no longer in base
  // (admin disabled it), fall back to base[0].
  useEffect(() => {
    if (base.length > 0 && !base.find((l) => l.key === baseStyle)) {
      setBaseStyle(base[0].key)
    }
  }, [base, baseStyle])

  const selectedBaseLayer = base.find((l) => l.key === baseStyle) ?? base[0]

  // Refs kept current so the initMap closure (captured once) always sees
  // the latest layer/caps even when layers arrive after mount.
  const selectedBaseLayerRef = useRef(selectedBaseLayer)
  const capsRef = useRef(caps)
  useEffect(() => { selectedBaseLayerRef.current = selectedBaseLayer }, [selectedBaseLayer])
  useEffect(() => { capsRef.current = caps }, [caps])

  // initMap is defined inside the mount-time useEffect (line ~337) and bails
  // early when `selectedBaseLayer` or `caps` haven't loaded yet (both are async
  // queries). The ResizeObserver only re-fires on container size changes — so
  // without an explicit retry, late-arriving layer/caps data leaves the map
  // permanently uninitialized (black screen until the user resizes the window).
  // This ref exposes initMap so a separate effect can retry once the data is in.
  const initMapRef = useRef<(() => void) | null>(null)

  const [showHeatmap, setShowHeatmap] = useState(false)
  const [heatmapColor, setHeatmapColor] = useState<string>(loadHeatmapColor)
  const [styleVersion, setStyleVersion] = useState(0)
  const pendingCameraRestoreRef = useRef<{ center: LngLatLike; zoom: number } | null>(null)
  const prevBaseStyleRef = useRef<string>(baseStyle)
  const initialFitDoneRef = useRef(false)

  const { data: meData } = useQuery({
    queryKey: ['me'],
    queryFn: () => apiClient<{ user_id: string }>('/me'),
    staleTime: Infinity,
  })
  const userId = meData?.user_id ?? ''

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

  const data = useMemo(() => buildChartData(streams), [streams])

  // Auto-switch to time axis for stationary recordings (all distance_m=0):
  // buildChartData dedup collapses all zero-distance samples into a single bucket,
  // leaving distanceKm=[0] and maxDist=0 — a distance axis would show only 1 point.
  useEffect(() => {
    if (data.maxDist === 0) {
      setXMode('time')
    }
  }, [data.maxDist])

  // Filter to only streams with actual data
  const availableStreams = useMemo(() => {
    const motionProfile = motionProfileForSport(sportType, subType)
    const all = buildAllStreams(motionProfile, {
      cadenceFoot: t('components.mapFullscreen.metrics.cadenceFoot'),
      cadencePaddle: t('components.mapFullscreen.metrics.cadencePaddle'),
      cadenceDefault: t('components.mapFullscreen.metrics.cadenceDefault'),
      altitude: t('components.mapFullscreen.metrics.altitude'),
      heartRate: t('components.mapFullscreen.metrics.heartRate'),
      power: t('components.mapFullscreen.metrics.power'),
      speed: t('components.mapFullscreen.metrics.speed'),
      temperature: t('components.mapFullscreen.metrics.temperature'),
    }, {
      elevation: elevationInputUnit(unitOptions.units),
      speed: speedDisplayUnit(unitOptions.units),
      temperature: temperatureDisplayUnit(unitOptions.units),
    })
    return all.filter(s => hasValidData(data[s.key]))
  }, [data, sportType, subType, t, unitOptions.units])

  // Build coordinates for map
  const coordinates = useMemo(
    () => streams
      .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
      .map(s => [s.lng, s.lat] as [number, number]),
    [streams],
  )

  const metricColoring = useMetricColoring({
    streams,
    activity: { sport_type: sportType, sub_type: subType ?? null, fit_sport_id: fitSportId },
    hasGps,
    zoomRange,
    map: mapInstance,
    styleVersion,
  })

  useEscapeKey(onClose)

  // Lock body scroll
  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = prev }
  }, [])

  // ── Initialize map ────────────────────────────────────────────────────────
  // MapLibre needs a container with non-zero dimensions. In a flex-col layout
  // the container may have 0×0 at mount time. We use ResizeObserver to defer
  // initialization until the browser has computed the layout, then keep
  // observing for future size changes (e.g. window resize).
  useEffect(() => {
    const el = mapContainerRef.current
    if (!el || coordinates.length < 2) return

    let map: maplibregl.Map | null = null
    let disposed = false

    const addTrackLayers = (m: maplibregl.Map) => {
      if (m.getSource('track')) return // guard: don't double-add

      // Build initial base-layer track with GPS-gap filter (teleport lines suppressed).
      // useMetricColoring will replace this data once the metric/scale is resolved.
      const gpsStreams = streams.filter(
        (s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null,
      )
      const gpsPoints = gpsStreams.map(s => [s.lng, s.lat] as [number, number])
      const gpsDistancesM = gpsStreams.map(s => s.distance_m ?? 0)
      const segments = splitTrackAtGaps(gpsPoints, gpsDistancesM)
      const initialData: GeoJSON.FeatureCollection = {
        type: 'FeatureCollection',
        features: segments.map(seg => ({
          type: 'Feature',
          geometry: { type: 'LineString', coordinates: seg },
          properties: { inside: true, has_value: false },
        })),
      }

      m.addSource('track', {
        type: 'geojson',
        data: initialData,
      })
      m.addLayer({
        id: 'track-line',
        type: 'line',
        source: 'track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': sportColor, 'line-width': 3, 'line-opacity': 0.85 },
      })

      // Summit/pass markers from geoTags — gray triangle, label-less. Style
      // mirrors TrackMap's track-summary summit layer; no hover-grow here
      // because MapFullscreen has no driving pill list. Click on the icon
      // surfaces name + elevation via the existing popup handler below.
      const summitTags = (geoTags ?? []).filter(
        (t) => (t.category === 'summit' || t.category === 'pass') && t.lat != null,
      )
      if (summitTags.length > 0 && !m.getSource('fullscreen-summits')) {
        m.addSource('fullscreen-summits', {
          type: 'geojson',
          data: {
            type: 'FeatureCollection',
            features: summitTags.map((tag) => ({
              type: 'Feature' as const,
              geometry: { type: 'Point' as const, coordinates: [tag.lng!, tag.lat!] },
              properties: {
                name: tag.name ?? '',
                ele_m: tag.ele_m ?? tag.ele ?? null,
                category: tag.category,
              },
            })),
          },
        })
        const addSummitLayer = () => {
          if (m.getLayer('fullscreen-summits-layer')) return
          m.addLayer({
            id: 'fullscreen-summits-layer',
            type: 'symbol',
            source: 'fullscreen-summits',
            layout: {
              'icon-image': 'fullscreen-summit-icon',
              // Slightly larger than TrackMap's default (0.6) — fullscreen has
              // more screen real estate and benefits from more presence.
              'icon-size': 0.9,
              'icon-allow-overlap': true,
              'icon-ignore-placement': true,
            },
            paint: {
              'icon-opacity': 0.9,
            },
          })
        }
        if (m.hasImage('fullscreen-summit-icon')) {
          addSummitLayer()
        } else {
          const sz = 24
          const cvs = document.createElement('canvas')
          cvs.width = sz
          cvs.height = sz
          const ctx = cvs.getContext('2d')
          if (ctx) {
            // Outer gray triangle with white stroke
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
            // Inner white "snow cap" triangle
            ctx.beginPath()
            ctx.moveTo(sz / 2, 3)
            ctx.lineTo(sz / 2 - 4, 11)
            ctx.lineTo(sz / 2 + 4, 11)
            ctx.closePath()
            ctx.fillStyle = '#f8fafc'
            ctx.fill()
            m.addImage('fullscreen-summit-icon', ctx.getImageData(0, 0, sz, sz))
          }
          addSummitLayer()
        }

        // Click-popup with summit name + category + elevation — same HTML
        // shape as TrackMap's track-summary popup so the two views look
        // identical when the user clicks a triangle.
        const summitPopup = new maplibregl.Popup({ closeButton: false, closeOnClick: true, offset: 10 })
        const escapeHtml = (s: string) =>
          s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        m.on('click', 'fullscreen-summits-layer', (e) => {
          const props = e.features?.[0]?.properties
          if (!props) return
          const fallbackLabel = props.category === 'pass'
            ? t('components.mapFullscreen.poi.pass')
            : t('components.mapFullscreen.poi.summit')
          const name = escapeHtml(String(props.name || fallbackLabel))
          const label = escapeHtml(fallbackLabel)
          const ele = props.ele_m != null
            ? ` · ${formatElevation(Number(props.ele_m))}`
            : ''
          summitPopup
            .setLngLat(e.lngLat)
            .setHTML(`<span style="font-size:12px;font-weight:600">${name}</span><br><span style="font-size:11px;color:#6b7280">${label}${ele}</span>`)
            .addTo(m)
        })
        m.on('mouseenter', 'fullscreen-summits-layer', () => { m.getCanvas().style.cursor = 'pointer' })
        m.on('mouseleave', 'fullscreen-summits-layer', () => { m.getCanvas().style.cursor = '' })
      }

      if (!initialFitDoneRef.current) {
        const bounds = new maplibregl.LngLatBounds()
        for (const c of coordinates) bounds.extend(c)
        m.fitBounds(bounds, { padding: 60, maxZoom: 15 })
        initialFitDoneRef.current = true
      }
    }

    const initMap = () => {
      if (disposed || map) return
      if (el.clientWidth === 0 || el.clientHeight === 0) return

      const layer = selectedBaseLayerRef.current
      const initStyle = layer ? buildLayerStyle(layer, capsRef.current ?? {}) : null
      if (!initStyle) return

      map = new maplibregl.Map({
        container: el,
        style: initStyle as import('maplibre-gl').StyleSpecification | string,
        attributionControl: {},
        transformRequest: (url: string): RequestParameters => {
          if (url.includes('/api/v1/tiles/')) {
            const token = getToken()
            if (token) return { url, headers: { Authorization: `Bearer ${token}` } }
          }
          return { url }
        },
      })

      map.addControl(new maplibregl.NavigationControl(), 'top-right')

      map.on('load', () => {
        map!.resize()
        addTrackLayers(map!)
        setMapLoaded(true)
      })

      map.on('style.load', () => {
        if (pendingCameraRestoreRef.current) {
          map!.jumpTo(pendingCameraRestoreRef.current)
          pendingCameraRestoreRef.current = null
        }
        addTrackLayers(map!)
        setStyleVersion(v => v + 1)
      })

      mapRef.current = map
      setMapInstance(map)
    }

    // Expose initMap to the late-arrival retry effect below.
    initMapRef.current = initMap

    // Observe container: init when sized, resize when dimensions change
    const ro = new ResizeObserver(() => {
      if (!map) {
        initMap()
      } else {
        map.resize()
      }
    })
    ro.observe(el)

    // Try immediately (container might already have dimensions)
    initMap()

    return () => {
      disposed = true
      ro.disconnect()
      initMapRef.current = null
      photoMarkersRef.current.forEach(m => m.remove())
      photoMarkersRef.current = []
      photoBlobUrlsRef.current.forEach(u => URL.revokeObjectURL(u))
      photoBlobUrlsRef.current = []
      map?.remove()
      mapRef.current = null
      setMapInstance(null)
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Retry initMap when the async queries (useMapLayers / useCapabilities)
  // resolve after mount. Without this, an empty `base[]` or missing `caps`
  // at mount time leaves the map permanently un-initialised — the only
  // existing trigger (ResizeObserver) doesn't fire on data changes.
  useEffect(() => {
    if (mapRef.current) return // already initialised — nothing to do
    initMapRef.current?.()
  }, [selectedBaseLayer, caps])

  // ── Style switcher effect ─────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || prevBaseStyleRef.current === baseStyle || !selectedBaseLayer) return
    prevBaseStyleRef.current = baseStyle
    pendingCameraRestoreRef.current = {
      center: map.getCenter() as LngLatLike,
      zoom: map.getZoom(),
    }
    map.setStyle(buildLayerStyle(selectedBaseLayer, caps ?? {}) as import('maplibre-gl').StyleSpecification | string)
  }, [baseStyle, selectedBaseLayer, caps]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Heatmap effect ────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    removeHeatmapLayers(map)
    if (showHeatmap && userId) {
      addHeatmapLayers(map, buildHeatmapTileUrl(userId), heatmapColor, 'track-line')
    }
  }, [showHeatmap, heatmapColor, styleVersion, userId])

  // ── Heatmap color save handler ────────────────────────────────────────────
  const handleHeatmapColorChange = (hex: string) => {
    setHeatmapColor(hex)
    saveHeatmapColor(hex)
  }

  // ── Photo thumbnail markers ───────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !mapLoaded) return

    // Remove existing photo markers + blob URLs
    photoMarkersRef.current.forEach(m => m.remove())
    photoMarkersRef.current = []
    photoBlobUrlsRef.current.forEach(u => URL.revokeObjectURL(u))
    photoBlobUrlsRef.current = []

    if (!showPhotos) return

    for (const photo of positionedPhotos) {
      // NOTE: Do NOT set position:relative on el — that would override
      // .maplibregl-marker's position:absolute and break zoom-stable positioning.
      const el = document.createElement('div')
      el.style.cssText = 'cursor:pointer;'

      const imgBox = document.createElement('div')
      imgBox.style.cssText =
        'width:40px;height:30px;border-radius:3px;border:2px solid white;overflow:hidden;background:#444;box-shadow:0 1px 4px rgba(0,0,0,0.4);'

      const tri = document.createElement('div')
      tri.style.cssText =
        'width:0;height:0;border-left:5px solid transparent;border-right:5px solid transparent;border-top:6px solid white;margin:0 auto;'

      el.appendChild(imgBox)
      el.appendChild(tri)

      const marker = new maplibregl.Marker({ element: el, anchor: 'bottom' })
        .setLngLat([photo.lng, photo.lat])
        .addTo(map)
      photoMarkersRef.current.push(marker)

      // Load thumbnail async
      const thumbUrl = photo.thumb_url
      if (thumbUrl) {
        apiDownload(thumbUrl).then(({ blob }) => {
          const blobUrl = URL.createObjectURL(blob)
          photoBlobUrlsRef.current.push(blobUrl)
          const img = document.createElement('img')
          img.src = blobUrl
          img.style.cssText = 'width:100%;height:100%;object-fit:cover;display:block;'
          imgBox.innerHTML = ''
          imgBox.appendChild(img)
        }).catch(() => {/* ignore */})
      }
    }
  }, [positionedPhotos, mapLoaded, showPhotos])

  // Summit-layer visibility — toggleable via toolbar button.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !mapLoaded) return
    if (map.getLayer('fullscreen-summits-layer')) {
      map.setLayoutProperty('fullscreen-summits-layer', 'visibility', showSummits ? 'visible' : 'none')
    }
  }, [showSummits, mapLoaded])

  // ── Move hover marker on map ──────────────────────────────────────────────
  const moveMarker = useCallback((index: number | null) => {
    const map = mapRef.current
    if (!map || coordinates.length < 2) return

    const point = index != null ? streams[index] : null
    const hasPos = point?.lat != null && point?.lng != null

    if (hasPos) {
      const lngLat: [number, number] = [point.lng!, point.lat!]
      if (!markerRef.current) {
        const el = document.createElement('div')
        el.style.cssText = 'width:12px;height:12px;border-radius:50%;background:#fff;border:2px solid #f97316;box-shadow:0 0 4px rgba(0,0,0,0.3);'
        markerRef.current = new maplibregl.Marker({ element: el }).setLngLat(lngLat).addTo(map)
      } else {
        markerRef.current.setLngLat(lngLat)
      }
    } else if (markerRef.current) {
      markerRef.current.remove()
      markerRef.current = null
    }
  }, [coordinates, streams])

  // ── Highlight map segment on zoom ─────────────────────────────────────────
  // Note: line-color/opacity overrides removed — useMetricColoring is authoritative for track-line
  // paint properties. Brush only does fitBounds zoom to the selected segment.
  const highlightMapSegment = useCallback((startIdx: number, endIdx: number) => {
    const map = mapRef.current
    if (!map) return

    const applyHighlight = () => {
      const segCoords = streams
        .slice(startIdx, endIdx + 1)
        .filter((s): s is StreamPoint & { lat: number; lng: number } =>
          s.lat != null && s.lng != null)
        .map(s => [s.lng, s.lat] as [number, number])

      if (segCoords.length >= 2) {
        const bounds = new maplibregl.LngLatBounds()
        for (const c of segCoords) bounds.extend(c)
        map.fitBounds(bounds, { padding: 50, maxZoom: 18, duration: 600 })
      }
    }

    if (map.isStyleLoaded()) applyHighlight()
    else map.once('load', applyHighlight)
  }, [streams, metricColoring])

  const clearMapHighlight = useCallback(() => {
    const map = mapRef.current
    if (!map) return

    // Note: line-color/opacity overrides removed — useMetricColoring is authoritative.
    const apply = () => {
      const target = buildTrackMapFitTarget(streams, coordinates, null)
      if (!target) return

      const bounds = new maplibregl.LngLatBounds()
      for (const c of target.coordinates) bounds.extend(c)
      map.fitBounds(bounds, { padding: 60, maxZoom: target.maxZoom, duration: 600 })
    }

    if (map.isStyleLoaded()) apply()
    else map.once('load', apply)
  }, [coordinates])

  // ── Initialize + update ECharts ───────────────────────────────────────────
  const chartContainerRef = useECharts(
    () => {
      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      const yAxes: echarts.EChartsCoreOption[] = []
      const series: echarts.EChartsCoreOption[] = []
      const legendData: { name: string; itemStyle: { color: string } }[] = []

      const enabledList = availableStreams.filter(s => enabledStreams.has(s.key))
      const displayMetricValue = (key: MetricFieldKey, value: number) => {
        if (isNaN(value)) return value
        if (key === 'altitude') return elevationToDisplayValue(value, unitOptions.units)
        if (key === 'speed') return speedToDisplayValue(value / 3.6, unitOptions.units)
        if (key === 'temperature') return temperatureToDisplayValue(value, unitOptions.units)
        return value
      }

      availableStreams.forEach((stream, idx) => {
        const isEnabled = enabledStreams.has(stream.key)
        const enabledIdx = enabledList.indexOf(stream)

        legendData.push({ name: stream.label, itemStyle: { color: stream.color } })

        const showAxis = isEnabled && enabledIdx >= 0
        const isRight = enabledIdx > 0

        yAxes.push({
          type: 'value',
          show: showAxis,
          position: isRight ? 'right' : 'left',
          offset: isRight ? Math.max(0, (enabledIdx - 1) * 55) : 0,
          axisLabel: {
            fontSize: 10,
            color: stream.color,
            formatter: (v: number) => `${Math.round(v)}`,
          },
          splitLine: { show: !isRight, lineStyle: { color: 'rgba(200,200,200,0.3)', type: 'dashed' } },
          axisLine: { show: showAxis, lineStyle: { color: stream.color } },
          axisTick: { show: false },
        })

        series.push({
          name: stream.label,
          type: 'line',
          yAxisIndex: idx,
          data: xData.map((x, i) => [x, displayMetricValue(stream.key, data[stream.key][i])]),
          showSymbol: false,
          lineStyle: { width: 1.5, color: stream.color },
          itemStyle: { color: stream.color },
          areaStyle: stream.key === 'altitude' && isEnabled ? { color: `${stream.color}18` } : undefined,
          connectNulls: false,
        })
      })

      const legendSelected: Record<string, boolean> = {}
      availableStreams.forEach(s => {
        legendSelected[s.label] = enabledStreams.has(s.key)
      })

      const rightAxes = Math.max(0, enabledList.length - 1)
      const rightMargin = 12 + rightAxes * 55

      return {
        animation: false,

        legend: {
          show: true,
          top: 4,
          left: 'center',
          itemWidth: 26,
          itemHeight: 16,
          itemGap: 20,
          textStyle: { fontSize: 13, color: '#888' },
          data: legendData,
          selected: legendSelected,
        },

        grid: { left: 50, right: rightMargin, top: 30, bottom: 8 },

        brush: {
          toolbox: [],
          brushType: 'lineX',
          brushStyle: {
            borderWidth: 1,
            color: 'rgba(249,115,22,0.12)',
            borderColor: '#f97316',
          },
          throttleType: 'debounce',
          throttleDelay: 100,
        },

        toolbox: {
          feature: {
            brush: { show: false },
            saveAsImage: { title: t('components.mapFullscreen.saveAsPng'), pixelRatio: 2 },
          },
        },

        tooltip: {
          trigger: 'axis',
          transitionDuration: 0,
          confine: true,
          backgroundColor: 'rgba(255,255,255,0.96)',
          borderColor: '#e5e5e5',
          borderWidth: 1,
          padding: [6, 10],
          textStyle: { fontSize: 11, color: '#333' },
          axisPointer: {
            type: 'line',
            lineStyle: { color: '#f97316', width: 1, type: 'dashed' },
            animation: false,
          },
          formatter: (params: unknown) => {
            const arr = Array.isArray(params) ? params : [params]
            if (arr.length === 0) return ''
            const first = arr[0] as { dataIndex?: number }
            if (first?.dataIndex == null) return ''
            const idx = first.dataIndex

            const distanceStr = formatDistance((data.distanceKm[idx] ?? 0) * 1000)
            const elapsed = data.elapsedMin[idx] ?? 0
            const timeStr = formatElapsed(elapsed, data.maxElapsed)
            const posLine = xMode === 'distance'
              ? distanceStr
              : `${timeStr}\u2009\u00b7\u2009${distanceStr}`

            let html = `<div style="line-height:1.6;min-width:110px">`
            html += `<div style="color:#888;font-size:10px;margin-bottom:1px">${posLine}</div>`

            for (const item of arr as Array<{ seriesName?: string; value?: [number, number]; color?: string }>) {
              if (!item.value || isNaN(item.value[1])) continue
              const stream = availableStreams.find(s => s.label === item.seriesName)
              if (!stream) continue
              const decimals = ['altitude', 'temperature', 'heartRate', 'power', 'cadence'].includes(stream.key) ? 0 : 1
              html += `<div style="font-size:12px">`
              html += `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${stream.color};margin-right:4px;vertical-align:middle"></span>`
              html += `<strong>${item.value[1].toFixed(decimals)}</strong>\u2009${stream.unit}`
              html += `</div>`
            }

            return html + '</div>'
          },
        },

        xAxis: {
          type: 'value',
          min: 0,
          max: xMax,
          axisLabel: { show: false },
          splitLine: { show: false },
          axisLine: { show: false },
          axisTick: { show: false },
        },

        yAxis: yAxes,
        series,

        dataZoom: [
          {
            type: 'inside',
            xAxisIndex: 0,
            filterMode: 'none',
            zoomOnMouseWheel: true,
            moveOnMouseMove: true,
            moveOnMouseWheel: false,
          },
        ],
      } as echarts.EChartsCoreOption
    },
    [data, xMode, enabledStreams, availableStreams, unitOptions.units, formatDistance, formatElevation], // eslint-disable-line react-hooks/exhaustive-deps
    (chart) => {
      chartInstanceRef.current = chart

      const xData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      const xMax = xMode === 'distance' ? data.maxDist : data.maxElapsed

      // Activate brush cursor
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: 'lineX', brushMode: 'single' },
      })

      // Use ZRender for reliable hover-to-map-marker sync
      // (type:'value' xAxis showTip does not include dataIndex for real hover)
      const zr = chart.getZr()
      const chartXData = xMode === 'distance' ? data.distanceKm : data.elapsedMin
      let lastMarkerIdx: number | null = null

      const zrMarkerMove = (e: unknown) => {
        const event = e as { offsetX?: number; offsetY?: number }
        if (event.offsetX == null || event.offsetY == null) return
        if (!chart.containPixel({ gridIndex: 0 }, [event.offsetX, event.offsetY])) {
          if (lastMarkerIdx !== null) { lastMarkerIdx = null; moveMarker(null) }
          return
        }
        const xVal = chart.convertFromPixel({ xAxisIndex: 0 }, event.offsetX) as number | null
        if (xVal == null) { if (lastMarkerIdx !== null) { lastMarkerIdx = null; moveMarker(null) } return }
        const idx = findClosestIdx(chartXData, xVal)
        if (idx !== lastMarkerIdx) { lastMarkerIdx = idx; moveMarker(idx) }
      }
      const zrMarkerOut = () => {
        if (lastMarkerIdx !== null) { lastMarkerIdx = null; moveMarker(null) }
      }
      zr.on('mousemove', zrMarkerMove)
      zr.on('mouseout', zrMarkerOut)

      // Legend toggle -> update enabledStreams
      chart.on('legendselectchanged', (params: unknown) => {
        const typed = params as { selected: Record<string, boolean> }
        const next = new Set<MetricFieldKey>()
        availableStreams.forEach(s => {
          if (typed.selected[s.label]) next.add(s.key)
        })
        setEnabledStreams(next)
      })

      // Brush handling (same pattern as ActivityCharts)
      type BrushArea = { range?: [number, number]; coordRange?: [number, number] }
      let brushZoomLock = false

      chart.on('brushEnd', (params: unknown) => {
        if (brushZoomLock) return
        brushZoomLock = true

        const p = params as {
          areas?: BrushArea[]
          batch?: Array<{ areas?: BrushArea[] }>
        }
        const areas = p.areas ?? p.batch?.[0]?.areas
        if (!areas || areas.length === 0) {
          brushZoomLock = false
          return
        }

        const area = areas[0]
        let startData: number
        let endData: number

        if (area.coordRange) {
          [startData, endData] = area.coordRange
        } else if (area.range) {
          const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
          const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
          if (s == null || e == null) { brushZoomLock = false; return }
          startData = s as number
          endData = e as number
        } else {
          brushZoomLock = false
          return
        }

        if (endData <= startData) { brushZoomLock = false; return }

        const sv = Math.max(0, startData)
        const ev = Math.min(xMax, endData)

        const startPct = Math.max(0, (sv / xMax) * 100)
        const endPct = Math.min(100, (ev / xMax) * 100)
        if (endPct - startPct < 0.5) { brushZoomLock = false; return }

        chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: startPct, end: endPct })
        chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
        chart.dispatchAction({
          type: 'takeGlobalCursor',
          key: 'brush',
          brushOption: { brushType: false },
        })

        setIsZoomed(true)

        const xArr = xMode === 'distance' ? data.distanceKm : data.elapsedMin
        const startIdx = findNearestIdx(xArr, sv)
        const endIdx = findNearestIdx(xArr, ev)
        highlightMapSegment(startIdx, endIdx)

        const startKm = xMode === 'distance' ? sv : (data.distanceKm[startIdx] ?? 0)
        const endKm = xMode === 'distance' ? ev : (data.distanceKm[endIdx] ?? data.maxDist)
        setZoomRange({ startKm, endKm, startPct, endPct, startIdx, endIdx })

        requestAnimationFrame(() => { brushZoomLock = false })
      })

      // Mouse-wheel / pan zoom
      chart.on('dataZoom', (params: unknown) => {
        const p = params as {
          batch?: Array<{ start: number; end: number }>
          start?: number
          end?: number
        }
        const z = p.batch?.[0] ?? (p.start != null && p.end != null ? p as { start: number; end: number } : null)
        if (z == null) return

        const zoomed = z.start > 0.5 || z.end < 99.5
        setIsZoomed(zoomed)

        if (zoomed) {
          const startVal = (z.start / 100) * xMax
          const endVal = (z.end / 100) * xMax
          const startIdx = findNearestIdx(xData, startVal)
          const endIdx = findNearestIdx(xData, endVal)
          highlightMapSegment(startIdx, endIdx)
          const startKm = xMode === 'distance' ? startVal : (data.distanceKm[startIdx] ?? 0)
          const endKm = xMode === 'distance' ? endVal : (data.distanceKm[endIdx] ?? data.maxDist)
          setZoomRange({ startKm, endKm, startPct: z.start, endPct: z.end, startIdx, endIdx })
        } else {
          clearMapHighlight()
          setZoomRange(null)
        }
      })

      return () => {
        chartInstanceRef.current = null
        zr.off('mousemove', zrMarkerMove)
        zr.off('mouseout', zrMarkerOut)
      }
    },
  )

  // ── Reset zoom handler ──────────────────────────────────────────────────
  const handleResetZoom = useCallback(() => {
    const chart = chartInstanceRef.current
    if (!chart) return

    chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })

    setIsZoomed(false)
    setZoomRange(null)
    clearMapHighlight()
  }, [clearMapHighlight])

  // ── Switch x-mode ─────────────────────────────────────────────────────────
  const switchXMode = useCallback((mode: XMode) => {
    if (isZoomed) {
      handleResetZoom()
    }
    setXMode(mode)
  }, [isZoomed, handleResetZoom])

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-black overflow-hidden">
      {/* Map fills the space */}
      <div className="relative flex-1 min-h-0 overflow-hidden">
        <div ref={mapContainerRef} className="w-full h-full" />

        {/* Kartenstil-Tabs + Heatmap */}
        <div className="absolute top-3 left-3 z-10 flex items-center gap-1.5 flex-wrap">
          <div className="flex items-center gap-0.5 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md p-0.5">
            {base.map((l) => (
              <button
                key={l.key}
                onClick={() => setBaseStyle(l.key)}
                className={[
                  'px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer',
                  baseStyle === l.key
                    ? 'bg-accent-DEFAULT text-white'
                    : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700',
                ].join(' ')}
              >
                {l.name}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-1 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md px-2 py-1">
            <button
              onClick={() => setShowHeatmap(v => !v)}
              title={showHeatmap ? t('components.mapFullscreen.hideHeatmap') : t('components.mapFullscreen.showHeatmap')}
              className={[
                'flex items-center gap-1.5 text-xs font-medium rounded-md px-1.5 py-0.5 transition-colors cursor-pointer',
                showHeatmap
                  ? 'text-accent-DEFAULT'
                  : 'text-neutral-600 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-white',
              ].join(' ')}
            >
              <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
                <path d="M12 2C8.5 6 6 9 6 13a6 6 0 0012 0c0-2.5-1-4.5-2.5-6.5C14.5 8.5 13.5 10 13 12c-.5-1.5-.5-3 0-5.5C12.5 5 12.2 3.5 12 2z"/>
              </svg>
              {t('components.mapFullscreen.heatmap')}
            </button>
            {showHeatmap && (
              <ColorPicker value={heatmapColor} onChange={handleHeatmapColorChange} />
            )}
          </div>

          {/* Metric color dropdown — sits right of the Heatmap control */}
          {hasGps && (
            <MapColorDropdown
              streams={streams}
              sportType={sportType}
              subType={subType}
              metric={metricColoring.metric}
              onChange={metricColoring.setMetric}
              modeByMetric={metricColoring.modeByMetric}
              setMode={metricColoring.setMode}
              zonesAvailable={metricColoring.zonesAvailable}
              effectiveMode={metricColoring.effectiveMode}
            />
          )}
        </div>

        {/* Metric color legend */}
        {hasGps && metricColoring.scale && (
          <div className="absolute bottom-3 left-3 z-10">
            <MapColorLegend scale={metricColoring.scale} />
          </div>
        )}

        {/* Summit toggle button (only if activity has summits/passes) */}
        {(geoTags ?? []).some(t => (t.category === 'summit' || t.category === 'pass') && t.lat != null) && (
          <button
            onClick={() => setShowSummits(v => !v)}
            title={showSummits ? t('trackMap.hideSummits') : t('trackMap.showSummits')}
            className={[
              // Position: left of the photo button (or in the photo slot if no photos).
              positionedPhotos.length > 0 ? 'absolute top-3 right-[9.5rem]' : 'absolute top-3 right-[6.5rem]',
              'z-10 flex items-center justify-center w-9 h-9',
              'rounded-lg backdrop-blur-sm shadow-md cursor-pointer transition-colors',
              showSummits
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 hover:bg-white dark:hover:bg-neutral-700',
            ].join(' ')}
          >
            <Mountain className="w-4 h-4" aria-hidden="true" />
          </button>
        )}

        {/* Photo toggle button (only if activity has positioned photos) */}
        {positionedPhotos.length > 0 && (
          <button
            onClick={() => setShowPhotos(v => !v)}
            title={showPhotos ? t('trackMap.hidePhotos') : t('trackMap.showPhotos')}
            className={[
              'absolute top-3 right-[6.5rem] z-10 flex items-center justify-center w-9 h-9',
              'rounded-lg backdrop-blur-sm shadow-md cursor-pointer transition-colors',
              showPhotos
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 hover:bg-white dark:hover:bg-neutral-700',
            ].join(' ')}
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
              <path fillRule="evenodd" d="M1 5.25A2.25 2.25 0 013.25 3h13.5A2.25 2.25 0 0119 5.25v9.5A2.25 2.25 0 0116.75 17H3.25A2.25 2.25 0 011 14.75v-9.5zm1.5 5.81v3.69c0 .414.336.75.75.75h13.5a.75.75 0 00.75-.75v-2.69l-2.22-2.219a.75.75 0 00-1.06 0l-1.91 1.909.47.47a.75.75 0 11-1.06 1.06L6.53 8.091a.75.75 0 00-1.06 0l-2.97 2.97zM12 7a1 1 0 11-2 0 1 1 0 012 0z" clipRule="evenodd" />
            </svg>
          </button>
        )}

        {/* Close button */}
        <button
          onClick={onClose}
          className="absolute top-3 right-14 z-10 flex items-center justify-center w-9 h-9
                     rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm
                     text-neutral-700 dark:text-neutral-200 shadow-md cursor-pointer
                     hover:bg-white dark:hover:bg-neutral-700 transition-colors"
          title={t('components.mapFullscreen.closeEsc')}
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-5 h-5">
            <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
          </svg>
        </button>
      </div>

      {/* Overlay chart panel at the bottom */}
      <div className="relative z-10 bg-white/80 dark:bg-neutral-900/80 backdrop-blur-sm border-t border-neutral-200/50 dark:border-neutral-700/50">
        {/* Controls bar */}
        <div className="flex items-center justify-between px-4 py-1.5">
          <div className="flex items-center gap-2">
            {isZoomed && (
              <button
                onClick={handleResetZoom}
                className="cursor-pointer text-xs px-2.5 py-1 rounded-md bg-accent-100 text-orange-700 hover:bg-orange-200
                           dark:bg-orange-900/30 dark:text-orange-300 dark:hover:bg-orange-900/50 transition-colors"
              >
                {t('components.mapFullscreen.resetZoom')}
              </button>
            )}
          </div>
          <div className="flex items-center gap-2">
            <div className="flex rounded-md border border-neutral-300 dark:border-neutral-600 overflow-hidden text-xs">
              <button
                onClick={() => switchXMode('distance')}
                className={`cursor-pointer px-2.5 py-0.5 transition-colors ${xMode === 'distance'
                  ? 'bg-neutral-800 text-white dark:bg-neutral-200 dark:text-neutral-900'
                  : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}
              >
                {distanceInputUnit(unitOptions.units)}
              </button>
              <button
                onClick={() => switchXMode('time')}
                className={`cursor-pointer px-2.5 py-0.5 transition-colors border-l border-neutral-300 dark:border-neutral-600 ${xMode === 'time'
                  ? 'bg-neutral-800 text-white dark:bg-neutral-200 dark:text-neutral-900'
                  : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}
              >
                {t('components.mapFullscreen.time')}
              </button>
            </div>
          </div>
        </div>

        {/* Chart */}
        <div ref={chartContainerRef} className="h-[200px] w-full [&_canvas]:!cursor-crosshair" />
      </div>
    </div>
  )
}
