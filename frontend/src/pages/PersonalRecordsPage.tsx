import { useState, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Trophy, RotateCcw, XCircle, MapPin } from 'lucide-react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import {
  fetchPersonalRecords,
  fetchExclusions,
  addExclusion,
  removeExclusion,
  recalcBestEfforts,
  type BestEffortRanked,
  type BestEffortExclusion,
} from '@/api/bestEfforts'
import {
  fetchSegmentPRs,
  fetchGlobalTopSegmentPRs,
  type SegmentEffort,
  type GlobalTopSegmentEffort,
} from '@/api/segments'
import { fmtDateMedium, fmtDuration } from '@/lib/format'
import { useToast } from '@/lib/toast'
import { cn } from '@/lib/utils'
import { CATEGORY_LABELS, categoryLabel as getCategoryLabel, formatBestEffortValue } from '@/lib/bestEffortLabels'

// ── Constants ──────────────────────────────────────────────────────────────

const SPORT_GROUP_LABELS: Record<string, string> = {
  running:          'Laufen',
  running_indoor:   'Laufen (Indoor)',
  running_virtual:  'Laufen (Virtuell)',
  cycling:          'Radfahren',
  cycling_electric: 'E-Bike',
  cycling_virtual:  'Radfahren (Virtuell)',
  hiking:           'Wandern',
  swim_pool:        'Schwimmen (Becken)',
  swim_ow:          'Schwimmen (Freiwasser)',
}

const CURRENT_YEAR = new Date().getFullYear()
const YEAR_OPTIONS = [
  { value: '', label: 'Gesamt' },
  { value: String(CURRENT_YEAR), label: String(CURRENT_YEAR) },
  { value: String(CURRENT_YEAR - 1), label: String(CURRENT_YEAR - 1) },
  { value: String(CURRENT_YEAR - 2), label: String(CURRENT_YEAR - 2) },
]

// Categories prefixes by tab
const DIST_PREFIX = 'dist_'
const ELEV_PREFIX = 'elev_'
const POWER_PREFIX = 'power_'
const SUP_PREFIX = 'sup_'

// E-9: Extracted once to avoid O(n) indexOf inside sort comparator
const CATEGORY_KEYS = Object.keys(CATEGORY_LABELS)
const SPORT_GROUP_ORDER = Object.keys(SPORT_GROUP_LABELS)

// Sort helper: order by known key list, unknowns at end
function sortByKnownOrder<T>(items: T[], keyFn: (item: T) => string, order: string[]): T[] {
  return [...items].sort((a, b) => {
    const ia = order.indexOf(keyFn(a))
    const ib = order.indexOf(keyFn(b))
    if (ia === -1 && ib === -1) return keyFn(a).localeCompare(keyFn(b))
    if (ia === -1) return 1
    if (ib === -1) return -1
    return ia - ib
  })
}

// ── Rank Badge ─────────────────────────────────────────────────────────────

const RANK_STYLES: Record<number, { bg: string; text: string; label: string }> = {
  1: { bg: 'bg-yellow-100 dark:bg-yellow-900/40', text: 'text-yellow-700 dark:text-yellow-400', label: '#1' },
  2: { bg: 'bg-neutral-100 dark:bg-neutral-700/60', text: 'text-neutral-500 dark:text-neutral-400', label: '#2' },
  3: { bg: 'bg-orange-100/80 dark:bg-orange-900/30', text: 'text-orange-600 dark:text-orange-400', label: '#3' },
}

function RankBadge({ rank }: { rank: number }) {
  const style = RANK_STYLES[rank]
  if (!style) return null
  return (
    <span
      className={cn(
        'inline-flex items-center justify-center w-7 h-7 rounded-full text-xs font-bold shrink-0',
        style.bg, style.text,
      )}
    >
      {style.label}
    </span>
  )
}


// ── Power tooltip ──────────────────────────────────────────────────────────

function PowerNote() {
  return (
    <span
      className="ml-1.5 inline-flex items-center justify-center w-4 h-4 rounded-full bg-neutral-200 dark:bg-neutral-700 text-neutral-500 dark:text-neutral-400 text-[10px] font-bold cursor-help"
      title="Durchschnittsleistung inkl. Freilauf-Phasen (0 W)"
    >
      ?
    </span>
  )
}

// ── Overview Grid (Alle-Ansicht) ───────────────────────────────────────────

interface OverviewGridProps {
  records: BestEffortRanked[]
}

/** For the "Alle" view: one compact card per sport group, showing only #1 per category as a horizontal tag row. */
function OverviewGrid({ records }: OverviewGridProps) {
  // Only #1 records
  const topRecords = useMemo(() => records.filter((r) => r.rank === 1), [records])

  // Group by sport_group, preserving label order
  const groups = useMemo(() => {
    const map = new Map<string, BestEffortRanked[]>()
    for (const r of topRecords) {
      if (!map.has(r.sport_group)) map.set(r.sport_group, [])
      map.get(r.sport_group)!.push(r)
    }
    return sortByKnownOrder(Array.from(map.entries()), ([k]) => k, SPORT_GROUP_ORDER)
  }, [topRecords])

  if (topRecords.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-3 text-neutral-400 dark:text-neutral-500">
        <Trophy className="w-10 h-10 opacity-30" />
        <p className="text-sm">Noch keine Bestleistungen vorhanden</p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {groups.map(([group, entries]) => {
        // Sort entries by CATEGORY_KEYS order
        const sorted = entries.slice().sort((a, b) => {
          const ia = CATEGORY_KEYS.indexOf(a.category)
          const ib = CATEGORY_KEYS.indexOf(b.category)
          if (ia === -1 && ib === -1) return a.category.localeCompare(b.category)
          if (ia === -1) return 1
          if (ib === -1) return -1
          return ia - ib
        })
        const groupLabel = SPORT_GROUP_LABELS[group] ?? group

        return (
          <div
            key={group}
            className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden"
          >
            <div className="px-4 py-2.5 bg-neutral-50 dark:bg-neutral-800/60 border-b border-neutral-100 dark:border-neutral-800">
              <span className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">{groupLabel}</span>
            </div>
            <div className="px-4 py-3 flex flex-wrap gap-2">
              {sorted.map((record) => (
                <Link
                  key={record.id}
                  to={`/activities/${record.activity_id}`}
                  title={`${getCategoryLabel(record.category)} — ${fmtDateMedium(record.achieved_at)}`}
                  className={cn(
                    'inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs font-medium transition-colors',
                    'bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700',
                    'text-neutral-700 dark:text-neutral-300',
                    'border border-transparent hover:border-neutral-300 dark:hover:border-neutral-600',
                  )}
                >
                  <span className="text-neutral-500 dark:text-neutral-400">{getCategoryLabel(record.category)}:</span>
                  <span className="font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
                    {formatBestEffortValue(record.category, record.elapsed_time_s, record.value_numeric)}
                  </span>
                </Link>
              ))}
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ── Records Table ──────────────────────────────────────────────────────────

interface RecordsTableProps {
  records: BestEffortRanked[]
  showPowerNote?: boolean
  onExclude?: (activityId: string, category: string) => void
  excludingKey?: string | null
}

function RecordsTable({ records, showPowerNote, onExclude, excludingKey }: RecordsTableProps) {
  // Group by category, then by rank within category
  const grouped = useMemo(() => {
    const map = new Map<string, BestEffortRanked[]>()
    for (const r of records) {
      if (!map.has(r.category)) map.set(r.category, [])
      map.get(r.category)!.push(r)
    }
    // Sort by category order: use CATEGORY_KEYS (computed once outside component)
    return Array.from(map.entries()).sort(([a], [b]) => {
      const ia = CATEGORY_KEYS.indexOf(a)
      const ib = CATEGORY_KEYS.indexOf(b)
      if (ia === -1 && ib === -1) return a.localeCompare(b)
      if (ia === -1) return 1
      if (ib === -1) return -1
      return ia - ib
    })
  }, [records])

  if (records.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-3 text-neutral-400 dark:text-neutral-500">
        <Trophy className="w-10 h-10 opacity-30" />
        <p className="text-sm">Noch keine Bestleistungen für diese Auswahl</p>
      </div>
    )
  }

  return (
    <div className="space-y-1">
      {grouped.map(([category, entries]) => (
        <div key={category} className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
          <div className="px-4 py-2.5 bg-neutral-50 dark:bg-neutral-800/60 border-b border-neutral-100 dark:border-neutral-800 flex items-center gap-2">
            <span className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
              {getCategoryLabel(category)}
            </span>
            {showPowerNote && category.startsWith(POWER_PREFIX) && <PowerNote />}
          </div>
          <div className="divide-y divide-neutral-50 dark:divide-neutral-800/50">
            {entries.sort((a, b) => a.rank - b.rank).map((record) => {
              const key = `${record.activity_id}-${record.category}`
              const isExcluding = excludingKey === key
              return (
                <div
                  key={record.id}
                  className="flex items-center gap-3 px-4 py-2.5 hover:bg-neutral-50 dark:hover:bg-neutral-800/40 transition-colors group"
                >
                  <RankBadge rank={record.rank} />
                  <div className="flex-1 min-w-0">
                    <Link
                      to={`/activities/${record.activity_id}`}
                      className="text-sm font-medium text-neutral-800 dark:text-neutral-200 hover:text-accent-DEFAULT dark:hover:text-accent-DEFAULT transition-colors truncate block"
                    >
                      {fmtDateMedium(record.achieved_at)}
                    </Link>
                  </div>
                  <span className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums shrink-0">
                    {formatBestEffortValue(record.category, record.elapsed_time_s, record.value_numeric)}
                  </span>
                  {onExclude && (
                    <button
                      onClick={() => onExclude(record.activity_id, record.category)}
                      disabled={isExcluding}
                      aria-label="Aktivität ausschließen"
                      className={cn(
                        'opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded text-neutral-400 hover:text-rose-500 dark:hover:text-rose-400',
                        isExcluding && 'opacity-50 cursor-not-allowed',
                      )}
                      title="Von Bestleistungen ausschließen"
                    >
                      <XCircle className="w-4 h-4" />
                    </button>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}

// ── Segment PRs Table ──────────────────────────────────────────────────────

interface SegmentPRsTableProps {
  efforts: SegmentEffort[]
}

function SegmentPRsTable({ efforts }: SegmentPRsTableProps) {
  // Group by segment_name, then sort by personal_rank within each segment
  const grouped = useMemo(() => {
    const map = new Map<string, SegmentEffort[]>()
    for (const e of efforts) {
      const name = e.segment_name ?? e.segment_id
      if (!map.has(name)) map.set(name, [])
      map.get(name)!.push(e)
    }
    return Array.from(map.entries()).sort(([a], [b]) => a.localeCompare(b, 'de'))
  }, [efforts])

  if (efforts.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-3 text-neutral-400 dark:text-neutral-500">
        <MapPin className="w-10 h-10 opacity-30" />
        <p className="text-sm text-center">
          Noch keine Segment-Bestzeiten (Platz 1–3)
        </p>
        <Link
          to="/segments"
          className="text-sm text-accent-DEFAULT hover:underline"
        >
          Zu den Segmenten
        </Link>
      </div>
    )
  }

  return (
    <div className="space-y-1">
      {grouped.map(([segmentName, entries]) => (
        <div
          key={segmentName}
          className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden"
        >
          <div className="px-4 py-2.5 bg-neutral-50 dark:bg-neutral-800/60 border-b border-neutral-100 dark:border-neutral-800 flex items-center gap-2">
            <MapPin className="w-3.5 h-3.5 text-neutral-400 dark:text-neutral-500 shrink-0" />
            <Link
              to={`/segments/${entries[0].segment_id}`}
              className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 hover:text-accent-DEFAULT dark:hover:text-accent-DEFAULT transition-colors truncate"
            >
              {segmentName}
            </Link>
          </div>
          <div className="divide-y divide-neutral-50 dark:divide-neutral-800/50">
            {entries
              .slice()
              .sort((a, b) => (a.personal_rank ?? 99) - (b.personal_rank ?? 99))
              .map((effort) => (
                <div
                  key={effort.id}
                  className="flex items-center gap-3 px-4 py-2.5 hover:bg-neutral-50 dark:hover:bg-neutral-800/40 transition-colors"
                >
                  <RankBadge rank={effort.personal_rank ?? 0} />
                  <div className="flex-1 min-w-0">
                    <Link
                      to={`/activities/${effort.activity_id}`}
                      className="text-sm font-medium text-neutral-800 dark:text-neutral-200 hover:text-accent-DEFAULT dark:hover:text-accent-DEFAULT transition-colors truncate block"
                    >
                      {fmtDateMedium(effort.started_at)}
                    </Link>
                  </div>
                  <span className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums shrink-0">
                    {fmtDuration(effort.elapsed_time_s)}
                  </span>
                </div>
              ))}
          </div>
        </div>
      ))}
    </div>
  )
}

// ── Global Top 10 Table ────────────────────────────────────────────────────

interface GlobalTopTableProps {
  efforts: GlobalTopSegmentEffort[]
}

function GlobalTopTable({ efforts }: GlobalTopTableProps) {
  if (efforts.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-10 gap-2 text-neutral-400 dark:text-neutral-500">
        <p className="text-sm text-center">
          Noch keine Top-10-Platzierungen — wird relevant sobald mehrere Nutzer Segmente befahren.
        </p>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden divide-y divide-neutral-50 dark:divide-neutral-800/50">
      {efforts.map((effort) => {
        const rank = effort.rank_overall ?? 0
        const rankLabel = `#${rank}${effort.athlete_count > 1 ? ` von ${effort.athlete_count}` : ''}`
        const segmentName = effort.segment_name ?? effort.segment_id
        return (
          <div
            key={effort.id}
            className="flex items-center gap-3 px-4 py-2.5 hover:bg-neutral-50 dark:hover:bg-neutral-800/40 transition-colors"
          >
            {/* Global rank badge */}
            <span
              className={cn(
                'inline-flex items-center justify-center min-w-[2.75rem] h-7 px-2 rounded-full text-xs font-bold shrink-0 tabular-nums',
                rank === 1
                  ? 'bg-yellow-100 dark:bg-yellow-900/40 text-yellow-700 dark:text-yellow-400'
                  : rank === 2
                  ? 'bg-neutral-100 dark:bg-neutral-700/60 text-neutral-500 dark:text-neutral-400'
                  : rank === 3
                  ? 'bg-orange-100/80 dark:bg-orange-900/30 text-orange-600 dark:text-orange-400'
                  : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400',
              )}
            >
              {rankLabel}
            </span>

            {/* Segment name → segment detail */}
            <div className="flex-1 min-w-0">
              <Link
                to={`/segments/${effort.segment_id}`}
                className="text-sm font-medium text-neutral-800 dark:text-neutral-200 hover:text-accent-DEFAULT dark:hover:text-accent-DEFAULT transition-colors truncate block"
              >
                {segmentName}
              </Link>
              {/* Activity link + date */}
              <Link
                to={`/activities/${effort.activity_id}`}
                className="text-xs text-neutral-400 dark:text-neutral-500 hover:text-accent-DEFAULT dark:hover:text-accent-DEFAULT transition-colors"
              >
                {fmtDateMedium(effort.started_at)}
              </Link>
            </div>

            {/* Elapsed time */}
            <span className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums shrink-0">
              {fmtDuration(effort.elapsed_time_s)}
            </span>
          </div>
        )
      })}
    </div>
  )
}

// ── Excluded Activity Row ──────────────────────────────────────────────────

type ExcludedEntry = Pick<BestEffortExclusion, 'activity_id' | 'category'>

interface ExcludedListProps {
  excluded: ExcludedEntry[]
  onReinclude: (activityId: string, category: string) => void
  reincludingKey: string | null
}

function ExcludedList({ excluded, onReinclude, reincludingKey }: ExcludedListProps) {
  if (excluded.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-10 gap-2 text-neutral-400 dark:text-neutral-500">
        <p className="text-sm">Keine ausgeschlossenen Aktivitäten</p>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden divide-y divide-neutral-50 dark:divide-neutral-800/50">
      {excluded.map((entry) => {
        const key = `${entry.activity_id}-${entry.category}`
        const isProcessing = reincludingKey === key
        return (
          <div key={key} className="flex items-center gap-3 px-4 py-3">
            <div className="flex-1 min-w-0">
              <Link
                to={`/activities/${entry.activity_id}`}
                className="text-sm text-neutral-700 dark:text-neutral-300 hover:text-accent-DEFAULT transition-colors"
              >
                Aktivität {entry.activity_id.slice(0, 8)}…
              </Link>
              <span className="ml-2 text-xs text-neutral-400">{getCategoryLabel(entry.category)}</span>
            </div>
            <button
              onClick={() => onReinclude(entry.activity_id, entry.category)}
              disabled={isProcessing}
              className={cn(
                'text-xs px-3 py-1 rounded-md font-medium transition-colors',
                'bg-neutral-100 hover:bg-neutral-200 dark:bg-neutral-800 dark:hover:bg-neutral-700',
                'text-neutral-700 dark:text-neutral-300',
                isProcessing && 'opacity-50 cursor-not-allowed',
              )}
            >
              {isProcessing ? 'Wird eingeschlossen…' : 'Wieder einschließen'}
            </button>
          </div>
        )
      })}
    </div>
  )
}

// ── Main Page ──────────────────────────────────────────────────────────────

export default function PersonalRecordsPage() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const [selectedGroup, setSelectedGroup] = useState('')
  const [selectedYear, setSelectedYear] = useState('')
  const [activeTab, setActiveTab] = useState('distanz')
  const [excludingKey, setExcludingKey] = useState<string | null>(null)
  const [reincludingKey, setReincludingKey] = useState<string | null>(null)

  const queryKey = ['personal-records', selectedGroup, selectedYear]
  const exclusionsQueryKey = ['best-effort-exclusions']

  const { data: records = [], isLoading, isError } = useQuery({
    queryKey,
    queryFn: () => fetchPersonalRecords({
      sport_group: selectedGroup || undefined,
      year: selectedYear || undefined,
    }),
    staleTime: 2 * 60 * 1000,
  })

  const { data: excluded = [] } = useQuery({
    queryKey: exclusionsQueryKey,
    queryFn: fetchExclusions,
    staleTime: 60 * 1000,
  })

  const { data: segmentPRsData } = useQuery({
    queryKey: ['segment-prs'],
    queryFn: fetchSegmentPRs,
    staleTime: 5 * 60 * 1000,
  })
  const segmentPRs = segmentPRsData?.efforts ?? []

  const { data: globalTopData } = useQuery({
    queryKey: ['segment-prs-global-top'],
    queryFn: fetchGlobalTopSegmentPRs,
    staleTime: 5 * 60 * 1000,
  })
  const globalTopEfforts = globalTopData?.efforts ?? []

  // Dynamic sport chips: always fetch sport_group='' to derive available groups.
  // When selectedGroup='' this duplicates the main query key — React Query deduplicates
  // via cache, so no extra network request occurs.
  const { data: allRecords = [] } = useQuery({
    queryKey: ['personal-records', '', selectedYear],
    queryFn: () => fetchPersonalRecords({ year: selectedYear || undefined }),
    staleTime: 2 * 60 * 1000,
  })

  const availableGroups = useMemo(() => {
    const groups = Array.from(new Set(allRecords.map((r) => r.sport_group)))
    return sortByKnownOrder(groups, g => g, SPORT_GROUP_ORDER)
  }, [allRecords])

  const excludeMutation = useMutation({
    mutationFn: ({ activityId, category }: { activityId: string; category: string }) =>
      addExclusion(activityId, category),
    onMutate: ({ activityId, category }) => {
      setExcludingKey(`${activityId}-${category}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      queryClient.invalidateQueries({ queryKey: exclusionsQueryKey })
      addToast({ type: 'success', title: 'Aktivität ausgeschlossen' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Ausschluss fehlgeschlagen', message: 'Die Aktivität konnte nicht ausgeschlossen werden' })
    },
    onSettled: () => setExcludingKey(null),
  })

  const reincludeMutation = useMutation({
    mutationFn: ({ activityId, category }: { activityId: string; category: string }) =>
      removeExclusion(activityId, category),
    onMutate: ({ activityId, category }) => {
      setReincludingKey(`${activityId}-${category}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      queryClient.invalidateQueries({ queryKey: exclusionsQueryKey })
      addToast({ type: 'success', title: 'Aktivität wieder eingeschlossen' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Einschließen', message: 'Die Aktivität konnte nicht wieder eingeschlossen werden' })
    },
    onSettled: () => setReincludingKey(null),
  })

  const recalcMutation = useMutation({
    mutationFn: recalcBestEfforts,
    onSuccess: (data) => {
      if (data.status === 'already_running') {
        addToast({ type: 'info', title: 'Neuberechnung läuft bereits' })
      } else {
        addToast({ type: 'success', title: 'Neuberechnung gestartet', message: 'Die Bestleistungen werden im Hintergrund neu berechnet' })
        queryClient.invalidateQueries({ queryKey: ['personal-records'] })
      }
    },
    onError: () => {
      addToast({ type: 'error', title: 'Neuberechnung fehlgeschlagen', duration: 0 })
    },
  })

  // Filter records by tab
  const distRecords = useMemo(() => records.filter((r) => r.category.startsWith(DIST_PREFIX)), [records])
  const elevRecords = useMemo(() => records.filter((r) => r.category.startsWith(ELEV_PREFIX)), [records])
  const powerRecords = useMemo(() => records.filter((r) => r.category.startsWith(POWER_PREFIX)), [records])
  const supRecords = useMemo(() => records.filter((r) => r.category.startsWith(SUP_PREFIX)), [records])

  const handleExclude = (activityId: string, category: string) => {
    excludeMutation.mutate({ activityId, category })
  }

  const handleReinclude = (activityId: string, category: string) => {
    reincludeMutation.mutate({ activityId, category })
  }

  const isOverview = selectedGroup === ''

  return (
    <div className="max-w-4xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-yellow-100 dark:bg-yellow-900/30">
            <Trophy className="w-5 h-5 text-yellow-600 dark:text-yellow-400" />
          </div>
          <div>
            <h1 className="text-xl font-bold text-neutral-900 dark:text-neutral-100">Bestleistungen</h1>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">Persönliche Rekorde nach Kategorie</p>
          </div>
        </div>
        <button
          onClick={() => recalcMutation.mutate()}
          disabled={recalcMutation.isPending}
          className={cn(
            'flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors',
            'text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100',
            'hover:bg-neutral-100 dark:hover:bg-neutral-800',
            recalcMutation.isPending && 'opacity-50 cursor-not-allowed',
          )}
          title="Bestleistungen neu berechnen"
        >
          <RotateCcw className={cn('w-4 h-4', recalcMutation.isPending && 'animate-spin')} />
          <span className="hidden sm:inline">Neu berechnen</span>
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3 mb-5">
        {/* Sport group chips — dynamic based on available data */}
        <div className="flex flex-wrap gap-1.5">
          {/* "Alle" chip — always present */}
          <button
            onClick={() => setSelectedGroup('')}
            className={cn(
              'px-3 py-1 rounded-full text-sm font-medium transition-colors',
              selectedGroup === ''
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700',
            )}
          >
            Alle
          </button>
          {availableGroups.map((group) => (
            <button
              key={group}
              onClick={() => setSelectedGroup(group)}
              className={cn(
                'px-3 py-1 rounded-full text-sm font-medium transition-colors',
                selectedGroup === group
                  ? 'bg-accent-DEFAULT text-white'
                  : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700',
              )}
            >
              {SPORT_GROUP_LABELS[group] ?? group}
            </button>
          ))}
        </div>

        {/* Year filter */}
        <select
          value={selectedYear}
          onChange={(e) => setSelectedYear(e.target.value)}
          className={cn(
            'ml-auto px-3 py-1 rounded-lg text-sm font-medium border transition-colors cursor-pointer',
            'border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900',
            'text-neutral-700 dark:text-neutral-300',
            'focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT/50',
          )}
        >
          {YEAR_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </div>

      {/* Loading / Error states */}
      {isLoading && (
        <div className="flex items-center justify-center py-16 text-sm text-neutral-400">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-neutral-300 border-t-accent-DEFAULT mr-3" />
          Lade Bestleistungen…
        </div>
      )}

      {isError && !isLoading && (
        <div className="flex items-center justify-center py-12 text-sm text-rose-500 dark:text-rose-400">
          Fehler beim Laden der Bestleistungen
        </div>
      )}

      {/* Overview grid for "Alle", tabbed view for specific sport groups */}
      {!isLoading && !isError && isOverview && (
        <>
          <OverviewGrid records={records} />
          {/* Segmente sind sport-agnostisch — auch in "Alle" anzeigen */}
          {(segmentPRs.length > 0 || globalTopEfforts.length > 0) && (
            <div className="mt-8 space-y-6">
              <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">Segmente</h2>
              {segmentPRs.length > 0 && (
                <section>
                  <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-3">Persönliche Bestzeiten (Top 3)</h3>
                  <SegmentPRsTable efforts={segmentPRs} />
                </section>
              )}
              {globalTopEfforts.length > 0 && (
                <section>
                  <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-3">Globale Top 10</h3>
                  <GlobalTopTable efforts={globalTopEfforts} />
                </section>
              )}
            </div>
          )}
        </>
      )}

      {/* Tabbed view — only shown when a specific sport group is selected */}
      {!isLoading && !isError && !isOverview && (
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList className="w-full justify-start overflow-x-auto flex-nowrap gap-0">
            <TabsTrigger value="distanz">
              Distanz
              {distRecords.length > 0 && (
                <span className="ml-1.5 text-xs opacity-60">({distRecords.length})</span>
              )}
            </TabsTrigger>
            <TabsTrigger value="hoehenmeter">
              Höhenmeter
              {elevRecords.length > 0 && (
                <span className="ml-1.5 text-xs opacity-60">({elevRecords.length})</span>
              )}
            </TabsTrigger>
            <TabsTrigger value="power">
              Power
              {powerRecords.length > 0 && (
                <span className="ml-1.5 text-xs opacity-60">({powerRecords.length})</span>
              )}
            </TabsTrigger>
            <TabsTrigger value="superlative">
              Superlative
              {supRecords.length > 0 && (
                <span className="ml-1.5 text-xs opacity-60">({supRecords.length})</span>
              )}
            </TabsTrigger>
            <TabsTrigger value="segmente">
              Segmente
              {segmentPRs.length > 0 && (
                <span className="ml-1.5 text-xs opacity-60">({segmentPRs.length})</span>
              )}
            </TabsTrigger>
            <TabsTrigger value="ausgeschlossen">
              Ausgeschlossen
              {excluded.length > 0 && (
                <span className="ml-1.5 text-xs opacity-60">({excluded.length})</span>
              )}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="distanz" className="mt-4">
            <RecordsTable
              records={distRecords}
              onExclude={handleExclude}
              excludingKey={excludingKey}
            />
          </TabsContent>

          <TabsContent value="hoehenmeter" className="mt-4">
            <RecordsTable
              records={elevRecords}
              onExclude={handleExclude}
              excludingKey={excludingKey}
            />
          </TabsContent>

          <TabsContent value="power" className="mt-4">
            <RecordsTable
              records={powerRecords}
              showPowerNote
              onExclude={handleExclude}
              excludingKey={excludingKey}
            />
          </TabsContent>

          <TabsContent value="superlative" className="mt-4">
            <RecordsTable
              records={supRecords}
              onExclude={handleExclude}
              excludingKey={excludingKey}
            />
          </TabsContent>

          <TabsContent value="segmente" className="mt-4">
            <div className="space-y-8">
              <section>
                <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
                  Persönliche Bestzeiten (Top 3)
                </h3>
                <SegmentPRsTable efforts={segmentPRs} />
              </section>
              <section>
                <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
                  Globale Top 10
                </h3>
                <GlobalTopTable efforts={globalTopEfforts} />
              </section>
            </div>
          </TabsContent>

          <TabsContent value="ausgeschlossen" className="mt-4">
            <div className="space-y-3">
              <p className="text-sm text-neutral-500 dark:text-neutral-400">
                Diese Aktivitäten wurden von der Bestleistungs-Wertung ausgeschlossen.
              </p>
              <ExcludedList
                excluded={excluded}
                onReinclude={handleReinclude}
                reincludingKey={reincludingKey}
              />
            </div>
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}
