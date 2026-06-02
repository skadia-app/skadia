import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { fetchYearlyOverview } from '@/api/analytics'
import type { YearlyOverviewRow } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { fmtDuration } from '@/lib/format'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

const MONTH_LABEL_KEYS = [
  'analyticsTrainingStats.months.short.jan',
  'analyticsTrainingStats.months.short.feb',
  'analyticsTrainingStats.months.short.mar',
  'analyticsTrainingStats.months.short.apr',
  'analyticsTrainingStats.months.short.may',
  'analyticsTrainingStats.months.short.jun',
  'analyticsTrainingStats.months.short.jul',
  'analyticsTrainingStats.months.short.aug',
  'analyticsTrainingStats.months.short.sep',
  'analyticsTrainingStats.months.short.oct',
  'analyticsTrainingStats.months.short.nov',
  'analyticsTrainingStats.months.short.dec',
]

function fmtNum(n: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(Math.round(n))
}

interface YearGroup {
  year: number
  months: YearlyOverviewRow[]
  totals: {
    count: number
    distance_m: number
    elevation_m: number
    duration_s: number
  }
  hasMonths: boolean
}

function groupByYear(rows: YearlyOverviewRow[]): YearGroup[] {
  const map = new Map<number, YearlyOverviewRow[]>()
  for (const row of rows) {
    if (!map.has(row.year)) map.set(row.year, [])
    map.get(row.year)!.push(row)
  }

  const groups: YearGroup[] = []
  for (const [year, yearRows] of map) {
    const hasMonths = yearRows.some(r => r.month != null)
    // If rows have month data, sort by month; otherwise keep as-is
    const sortedRows = hasMonths
      ? yearRows.sort((a, b) => (a.month ?? 0) - (b.month ?? 0))
      : yearRows

    const totals = sortedRows.reduce(
      (acc, r) => ({
        count: acc.count + r.count,
        distance_m: acc.distance_m + r.distance_m,
        elevation_m: acc.elevation_m + r.elevation_m,
        duration_s: acc.duration_s + r.duration_s,
      }),
      { count: 0, distance_m: 0, elevation_m: 0, duration_s: 0 }
    )
    groups.push({ year, months: sortedRows, totals, hasMonths })
  }
  return groups.sort((a, b) => b.year - a.year)
}

export default function YearlyOverview() {
  const { t } = useTranslation()
  const { options, formatDistance, formatElevation } = useUnitFormatters()
  const { toQueryString } = useAnalyticsFilters()
  const params = toQueryString()
  const currentYear = new Date().getFullYear()
  const currentMonth = new Date().getMonth() + 1

  const { data, isLoading, isError } = useQuery<YearlyOverviewRow[]>({
    queryKey: ['yearly-overview', params],
    queryFn: () => fetchYearlyOverview(params),
    retry: 1,
  })

  const [openYears, setOpenYears] = useState<Set<number>>(new Set([currentYear]))

  if (isLoading) {
    return (
      <div className="space-y-2 animate-pulse">
        {[2024, 2023, 2022].map(y => (
          <div key={y} className="h-14 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-100 dark:bg-neutral-800" />
        ))}
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-400">
        {t('analyticsTrainingStats.yearlyOverview.loadError')}
      </div>
    )
  }

  if (data.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 text-center text-sm text-neutral-400">
        {t('analyticsTrainingStats.states.noData')}
      </div>
    )
  }

  const groups = groupByYear(data)

  const toggleYear = (year: number) => {
    setOpenYears(prev => {
      const next = new Set(prev)
      if (next.has(year)) next.delete(year)
      else next.add(year)
      return next
    })
  }

  return (
    <div className="space-y-2">
      {groups.map(group => {
        const isOpen = openYears.has(group.year)
        const isCurrent = group.year === currentYear

        return (
          <div
            key={group.year}
            className={`rounded-xl border overflow-hidden ${
              isCurrent
                ? 'border-[var(--color-accent-DEFAULT)]/40 dark:border-[var(--color-accent-DEFAULT)]/30'
                : 'border-neutral-200 dark:border-neutral-800'
            }`}
          >
            {/* Year header */}
            <button
              onClick={() => toggleYear(group.year)}
              className={`w-full flex items-center justify-between px-4 py-3 text-left cursor-pointer transition-colors
                ${isCurrent
                  ? 'bg-[var(--color-accent-DEFAULT)]/5 hover:bg-[var(--color-accent-DEFAULT)]/10 dark:bg-[var(--color-accent-DEFAULT)]/10'
                  : 'bg-white dark:bg-neutral-900 hover:bg-neutral-50 dark:hover:bg-neutral-800'
                }`}
            >
              <div className="flex items-center gap-3">
                <span className={`text-base font-bold ${isCurrent ? 'text-[var(--color-accent-DEFAULT)]' : 'text-neutral-900 dark:text-neutral-100'}`}>
                  {group.year}
                  {isCurrent && <span className="ml-2 text-xs font-normal text-[var(--color-accent-DEFAULT)]/70">{t('analyticsTrainingStats.yearlyOverview.current')}</span>}
                </span>
                <div className="hidden sm:flex items-center gap-4 text-sm text-neutral-600 dark:text-neutral-400">
                  <span className="font-semibold text-neutral-800 dark:text-neutral-200">{t('analyticsTrainingStats.metrics.activitiesCount', { count: group.totals.count })}</span>
                  <span>{formatDistance(group.totals.distance_m)}</span>
                  <span>{formatElevation(group.totals.elevation_m)}</span>
                  <span>{fmtDuration(group.totals.duration_s, 'compact')}</span>
                </div>
              </div>
              <span className={`text-neutral-400 text-sm transition-transform duration-200 ${isOpen ? 'rotate-180' : ''}`}>
                ▼
              </span>
            </button>

            {/* Mobile summary */}
            <div className="sm:hidden px-4 pb-2 flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400 bg-white dark:bg-neutral-900">
              <span className="font-semibold">{group.totals.count}×</span>
              <span>{formatDistance(group.totals.distance_m)}</span>
              <span>{formatElevation(group.totals.elevation_m)}</span>
            </div>

            {/* Month rows — only shown when accordion is open */}
            {isOpen && group.hasMonths && (
              <div className="border-t border-neutral-100 dark:border-neutral-800 divide-y divide-neutral-50 dark:divide-neutral-800/50">
                {group.months.map((row, idx) => {
                  const isCurrentMonth = isCurrent && row.month === currentMonth
                  const monthLabel = row.month != null ? t(MONTH_LABEL_KEYS[(row.month - 1) % 12]) : `#${idx + 1}`
                  return (
                    <div
                      key={row.month ?? idx}
                      className={`flex items-center gap-2 px-4 py-2.5 text-sm ${
                        isCurrentMonth
                          ? 'bg-[var(--color-accent-DEFAULT)]/5 dark:bg-[var(--color-accent-DEFAULT)]/10'
                          : 'bg-white dark:bg-neutral-900'
                      }`}
                    >
                      <span className={`w-8 text-xs font-medium shrink-0 ${
                        isCurrentMonth
                          ? 'text-[var(--color-accent-DEFAULT)]'
                          : 'text-neutral-500 dark:text-neutral-400'
                      }`}>
                        {monthLabel}
                      </span>
                      <span className="w-16 text-right text-neutral-700 dark:text-neutral-300 tabular-nums">
                        {t('analyticsTrainingStats.metrics.activitiesAbbr', { count: row.count })}
                      </span>
                      <span className="flex-1 text-right text-neutral-600 dark:text-neutral-400 tabular-nums text-xs">
                        {formatDistance(row.distance_m)}
                      </span>
                      <span className="flex-1 text-right text-neutral-600 dark:text-neutral-400 tabular-nums text-xs hidden sm:block">
                        {formatElevation(row.elevation_m)}
                      </span>
                      <span className="flex-1 text-right text-neutral-500 dark:text-neutral-500 tabular-nums text-xs hidden sm:block">
                        {fmtDuration(row.duration_s, 'compact')}
                      </span>
                    </div>
                  )
                })}
              </div>
            )}

            {/* Year-level data (no month breakdown) */}
            {isOpen && !group.hasMonths && (
              <div className="border-t border-neutral-100 dark:border-neutral-800 px-4 py-3 bg-white dark:bg-neutral-900">
                <div className="flex items-center gap-6 text-sm text-neutral-600 dark:text-neutral-400">
                  <span><span className="font-semibold text-neutral-800 dark:text-neutral-200">{fmtNum(group.totals.count, options.locale)}</span> {t('analyticsTrainingStats.metrics.activities')}</span>
                  <span><span className="font-semibold text-neutral-800 dark:text-neutral-200">{formatDistance(group.totals.distance_m)}</span> {t('analyticsTrainingStats.metrics.distance')}</span>
                  <span><span className="font-semibold text-neutral-800 dark:text-neutral-200">{formatElevation(group.totals.elevation_m)}</span> {t('analyticsTrainingStats.metrics.elevationGain')}</span>
                  <span><span className="font-semibold text-neutral-800 dark:text-neutral-200">{fmtDuration(group.totals.duration_s, 'compact')}</span> {t('analyticsTrainingStats.metrics.duration')}</span>
                </div>
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
