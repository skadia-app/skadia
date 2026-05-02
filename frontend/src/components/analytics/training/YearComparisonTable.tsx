import { Compass } from 'lucide-react'
import type { YearProgressionYearData } from '@/api/analytics'
import type { MetricKey } from '@/hooks/useYearProgression'

// Metric display metadata — mirrors METRIC_OPTIONS in YearProgressionChart.
const METRIC_META: Record<MetricKey, { unit: string; factor: number; decimals: number }> = {
  distance_m:      { unit: 'km',   factor: 0.001,    decimals: 1 },
  elevation_gain_m:{ unit: 'm',    factor: 1,         decimals: 0 },
  moving_time_s:   { unit: 'h',    factor: 1 / 3600,  decimals: 1 },
  tss:             { unit: 'TSS',  factor: 1,         decimals: 0 },
  total_calories:  { unit: 'kcal', factor: 1,         decimals: 0 },
}

// ECharts palette, same order as YearProgressionChart so pills match lines.
const YEAR_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
  '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16',
]

const MONTH_NAMES_DE = [
  'Januar', 'Februar', 'März', 'April', 'Mai', 'Juni',
  'Juli', 'August', 'September', 'Oktober', 'November', 'Dezember',
]

// Months are 0-indexed throughout (Jan = 0).
const MONTH_LAST_DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]

// Returns month index (0-based) of the last fully completed month before `date`.
// If date is the 1st of a month, returns the previous month.
export function lockedMonthIndex(date: Date): number {
  const day = date.getDate()
  const month = date.getMonth()
  if (day <= 1) {
    // On the 1st, the current month has no full data yet — use previous month.
    return month === 0 ? 11 : month - 1
  }
  // On any later day, the previous month is fully completed.
  return month === 0 ? 11 : month - 1
}

// Cumulative sum up to and including `upToMonth` (0-based, inclusive).
// Uses the `cumulative` array from the API if available (avoids re-summing),
// otherwise falls back to summing `monthly`.
function cumulativeUpTo(yearData: YearProgressionYearData, upToMonth: number): number {
  if (yearData.cumulative?.length > upToMonth) {
    return yearData.cumulative[upToMonth]
  }
  return yearData.monthly.slice(0, upToMonth + 1).reduce((a, b) => a + b, 0)
}

// Format a value using the metric's factor + unit.
function fmt(raw: number, meta: { unit: string; factor: number; decimals: number }): string {
  return `${(raw * meta.factor).toFixed(meta.decimals)} ${meta.unit}`
}

// Format a delta value with + / - sign.
function fmtDelta(diff: number, meta: { unit: string; factor: number; decimals: number }): string {
  const converted = diff * meta.factor
  const abs = Math.abs(converted).toFixed(meta.decimals)
  const sign = diff >= 0 ? '+' : '−'
  return `${sign}${abs} ${meta.unit}`
}

function deltaColor(diff: number): string {
  if (diff > 0) return 'text-emerald-600 dark:text-emerald-400'
  if (diff < 0) return 'text-red-500 dark:text-red-400'
  return 'text-neutral-400'
}

// Header date range: "1. Januar → 30. April" (German, day-granular names).
function buildHeaderLabel(upToMonth: number | null): string {
  if (upToMonth === null) {
    // No lock — show full year range.
    return `1. Januar → 31. Dezember`
  }
  const lastDay = MONTH_LAST_DAYS[upToMonth]
  return `1. Januar → ${lastDay}. ${MONTH_NAMES_DE[upToMonth]}`
}

export interface YearComparisonTableProps {
  years: YearProgressionYearData[]
  // Color index matches the year's position in the full dataset (not selectedYears subset).
  yearColorIndex: Map<number, number>
  metric: MetricKey
  // null = no lock (show full year total). Otherwise: date clicked in chart.
  lockedDate: Date | null
  onLockedDateChange: (date: Date | null) => void
  // The current calendar year — its own Δ-with-current-year is shown as 'Ø'.
  currentYear?: number
}

export default function YearComparisonTable({
  years,
  yearColorIndex,
  metric,
  lockedDate,
  onLockedDateChange,
  currentYear = new Date().getFullYear(),
}: YearComparisonTableProps) {
  const meta = METRIC_META[metric]

  // Determine up-to-month index (0-based) from lockedDate.
  const upToMonth: number | null = lockedDate !== null
    ? lockedMonthIndex(lockedDate)
    : null

  // Compute values for each year.
  const rows = years.map((yd) => {
    const value = upToMonth !== null
      ? cumulativeUpTo(yd, upToMonth)
      : yd.total
    return { year: yd.year, value }
  })

  // Δ prev. year: diff to the chronologically prior year in the list.
  // Δ with current year: diff to the current calendar year's value.
  const currentYearRow = rows.find(r => r.year === currentYear)
  const currentYearValue = currentYearRow?.value ?? 0

  const headerLabel = buildHeaderLabel(upToMonth)

  function handleRowClick(month: number) {
    // Row click locks to that month's end: use last day of the month.
    const now = new Date()
    const year = now.getFullYear()
    const lastDay = MONTH_LAST_DAYS[month]
    onLockedDateChange(new Date(year, month, lastDay))
  }

  return (
    <div className="flex flex-col gap-2">
      {/* Header with date range + lock-reset button */}
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
          {headerLabel}
        </span>
        {lockedDate !== null && (
          <button
            type="button"
            aria-label="Datumslock zurücksetzen"
            onClick={() => onLockedDateChange(null)}
            className="flex items-center gap-1 text-xs text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200 transition-colors cursor-pointer"
          >
            <Compass size={12} />
            <span>Zurücksetzen</span>
          </button>
        )}
      </div>

      {/* Hint when no lock is active */}
      {lockedDate === null && (
        <p className="text-[10px] text-neutral-400 dark:text-neutral-600 italic">
          Klicke auf eine Zeile um einen Monatsstand zu sperren
        </p>
      )}

      {/* Table */}
      <div className="overflow-auto">
        <table className="w-full text-xs border-collapse">
          <thead>
            <tr className="text-neutral-500 dark:text-neutral-500 border-b border-neutral-200 dark:border-neutral-800">
              <th className="text-left py-1.5 pr-2 font-medium">Jahr</th>
              <th className="text-right py-1.5 pr-2 font-medium">Wert</th>
              <th className="text-right py-1.5 pr-2 font-medium">Δ Vorjahr</th>
              <th className="text-right py-1.5 font-medium">Δ {currentYear}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, idx) => {
              const colorIdx = yearColorIndex.get(row.year) ?? idx
              const color = YEAR_COLORS[colorIdx % YEAR_COLORS.length]

              // Δ prev year: compare to the next entry in the sorted-descending list.
              // rows are assumed sorted descending (most recent first).
              const prevRow = rows[idx + 1]
              const deltaVsPrev = prevRow !== undefined ? row.value - prevRow.value : null

              // Δ with current year: show 'Ø' for the current year itself.
              const isCurrentYear = row.year === currentYear
              const deltaVsCurrent = isCurrentYear ? null : row.value - currentYearValue

              const valueLabel = fmt(row.value, meta)

              const monthLabel = upToMonth !== null
                ? MONTH_NAMES_DE[upToMonth]
                : 'gesamt'

              return (
                <tr
                  key={row.year}
                  onClick={() => upToMonth !== null
                    ? onLockedDateChange(null)
                    : handleRowClick((new Date().getMonth() + 11) % 12) // lock to prev month on click
                  }
                  title={`${monthLabel} anzeigen`}
                  className="border-b border-neutral-100 dark:border-neutral-800/60 hover:bg-neutral-50 dark:hover:bg-neutral-800/40 cursor-pointer transition-colors"
                >
                  {/* Year pill */}
                  <td className="py-1.5 pr-2">
                    <span
                      className="inline-flex items-center px-2 py-0.5 rounded-full text-white font-medium text-[10px]"
                      style={{ backgroundColor: color }}
                    >
                      {row.year}
                    </span>
                  </td>

                  {/* Value */}
                  <td className="py-1.5 pr-2 text-right font-medium text-neutral-800 dark:text-neutral-200 tabular-nums">
                    {valueLabel}
                  </td>

                  {/* Δ Prev year */}
                  <td className="py-1.5 pr-2 text-right tabular-nums">
                    {deltaVsPrev === null ? (
                      <span className="text-neutral-400">—</span>
                    ) : (
                      <span className={deltaColor(deltaVsPrev)}>{fmtDelta(deltaVsPrev, meta)}</span>
                    )}
                  </td>

                  {/* Δ With current year */}
                  <td className="py-1.5 text-right tabular-nums">
                    {isCurrentYear ? (
                      <span className="text-neutral-400">Ø</span>
                    ) : deltaVsCurrent === null ? (
                      <span className="text-neutral-400">—</span>
                    ) : (
                      <span className={deltaColor(deltaVsCurrent)}>{fmtDelta(deltaVsCurrent, meta)}</span>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
