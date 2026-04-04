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
import { fmtDateMedium } from '@/lib/format'
import { useToast } from '@/lib/toast'
import { cn } from '@/lib/utils'
import { CATEGORY_LABELS, categoryLabel as getCategoryLabel, formatBestEffortValue } from '@/lib/bestEffortLabels'

// ── Constants ──────────────────────────────────────────────────────────────

const SPORT_GROUPS = [
  { value: '', label: 'Alle' },
  { value: 'running', label: 'Laufen' },
  { value: 'cycling', label: 'Radfahren' },
  { value: 'hiking', label: 'Wandern' },
  { value: 'swim_pool', label: 'Schwimmen (Becken)' },
  { value: 'swim_ow', label: 'Schwimmen (Freiwasser)' },
]

const CURRENT_YEAR = new Date().getFullYear()
const YEAR_OPTIONS = [
  { value: '', label: 'All-Time' },
  { value: String(CURRENT_YEAR), label: String(CURRENT_YEAR) },
  { value: String(CURRENT_YEAR - 1), label: String(CURRENT_YEAR - 1) },
  { value: String(CURRENT_YEAR - 2), label: String(CURRENT_YEAR - 2) },
]

// Categories prefixes by tab
const DIST_PREFIX = 'dist_'
const ELEV_PREFIX = 'elev_'
const POWER_PREFIX = 'power_'
const SUP_PREFIX = 'sup_'

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
    // Sort by category order: use the CATEGORY_LABELS key order as a proxy
    return Array.from(map.entries()).sort(([a], [b]) => {
      const keys = Object.keys(CATEGORY_LABELS)
      const ia = keys.indexOf(a)
      const ib = keys.indexOf(b)
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
                      Aktivität ansehen
                    </Link>
                    <span className="text-xs text-neutral-400">{fmtDateMedium(record.achieved_at)}</span>
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
        {/* Sport group chips */}
        <div className="flex flex-wrap gap-1.5">
          {SPORT_GROUPS.map((group) => (
            <button
              key={group.value}
              onClick={() => setSelectedGroup(group.value)}
              className={cn(
                'px-3 py-1 rounded-full text-sm font-medium transition-colors',
                selectedGroup === group.value
                  ? 'bg-accent-DEFAULT text-white'
                  : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700',
              )}
            >
              {group.label}
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

      {/* Tabs */}
      {!isLoading && !isError && (
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
            <TabsTrigger value="segmente">Segmente</TabsTrigger>
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
            <div className="flex flex-col items-center justify-center py-16 gap-4 text-neutral-400 dark:text-neutral-500">
              <MapPin className="w-10 h-10 opacity-30" />
              <p className="text-sm text-center">
                Segment-Bestzeiten — kommt in einem zukünftigen Update
              </p>
              <Link
                to="/segments"
                className="text-sm text-accent-DEFAULT hover:underline"
              >
                Zu den Segmenten
              </Link>
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
