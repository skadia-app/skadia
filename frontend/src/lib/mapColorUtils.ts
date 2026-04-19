// frontend/src/lib/mapColorUtils.ts
import type { StreamPoint } from '@/api/types'
import type { SportZones } from '@/api/trainingZones'
import {
  SPEED_PROFILES, PACE_PROFILES, CADENCE_PROFILES,
  TEMPERATURE_RANGE, ALTITUDE_MIN_SPREAD,
  HR_FALLBACK_MIN_SPREAD, POWER_FALLBACK_MIN_SPREAD,
  expandRange,
  type MotionProfile,
} from './colorScaleRanges'
import { VIRIDIS_9, DIVERGING_TEMP_STOPS, MISSING_COLOR, HR_ZONE_BASE, POWER_ZONE_BASE, SPEED_ZONE_BASE, interpolateHex } from './colorPalettes'
import type { ZoomRange } from '@/components/ActivityCharts'

// ─── Types ────────────────────────────────────────────────────────────────────

export type Metric =
  | 'sport' | 'hr' | 'speed' | 'pace'
  | 'altitude' | 'cadence' | 'power' | 'temperature'

export type Mode = 'linear' | 'zones'

export interface ScaleStop {
  value: number
  color: string
}

export type ScaleDefinition =
  | { type: 'linear';   stops: ScaleStop[]; clamped: boolean; metric: Metric; unit: string }
  | { type: 'zones';    zoneHighs: number[]; zoneColors: string[]; metric: Metric; unit: string }
  | { type: 'constant'; color: string; metric: Metric }

// ─── valueToZone ──────────────────────────────────────────────────────────────

/**
 * Map a raw metric value to a zone index (1-based).
 * zone_highs semantics: inclusive upper bound per zone. Last zone may be Infinity
 * (9999 sentinel per ZoneEditor convention). Values above all boundaries map to
 * last zone index.
 */
export function valueToZone(value: number, zoneHighs: number[]): number {
  for (let i = 0; i < zoneHighs.length; i++) {
    if (value <= zoneHighs[i]) return i + 1
  }
  return zoneHighs.length
}

// ─── Stop generation for uniform palettes (Viridis) ───────────────────────────

function uniformStops(min: number, max: number, palette: readonly string[]): ScaleStop[] {
  if (palette.length < 2) throw new Error('Palette must have at least 2 stops')
  return palette.map((color, i) => ({
    value: min + (max - min) * (i / (palette.length - 1)),
    color,
  }))
}

// ─── resolveScale ─────────────────────────────────────────────────────────────

export interface ResolveScaleOptions {
  /** Default color for `sport` metric (from sportColors). */
  sportColor?: string
}

export function resolveScale(
  metric: Metric,
  motionProfile: MotionProfile,
  fullStreams: StreamPoint[],
  sportZones: SportZones | null,
  mode: Mode,
  opts: ResolveScaleOptions = {},
): ScaleDefinition {
  switch (metric) {
    case 'sport':
      return { type: 'constant', color: opts.sportColor ?? '#9ca3af', metric }

    case 'temperature': {
      return {
        type: 'linear',
        stops: DIVERGING_TEMP_STOPS.map(s => ({ value: s.value, color: s.color })),
        clamped: true,
        metric,
        unit: '°C',
      }
    }

    case 'altitude': {
      const values = fullStreams.map(s => s.altitude_m).filter((v): v is number => v != null)
      if (values.length === 0) return fallbackLinear(metric, 'm', 0, 100, VIRIDIS_9)
      const [min, max] = expandRange(Math.min(...values), Math.max(...values), ALTITUDE_MIN_SPREAD)
      return {
        type: 'linear',
        stops: uniformStops(min, max, VIRIDIS_9),
        clamped: false, // activity-relative, no clamping
        metric,
        unit: 'm',
      }
    }

    case 'speed': {
      if (mode === 'zones' && sportZones?.zones.speed) {
        return zonesScale(metric, 'km/h', sportZones.zones.speed.zone_highs.map(v => v * 3.6 /* mps→kmh */))
      }
      const p = SPEED_PROFILES[motionProfile.speed]
      return {
        type: 'linear',
        stops: uniformStops(p.minKmh, p.maxKmh, VIRIDIS_9),
        clamped: true, metric, unit: 'km/h',
      }
    }

    case 'pace': {
      const paceKey = motionProfile.pace ?? 'generic'
      const p = PACE_PROFILES[paceKey]
      return {
        type: 'linear',
        stops: uniformStops(p.minMinPerKm, p.maxMinPerKm, VIRIDIS_9),
        clamped: true, metric, unit: 'min/km',
      }
    }

    case 'cadence': {
      const p = CADENCE_PROFILES[motionProfile.cadence]
      const unit = motionProfile.cadence.startsWith('foot_') ? 'spm'
        : motionProfile.cadence === 'bike' ? 'rpm'
        : motionProfile.cadence === 'paddle' ? 'spm'
        : 'spm'
      return {
        type: 'linear',
        stops: uniformStops(p.min, p.max, VIRIDIS_9),
        clamped: true, metric, unit,
      }
    }

    case 'hr': {
      if (mode === 'zones' && sportZones?.zones.hr) {
        return zonesScale(metric, 'bpm', sportZones.zones.hr.zone_highs)
      }
      const values = fullStreams.map(s => s.heart_rate).filter((v): v is number => v != null)
      if (values.length === 0) return fallbackLinear(metric, 'bpm', 100, 180, VIRIDIS_9)
      const [min, max] = expandRange(Math.min(...values), Math.max(...values), HR_FALLBACK_MIN_SPREAD)
      return {
        type: 'linear',
        stops: uniformStops(min, max, VIRIDIS_9),
        clamped: false, metric, unit: 'bpm',
      }
    }

    case 'power': {
      if (mode === 'zones' && sportZones?.zones.power) {
        return zonesScale(metric, 'W', sportZones.zones.power.zone_highs)
      }
      const values = fullStreams.map(s => s.power_w).filter((v): v is number => v != null)
      if (values.length === 0) return fallbackLinear(metric, 'W', 0, 400, VIRIDIS_9)
      const [min, max] = expandRange(Math.min(...values), Math.max(...values), POWER_FALLBACK_MIN_SPREAD)
      return {
        type: 'linear',
        stops: uniformStops(min, max, VIRIDIS_9),
        clamped: false, metric, unit: 'W',
      }
    }
  }
}

function fallbackLinear(metric: Metric, unit: string, min: number, max: number, palette: readonly string[]): ScaleDefinition {
  return { type: 'linear', stops: uniformStops(min, max, palette), clamped: true, metric, unit }
}

export function zoneColors(count: number, base: readonly string[]): string[] {
  if (count < 1) throw new Error(`zoneColors: invalid count=${count}`)
  if (count === 1) return [base[Math.floor(base.length / 2)]]
  if (count <= base.length) return base.slice(0, count)
  return Array.from({ length: count }, (_, i) => {
    const t = i / (count - 1)
    const fIdx = t * (base.length - 1)
    const lo = Math.floor(fIdx), hi = Math.ceil(fIdx)
    return lo === hi ? base[lo] : interpolateHex(base[lo], base[hi], fIdx - lo)
  })
}

function zonesScale(metric: Metric, unit: string, zoneHighs: number[]): ScaleDefinition {
  const base =
    metric === 'hr'    ? HR_ZONE_BASE
    : metric === 'power' ? POWER_ZONE_BASE
    : SPEED_ZONE_BASE
  return {
    type: 'zones',
    zoneHighs: [...zoneHighs],
    zoneColors: zoneColors(zoneHighs.length, base),
    metric,
    unit,
  }
}

export { TEMPERATURE_RANGE } // re-export for component usage

// ─── Sport zone matching ──────────────────────────────────────────────────────

/**
 * Match an activity to its training-zones entry via fit_sport_id.
 *
 * Label-based fallback is intentionally NOT implemented — backend sport_label
 * is locale-dependent (deutsch) while frontend labels may differ for sub-sport
 * variants (e.g. "Gravel" vs backend-aggregated "Radfahren").
 *
 * Activities without fit_sport_id (pre-FIT-SDK-Phase-3 imports) cannot use
 * zone coloring — documented limitation.
 */
export function matchSportZones(
  activity: { fit_sport_id?: number | null },
  allZones: SportZones[],
): SportZones | null {
  if (activity.fit_sport_id == null) return null
  const matches = allZones.filter(z => z.sport === activity.fit_sport_id)
  if (matches.length === 0) return null
  return matches[0]
}

// ─── Pace derivation ──────────────────────────────────────────────────────────

/** Minimum speed (m/s) for defining pace — below this counts as stopped. */
export const PACE_NULL_THRESHOLD_MS = 0.5

export function derivePaceMinPerKm(speedMs: number | null | undefined): number | null {
  if (speedMs == null || speedMs < PACE_NULL_THRESHOLD_MS) return null
  // speedMs (m/s) → min/km: (1000 m / speedMs) / 60
  return 1000 / (speedMs * 60)
}

// ─── Aggregation over downsampling window ────────────────────────────────────

/**
 * Aggregate a window of raw values to one representative value.
 *
 * - Linear mode: arithmetic mean of non-null values; returns null if all null.
 * - Zones mode: majority vote over zone indices derived via valueToZone.
 *   Ties are broken by returning the lowest zone (deterministic, conservative).
 */
export function aggregateWindow(
  values: Array<number | null>,
  mode: Mode,
  zoneHighs?: number[],
): number | null {
  const defined = values.filter((v): v is number => v != null && !Number.isNaN(v))
  if (defined.length === 0) return null

  if (mode === 'linear') {
    return defined.reduce((s, v) => s + v, 0) / defined.length
  }

  // Zones — majority vote
  if (!zoneHighs || zoneHighs.length === 0) return null
  const counts = new Map<number, number>()
  for (const v of defined) {
    const z = valueToZone(v, zoneHighs)
    counts.set(z, (counts.get(z) ?? 0) + 1)
  }
  let bestZone = Infinity, bestCount = -1
  for (const [z, c] of counts) {
    if (c > bestCount || (c === bestCount && z < bestZone)) {
      bestZone = z
      bestCount = c
    }
  }
  return bestZone
}

// ─── Downsampling ─────────────────────────────────────────────────────────────

export interface DownsampleResult {
  points: StreamPoint[]
  streamIndices: number[]     // streamIndices[i] = original index of points[i]
  strides: number[]            // strides[i] = count of original points the segment i→i+1 covers
  /**
   * Aggregated metric value per segment (length = strides.length).
   * Linear mode: window mean. Zones mode: majority zone index.
   * Null when all window values were null.
   * For 'sport' metric this array is empty.
   */
  aggregated: Array<number | null>
}

export interface LngLatBoundsLike {
  west: number; south: number; east: number; north: number
}

/**
 * Uniform-stride downsampling of a stream. If `viewportBounds` is provided,
 * only points inside the bounds are considered.
 *
 * Returns points and metadata required to build segment features with
 * accurate segStartIdx/segEndIdx mapping.
 */
export function downsampleUniform(
  streams: StreamPoint[],
  metric: Metric,
  maxSegments: number,
  viewportBounds?: LngLatBoundsLike,
  mode: Mode = 'linear',
  zoneHighs?: number[],
): DownsampleResult {
  if (streams.length === 0) return { points: [], streamIndices: [], strides: [], aggregated: [] }

  // Filter by viewport if given
  const filteredIdx: number[] = []
  for (let i = 0; i < streams.length; i++) {
    const s = streams[i]
    if (s.lat == null || s.lng == null) continue
    if (viewportBounds) {
      if (s.lng < viewportBounds.west || s.lng > viewportBounds.east) continue
      if (s.lat < viewportBounds.south || s.lat > viewportBounds.north) continue
    }
    filteredIdx.push(i)
  }

  if (filteredIdx.length === 0) return { points: [], streamIndices: [], strides: [], aggregated: [] }

  // If ≤ maxSegments+1 points, no downsampling
  const targetPoints = maxSegments + 1
  if (filteredIdx.length <= targetPoints) {
    const points = filteredIdx.map(i => streams[i])
    const streamIndices = [...filteredIdx]
    const strides = points.slice(0, -1).map((_, i) => streamIndices[i + 1] - streamIndices[i])
    const aggregated = aggregateSegments(streams, streamIndices, strides, metric, mode, zoneHighs)
    return { points, streamIndices, strides, aggregated }
  }

  // Stride-based downsampling (ceil so stride >= len/target → result <= target)
  const stride = Math.max(1, Math.ceil(filteredIdx.length / targetPoints))
  const points: StreamPoint[] = []
  const streamIndices: number[] = []
  for (let i = 0; i < filteredIdx.length; i += stride) {
    const origIdx = filteredIdx[i]
    points.push(streams[origIdx])
    streamIndices.push(origIdx)
  }
  // Ensure last point is included (prevents truncated tail)
  const lastOrigIdx = filteredIdx[filteredIdx.length - 1]
  if (streamIndices[streamIndices.length - 1] !== lastOrigIdx) {
    points.push(streams[lastOrigIdx])
    streamIndices.push(lastOrigIdx)
  }
  const strides = points.slice(0, -1).map((_, i) => streamIndices[i + 1] - streamIndices[i])
  const aggregated = aggregateSegments(streams, streamIndices, strides, metric, mode, zoneHighs)
  return { points, streamIndices, strides, aggregated }
}

/** Collect raw values in each stride window and aggregate via aggregateWindow. */
function aggregateSegments(
  streams: StreamPoint[],
  streamIndices: number[],
  strides: number[],
  metric: Metric,
  mode: Mode,
  zoneHighs: number[] | undefined,
): Array<number | null> {
  if (metric === 'sport') return strides.map(() => null)
  return strides.map((stride, i) => {
    const startIdx = streamIndices[i]
    const endIdx = Math.min(streams.length - 1, startIdx + stride - 1)
    const values: Array<number | null> = []
    for (let j = startIdx; j <= endIdx; j++) {
      values.push(readMetricValueForAggregation(streams[j], metric))
    }
    return aggregateWindow(values, mode, zoneHighs)
  })
}

function readMetricValueForAggregation(s: StreamPoint, metric: Metric): number | null {
  switch (metric) {
    case 'sport':       return null
    case 'hr':          return s.heart_rate ?? null
    case 'speed':       return s.speed_ms != null ? s.speed_ms * 3.6 : null
    case 'pace':        return derivePaceMinPerKm(s.speed_ms)
    case 'altitude':    return s.altitude_m ?? null
    case 'cadence':     return s.cadence ?? null
    case 'power':       return s.power_w ?? null
    case 'temperature': return s.temperature_c ?? null
  }
}

// ─── Segment feature building ─────────────────────────────────────────────────

export function buildSegmentFeatures(
  ds: DownsampleResult,
  metric: Metric,
  scale: ScaleDefinition,
): GeoJSON.Feature[] {
  const features: GeoJSON.Feature[] = []
  for (let i = 0; i < ds.points.length - 1; i++) {
    const a = ds.points[i]
    const b = ds.points[i + 1]
    if (a.lat == null || a.lng == null || b.lat == null || b.lng == null) continue

    const segStartIdx = ds.streamIndices[i]
    const segEndIdx   = ds.streamIndices[i] + ds.strides[i] - 1

    // aggregated[i] is already mean-of-window (linear) or majority-zone (zones)
    const agg = metric === 'sport' ? null : (ds.aggregated[i] ?? null)
    const hasValue = agg != null && !Number.isNaN(agg)

    const properties: Record<string, unknown> = {
      segStartIdx,
      segEndIdx,
      inside: true,          // default; applyInsideFlag may override
      has_value: hasValue,
    }
    if (hasValue) {
      if (scale.type === 'zones') {
        // aggregated value in zones mode is already a zone index
        properties.zone = agg
      } else if (scale.type === 'linear') {
        properties.value = agg
      }
    }

    features.push({
      type: 'Feature',
      geometry: { type: 'LineString', coordinates: [[a.lng, a.lat], [b.lng, b.lat]] },
      properties,
    })
  }
  return features
}

/**
 * Apply the `inside` flag based on zoomRange overlap.
 * Does NOT mutate input features — returns new feature objects.
 *
 * Overlap formula (inclusive): segEndIdx >= zoomRange.startIdx && segStartIdx <= zoomRange.endIdx
 */
export function applyInsideFlag(
  features: GeoJSON.Feature[],
  zoomRange: ZoomRange | null,
): GeoJSON.Feature[] {
  if (!zoomRange) {
    // No brush active → all inside
    return features.map(f => ({
      ...f,
      properties: { ...(f.properties ?? {}), inside: true },
    }))
  }
  const { startIdx, endIdx } = zoomRange
  return features.map(f => {
    const p = f.properties ?? {}
    const segStart = (p as { segStartIdx?: number }).segStartIdx ?? 0
    const segEnd   = (p as { segEndIdx?:   number }).segEndIdx   ?? 0
    const inside = segEnd >= startIdx && segStart <= endIdx
    return { ...f, properties: { ...p, inside } }
  })
}

// ─── MapLibre expressions ─────────────────────────────────────────────────────

export type LineColorExpression =
  | string
  | (readonly [string, ...unknown[]])   // MapLibre ExpressionSpecification approximation

export function buildLineColorExpression(scale: ScaleDefinition): LineColorExpression {
  if (scale.type === 'constant') return scale.color

  if (scale.type === 'linear') {
    const interp: unknown[] = ['interpolate', ['linear'], ['get', 'value']]
    for (const { value, color } of scale.stops) {
      interp.push(value, color)
    }
    return ['case',
      ['!', ['get', 'has_value']], MISSING_COLOR,
      interp,
    ] as const
  }

  // zones
  const match: unknown[] = ['match', ['get', 'zone']]
  scale.zoneColors.forEach((color, i) => {
    match.push(i + 1, color)
  })
  match.push(MISSING_COLOR) // default
  return ['case',
    ['!', ['get', 'has_value']], MISSING_COLOR,
    match,
  ] as const
}

/**
 * Stable opacity expression — `inside` is always set by applyInsideFlag.
 * Inside=true → 0.85 (full); inside=false → 0.2 (dimmed during brush).
 */
export const LINE_OPACITY_EXPRESSION = ['case',
  ['==', ['get', 'inside'], true], 0.85, 0.2,
] as const
