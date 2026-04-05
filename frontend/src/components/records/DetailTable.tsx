import { useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown, TrendingUp } from 'lucide-react'
import { formatBestEffortDisplay, categoryLabel, CATEGORY_LABELS } from '@/lib/bestEffortLabels'
import { fmtDateMedium, fmtDuration } from '@/lib/format'
import type { BestEffortRanked } from '@/api/bestEfforts'
import { cn } from '@/lib/utils'

const RANK_STYLES: Record<number, { bg: string; text: string }> = {
  1: { bg: 'bg-yellow-100 dark:bg-yellow-500/20', text: 'text-yellow-700 dark:text-yellow-400' },
  2: { bg: 'bg-neutral-200 dark:bg-neutral-700', text: 'text-neutral-500 dark:text-neutral-400' },
  3: { bg: 'bg-orange-100 dark:bg-orange-500/15', text: 'text-orange-600 dark:text-orange-400' },
}

const CATEGORY_KEYS = Object.keys(CATEGORY_LABELS)

function categoryColorHint(cat: string): string {
  if (cat.startsWith('power_')) return 'text-orange-600 dark:text-orange-300'
  if (cat.startsWith('elev_')) return 'text-green-600 dark:text-green-300'
  return ''
}

interface DetailTableProps {
  records: BestEffortRanked[]
  sportGroup?: string
  /** The category key whose chart is currently expanded — controlled from outside. */
  chartExpandedCategory?: string | null
  /** Called when the user clicks the chart-toggle button for a category. */
  onChartToggle?: (category: string) => void
  /** Render function for the chart row content (receives category + sportGroup). */
  renderChart?: (category: string, sportGroup: string) => React.ReactNode
}

export function DetailTable({
  records,
  sportGroup,
  chartExpandedCategory,
  onChartToggle,
  renderChart,
}: DetailTableProps) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const toggle = useCallback((category: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(category)) next.delete(category)
      else next.add(category)
      return next
    })
  }, [])

  const categories = Array.from(new Set(records.map((r) => r.category)))
    .sort((a, b) => {
      const ia = CATEGORY_KEYS.indexOf(a)
      const ib = CATEGORY_KEYS.indexOf(b)
      return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib)
    })

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-[11px] text-neutral-500 border-b border-neutral-200 dark:border-neutral-800 uppercase tracking-wider">
            <th className="w-10" />
            <th className="text-left px-3 py-2.5 font-medium">Kategorie</th>
            <th className="text-right px-4 py-2.5 font-medium">Bestzeit</th>
            <th className="text-left px-4 py-2.5 font-medium">Datum</th>
            <th className="w-8" />
            {renderChart && <th className="w-8" />}
          </tr>
        </thead>
        <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
          {categories.map((cat) => {
            const catRecords = records.filter((r) => r.category === cat).sort((a, b) => a.rank - b.rank)
            const rank1 = catRecords[0]
            if (!rank1) return null
            const hasMore = catRecords.length > 1
            const isExpanded = expanded.has(cat)
            const display = formatBestEffortDisplay(cat, rank1.elapsed_time_s, rank1.value_numeric, sportGroup)
            const valueColor = categoryColorHint(cat)

            const isChartExpanded = chartExpandedCategory === cat
            const colCount = renderChart ? 6 : 5

            return [
              <tr
                key={rank1.id}
                id={`detail-${cat}`}
                className="hover:bg-neutral-50 dark:hover:bg-neutral-800/30 cursor-pointer group"
                onClick={hasMore ? () => toggle(cat) : undefined}
              >
                <td className="px-4 py-2.5">
                  <span className={cn('w-5 h-5 inline-flex items-center justify-center rounded-full text-[9px] font-bold', RANK_STYLES[1].bg, RANK_STYLES[1].text)}>
                    #1
                  </span>
                </td>
                <td className="px-3 font-medium text-neutral-800 dark:text-neutral-200">
                  {categoryLabel(cat)}
                  {rank1.suspicious && (
                    <span
                      className="ml-2 inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[9px] bg-amber-100 dark:bg-amber-500/10 text-amber-700 dark:text-amber-400 border border-amber-300 dark:border-amber-500/20"
                      title="Möglicherweise fehlerhafter Sensor-Wert (Stuck Value)"
                    >
                      Verdächtig
                    </span>
                  )}
                </td>
                <td className="text-right px-4 font-bold tabular-nums">
                  <span className={cn(valueColor)}>{display.primary}</span>
                  {display.secondary && <div className="text-[9px] text-neutral-500 font-normal">{display.secondary}</div>}
                </td>
                <td className="px-4 text-xs">
                  <Link
                    to={`/activities/${rank1.activity_id}`}
                    className="text-neutral-500 dark:text-neutral-400 hover:text-blue-500 dark:hover:text-blue-400"
                    onClick={(e) => e.stopPropagation()}
                  >
                    {fmtDateMedium(rank1.achieved_at)}
                  </Link>
                </td>
                <td className="px-2">
                  {hasMore && (
                    <ChevronDown className={cn('w-3.5 h-3.5 text-neutral-400 dark:text-neutral-600 transition-transform', isExpanded && 'rotate-180')} />
                  )}
                </td>
                {renderChart && onChartToggle && (
                  <td className="px-2">
                    <button
                      onClick={(e) => { e.stopPropagation(); onChartToggle(cat) }}
                      title={isChartExpanded ? 'Verlauf ausblenden' : 'Verlauf anzeigen'}
                      className={cn(
                        'p-1 rounded transition-colors',
                        isChartExpanded
                          ? 'text-orange-500 dark:text-orange-400 bg-orange-50 dark:bg-orange-500/10'
                          : 'text-neutral-400 dark:text-neutral-600 hover:text-orange-500 dark:hover:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-500/10',
                      )}
                    >
                      <TrendingUp className="w-3.5 h-3.5" />
                    </button>
                  </td>
                )}
              </tr>,
              ...(isExpanded
                ? catRecords.slice(1).map((r) => {
                    const rank1Time = rank1.elapsed_time_s
                    const rank1Val = rank1.value_numeric
                    let delta = ''
                    if (r.elapsed_time_s != null && rank1Time != null) {
                      const diff = Math.abs(r.elapsed_time_s - rank1Time)
                      delta = `+${fmtDuration(diff)}`
                    } else if (r.value_numeric != null && rank1Val != null) {
                      const diff = Math.abs(r.value_numeric - rank1Val)
                      if (cat.startsWith('power_')) {
                        delta = `−${Math.round(diff)} W`
                      } else if (cat === 'sup_longest_distance') {
                        delta = diff >= 1000
                          ? `−${(diff / 1000).toLocaleString('de-DE', { maximumFractionDigits: 1 })} km`
                          : `−${Math.round(diff)} m`
                      } else if (cat === 'sup_most_elevation' || cat === 'sup_biggest_climb') {
                        delta = `−${Math.round(diff)} m`
                      } else if (cat === 'sup_longest_duration') {
                        delta = `−${fmtDuration(diff)}`
                      } else {
                        delta = `−${Math.round(diff)}`
                      }
                    }
                    const d = formatBestEffortDisplay(cat, r.elapsed_time_s, r.value_numeric, sportGroup)
                    const style = RANK_STYLES[r.rank] ?? RANK_STYLES[3]
                    return (
                      <tr key={r.id} className="bg-neutral-50/50 dark:bg-neutral-800/10">
                        <td className="px-4 py-1.5">
                          <span className={cn('w-4 h-4 inline-flex items-center justify-center rounded-full text-[8px] font-bold', style.bg, style.text)}>
                            #{r.rank}
                          </span>
                        </td>
                        <td className="px-3 text-neutral-400 dark:text-neutral-500 text-xs">{categoryLabel(cat)}</td>
                        <td className="text-right px-4 tabular-nums text-neutral-500 dark:text-neutral-400 text-xs">{d.primary}</td>
                        <td className="px-4 text-neutral-500 dark:text-neutral-600 text-[10px]">
                          <Link to={`/activities/${r.activity_id}`} className="hover:text-blue-500 dark:hover:text-blue-400">
                            {fmtDateMedium(r.achieved_at)}
                          </Link>
                          {delta && <span className="text-neutral-400 dark:text-neutral-600 ml-2">{delta}</span>}
                        </td>
                        <td />
                        {renderChart && <td />}
                      </tr>
                    )
                  })
                : []),
              // Chart expansion row
              ...(isChartExpanded && renderChart && rank1.sport_group
                ? [
                    <tr key={`chart-${cat}`} className="bg-neutral-50/30 dark:bg-neutral-800/10 border-t border-neutral-100 dark:border-neutral-800/50">
                      <td colSpan={colCount} className="p-0">
                        {renderChart(cat, rank1.sport_group)}
                      </td>
                    </tr>,
                  ]
                : []),
            ]
          })}
        </tbody>
      </table>
    </div>
  )
}
