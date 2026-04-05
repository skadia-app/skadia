import { useMemo } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Trophy, RotateCcw } from 'lucide-react'
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
} from '@/api/segments'
import { categoryLabel as getCategoryLabel } from '@/lib/bestEffortLabels'
import { useToast } from '@/lib/toast'
import { cn } from '@/lib/utils'
import {
  MAIN_CATEGORIES,
  MAIN_CATEGORY_LABELS,
  MAIN_CATEGORY_COLORS,
  SUB_FILTERS,
  POWER_CURVE_GROUPS,
  CATEGORY_PREFIXES,
  type MainCategory,
} from '@/lib/recordsConstants'
import { HighlightBanner } from '@/components/records/HighlightBanner'
import { SportCard } from '@/components/records/SportCard'
import { CategoryGrid } from '@/components/records/CategoryGrid'
import { DetailTable } from '@/components/records/DetailTable'
import { SuperlativeBar } from '@/components/records/SuperlativeBar'
import { SubFilterPills } from '@/components/records/SubFilterPills'
import { PowerDurationChart } from '@/components/records/PowerDurationChart'
import { SegmentSection } from '@/components/records/SegmentSection'

// ── Constants ──────────────────────────────────────────────────────────────

const CURRENT_YEAR = new Date().getFullYear()
const YEAR_OPTIONS = [
  { value: '', label: 'Gesamt' },
  { value: String(CURRENT_YEAR), label: String(CURRENT_YEAR) },
  { value: String(CURRENT_YEAR - 1), label: String(CURRENT_YEAR - 1) },
  { value: String(CURRENT_YEAR - 2), label: String(CURRENT_YEAR - 2) },
]

const ALL_MAIN_CATEGORIES: MainCategory[] = ['laufen', 'radfahren', 'wandern', 'schwimmen']

// ── Excluded List ──────────────────────────────────────────────────────────

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

  // ── URL state ────────────────────────────────────────────────────────────
  const [searchParams, setSearchParams] = useSearchParams()
  const mainCategory = (searchParams.get('sport') as MainCategory) ?? null
  const subFilter = searchParams.get('sub') ?? null
  const year = searchParams.get('year') ?? ''
  const categoryTab = searchParams.get('tab') ?? 'distanz'

  const setMainCategory = (cat: MainCategory | null) => {
    setSearchParams(prev => {
      if (cat) prev.set('sport', cat); else prev.delete('sport')
      prev.delete('sub')
      prev.delete('tab')
      return prev
    }, { replace: true })
  }

  const setSubFilter = (label: string | null) => {
    setSearchParams(prev => {
      if (label) prev.set('sub', label); else prev.delete('sub')
      return prev
    }, { replace: true })
  }

  const setYear = (y: string) => {
    setSearchParams(prev => {
      if (y) prev.set('year', y); else prev.delete('year')
      return prev
    }, { replace: true })
  }

  const setCategoryTab = (tab: string) => {
    setSearchParams(prev => {
      prev.set('tab', tab)
      return prev
    }, { replace: true })
  }

  // ── Queries ───────────────────────────────────────────────────────────────
  const queryKey = ['personal-records', '', year]

  const { data: allRecords = [], isLoading, isError } = useQuery({
    queryKey,
    queryFn: () => fetchPersonalRecords({ year: year || undefined }),
    staleTime: 2 * 60 * 1000,
  })

  const { data: excluded = [] } = useQuery({
    queryKey: ['best-effort-exclusions'],
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

  // ── Mutations ─────────────────────────────────────────────────────────────
  const excludeMutation = useMutation({
    mutationFn: ({ activityId, category }: { activityId: string; category: string }) =>
      addExclusion(activityId, category),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      queryClient.invalidateQueries({ queryKey: ['best-effort-exclusions'] })
      addToast({ type: 'success', title: 'Aktivität ausgeschlossen' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Ausschluss fehlgeschlagen', message: 'Die Aktivität konnte nicht ausgeschlossen werden' })
    },
  })

  const reincludeMutation = useMutation({
    mutationFn: ({ activityId, category }: { activityId: string; category: string }) =>
      removeExclusion(activityId, category),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      queryClient.invalidateQueries({ queryKey: ['best-effort-exclusions'] })
      addToast({ type: 'success', title: 'Aktivität wieder eingeschlossen' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Einschließen', message: 'Die Aktivität konnte nicht wieder eingeschlossen werden' })
    },
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

  // ── Filtered records ──────────────────────────────────────────────────────
  const filteredRecords = useMemo(() => {
    let filtered = allRecords
    if (mainCategory) {
      const groups = MAIN_CATEGORIES[mainCategory]
      filtered = filtered.filter(r => groups.includes(r.sport_group))
    }
    if (subFilter && mainCategory) {
      const sf = SUB_FILTERS[mainCategory].find(f => f.label === subFilter)
      if (sf) filtered = filtered.filter(r => sf.sportGroups.includes(r.sport_group))
    }
    return filtered
  }, [allRecords, mainCategory, subFilter])

  // Category tab splits
  const distRecords = useMemo(() => filteredRecords.filter(r => r.category.startsWith(CATEGORY_PREFIXES.dist)), [filteredRecords])
  const elevRecords = useMemo(() => filteredRecords.filter(r => r.category.startsWith(CATEGORY_PREFIXES.elev)), [filteredRecords])
  const powerRecords = useMemo(() => filteredRecords.filter(r => r.category.startsWith(CATEGORY_PREFIXES.power)), [filteredRecords])
  const supRecords = useMemo(() => filteredRecords.filter(r => r.category.startsWith(CATEGORY_PREFIXES.sup)), [filteredRecords])

  const handleExclude = (activityId: string, category: string) => {
    excludeMutation.mutate({ activityId, category })
  }

  const handleReinclude = (activityId: string, category: string) => {
    reincludeMutation.mutate({ activityId, category })
  }

  // Segment sport filter for detail mode
  const detailSegmentFilter = mainCategory
    ? MAIN_CATEGORIES[mainCategory]
    : undefined

  // Power series for detail mode
  const detailPowerSeries = useMemo(() => {
    if (!mainCategory) return []
    const groups = POWER_CURVE_GROUPS[mainCategory]
    if (!groups) return []
    return [{ name: MAIN_CATEGORY_LABELS[mainCategory], color: MAIN_CATEGORY_COLORS[mainCategory], sportGroups: groups }]
  }, [mainCategory])

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div className="max-w-6xl mx-auto px-6 pb-20">
      {/* Header */}
      <div className="flex items-center justify-between mb-6 pt-2">
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

      {/* Sport tabs + Year dropdown */}
      <div className="flex items-center gap-2 mb-5 flex-wrap">
        {/* Alle tab */}
        <button
          onClick={() => setMainCategory(null)}
          className={cn(
            'flex items-center gap-1.5 px-3 py-1.5 rounded-full text-sm font-medium transition-colors border',
            mainCategory === null
              ? 'bg-orange-500 text-white border-orange-500'
              : 'bg-transparent text-neutral-600 dark:text-neutral-400 border-neutral-300 dark:border-neutral-700 hover:border-neutral-400 dark:hover:border-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200',
          )}
        >
          Alle
        </button>

        {/* Per-sport tabs */}
        {ALL_MAIN_CATEGORIES
          .filter(cat => allRecords.some(r => MAIN_CATEGORIES[cat].includes(r.sport_group)))
          .map(cat => (
            <button
              key={cat}
              onClick={() => setMainCategory(cat)}
              className={cn(
                'flex items-center gap-1.5 px-3 py-1.5 rounded-full text-sm font-medium transition-colors border',
                mainCategory === cat
                  ? 'bg-orange-500 text-white border-orange-500'
                  : 'bg-transparent text-neutral-600 dark:text-neutral-400 border-neutral-300 dark:border-neutral-700 hover:border-neutral-400 dark:hover:border-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200',
              )}
            >
              <span
                className="w-2 h-2 rounded-full shrink-0"
                style={{ backgroundColor: mainCategory === cat ? 'white' : MAIN_CATEGORY_COLORS[cat] }}
              />
              {MAIN_CATEGORY_LABELS[cat]}
            </button>
          ))
        }

        {/* Year dropdown — pushed to right */}
        <select
          value={year}
          onChange={(e) => setYear(e.target.value)}
          className={cn(
            'ml-auto px-3 py-1.5 rounded-lg text-sm font-medium border transition-colors cursor-pointer',
            'border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900',
            'text-neutral-700 dark:text-neutral-300',
            'focus:outline-none focus:ring-2 focus:ring-orange-500/30',
          )}
        >
          {YEAR_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </div>

      {/* Loading state */}
      {isLoading && (
        <div className="flex items-center justify-center py-16 text-sm text-neutral-400">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-neutral-300 border-t-orange-500 mr-3" />
          Lade Bestleistungen…
        </div>
      )}

      {/* Error state */}
      {isError && !isLoading && (
        <div className="flex items-center justify-center py-12 text-sm text-rose-500 dark:text-rose-400">
          Fehler beim Laden der Bestleistungen
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !isError && allRecords.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 gap-3 text-neutral-400 dark:text-neutral-500">
          <Trophy className="w-10 h-10 opacity-30" />
          <p className="text-sm">Noch keine Bestleistungen vorhanden</p>
        </div>
      )}

      {/* ── OVERVIEW MODE (mainCategory === null) ── */}
      {!isLoading && !isError && allRecords.length > 0 && mainCategory === null && (
        <div className="space-y-6">
          <HighlightBanner records={allRecords} />

          {ALL_MAIN_CATEGORIES
            .filter(cat => allRecords.some(r => MAIN_CATEGORIES[cat].includes(r.sport_group)))
            .map(cat => {
              const catRecords = allRecords.filter(r => MAIN_CATEGORIES[cat].includes(r.sport_group))
              return (
                <SportCard
                  key={cat}
                  mainCategory={cat}
                  records={catRecords}
                  color={MAIN_CATEGORY_COLORS[cat]}
                  onNavigate={() => setMainCategory(cat)}
                />
              )
            })
          }

          <PowerDurationChart
            records={allRecords}
            series={[
              { name: 'Laufen', color: '#3B82F6', sportGroups: POWER_CURVE_GROUPS.laufen! },
              { name: 'Radfahren', color: '#F97316', sportGroups: POWER_CURVE_GROUPS.radfahren! },
            ]}
          />

          <SegmentSection
            personalPRs={segmentPRs}
            globalTop={globalTopEfforts}
          />
        </div>
      )}

      {/* ── DETAIL MODE (mainCategory !== null) ── */}
      {!isLoading && !isError && allRecords.length > 0 && mainCategory !== null && (
        <div className="space-y-5">
          {/* Sub-filter pills */}
          {SUB_FILTERS[mainCategory].length > 0 && (
            <SubFilterPills
              filters={SUB_FILTERS[mainCategory]}
              active={subFilter}
              onChange={setSubFilter}
            />
          )}

          {/* Superlative banner for this category */}
          <SuperlativeBar records={filteredRecords} />

          {/* Category tabs */}
          <Tabs value={categoryTab} onValueChange={setCategoryTab}>
            <TabsList className="w-full justify-start overflow-x-auto flex-nowrap gap-0">
              <TabsTrigger value="distanz" disabled={distRecords.length === 0}>
                Distanz
                {distRecords.length > 0 && (
                  <span className="ml-1.5 text-xs opacity-60">({distRecords.length})</span>
                )}
              </TabsTrigger>
              <TabsTrigger value="hoehenmeter" disabled={elevRecords.length === 0}>
                Höhenmeter
                {elevRecords.length > 0 && (
                  <span className="ml-1.5 text-xs opacity-60">({elevRecords.length})</span>
                )}
              </TabsTrigger>
              <TabsTrigger value="leistung" disabled={powerRecords.length === 0}>
                Leistung
                {powerRecords.length > 0 && (
                  <span className="ml-1.5 text-xs opacity-60">({powerRecords.length})</span>
                )}
              </TabsTrigger>
              <TabsTrigger value="segmente">
                Segmente
              </TabsTrigger>
              <TabsTrigger value="ausgeschlossen">
                Ausgeschlossen
                {excluded.length > 0 && (
                  <span className="ml-1.5 text-xs opacity-60">({excluded.length})</span>
                )}
              </TabsTrigger>
            </TabsList>

            {/* Distanz tab */}
            <TabsContent value="distanz" className="mt-5 space-y-5">
              {distRecords.length === 0 ? (
                <EmptyTabState />
              ) : (
                <>
                  <CategoryGrid records={distRecords} />
                  <DetailTable records={distRecords} />
                </>
              )}
            </TabsContent>

            {/* Höhenmeter tab */}
            <TabsContent value="hoehenmeter" className="mt-5 space-y-5">
              {elevRecords.length === 0 ? (
                <EmptyTabState />
              ) : (
                <>
                  <CategoryGrid records={elevRecords} />
                  <DetailTable records={elevRecords} />
                </>
              )}
            </TabsContent>

            {/* Leistung tab */}
            <TabsContent value="leistung" className="mt-5 space-y-5">
              {powerRecords.length === 0 ? (
                <EmptyTabState />
              ) : (
                <>
                  <CategoryGrid records={powerRecords} />
                  <DetailTable records={powerRecords} />
                  {detailPowerSeries.length > 0 && (
                    <PowerDurationChart
                      records={filteredRecords}
                      series={detailPowerSeries}
                    />
                  )}
                </>
              )}
            </TabsContent>

            {/* Segmente tab */}
            <TabsContent value="segmente" className="mt-5">
              <SegmentSection
                personalPRs={segmentPRs}
                globalTop={globalTopEfforts}
                sportFilter={detailSegmentFilter}
              />
              {segmentPRs.length === 0 && globalTopEfforts.length === 0 && (
                <EmptyTabState message="Keine Segment-Bestzeiten für diese Sportart" />
              )}
            </TabsContent>

            {/* Ausgeschlossen tab */}
            <TabsContent value="ausgeschlossen" className="mt-5">
              <div className="space-y-3">
                <p className="text-sm text-neutral-500 dark:text-neutral-400">
                  Diese Aktivitäten wurden von der Bestleistungs-Wertung ausgeschlossen.
                </p>
                <ExcludedList
                  excluded={excluded}
                  onReinclude={handleReinclude}
                  reincludingKey={reincludeMutation.isPending
                    ? `${reincludeMutation.variables?.activityId}-${reincludeMutation.variables?.category}`
                    : null}
                />
              </div>
            </TabsContent>
          </Tabs>

          {/* Superlative tab — shown below tabs when viewing dist/elev/leistung */}
          {(categoryTab === 'distanz' || categoryTab === 'hoehenmeter') && supRecords.length > 0 && (
            <div className="space-y-2">
              <h3 className="text-xs font-semibold uppercase tracking-wider text-neutral-500 dark:text-neutral-600">Superlative</h3>
              <DetailTable records={supRecords} />
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ── Small helper ───────────────────────────────────────────────────────────

function EmptyTabState({ message = 'Keine Daten für diese Auswahl' }: { message?: string }) {
  return (
    <div className="flex flex-col items-center justify-center py-12 gap-3 text-neutral-400 dark:text-neutral-500">
      <Trophy className="w-8 h-8 opacity-30" />
      <p className="text-sm">{message}</p>
    </div>
  )
}
