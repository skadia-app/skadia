// frontend/src/components/activity/useMetricColoring.ts
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import maplibregl from 'maplibre-gl'
import { useQuery } from '@tanstack/react-query'
import { useDebounce } from '@/lib/useDebounce'
import type { Activity, StreamPoint } from '@/api/types'
import type { ZoomRange } from '@/components/ActivityCharts'
import {
  resolveScale,
  downsampleUniform,
  buildSegmentFeatures,
  applyInsideFlag,
  buildLineColorExpression,
  LINE_OPACITY_EXPRESSION,
  matchSportZones,
  type Metric,
  type Mode,
  type LngLatBoundsLike,
  type ScaleDefinition,
} from '@/lib/mapColorUtils'
import { fetchTrainingZones } from '@/api/trainingZones'
import { getHeatmapSettings } from '@/api/settings'
import { MOTION_PROFILE_BY_SPORT_TYPE, METRIC_AVAILABILITY_THRESHOLDS } from '@/lib/colorScaleRanges'
import { sportChartColor } from '@/lib/sportColors'

const MAX_MAP_SEGMENTS = 2000
const MAX_MAP_SEGMENTS_MOBILE_FACTOR = 1.5
const VIEWPORT_RESAMPLE_DEBOUNCE_MS = 250
const VIEWPORT_BOUNDS_DECIMALS = 4
const VIEWPORT_MIN_RATIO_FOR_RESAMPLE = 0.8

export interface UseMetricColoringArgs {
  streams: StreamPoint[]
  activity: Pick<Activity, 'sport_type' | 'fit_sport_id'>
  hasGps: boolean
  zoomRange: ZoomRange | null
  map: maplibregl.Map | null
}

export interface UseMetricColoringResult {
  metric: Metric
  setMetric: (m: Metric) => void
  scale: ScaleDefinition | null
  programmaticFit: (bounds: maplibregl.LngLatBounds) => void

  // Phase B additions:
  modeByMetric: Partial<Record<Metric, Mode>>
  setMode: (metric: Metric, mode: Mode) => void
  zonesAvailable: (metric: Metric) => boolean
  effectiveMode: Mode  // resolved mode after zones-availability fallback (N9)
}

export function useMetricColoring(args: UseMetricColoringArgs): UseMetricColoringResult {
  const { streams, activity, hasGps, zoomRange, map } = args

  const [metric, setMetric] = useState<Metric>('sport')
  const [modeByMetric, setModeByMetric] = useState<Partial<Record<Metric, Mode>>>({})

  // Track window width for viewport-adaptive segment budget. Debounced to avoid thrashing on resize.
  const [windowWidth, setWindowWidth] = useState(() => typeof window !== 'undefined' ? window.innerWidth : 1920)
  useEffect(() => {
    if (typeof window === 'undefined') return
    let timer: number | null = null
    const onResize = () => {
      if (timer != null) window.clearTimeout(timer)
      timer = window.setTimeout(() => setWindowWidth(window.innerWidth), 200)
    }
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      if (timer != null) window.clearTimeout(timer)
    }
  }, [])

  const maxSegments = useMemo(() =>
    Math.min(MAX_MAP_SEGMENTS, Math.round(windowWidth * MAX_MAP_SEGMENTS_MOBILE_FACTOR)),
    [windowWidth],
  )

  // Ref to read current metric inside the auto-reset effect without stale closure.
  const metricRef = useRef(metric)
  // Retry budget for applyPaintAndData when the 'track' source isn't yet registered.
  const retriesRef = useRef(0)
  useEffect(() => { metricRef.current = metric }, [metric])

  /**
   * Auto-reset metric to 'sport' when:
   *   - activity changes (different streams, different availabilities)
   *   - current metric's data is not available in the new streams (per-metric threshold)
   * Prevents sticky metric across activities with different sensor coverage.
   */
  useEffect(() => {
    const current = metricRef.current
    if (current === 'sport') return
    const fieldForMetric: Partial<Record<Metric, keyof StreamPoint>> = {
      hr: 'heart_rate', speed: 'speed_ms', pace: 'speed_ms',
      altitude: 'altitude_m', cadence: 'cadence',
      power: 'power_w', temperature: 'temperature_c',
    }
    const thresholdForMetric: Partial<Record<Metric, number>> = {
      hr:          METRIC_AVAILABILITY_THRESHOLDS.hr,
      speed:       METRIC_AVAILABILITY_THRESHOLDS.speed,
      pace:        METRIC_AVAILABILITY_THRESHOLDS.speed, // pace = speed coverage
      altitude:    METRIC_AVAILABILITY_THRESHOLDS.altitude,
      cadence:     METRIC_AVAILABILITY_THRESHOLDS.cadence,
      power:       METRIC_AVAILABILITY_THRESHOLDS.power,
      temperature: METRIC_AVAILABILITY_THRESHOLDS.temperature,
    }
    const field = fieldForMetric[current]
    const threshold = thresholdForMetric[current] ?? 0.3
    if (!field) return
    const defined = streams.reduce((n, s) => s[field] != null ? n + 1 : n, 0)
    const pct = streams.length > 0 ? defined / streams.length : 0
    if (pct < threshold) setMetric('sport')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streams, activity.sport_type, activity.fit_sport_id])

  // Reset mode overrides when activity changes — prevents 'linear' from a
  // previous activity bleeding into an activity with configured zones.
  useEffect(() => {
    setModeByMetric({})
  }, [activity.sport_type, activity.fit_sport_id])

  const { data: zonesData } = useQuery({
    queryKey: ['training-zones'],
    queryFn: fetchTrainingZones,
    staleTime: 1000 * 60 * 10,
  })

  const { data: heatmapSettings } = useQuery({
    queryKey: ['heatmap-settings'],
    queryFn: getHeatmapSettings,
    staleTime: 60_000,
  })

  const teleportThresholdM = heatmapSettings?.teleport_filter_activity
    ? (heatmapSettings.teleport_threshold_m ?? 2000)
    : 0

  // IMPORTANT: depend on scalar activity fields, not the activity object itself.
  // TrackMap builds { sport_type, fit_sport_id } as a fresh object on every render.
  const sportZones = useMemo(() => {
    if (!zonesData) return null
    return matchSportZones(activity, zonesData.sports)
  }, [zonesData, activity.sport_type, activity.fit_sport_id])

  const zonesAvailable = useCallback((m: Metric): boolean => {
    if (!sportZones) return false
    if (m === 'hr')    return sportZones.zones.hr    != null && (sportZones.zones.hr.zone_highs?.length ?? 0) > 0
    if (m === 'power') return sportZones.zones.power != null && (sportZones.zones.power.zone_highs?.length ?? 0) > 0
    if (m === 'speed') return sportZones.zones.speed != null && (sportZones.zones.speed.zone_highs?.length ?? 0) > 0
    return false
  }, [sportZones])

  const setMode = useCallback((m: Metric, mode: Mode) => {
    setModeByMetric(prev => ({ ...prev, [m]: mode }))
  }, [])

  const effectiveMode: Mode = useMemo(() => {
    if (metric === 'hr' || metric === 'power' || metric === 'speed') {
      const explicit = modeByMetric[metric]
      if (explicit) return explicit
      return zonesAvailable(metric) ? 'zones' : 'linear'
    }
    return 'linear'
  }, [metric, modeByMetric, zonesAvailable])

  const motionProfile = useMemo(() => {
    return MOTION_PROFILE_BY_SPORT_TYPE[activity.sport_type]
      ?? MOTION_PROFILE_BY_SPORT_TYPE.generic
  }, [activity.sport_type])

  // Scale computed on FULL streams (not visible subset) — see spec §Scale-auf-Full-Streams
  const scale = useMemo<ScaleDefinition | null>(() => {
    if (!hasGps) return null
    return resolveScale(metric, motionProfile, streams, sportZones, effectiveMode, {
      sportColor: sportChartColor(activity.sport_type),
    })
  }, [metric, motionProfile, streams, sportZones, effectiveMode, activity.sport_type, hasGps])

  // Viewport bounds tracking
  const [viewportBoundsKey, setViewportBoundsKey] = useState<string | null>(null)
  // R4: ID-based tracking Set replaces a plain counter — parallel fits no longer multiplex
  // a single moveend into all decrement closures; each fit resolves independently.
  const pendingFitIdsRef = useRef<Set<number>>(new Set())
  const nextFitIdRef = useRef(0)

  const handleMoveEndRaw = useCallback(() => {
    if (!map) return
    if (pendingFitIdsRef.current.size > 0) return
    const b = map.getBounds()
    const key = [b.getWest(), b.getSouth(), b.getEast(), b.getNorth()]
      .map(v => v.toFixed(VIEWPORT_BOUNDS_DECIMALS)).join(',')
    setViewportBoundsKey(key)
  }, [map])

  const debouncedBoundsKey = useDebounce(viewportBoundsKey, VIEWPORT_RESAMPLE_DEBOUNCE_MS)

  // Register moveend listener with proper cleanup; reset tracking set on map identity change
  useEffect(() => {
    if (!map) return
    map.on('moveend', handleMoveEndRaw)
    return () => {
      map.off('moveend', handleMoveEndRaw)
      // If the map identity changes (re-mount, style switch), pending programmaticFit
      // callbacks would be lost — clear set to avoid permanent skip-state.
      pendingFitIdsRef.current.clear()
      nextFitIdRef.current = 0
    }
  }, [map, handleMoveEndRaw])

  // Parse viewportBoundsKey back into LngLatBoundsLike
  const viewportBounds = useMemo<LngLatBoundsLike | undefined>(() => {
    if (!debouncedBoundsKey) return undefined
    const [west, south, east, north] = debouncedBoundsKey.split(',').map(Number)
    return { west, south, east, north }
  }, [debouncedBoundsKey])

  // Decide whether to apply viewport filter (only if viewport is significantly smaller than full track)
  const effectiveBounds = useMemo<LngLatBoundsLike | undefined>(() => {
    if (!viewportBounds || streams.length === 0) return undefined

    let minLat = Infinity, maxLat = -Infinity, minLng = Infinity, maxLng = -Infinity
    for (const s of streams) {
      if (s.lat == null || s.lng == null) continue
      if (s.lat < minLat) minLat = s.lat
      if (s.lat > maxLat) maxLat = s.lat
      if (s.lng < minLng) minLng = s.lng
      if (s.lng > maxLng) maxLng = s.lng
    }
    if (!Number.isFinite(minLat)) return undefined

    // Simplified: since we only use the ratio, cos(lat) cancels between full and viewport
    // (both measured in the same local tangent plane approximately at the track center).
    // Using two different centers (fullLatC vs viewLatC) caused bias when the viewport
    // was panned away from the track (e.g. track in Alps, view over Northern Germany).
    const fullArea = (maxLat - minLat) * (maxLng - minLng)
    const viewArea = (viewportBounds.north - viewportBounds.south) *
                     (viewportBounds.east - viewportBounds.west)
    if (fullArea <= 0 || viewArea / fullArea >= VIEWPORT_MIN_RATIO_FOR_RESAMPLE) return undefined
    return viewportBounds
  }, [viewportBounds, streams])

  // Downsample + build features — aggregation mode matches scale type
  const downsampled = useMemo(() => {
    const zoneHighs = scale && scale.type === 'zones' ? scale.zoneHighs : undefined
    const aggMode: Mode = scale && scale.type === 'zones' ? 'zones' : 'linear'
    return downsampleUniform(streams, metric, maxSegments, effectiveBounds, aggMode, zoneHighs)
  }, [streams, metric, maxSegments, effectiveBounds, scale])

  const features = useMemo(() => {
    if (!scale) return []
    return buildSegmentFeatures(downsampled, metric, scale, teleportThresholdM)
  }, [downsampled, metric, scale, teleportThresholdM])

  const featuresWithInside = useMemo(() =>
    applyInsideFlag(features, zoomRange),
    [features, zoomRange])

  const colorExpression = useMemo(() => scale ? buildLineColorExpression(scale) : '#9ca3af', [scale])

  // Apply to map
  useEffect(() => {
    if (!map || !hasGps) return
    // Fresh retry budget for each new trigger (featuresWithInside / colorExpression change).
    // Without this reset a previously exhausted counter would block legitimate future updates.
    retriesRef.current = 0
    if (!map.isStyleLoaded()) {
      map.once('load', () => applyPaintAndData())
      return
    }
    applyPaintAndData()

    function applyPaintAndData() {
      if (!map) return
      const src = map.getSource('track') as maplibregl.GeoJSONSource | undefined
      if (!src) {
        // Source/Layer not yet registered by TrackMap — retry on next idle, bounded.
        // Typical: source appears on the first idle after TrackMap's load handler.
        // If not: activity has no track (hasGps=true but <2 valid GPS points) — give up silently.
        if (retriesRef.current >= 3) return
        retriesRef.current += 1
        map.once('idle', applyPaintAndData)
        return
      }
      // Source available — reset counter so future scale/features updates start fresh.
      retriesRef.current = 0
      src.setData({ type: 'FeatureCollection', features: featuresWithInside })
      if (map.getLayer('track-line')) {
        map.setPaintProperty('track-line', 'line-color', colorExpression as maplibregl.DataDrivenPropertyValueSpecification<string>)
        map.setPaintProperty('track-line', 'line-opacity', LINE_OPACITY_EXPRESSION as unknown as maplibregl.DataDrivenPropertyValueSpecification<number>)
      }
    }
  }, [map, hasGps, featuresWithInside, colorExpression])

  // Heading arrows visibility toggle — also handles late layer registration.
  // R3: cancel flag + explicit off() prevent listener accumulation on metric-change spam.
  useEffect(() => {
    if (!map) return
    let cancelled = false
    const apply = () => {
      if (cancelled || !map.getLayer('heading-arrows')) return
      map.setLayoutProperty('heading-arrows', 'visibility', metric === 'sport' ? 'visible' : 'none')
    }
    if (map.isStyleLoaded()) {
      apply()
    } else {
      map.once('load', apply)
    }
    // Also re-apply if layer is added late
    map.once('idle', apply)
    return () => {
      cancelled = true
      map.off('load', apply)
      map.off('idle', apply)
    }
  }, [map, metric])

  // programmaticFit helper — R4: ID-based tracking so parallel fits each resolve independently.
  // The old counter approach had a multiplexing bug: parallel fitBounds calls shared a single
  // once('moveend') decrement, so the first moveend would fire all closures at once.
  const programmaticFit = useCallback((bounds: maplibregl.LngLatBounds) => {
    if (!map) return
    const id = ++nextFitIdRef.current
    pendingFitIdsRef.current.add(id)
    let resolved = false
    const resolve = () => {
      if (resolved) return
      resolved = true
      pendingFitIdsRef.current.delete(id)
      map.off('moveend', onMoveEnd)
      window.clearTimeout(fallback)
    }
    const onMoveEnd = resolve
    // Fallback in case fitBounds emits no moveend (duration=0 or bounds already match):
    const fallback = window.setTimeout(resolve, 900) // duration 600 + 300 margin
    map.once('moveend', onMoveEnd)
    map.fitBounds(bounds, { padding: 50, maxZoom: 18, duration: 600 })
  }, [map])

  return { metric, setMetric, scale, programmaticFit, modeByMetric, setMode, zonesAvailable, effectiveMode }
}
