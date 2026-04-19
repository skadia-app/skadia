import { describe, it, expect } from 'vitest'
import { buildChartData } from '../MapFullscreen'
import type { StreamPoint } from '@/api/types'

function sp(
  time: string,
  distance_m: number | null,
  altitude_m: number | null = null,
  heart_rate: number | null = null,
  power_w: number | null = null,
  cadence: number | null = null,
  speed_ms: number | null = null,
  temperature_c: number | null = null,
): StreamPoint {
  return { time, distance_m, altitude_m, heart_rate, power_w, cadence, speed_ms, temperature_c } as StreamPoint
}

describe('buildChartData — dedup and maxDist semantics', () => {
  it('collapses samples with identical distance_m (last-non-NaN wins)', () => {
    const streams: StreamPoint[] = [
      sp('2026-02-17T15:00:00Z', 100, 10, 120),
      sp('2026-02-17T15:00:01Z', 100, 11, 122),
      sp('2026-02-17T15:00:02Z', 200, 12, 124),
    ]
    const result = buildChartData(streams)
    expect(result.distanceKm).toEqual([0.1, 0.2])
    expect(result.altitude).toEqual([11, 12])
    expect(result.heartRate).toEqual([122, 124])
  })

  it('preserves NaN when a dedup-group has only NaN values', () => {
    const streams: StreamPoint[] = [
      sp('2026-02-17T15:00:00Z', 100, 10, null),
      sp('2026-02-17T15:00:01Z', 100, null, null),
    ]
    const result = buildChartData(streams)
    expect(result.distanceKm).toHaveLength(1)
    expect(result.altitude[0]).toBe(10) // non-NaN wins
    expect(isNaN(result.heartRate[0])).toBe(true) // all-NaN stays NaN
  })

  it('keeps single-source streams unchanged (no spurious collapse)', () => {
    const streams: StreamPoint[] = [
      sp('2026-02-17T15:00:00Z', 100, 10),
      sp('2026-02-17T15:00:01Z', 200, 12),
      sp('2026-02-17T15:00:02Z', 300, 14),
    ]
    const result = buildChartData(streams)
    expect(result.distanceKm).toEqual([0.1, 0.2, 0.3])
  })

  it('maxDist uses Math.ceil(Math.max()*10)/10 (non-monotonic tolerant)', () => {
    const streams: StreamPoint[] = [
      sp('2026-02-17T15:00:00Z', 100),
      sp('2026-02-17T15:00:01Z', 500),  // max here
      sp('2026-02-17T15:00:02Z', 300),  // non-monotonic dip
    ]
    const result = buildChartData(streams)
    expect(result.maxDist).toBeCloseTo(0.5) // 500m = 0.5km; ceil(0.5 * 10)/10 = 0.5
  })
})
