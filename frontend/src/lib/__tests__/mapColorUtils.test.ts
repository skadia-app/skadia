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
