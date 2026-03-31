// Prepares stream data for comparison charts:
// - LTTB downsampling for performance
// - Pace conversion for running sports
// - GPS point extraction for hover sync

import { useMemo } from 'react'
import type { StreamPoint } from '@/api/types'
import { defaultSpeedMode } from '@/lib/sportFieldLabels'

// ── LTTB downsampling ─────────────────────────────────────────────────────────
// Largest-Triangle-Three-Buckets algorithm — preserves visual shape.

function lttb(data: [number, number][], threshold: number): [number, number][] {
  const n = data.length
  if (n <= threshold || threshold <= 2) return data

  const sampled: [number, number][] = [data[0]]
  const every = (n - 2) / (threshold - 2)
  let a = 0

  for (let i = 0; i < threshold - 2; i++) {
    const avgRangeStart = Math.floor((i + 1) * every) + 1
    const avgRangeEnd = Math.min(Math.floor((i + 2) * every) + 1, n)
    let avgX = 0, avgY = 0
    const avgRangeLen = avgRangeEnd - avgRangeStart
    for (let j = avgRangeStart; j < avgRangeEnd; j++) {
      avgX += data[j][0]
      avgY += data[j][1]
    }
    avgX /= avgRangeLen
    avgY /= avgRangeLen

    const rangeStart = Math.floor(i * every) + 1
    const rangeEnd = Math.min(Math.floor((i + 1) * every) + 1, n)
    const [ax, ay] = data[a]
    let maxArea = -1, maxIdx = rangeStart
    for (let j = rangeStart; j < rangeEnd; j++) {
      const area = Math.abs((ax - avgX) * (data[j][1] - ay) - (ax - data[j][0]) * (avgY - ay))
      if (area > maxArea) { maxArea = area; maxIdx = j }
    }
    sampled.push(data[maxIdx])
    a = maxIdx
  }

  sampled.push(data[n - 1])
  return sampled
}

// ── Speed → pace conversion ───────────────────────────────────────────────────

/** Convert speed_ms to display value for charts (km/h or min/km depending on sport) */
function speedToDisplay(speedMs: number, sportType: string): number {
  const mode = defaultSpeedMode(sportType)
  if (mode === 'pace') {
    // min/km as decimal minutes — e.g. 4:30 → 4.5
    if (speedMs <= 0) return 0
    return 1000 / speedMs / 60
  }
  return speedMs * 3.6
}

// ── GPS point extraction ──────────────────────────────────────────────────────

export interface GpsPoint {
  elapsed: number
  lat: number
  lng: number
}

// ── Data extraction helpers ───────────────────────────────────────────────────

/** Pre-compute elapsed seconds for every stream point in one pass. */
function buildElapsedMap(streams: StreamPoint[]): Float64Array {
  const t0 = new Date(streams[0].time).getTime()
  const elapsed = new Float64Array(streams.length)
  for (let i = 0; i < streams.length; i++) {
    elapsed[i] = (new Date(streams[i].time).getTime() - t0) / 1000
  }
  return elapsed
}

function extractTimeSeries(
  streams: StreamPoint[],
  elapsed: Float64Array,
  getValue: (s: StreamPoint, elapsed: number) => number | null | undefined,
  downsampleTo = 1500,
): [number, number][] {
  const raw: [number, number][] = []
  for (let i = 0; i < streams.length; i++) {
    const v = getValue(streams[i], elapsed[i])
    if (v != null && isFinite(v) && v >= 0) raw.push([elapsed[i], v])
  }
  return lttb(raw, downsampleTo)
}

function extractGpsPoints(streams: StreamPoint[], elapsed: Float64Array): GpsPoint[] {
  const points: GpsPoint[] = []
  for (let i = 0; i < streams.length; i++) {
    const s = streams[i]
    if (s.lat != null && s.lng != null) {
      points.push({ elapsed: elapsed[i], lat: s.lat, lng: s.lng })
    }
  }
  return points
}

// ── Main hook ─────────────────────────────────────────────────────────────────

export interface PreparedStreamData {
  /** elapsed_s → bpm */
  hrData: [number, number][]
  /** elapsed_s → W */
  powerData: [number, number][]
  /** elapsed_s → altitude_m */
  altitudeData: [number, number][]
  /** elapsed_s → display speed (km/h or min/km) */
  speedData: [number, number][]
  /** elapsed_s → cadence */
  cadenceData: [number, number][]
  /** elapsed_s → °C */
  temperatureData: [number, number][]
  /** elapsed_s → ms */
  groundContactTimeData: [number, number][]
  /** elapsed_s → mm */
  verticalOscillationData: [number, number][]
  /** elapsed_s → % */
  stanceTimePercentData: [number, number][]
  /** elapsed_s → mm */
  stepLengthData: [number, number][]
  /** elapsed_s → % */
  leftRightBalanceData: [number, number][]
  /** GPS points for map hover markers */
  gpsPoints: GpsPoint[]
  /** Whether the sport uses pace (min/km) for speed display */
  isPaceMode: boolean
}

export function useStreamData(
  streams: StreamPoint[] | undefined,
  sportType: string,
): PreparedStreamData {
  return useMemo(() => {
    const s = streams ?? []
    const isPaceMode = defaultSpeedMode(sportType) === 'pace'

    if (s.length === 0) {
      return {
        hrData: [], powerData: [], altitudeData: [], speedData: [],
        cadenceData: [], temperatureData: [], groundContactTimeData: [],
        verticalOscillationData: [], stanceTimePercentData: [],
        stepLengthData: [], leftRightBalanceData: [],
        gpsPoints: [], isPaceMode,
      }
    }

    // Single-pass elapsed time pre-computation shared by all extractors.
    const elapsed = buildElapsedMap(s)

    return {
      hrData:                  extractTimeSeries(s, elapsed, p => p.heart_rate),
      powerData:               extractTimeSeries(s, elapsed, p => p.power_w),
      altitudeData:            extractTimeSeries(s, elapsed, p => p.altitude_m),
      speedData:               extractTimeSeries(s, elapsed, p => p.speed_ms != null ? speedToDisplay(p.speed_ms, sportType) : null),
      cadenceData:             extractTimeSeries(s, elapsed, p => p.cadence),
      temperatureData:         extractTimeSeries(s, elapsed, p => p.temperature_c),
      groundContactTimeData:   extractTimeSeries(s, elapsed, p => p.ground_contact_time_ms),
      verticalOscillationData: extractTimeSeries(s, elapsed, p => p.vertical_oscillation_mm),
      stanceTimePercentData:   extractTimeSeries(s, elapsed, p => p.stance_time_percent),
      stepLengthData:          extractTimeSeries(s, elapsed, p => p.step_length_mm),
      leftRightBalanceData:    extractTimeSeries(s, elapsed, p => p.left_right_balance),
      gpsPoints:               extractGpsPoints(s, elapsed),
      isPaceMode,
    }
  }, [streams, sportType])
}
