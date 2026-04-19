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
import { MOTION_PROFILE_BY_SPORT_TYPE } from '@/lib/colorScaleRanges'
import { sportChartColor } from '@/lib/sportColors'

const MAX_MAP_SEGMENTS = 2000
const MAX_MAP_SEGMENTS_MOBILE_FACTOR = 1.5
const VIEWPORT_RESAMPLE_DEBOUNCE_MS = 250
const VIEWPORT_BOUNDS_DECIMALS = 4
const VIEWPORT_MIN_RATIO_FOR_RESAMPLE = 0.8

function maxSegmentsForViewport(): number {
  return Math.min(MAX_MAP_SEGMENTS, Math.round(window.innerWidth * MAX_MAP_SEGMENTS_MOBILE_FACTOR))
}

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
}

export function useMetricColoring(args: UseMetricColoringArgs): UseMetricColoringResult {
  const { streams, activity, hasGps, zoomRange, map } = args

  const [metric, setMetric] = useState<Metric>('sport')
  const [modeByMetric, setModeByMetric] = useState<Partial<Record<Metric, Mode>>>({})

  /**
   * Auto-reset metric to 'sport' when:
   *   - activity changes (different streams, different availabilities)
   *   - current metric's data is not available in the new streams (≥ 10 % threshold below)
   * Prevents sticky metric across activities with different sensor coverage.
   */
  useEffect(() => {
    if (metric === 'sport') return
    const fieldForMetric: Partial<Record<Metric, keyof StreamPoint>> = {
      hr: 'heart_rate', speed: 'speed_ms', pace: 'speed_ms',
      altitude: 'altitude_m', cadence: 'cadence',
      power: 'power_w', temperature: 'temperature_c',
    }
    const field = fieldForMetric[metric]
    if (!field) return
    const defined = streams.reduce((n, s) => s[field] != null ? n + 1 : n, 0)
    const pct = streams.length > 0 ? defined / streams.length : 0
    // Threshold = 0.3 (most lenient — Power's threshold). Simpler than per-metric thresholds here.
    if (pct < 0.3) setMetric('sport')
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
  const programmaticFitCountRef = useRef(0)

  const handleMoveEndRaw = useCallback(() => {
    if (!map) return
    if (programmaticFitCountRef.current > 0) return
    const b = map.getBounds()
    const key = [b.getWest(), b.getSouth(), b.getEast(), b.getNorth()]
      .map(v => v.toFixed(VIEWPORT_BOUNDS_DECIMALS)).join(',')
    setViewportBoundsKey(key)
  }, [map])

  const debouncedBoundsKey = useDebounce(viewportBoundsKey, VIEWPORT_RESAMPLE_DEBOUNCE_MS)

  // Register moveend listener with proper cleanup; reset counter on map identity change
  useEffect(() => {
    if (!map) return
    map.on('moveend', handleMoveEndRaw)
    return () => {
      map.off('moveend', handleMoveEndRaw)
      // If the map identity changes (re-mount, style switch), pending programmaticFit
      // callbacks would be lost — reset counter to avoid permanent skip-state.
      programmaticFitCountRef.current = 0
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

    const fullRange = (maxLat - minLat) * (maxLng - minLng)
    const viewRange = (viewportBounds.north - viewportBounds.south) *
                      (viewportBounds.east - viewportBounds.west)
    if (viewRange / fullRange >= VIEWPORT_MIN_RATIO_FOR_RESAMPLE) return undefined
    return viewportBounds
  }, [viewportBounds, streams])

  // Downsample + build features — aggregation mode matches scale type
  const downsampled = useMemo(() => {
    const zoneHighs = scale && scale.type === 'zones' ? scale.zoneHighs : undefined
    const aggMode: Mode = scale && scale.type === 'zones' ? 'zones' : 'linear'
    return downsampleUniform(streams, metric, maxSegmentsForViewport(), effectiveBounds, aggMode, zoneHighs)
  }, [streams, metric, effectiveBounds, scale])

  const features = useMemo(() => {
    if (!scale) return []
    return buildSegmentFeatures(downsampled, metric, scale)
  }, [downsampled, metric, scale])

  const featuresWithInside = useMemo(() =>
    applyInsideFlag(features, zoomRange),
    [features, zoomRange])

  const colorExpression = useMemo(() => scale ? buildLineColorExpression(scale) : '#9ca3af', [scale])

  // Apply to map
  useEffect(() => {
    if (!map || !hasGps) return
    if (!map.isStyleLoaded()) {
      map.once('load', () => applyPaintAndData())
      return
    }
    applyPaintAndData()

    function applyPaintAndData() {
      if (!map) return
      const src = map.getSource('track') as maplibregl.GeoJSONSource | undefined
      src?.setData({ type: 'FeatureCollection', features: featuresWithInside })
      if (map.getLayer('track-line')) {
        map.setPaintProperty('track-line', 'line-color', colorExpression as maplibregl.DataDrivenPropertyValueSpecification<string>)
        map.setPaintProperty('track-line', 'line-opacity', LINE_OPACITY_EXPRESSION as unknown as maplibregl.DataDrivenPropertyValueSpecification<number>)
      }
    }
  }, [map, hasGps, featuresWithInside, colorExpression])

  // Heading arrows visibility toggle
  useEffect(() => {
    if (!map || !map.isStyleLoaded()) return
    if (map.getLayer('heading-arrows')) {
      map.setLayoutProperty('heading-arrows', 'visibility', metric === 'sport' ? 'visible' : 'none')
    }
  }, [map, metric])

  // programmaticFit helper — increments counter before fitBounds
  const programmaticFit = useCallback((bounds: maplibregl.LngLatBounds) => {
    if (!map) return
    programmaticFitCountRef.current += 1
    map.once('moveend', () => {
      programmaticFitCountRef.current = Math.max(0, programmaticFitCountRef.current - 1)
    })
    map.fitBounds(bounds, { padding: 50, maxZoom: 18, duration: 600 })
  }, [map])

  return { metric, setMetric, scale, programmaticFit, modeByMetric, setMode, zonesAvailable }
}
