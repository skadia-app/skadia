import { useState, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useClickOutside } from '@/hooks/useClickOutside'
import { addExclusion } from '@/api/bestEfforts'
import { useToast } from '@/lib/toast'
import { categoryLabel, formatBestEffortValue } from '@/lib/bestEffortLabels'
import type { BestEffortRanked, NearMiss } from '@/api/bestEfforts'

// ── Rank badge styling ────────────────────────────────────────────────────────

const RANK_STYLES: Record<number, { bg: string; text: string; border: string }> = {
  1: { bg: 'bg-yellow-50 dark:bg-yellow-950/40', text: 'text-yellow-700 dark:text-yellow-400', border: 'border-yellow-300 dark:border-yellow-700' },
  2: { bg: 'bg-neutral-100 dark:bg-neutral-800', text: 'text-neutral-600 dark:text-neutral-300', border: 'border-neutral-300 dark:border-neutral-600' },
  3: { bg: 'bg-orange-50 dark:bg-orange-950/40', text: 'text-orange-700 dark:text-orange-500', border: 'border-orange-300 dark:border-orange-700' },
}

// Gold / Silver / Bronze dot colors (inline for precision)
const RANK_DOT: Record<number, string> = {
  1: '#EAB308',
  2: '#9CA3AF',
  3: '#CD7F32',
}

function rankLabel(rank: number): string {
  if (rank === 1) return 'PR!'
  if (rank === 2) return '2. Bestzeit'
  if (rank === 3) return '3. Bestzeit'
  return `${rank}. Bestzeit`
}

// ── Percentile badge ──────────────────────────────────────────────────────────

interface PercentileBadgeProps {
  percentile: number | null | undefined
  category: string
}

function PercentileBadge({ percentile, category }: PercentileBadgeProps) {
  if (percentile == null || percentile < 0.50) return null

  const topPct = Math.round((1 - percentile) * 100)
  const label = categoryLabel(category)
  const tooltip = `Diese Leistung liegt in deinen besten ${topPct}% aller ${label} Zeiten`

  if (percentile >= 0.90) {
    return (
      <span
        className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400"
        title={tooltip}
      >
        Top {topPct}%
      </span>
    )
  }

  // percentile >= 0.75 (Top 25%)
  return (
    <span
      className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-blue-100 text-blue-700 dark:bg-blue-950/40 dark:text-blue-400"
      title={tooltip}
    >
      Top {topPct}%
    </span>
  )
}

// ── Single effort badge with popover ─────────────────────────────────────────

interface EffortBadgeProps {
  effort: BestEffortRanked
  activityId: string
  onExcluded: () => void
}

function EffortBadge({ effort, activityId, onExcluded }: EffortBadgeProps) {
  const [open, setOpen] = useState(false)
  const popoverRef = useRef<HTMLDivElement>(null)
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  useClickOutside(popoverRef, () => setOpen(false), open)

  const rank = Math.min(effort.rank, 3)
  const style = RANK_STYLES[rank] ?? RANK_STYLES[3]
  const dotColor = RANK_DOT[rank] ?? RANK_DOT[3]

  const excludeMut = useMutation({
    mutationFn: () => addExclusion(activityId, effort.category),
    onSuccess: () => {
      setOpen(false)
      queryClient.invalidateQueries({ queryKey: ['activity-best-efforts', activityId] })
      addToast({ type: 'success', title: 'Von PRs ausgeschlossen', message: `${categoryLabel(effort.category)} wird nicht mehr gewertet.` })
      onExcluded()
    },
    onError: () => {
      addToast({ type: 'error', title: 'Ausschluss fehlgeschlagen', message: 'Bitte versuche es erneut.' })
    },
  })

  return (
    <div className="relative inline-flex items-center gap-1.5" ref={popoverRef}>
      <button
        type="button"
        onClick={() => setOpen(v => !v)}
        className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full border text-xs font-semibold cursor-pointer transition-colors ${style.bg} ${style.text} ${style.border} hover:opacity-80`}
        title={`${categoryLabel(effort.category)} — ${rankLabel(effort.rank)} — ${formatBestEffortValue(effort.category, effort.elapsed_time_s, effort.value_numeric)}`}
      >
        <span className="w-2 h-2 rounded-full flex-shrink-0" style={{ background: dotColor }} />
        <span>{categoryLabel(effort.category)}</span>
        <span className="opacity-75">{rankLabel(effort.rank)}</span>
        <span className="font-mono opacity-60">{formatBestEffortValue(effort.category, effort.elapsed_time_s, effort.value_numeric)}</span>
      </button>
      <PercentileBadge percentile={effort.percentile} category={effort.category} />

      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 w-52 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg p-2">
          <div className="text-xs text-neutral-500 dark:text-neutral-400 px-1 pb-1.5 border-b border-neutral-100 dark:border-neutral-800">
            {categoryLabel(effort.category)} &mdash; {rankLabel(effort.rank)}
          </div>
          <button
            type="button"
            disabled={excludeMut.isPending}
            onClick={() => excludeMut.mutate()}
            className="w-full text-left px-2 py-1.5 mt-1 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 rounded cursor-pointer transition-colors disabled:opacity-50"
          >
            Von PRs ausschließen
          </button>
        </div>
      )}
    </div>
  )
}

// ── Near-miss badge ───────────────────────────────────────────────────────────

function NearMissBadge({ miss }: { miss: NearMiss }) {
  const label = categoryLabel(miss.category)

  let diffText: string
  if (miss.diff_seconds != null) {
    diffText = `${Math.round(miss.diff_seconds)} s`
  } else if (miss.diff_watts != null) {
    diffText = `${Math.round(miss.diff_watts)} W`
  } else {
    return null
  }

  return (
    <span
      className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full border text-xs font-medium bg-blue-50 dark:bg-blue-950/30 text-blue-700 dark:text-blue-300 border-blue-200 dark:border-blue-800"
      title={`Knapp verfehlt: ${diffText} von deinem ${label}-PR`}
    >
      <svg className="w-3 h-3 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      {diffText} von {label}-PR entfernt
    </span>
  )
}

// ── Public component ──────────────────────────────────────────────────────────

interface BestEffortBadgesProps {
  bestEfforts: BestEffortRanked[]
  nearMisses: NearMiss[]
  activityId: string
}

export function BestEffortBadges({ bestEfforts, nearMisses, activityId }: BestEffortBadgesProps) {
  // Track locally excluded efforts so UI updates immediately
  const [excluded, setExcluded] = useState<Set<string>>(new Set())

  const visibleEfforts = bestEfforts.filter(e => !excluded.has(e.id))

  if (visibleEfforts.length === 0 && nearMisses.length === 0) return null

  return (
    <div className="space-y-2">
      {visibleEfforts.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          {visibleEfforts.map(effort => (
            <EffortBadge
              key={effort.id}
              effort={effort}
              activityId={activityId}
              onExcluded={() => setExcluded(prev => new Set([...prev, effort.id]))}
            />
          ))}
        </div>
      )}

      {nearMisses.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          {nearMisses.map(miss => (
            <NearMissBadge key={`${miss.category}-${miss.sport_group}`} miss={miss} />
          ))}
        </div>
      )}
    </div>
  )
}
