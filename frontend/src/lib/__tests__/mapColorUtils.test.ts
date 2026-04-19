// frontend/src/lib/__tests__/mapColorUtils.test.ts
import { describe, it, expect } from 'vitest'
import { valueToZone, resolveScale } from '../mapColorUtils'
import type { StreamPoint } from '@/api/types'

describe('valueToZone', () => {
  const zoneHighs = [155, 165, 175, 190, 9999]
  it('value at boundary maps to that zone (inclusive)', () => {
    expect(valueToZone(155, zoneHighs)).toBe(1)
    expect(valueToZone(165, zoneHighs)).toBe(2)
  })
  it('value between boundaries maps to next zone', () => {
    expect(valueToZone(160, zoneHighs)).toBe(2)
    expect(valueToZone(180, zoneHighs)).toBe(4)
  })
  it('value above all boundaries maps to last zone', () => {
    expect(valueToZone(200, zoneHighs)).toBe(5)
  })
  it('handles missing sentinel (no 9999 on end)', () => {
    expect(valueToZone(200, [155, 165, 175])).toBe(3)
  })
})

describe('resolveScale — linear', () => {
  const mkStream = (hr: number[]): StreamPoint[] =>
    hr.map((v, i) => ({ time: `2026-01-01T00:0${i}:00Z`, heart_rate: v }))

  it('linear HR without zones uses full-stream min/max with spread', () => {
    const scale = resolveScale('hr', { speed: 'running', cadence: 'foot_running' }, mkStream([140, 150]), null, 'linear')
    expect(scale.type).toBe('linear')
    if (scale.type === 'linear') {
      // max-min=10 < 30 spread → expand symmetrically
      expect(scale.stops[0].value).toBe(130)
      expect(scale.stops[scale.stops.length - 1].value).toBe(160)
    }
  })

  it('temperature uses diverging stops directly (not min/max)', () => {
    // Temperature needs temperature_c values, not heart_rate:
    const tempStreams: StreamPoint[] = [
      { time: '2026-01-01T00:00:00Z', temperature_c: 18 },
      { time: '2026-01-01T00:01:00Z', temperature_c: 20 },
    ]
    const scale = resolveScale('temperature', { speed: 'generic', cadence: 'generic' }, tempStreams, null, 'linear')
    expect(scale.type).toBe('linear')
    if (scale.type === 'linear') {
      expect(scale.stops[0].value).toBe(-10)
      expect(scale.stops[scale.stops.length - 1].value).toBe(35)
    }
  })

  it('altitude uses activity min/max with 50m spread', () => {
    const streams: StreamPoint[] = [
      { time: '2026-01-01T00:00:00Z', altitude_m: 500 },
      { time: '2026-01-01T00:01:00Z', altitude_m: 510 },
    ]
    const scale = resolveScale('altitude', { speed: 'generic', cadence: 'generic' }, streams, null, 'linear')
    if (scale.type === 'linear') {
      expect(scale.stops[0].value).toBe(480) // (500+510)/2 − 25
      expect(scale.stops[scale.stops.length - 1].value).toBe(530)
    }
  })
})

describe('resolveScale — sport default (constant)', () => {
  it('returns constant color for sport metric', () => {
    const scale = resolveScale('sport', { speed: 'running', cadence: 'foot_running' }, [], null, 'linear', { sportColor: '#22c55e' })
    expect(scale.type).toBe('constant')
    if (scale.type === 'constant') expect(scale.color).toBe('#22c55e')
  })
})

// ─── Task 6 tests ─────────────────────────────────────────────────────────────

import { derivePaceMinPerKm, downsampleUniform, aggregateWindow } from '../mapColorUtils'

describe('derivePaceMinPerKm', () => {
  it('returns null for speed below threshold', () => {
    expect(derivePaceMinPerKm(0.3)).toBeNull()
    expect(derivePaceMinPerKm(null)).toBeNull()
    expect(derivePaceMinPerKm(undefined)).toBeNull()
  })
  it('converts 5.556 m/s to 3 min/km', () => {
    expect(derivePaceMinPerKm(5.556)).toBeCloseTo(3.0, 2)
  })
  it('converts 2.778 m/s to 6 min/km', () => {
    expect(derivePaceMinPerKm(2.778)).toBeCloseTo(6.0, 2)
  })
})

describe('aggregateWindow', () => {
  it('linear: returns arithmetic mean ignoring null', () => {
    expect(aggregateWindow([100, 110, null, 120] as (number | null)[], 'linear')).toBe(110)
  })
  it('linear: returns null when all values are null', () => {
    expect(aggregateWindow([null, null] as (number | null)[], 'linear')).toBeNull()
  })
  it('zones: returns most-frequent zone (majority vote)', () => {
    // values 150,155,160 → zones via zoneHighs [155,165] → zones 1,1,2 → majority 1
    expect(aggregateWindow([150, 155, 160] as (number | null)[], 'zones', [155, 165])).toBe(1)
  })
  it('zones: tie broken deterministically (lowest zone wins)', () => {
    // values 150,160 → zones 1,2 → tie → return 1
    expect(aggregateWindow([150, 160] as (number | null)[], 'zones', [155, 165])).toBe(1)
  })
  it('zones: returns null when all values are null', () => {
    expect(aggregateWindow([null, null] as (number | null)[], 'zones', [155])).toBeNull()
  })

  // #11: 3-way tie → lowest zone wins
  it('zones: 3-way tie → lowest zone wins deterministically', () => {
    // values 150,160,170 → zones via zoneHighs [155,165,175] → zones 1,2,3 each count=1
    expect(aggregateWindow([150, 160, 170] as (number | null)[], 'zones', [155, 165, 175])).toBe(1)
  })

  // #29b: NaN handling
  it('linear: ignores NaN values', () => {
    expect(aggregateWindow([100, NaN, 120] as (number | null)[], 'linear')).toBe(110)
  })
  it('zones: ignores NaN values', () => {
    expect(aggregateWindow([150, NaN, 160] as (number | null)[], 'zones', [155, 165])).toBe(1)
  })
})

describe('downsampleUniform', () => {
  const mkStreams = (n: number): StreamPoint[] => Array.from({ length: n }, (_, i) => ({
    time: `2026-01-01T00:00:${String(i).padStart(2, '0')}Z`,
    heart_rate: 100 + i,
    lat: 47.0 + i * 0.001,
    lng: 11.0 + i * 0.001,
  }))

  it('returns all points when n ≤ maxSegs', () => {
    const r = downsampleUniform(mkStreams(50), 'hr', 2000)
    expect(r.points).toHaveLength(50)
    expect(r.streamIndices).toEqual(Array.from({ length: 50 }, (_, i) => i))
    expect(r.strides).toHaveLength(49) // segments = points - 1
    expect(r.strides.every(s => s === 1)).toBe(true)
  })

  it('downsamples N=2000 to maxSegs=1000 with stride 2', () => {
    const r = downsampleUniform(mkStreams(2000), 'hr', 1000)
    expect(r.points.length).toBeLessThanOrEqual(1001)
    expect(r.strides[0]).toBe(2)
  })

  it('handles empty streams', () => {
    const r = downsampleUniform([], 'hr', 2000)
    expect(r.points).toHaveLength(0)
    expect(r.strides).toHaveLength(0)
  })

  it('last segment stride may be smaller', () => {
    const r = downsampleUniform(mkStreams(21), 'hr', 10)
    // stride = floor(21/10) = 2, so 11 points sampled, 10 segments; last stride may be 1
    expect(r.strides.length).toBe(r.points.length - 1)
  })

  // #10: single-point edge case
  it('handles single-point streams (returns 1 point, 0 segments)', () => {
    const single: StreamPoint[] = [{ time: 't0', heart_rate: 120, lat: 47, lng: 11 }]
    const r = downsampleUniform(single, 'hr', 2000)
    expect(r.points).toHaveLength(1)
    expect(r.strides).toHaveLength(0)
    expect(r.aggregated).toHaveLength(0)
  })

  // #29a: viewport bounds filtering
  it('downsampleUniform filters by viewportBounds', () => {
    const streams: StreamPoint[] = [
      { time: 't0', lat: 47.0, lng: 11.0, heart_rate: 120 },
      { time: 't1', lat: 48.0, lng: 12.0, heart_rate: 130 }, // outside
      { time: 't2', lat: 47.1, lng: 11.1, heart_rate: 140 },
    ]
    const bounds = { west: 10.9, south: 46.9, east: 11.2, north: 47.2 }
    const r = downsampleUniform(streams, 'hr', 2000, bounds)
    expect(r.points).toHaveLength(2)  // indices 0 and 2 only
    expect(r.streamIndices).toEqual([0, 2])
  })
})

// ─── Task 7 tests ─────────────────────────────────────────────────────────────

import { buildSegmentFeatures, applyInsideFlag } from '../mapColorUtils'
import type { ZoomRange } from '@/components/ActivityCharts'

describe('buildSegmentFeatures', () => {
  const streams: StreamPoint[] = [
    { time: 't0', heart_rate: 120, lat: 47.0, lng: 11.0 },
    { time: 't1', heart_rate: 140, lat: 47.1, lng: 11.1 },
    { time: 't2', heart_rate: 160, lat: 47.2, lng: 11.2 },
  ]
  const downsampled = { points: streams, streamIndices: [0, 1, 2], strides: [1, 1], aggregated: [120, 140] }
  const scale = { type: 'linear' as const, stops: [{ value: 100, color: '#000' }, { value: 200, color: '#fff' }], clamped: true, metric: 'hr' as const, unit: 'bpm' }

  it('creates N-1 segment features', () => {
    const features = buildSegmentFeatures(downsampled, 'hr', scale)
    expect(features).toHaveLength(2)
  })

  it('each feature has segStartIdx and segEndIdx', () => {
    const features = buildSegmentFeatures(downsampled, 'hr', scale)
    expect(features[0].properties).toMatchObject({ segStartIdx: 0, segEndIdx: 0, inside: true })
    expect(features[1].properties).toMatchObject({ segStartIdx: 1, segEndIdx: 1, inside: true })
  })

  it('uses aggregated value (not start-sample) for linear mode', () => {
    const features = buildSegmentFeatures(downsampled, 'hr', scale)
    expect(features[0].properties?.has_value).toBe(true)
    expect(features[0].properties?.value).toBe(120) // from aggregated[0]
    expect(features[1].properties?.value).toBe(140) // from aggregated[1]
  })

  it('has_value=false when aggregated value is null (all nulls in window)', () => {
    const streamsMissing: StreamPoint[] = [
      { time: 't0', lat: 47.0, lng: 11.0 /* no hr */ },
      { time: 't1', lat: 47.1, lng: 11.1 /* no hr */ },
    ]
    const ds = { points: streamsMissing, streamIndices: [0, 1], strides: [1], aggregated: [null] }
    const features = buildSegmentFeatures(ds, 'hr', scale)
    expect(features[0].properties?.has_value).toBe(false)
  })

  it('skips segments exceeding gapThresholdM (GPS teleport filter)', () => {
    const gapStreams: StreamPoint[] = [
      { time: 't0', heart_rate: 120, lat: 47.0, lng: 11.0 },
      { time: 't1', heart_rate: 140, lat: 48.0, lng: 12.0 }, // >100km jump
      { time: 't2', heart_rate: 160, lat: 48.0, lng: 12.001 },
    ]
    const ds = { points: gapStreams, streamIndices: [0, 1, 2], strides: [1, 1], aggregated: [120, 140] }
    const features = buildSegmentFeatures(ds, 'hr', scale, 2000) // 2 km threshold
    // Segment 0→1 spans >100km → skipped; segment 1→2 spans ~80m → kept
    expect(features).toHaveLength(1)
    expect(features[0].properties?.value).toBe(140)
  })

  it('keeps all segments when gapThresholdM is undefined', () => {
    const features = buildSegmentFeatures(downsampled, 'hr', scale)
    expect(features).toHaveLength(2)
  })

  it('keeps all segments when gapThresholdM is 0 (disabled)', () => {
    const features = buildSegmentFeatures(downsampled, 'hr', scale, 0)
    expect(features).toHaveLength(2)
  })
})

describe('applyInsideFlag', () => {
  const features: GeoJSON.Feature[] = [
    { type: 'Feature', geometry: { type: 'LineString', coordinates: [[0, 0], [1, 1]] }, properties: { segStartIdx: 0, segEndIdx: 5, inside: true, has_value: true, value: 100 } },
    { type: 'Feature', geometry: { type: 'LineString', coordinates: [[1, 1], [2, 2]] }, properties: { segStartIdx: 6, segEndIdx: 10, inside: true, has_value: true, value: 100 } },
    { type: 'Feature', geometry: { type: 'LineString', coordinates: [[2, 2], [3, 3]] }, properties: { segStartIdx: 11, segEndIdx: 15, inside: true, has_value: true, value: 100 } },
  ]

  it('null zoomRange → all inside=true', () => {
    const r = applyInsideFlag(features, null)
    expect(r.map(f => f.properties?.inside)).toEqual([true, true, true])
  })

  it('zoomRange [3, 8] → overlap on segments 0 and 1, not 2', () => {
    const zoomRange: ZoomRange = { startIdx: 3, endIdx: 8, startKm: 0, endKm: 0, startPct: 0, endPct: 0 }
    const r = applyInsideFlag(features, zoomRange)
    expect(r.map(f => f.properties?.inside)).toEqual([true, true, false])
  })

  it('boundary match on startIdx=5 includes segment 0 (segEndIdx=5)', () => {
    const zoomRange: ZoomRange = { startIdx: 5, endIdx: 5, startKm: 0, endKm: 0, startPct: 0, endPct: 0 }
    const r = applyInsideFlag(features, zoomRange)
    expect(r[0].properties?.inside).toBe(true)
    expect(r[1].properties?.inside).toBe(false)
    expect(r[2].properties?.inside).toBe(false)
  })

  it('does not mutate input features', () => {
    const original = JSON.parse(JSON.stringify(features))
    applyInsideFlag(features, { startIdx: 3, endIdx: 8, startKm: 0, endKm: 0, startPct: 0, endPct: 0 })
    expect(features).toEqual(original)
  })
})

// ─── Task 8 tests ─────────────────────────────────────────────────────────────

import { buildLineColorExpression, LINE_OPACITY_EXPRESSION } from '../mapColorUtils'

describe('buildLineColorExpression', () => {
  it('linear: builds case-wrapped interpolate', () => {
    const scale = { type: 'linear' as const, stops: [{ value: 0, color: '#000' }, { value: 100, color: '#fff' }], clamped: true, metric: 'hr' as const, unit: 'bpm' }
    const expr = buildLineColorExpression(scale) as unknown as unknown[]
    expect(expr[0]).toBe('case')
    // index 1 is the missing-value guard
    expect(expr[1]).toEqual(['!', ['get', 'has_value']])
  })

  it('zones: builds case-wrapped match', () => {
    const scale = { type: 'zones' as const, zoneHighs: [150, 160], zoneColors: ['#111', '#222'], metric: 'hr' as const, unit: 'bpm' }
    const expr = buildLineColorExpression(scale) as unknown as unknown[]
    expect(expr[0]).toBe('case')
    // The inner match expression:
    const matchExpr = expr[expr.length - 1] as unknown[]
    expect(matchExpr[0]).toBe('match')
  })

  it('constant: returns plain color string', () => {
    const scale = { type: 'constant' as const, color: '#22c55e', metric: 'sport' as const }
    const expr = buildLineColorExpression(scale)
    expect(expr).toBe('#22c55e')
  })
})

describe('LINE_OPACITY_EXPRESSION', () => {
  it('is well-formed', () => {
    const expr = LINE_OPACITY_EXPRESSION as unknown as unknown[]
    expect(expr[0]).toBe('case')
    // [case, [==, [get, inside], true], 0.85, 0.2]
    expect(expr[2]).toBe(0.85)
    expect(expr[3]).toBe(0.2)
  })
})

// ─── Task 15 tests ────────────────────────────────────────────────────────────

import { matchSportZones } from '../mapColorUtils'
import type { SportZones } from '@/api/trainingZones'

describe('matchSportZones', () => {
  const zones: SportZones[] = [
    { sport: 1, sport_label: 'Laufen', zones: { hr: null, power: null, speed: null } },
    { sport: 2, sport_label: 'Radfahren', zones: { hr: null, power: null, speed: null } },
  ]

  it('matches by fit_sport_id', () => {
    expect(matchSportZones({ fit_sport_id: 2 }, zones)?.sport).toBe(2)
  })

  it('returns null when fit_sport_id is missing', () => {
    expect(matchSportZones({}, zones)).toBeNull()
  })

  it('returns null when fit_sport_id does not match any zone', () => {
    expect(matchSportZones({ fit_sport_id: 99 }, zones)).toBeNull()
  })
})

// ─── Task 16 tests ────────────────────────────────────────────────────────────

import { zoneColors } from '../mapColorUtils'
import { HR_ZONE_BASE, POWER_ZONE_BASE } from '../colorPalettes'

describe('zoneColors', () => {
  it('3 zones: first 3 stops of base', () => {
    const r = zoneColors(3, HR_ZONE_BASE)
    expect(r).toHaveLength(3)
    expect(r).toEqual([HR_ZONE_BASE[0], HR_ZONE_BASE[1], HR_ZONE_BASE[2]])
  })

  it('6 zones: exact base palette', () => {
    const r = zoneColors(6, HR_ZONE_BASE)
    expect(r).toHaveLength(6)
  })

  it('8 power zones: uses full POWER_ZONE_BASE (8 stops)', () => {
    const r = zoneColors(8, POWER_ZONE_BASE)
    expect(r).toHaveLength(8)
  })

  it('10 zones with 6-stop base: interpolated', () => {
    const r = zoneColors(10, HR_ZONE_BASE)
    expect(r).toHaveLength(10)
    // All valid hex
    r.forEach(c => expect(c).toMatch(/^#[0-9a-f]{6}$/i))
  })

  it('1 zone: middle stop', () => {
    const r = zoneColors(1, HR_ZONE_BASE)
    expect(r).toHaveLength(1)
  })

  it('0 zones throws', () => {
    expect(() => zoneColors(0, HR_ZONE_BASE)).toThrow()
  })
})

// ─── Cadence activity-relative scale ─────────────────────────────────────────

describe('resolveScale — cadence (activity-relative)', () => {
  it('cadence uses activity min/max with CADENCE_MIN_SPREAD', () => {
    // Stream with 165-175 spm (10 spread, under 20) → should expand to 20 spread
    const streams: StreamPoint[] = [
      { time: 't0', cadence: 165 },
      { time: 't1', cadence: 170 },
      { time: 't2', cadence: 175 },
    ] as StreamPoint[]
    const scale = resolveScale('cadence', { speed: 'running', pace: 'running', cadence: 'foot_running' }, streams, null, 'linear')
    expect(scale.type).toBe('linear')
    if (scale.type === 'linear') {
      // (165+175)/2 = 170, spread 20 → 160..180
      expect(scale.stops[0].value).toBe(160)
      expect(scale.stops[scale.stops.length - 1].value).toBe(180)
      expect(scale.unit).toBe('spm')
    }
  })

  it('cadence falls back to profile when stream has no cadence data', () => {
    const streams: StreamPoint[] = [{ time: 't0' }] as StreamPoint[]
    const scale = resolveScale('cadence', { speed: 'roadCycling', cadence: 'bike' }, streams, null, 'linear')
    if (scale.type === 'linear') {
      expect(scale.unit).toBe('rpm')
      expect(scale.stops[0].value).toBe(0)     // bike profile min=0
      expect(scale.stops[scale.stops.length - 1].value).toBe(120)  // bike profile max=120
    }
  })
})

// ─── Finding #29c: resolveScale zones+speed with m/s → km/h conversion ───────

describe('resolveScale — zones speed (m/s → km/h)', () => {
  it('resolveScale zones mode for speed converts m/s zone_highs to km/h', () => {
    const zones: SportZones = {
      sport: 2, sport_label: 'Cycling',
      zones: {
        hr: null,
        power: null,
        speed: { zone_highs: [10, 15, 20, 25], threshold_speed_mps: 10, source_type: 'manual', zone_names: ['Z1', 'Z2', 'Z3', 'Z4'] } as any,
      },
    }
    const scale = resolveScale('speed', { speed: 'roadCycling', cadence: 'bike' }, [], zones, 'zones')
    expect(scale.type).toBe('zones')
    if (scale.type === 'zones') {
      // 10 m/s * 3.6 = 36 km/h, etc.
      expect(scale.zoneHighs).toEqual([36, 54, 72, 90])
      expect(scale.zoneColors).toHaveLength(4)
      expect(scale.unit).toBe('km/h')
    }
  })
})
