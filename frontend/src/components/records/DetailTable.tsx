import { useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown } from 'lucide-react'
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

interface DetailTableProps {
  records: BestEffortRanked[]
  sportGroup?: string
}

export function DetailTable({ records, sportGroup }: DetailTableProps) {
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
                <td className="px-3 font-medium text-neutral-800 dark:text-neutral-200">{categoryLabel(cat)}</td>
                <td className="text-right px-4 font-bold tabular-nums">
                  {display.primary}
                  {display.secondary && <div className="text-[9px] text-neutral-500 font-normal">{display.secondary}</div>}
                </td>
                <td className="px-4 text-neutral-500 dark:text-neutral-400 text-xs">{fmtDateMedium(rank1.achieved_at)}</td>
                <td className="px-2">
                  {hasMore && (
                    <ChevronDown className={cn('w-3.5 h-3.5 text-neutral-400 dark:text-neutral-600 transition-transform', isExpanded && 'rotate-180')} />
                  )}
                </td>
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
                      delta = `−${Math.round(diff)} W`
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
                      </tr>
                    )
                  })
                : []),
            ]
          })}
        </tbody>
      </table>
    </div>
  )
}
