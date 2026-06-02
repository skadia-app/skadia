import { Compass } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import type { YearProgressionYearData } from '@/api/analytics'
import type { MetricKey } from '@/hooks/useYearProgression'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

const FIXED_METRIC_META: Partial<Record<MetricKey, { unit: string; factor: number; decimals: number }>> = {
  moving_time_s:  { unit: 'h',    factor: 1 / 3600, decimals: 1 },
  tss:            { unit: 'TSS',  factor: 1,        decimals: 0 },
  total_calories: { unit: 'kcal', factor: 1,        decimals: 0 },
}

// ECharts palette, same order as YearProgressionChart so pills match lines.
const YEAR_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
  '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16',
]

const MONTH_NAME_KEYS = [
  'analyticsTrainingStats.months.long.january',
  'analyticsTrainingStats.months.long.february',
  'analyticsTrainingStats.months.long.march',
  'analyticsTrainingStats.months.long.april',
  'analyticsTrainingStats.months.long.may',
  'analyticsTrainingStats.months.long.june',
  'analyticsTrainingStats.months.long.july',
  'analyticsTrainingStats.months.long.august',
  'analyticsTrainingStats.months.long.september',
  'analyticsTrainingStats.months.long.october',
  'analyticsTrainingStats.months.long.november',
  'analyticsTrainingStats.months.long.december',
]

// Months are 0-indexed throughout (Jan = 0).
const MONTH_LAST_DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]

// Returns month index (0-based) of the last fully completed month before `date`,
// or null when no fully completed month exists (January with day <= 1 wraps into
// the previous year which is outside the current year's data range).
export function lockedMonthIndex(date: Date): number | null {
  const day = date.getDate()
  const month = date.getMonth()
  if (month === 0 && day <= 1) {
    // January 1st: no month of the current year is fully completed yet.
    return null
  }
  if (day <= 1) {
    // First day of any other month: step back one month.
    return month - 1
  }
  // After the first day: the previous month is fully completed.
  // January (month === 0): wraps to December (11) of the prior year, but that is
  // outside the current year's cumulative data — treat as "no lock" (null).
  if (month === 0) return null
  return month - 1
}

// Cumulative sum up to and including `upToMonth` (0-based, inclusive).
// Returns 0 when upToMonth is null (no completed month in the lock window).
// Uses the `cumulative` array from the API if available (avoids re-summing),
// otherwise falls back to summing `monthly`.
function cumulativeUpTo(yearData: YearProgressionYearData, upToMonth: number | null): number {
  if (upToMonth === null) return 0
  if (yearData.cumulative?.length > upToMonth) {
    return yearData.cumulative[upToMonth]
  }
  return yearData.monthly.slice(0, upToMonth + 1).reduce((a, b) => a + b, 0)
}

function fmtFixed(raw: number, meta: { unit: string; factor: number; decimals: number }): string {
  return `${(raw * meta.factor).toFixed(meta.decimals)} ${meta.unit}`
}

function fmtFixedDelta(diff: number, meta: { unit: string; factor: number; decimals: number }): string {
  const converted = diff * meta.factor
  const abs = Math.abs(converted).toFixed(meta.decimals)
  const sign = diff >= 0 ? '+' : '−'
  return `${sign}${abs} ${meta.unit}`
}

function signedUnitLabel(diff: number, formatAbs: (value: number) => string): string {
  const sign = diff >= 0 ? '+' : '−'
  return `${sign}${formatAbs(Math.abs(diff))}`
}

function deltaColor(diff: number): string {
  if (diff > 0) return 'text-emerald-600 dark:text-emerald-400'
  if (diff < 0) return 'text-red-500 dark:text-red-400'
  return 'text-neutral-400'
}

// Header date range: "1. Januar → 30. April" (German, day-granular names).
// locked=true with upToMonth=null means January edge case: no completed month yet.
function buildHeaderLabel(upToMonth: number | null, locked: boolean, t: TFunction): string {
  if (!locked) {
    return t('analyticsTrainingStats.yearComparison.fullYearRange')
  }
  if (upToMonth === null) {
    return t('analyticsTrainingStats.yearComparison.noCompletedMonth')
  }
  const lastDay = MONTH_LAST_DAYS[upToMonth]
  return t('analyticsTrainingStats.yearComparison.lockedRange', { day: lastDay, month: t(MONTH_NAME_KEYS[upToMonth]) })
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
  const { t } = useTranslation()
  const unitFormatters = useUnitFormatters()

  const formatValue = (raw: number): string => {
    if (metric === 'distance_m') return unitFormatters.formatDistance(raw)
    if (metric === 'elevation_gain_m' || metric === 'elevation_loss_m') return unitFormatters.formatElevation(raw)
    return fmtFixed(raw, FIXED_METRIC_META[metric]!)
  }

  const formatDelta = (diff: number): string => {
    if (metric === 'distance_m') return signedUnitLabel(diff, unitFormatters.formatDistance)
    if (metric === 'elevation_gain_m' || metric === 'elevation_loss_m') return signedUnitLabel(diff, unitFormatters.formatElevation)
    return fmtFixedDelta(diff, FIXED_METRIC_META[metric]!)
  }

  // Determine up-to-month index (0-based) from lockedDate.
  // null means either no lock or January edge case (no completed month available).
  const upToMonth: number | null = lockedDate !== null
    ? lockedMonthIndex(lockedDate)
    : null

  // Compute values for each year.
  const rows = years.map((yd) => {
    const value = (lockedDate !== null && upToMonth !== null)
      ? cumulativeUpTo(yd, upToMonth)
      : (lockedDate !== null && upToMonth === null)
        ? 0
        : yd.total
    return { year: yd.year, value }
  })

  // Δ prev. year: diff to the chronologically prior year (year - 1), not an
  // array-index neighbour. Backend delivers rows ORDER BY year ASC; index-based
  // access would compare against the next year, not the previous one.
  // When there is no row for year - 1 (gap in data), deltaVsPrev is null.
  // Δ with current year: diff to the current calendar year's value.
  const rowByYear = new Map(rows.map(r => [r.year, r.value]))
  const currentYearValue = rowByYear.get(currentYear) ?? 0

  const headerLabel = buildHeaderLabel(upToMonth, lockedDate !== null, t)

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
            aria-label={t('analyticsTrainingStats.yearComparison.resetDateLockAria')}
            onClick={() => onLockedDateChange(null)}
            className="flex items-center gap-1 text-xs text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200 transition-colors cursor-pointer"
          >
            <Compass size={12} />
            <span>{t('analyticsTrainingStats.actions.reset')}</span>
          </button>
        )}
      </div>

      {/* Hint when no lock is active */}
      {lockedDate === null && (
        <p className="text-[10px] text-neutral-400 dark:text-neutral-600 italic">
          {t('analyticsTrainingStats.yearComparison.lockHint')}
        </p>
      )}

      {/* Table */}
      <div className="overflow-auto">
        <table className="w-full text-xs border-collapse">
          <thead>
            <tr className="text-neutral-500 dark:text-neutral-500 border-b border-neutral-200 dark:border-neutral-800">
              <th className="text-left py-1.5 pr-2 font-medium">{t('analyticsTrainingStats.yearComparison.year')}</th>
              <th className="text-right py-1.5 pr-2 font-medium">{t('analyticsTrainingStats.yearComparison.value')}</th>
              <th className="text-right py-1.5 pr-2 font-medium">{t('analyticsTrainingStats.yearComparison.deltaPreviousYear')}</th>
              <th className="text-right py-1.5 font-medium">Δ {currentYear}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, idx) => {
              const colorIdx = yearColorIndex.get(row.year) ?? idx
              const color = YEAR_COLORS[colorIdx % YEAR_COLORS.length]

              // Δ prev year: compare to the chronologically prior year (year - 1).
              // Null when no data exists for that year (gap in history).
              const prevValue = rowByYear.get(row.year - 1)
              const deltaVsPrev = prevValue !== undefined ? row.value - prevValue : null

              // Δ with current year: show 'Ø' for the current year itself.
              const isCurrentYear = row.year === currentYear
              const deltaVsCurrent = isCurrentYear ? null : row.value - currentYearValue

              const valueLabel = formatValue(row.value)

              return (
                <tr
                  key={row.year}
                  className="border-b border-neutral-100 dark:border-neutral-800/60"
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
                      <span className={deltaColor(deltaVsPrev)}>{formatDelta(deltaVsPrev)}</span>
                    )}
                  </td>

                  {/* Δ With current year */}
                  <td className="py-1.5 text-right tabular-nums">
                    {isCurrentYear ? (
                      <span className="text-neutral-400">Ø</span>
                    ) : deltaVsCurrent === null ? (
                      <span className="text-neutral-400">—</span>
                    ) : (
                      <span className={deltaColor(deltaVsCurrent)}>{formatDelta(deltaVsCurrent)}</span>
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
