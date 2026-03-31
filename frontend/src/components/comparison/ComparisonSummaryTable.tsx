// ComparisonSummaryTable — N-column summary statistics per visible source.
// Best-value per row is highlighted (bold).
// When a timeRange is set in the comparison store, stats are computed for
// that range only using the source's stream data.

import { useMemo } from 'react'
import { useSources } from './SourcesContext'
import { useComparisonStore } from './useComparisonStore'
import type { Activity, StreamPoint } from '@/api/types'
import { formatDuration, formatDistance } from '@/lib/utils'
import { formatSpeedValue, defaultSpeedMode, formatPace } from '@/lib/sportFieldLabels'

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmtHr(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} bpm`
}

function fmtPower(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} W`
}

function fmtElevation(v: number | null | undefined): string {
  if (v == null || isNaN(v) || v <= 0) return '\u2014'
  return `${Math.round(v)} m`
}

function nanFallback(v: number): number | null {
  return isNaN(v) ? null : v
}

// Average of values (ignores NaN)
function avg(vals: number[]): number | null {
  const valid = vals.filter(v => !isNaN(v) && isFinite(v))
  if (valid.length === 0) return null
  return valid.reduce((a, b) => a + b, 0) / valid.length
}

// Maximum of values (ignores NaN)
function max(vals: number[]): number | null {
  const valid = vals.filter(v => !isNaN(v) && isFinite(v))
  if (valid.length === 0) return null
  return Math.max(...valid)
}

// ── Stream-range statistics ───────────────────────────────────────────────────

interface RangeStats {
  avgSpeedMs: number | null
  avgHr: number | null
  maxHr: number | null
  avgPower: number | null
  avgCadence: number | null
}

function computeRangeStats(
  streams: StreamPoint[] | undefined,
  timeRange: [number, number] | null,
): RangeStats {
  if (!streams || streams.length === 0) {
    return { avgSpeedMs: null, avgHr: null, maxHr: null, avgPower: null, avgCadence: null }
  }

  const t0 = new Date(streams[0].time).getTime()
  const filtered = timeRange
    ? streams.filter(s => {
        const elapsed = (new Date(s.time).getTime() - t0) / 1000
        return elapsed >= timeRange[0] && elapsed <= timeRange[1]
      })
    : streams

  return {
    avgSpeedMs: nanFallback(avg(filtered.map(s => s.speed_ms ?? NaN)) ?? NaN),
    avgHr:      nanFallback(avg(filtered.map(s => s.heart_rate ?? NaN)) ?? NaN),
    maxHr:      nanFallback(max(filtered.map(s => s.heart_rate ?? NaN)) ?? NaN),
    avgPower:   nanFallback(avg(filtered.map(s => s.power_w ?? NaN)) ?? NaN),
    avgCadence: nanFallback(avg(filtered.map(s => s.cadence ?? NaN)) ?? NaN),
  }
}

// ── Metric row definitions ────────────────────────────────────────────────────

interface MetricValue {
  display: string
  raw: number | null
}

interface SourceMetrics {
  distance: MetricValue
  duration: MetricValue
  elevationGain: MetricValue
  avgSpeed: MetricValue
  avgHr: MetricValue
  maxHr: MetricValue
  avgPower: MetricValue
  np: MetricValue
  avgCadence: MetricValue
}

const METRIC_LABELS: Array<{ key: keyof SourceMetrics; label: string; higherIsBetter?: boolean }> = [
  { key: 'distance',     label: 'Distanz',         higherIsBetter: true },
  { key: 'duration',     label: 'Dauer',            higherIsBetter: false },
  { key: 'elevationGain',label: 'Höhenmeter',       higherIsBetter: true },
  { key: 'avgSpeed',     label: 'Ø Geschwindigkeit',higherIsBetter: true },
  { key: 'avgHr',        label: 'Ø Herzfrequenz',   higherIsBetter: false },
  { key: 'maxHr',        label: 'Max. HF',          higherIsBetter: false },
  { key: 'avgPower',     label: 'Ø Leistung',       higherIsBetter: true },
  { key: 'np',           label: 'NP',               higherIsBetter: true },
  { key: 'avgCadence',   label: 'Ø Kadenz',         higherIsBetter: false },
]

function buildSourceMetrics(
  src: { activity?: Activity; streams?: StreamPoint[] },
  timeRange: [number, number] | null,
  sportType: string,
): SourceMetrics {
  const a = src.activity
  const rangeStats = computeRangeStats(src.streams, timeRange)
  const usingRange = timeRange !== null

  // Speed display
  const effectiveSpeedMs = usingRange ? (rangeStats.avgSpeedMs ?? a?.avg_speed_ms) : a?.avg_speed_ms
  const speedDisplay = effectiveSpeedMs != null && effectiveSpeedMs > 0
    ? formatSpeedValue(effectiveSpeedMs, sportType)
    : '\u2014'

  return {
    distance: {
      display: a?.distance_m != null ? formatDistance(a.distance_m) : '\u2014',
      raw: a?.distance_m ?? null,
    },
    duration: {
      display: a?.elapsed_time_s != null ? formatDuration(a.elapsed_time_s) : '\u2014',
      raw: a?.elapsed_time_s ?? null,
    },
    elevationGain: {
      display: fmtElevation(a?.elevation_gain_m),
      raw: a?.elevation_gain_m ?? null,
    },
    avgSpeed: {
      display: speedDisplay,
      raw: effectiveSpeedMs ?? null,
    },
    avgHr: {
      display: fmtHr(usingRange ? rangeStats.avgHr : a?.avg_heart_rate),
      raw: usingRange ? rangeStats.avgHr : (a?.avg_heart_rate ?? null),
    },
    maxHr: {
      display: fmtHr(usingRange ? rangeStats.maxHr : a?.max_heart_rate),
      raw: usingRange ? rangeStats.maxHr : (a?.max_heart_rate ?? null),
    },
    avgPower: {
      display: fmtPower(usingRange ? rangeStats.avgPower : a?.avg_power_w),
      raw: usingRange ? rangeStats.avgPower : (a?.avg_power_w ?? null),
    },
    np: {
      display: fmtPower(a?.normalized_power_w),
      raw: a?.normalized_power_w ?? null,
    },
    avgCadence: {
      display: usingRange && rangeStats.avgCadence != null
        ? `${Math.round(rangeStats.avgCadence)} rpm`
        : a?.avg_cadence != null ? `${Math.round(a.avg_cadence)} rpm` : '\u2014',
      raw: usingRange ? rangeStats.avgCadence : (a?.avg_cadence ?? null),
    },
  }
}

function findBestIdx(
  metrics: SourceMetrics[],
  key: keyof SourceMetrics,
  higherIsBetter?: boolean,
): number | null {
  const values = metrics.map(m => m[key].raw)
  const validIdxs = values
    .map((v, i) => ({ v, i }))
    .filter(({ v }) => v != null && !isNaN(v as number))
  if (validIdxs.length < 2) return null
  const hib = higherIsBetter ?? false
  return validIdxs.reduce((best, curr) =>
    hib
      ? ((curr.v as number) > (best.v as number) ? curr : best)
      : ((curr.v as number) < (best.v as number) ? curr : best)
  ).i
}

// ── Component ─────────────────────────────────────────────────────────────────

interface ComparisonSummaryTableProps {
  /** Label for the first column header (default: 'Metrik') */
  metricColumnLabel?: string
}

export default function ComparisonSummaryTable({
  metricColumnLabel = 'Metrik',
}: ComparisonSummaryTableProps) {
  const { visibleSources } = useSources()
  const timeRange = useComparisonStore(s => s.timeRange)

  const primarySportType = visibleSources[0]?.activity?.sport_type ?? 'generic'

  const metrics = useMemo<SourceMetrics[]>(
    () => visibleSources.map(src => buildSourceMetrics(src, timeRange, primarySportType)),
    [visibleSources, timeRange, primarySportType]
  )

  if (visibleSources.length === 0) return null

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      {timeRange && (
        <div className="px-4 py-2 text-xs text-neutral-400 dark:text-neutral-500 bg-neutral-50 dark:bg-neutral-900/50 border-b border-neutral-200 dark:border-neutral-800">
          Statistiken für ausgewählten Zeitbereich
        </div>
      )}
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
            <th className="text-left px-4 py-3 font-medium text-neutral-500 dark:text-neutral-400 w-36">
              {metricColumnLabel}
            </th>
            {visibleSources.map((src) => (
              <th key={src.id} className="text-left px-4 py-3">
                <div className="flex items-center gap-2">
                  <div
                    className="w-2.5 h-2.5 rounded-full flex-shrink-0"
                    style={{ backgroundColor: src.color }}
                  />
                  <span className="font-medium text-neutral-800 dark:text-neutral-200 truncate text-sm">
                    {src.label}
                  </span>
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
          {METRIC_LABELS.map(({ key, label, higherIsBetter }) => {
            const bestIdx = findBestIdx(metrics, key, higherIsBetter)
            return (
              <tr key={key}>
                <td className="px-4 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium">
                  {label}
                </td>
                {metrics.map((m, i) => (
                  <td
                    key={visibleSources[i]?.id ?? i}
                    className={`px-4 py-2.5 tabular-nums ${
                      bestIdx === i
                        ? 'text-neutral-900 dark:text-neutral-100 font-semibold'
                        : 'text-neutral-600 dark:text-neutral-400'
                    }`}
                  >
                    {m[key].display}
                  </td>
                ))}
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
