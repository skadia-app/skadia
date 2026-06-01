import { useState } from 'react'
import { useInfiniteQuery, useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useToast } from '@/lib/toast'
import { Link } from 'react-router-dom'
import { fmtDateCompact } from '@/lib/format'
import { fetchActivities, reprocessAllActivities, isReprocessAccepted } from '@/api/activities'
import type { ReprocessFilter, ReprocessCompleted } from '@/api/activities'
import type { Activity } from '@/api/types'
import { sportLabel } from '@/lib/sportTaxonomy'
import { displayActivityTitle } from '@/lib/activityTitle'
import { availableReprocessScopeKeys, DEFAULT_SCOPES, toBackendScope, toggleScopeKey, type AnyReprocessKey } from '@/lib/reprocessScopes'
import { analysisModuleLabelKeyForScope, ScopeCheckboxList } from '@/components/ScopeCheckboxList'
import { useCapabilities } from '@/hooks/useCapabilities'
import { isSupportedLanguage } from '@/i18n/languages'

type ReprocessMode = 'all' | 'selection'
const SELECTION_PAGE_SIZE = 100

export default function WorkshopReprocessPage() {
  const { t, i18n } = useTranslation()
  const language = isSupportedLanguage(i18n.language) ? i18n.language : 'de'
  const [mode, setMode] = useState<ReprocessMode>('all')
  const [sportFilter, setSportFilter] = useState('')
  const [formatFilter, setFormatFilter] = useState('')
  const [afterFilter, setAfterFilter] = useState('')
  const [beforeFilter, setBeforeFilter] = useState('')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState('')
  const [showConfirm, setShowConfirm] = useState(false)
  const [result, setResult] = useState<ReprocessCompleted | null>(null)
  const [scope, setScope] = useState<Set<AnyReprocessKey>>(new Set(DEFAULT_SCOPES))
  const searchQuery = search.trim()
  const { data: capabilities } = useCapabilities()
  const availableScopeKeys = availableReprocessScopeKeys(capabilities)
  const selectedScopeKeys = toBackendScope(scope, availableScopeKeys)

  const {
    data: activitiesData,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading: activitiesLoading,
  } = useInfiniteQuery({
    queryKey: ['activities-for-reprocess', searchQuery],
    queryFn: ({ pageParam }) => fetchActivities(
      pageParam as string | undefined,
      SELECTION_PAGE_SIZE,
      undefined,
      undefined,
      searchQuery ? { search: searchQuery } : undefined,
    ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: lastPage => lastPage.next_cursor ?? undefined,
    enabled: mode === 'selection',
  })

  const { addToast } = useToast()

  const mutation = useMutation({
    mutationFn: (filter?: ReprocessFilter) => reprocessAllActivities(filter),
    onSuccess: (data) => {
      setShowConfirm(false)
      if (isReprocessAccepted(data)) {
        // 202: background job started — progress tracked via global job banner
        addToast({ type: 'info', title: t('workshopOperations.reprocess.toasts.startedTitle'), message: t('workshopOperations.reprocess.toasts.startedMessage', { count: data.total }) })
      } else if (data.status === 'already_running') {
        addToast({ type: 'warning', title: t('workshopOperations.reprocess.toasts.alreadyRunningTitle'), message: t('workshopOperations.reprocess.toasts.alreadyRunningMessage') })
      } else {
        setResult(data)
      }
    },
    onError: () => {
      setShowConfirm(false)
    },
  })

  function toggleScope(key: AnyReprocessKey) {
    setScope(prev => toggleScopeKey(prev, key))
  }

  function buildFilter(): ReprocessFilter | undefined {
    const scopeArr = selectedScopeKeys.length > 0 ? selectedScopeKeys : undefined

    if (mode === 'selection') {
      if (selectedIds.size === 0) return undefined
      return { ids: Array.from(selectedIds), scope: scopeArr }
    }

    const filter: ReprocessFilter = {}
    if (sportFilter) filter.sport_types = [sportFilter]
    if (formatFilter) filter.format = formatFilter
    if (afterFilter) filter.after = new Date(afterFilter).toISOString()
    if (beforeFilter) filter.before = new Date(beforeFilter).toISOString()
    if (scopeArr) filter.scope = scopeArr

    return Object.keys(filter).length > 0 ? filter : undefined
  }

  function handleStart() {
    if (selectedScopeKeys.length === 0) return
    if (mode === 'selection' && selectedIds.size === 0) return
    setShowConfirm(true)
  }

  function handleConfirm() {
    setResult(null)
    mutation.mutate(buildFilter())
  }

  function reprocessScopeLabel(key: string): string {
    const scopeKey = `reprocessScopes.items.${key}.label`
    return t(i18n.exists(scopeKey) ? scopeKey : analysisModuleLabelKeyForScope(key))
  }

  function toggleId(id: string) {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleAll(visible: Activity[]) {
    const allVisible = visible.every(a => selectedIds.has(a.id))
    if (allVisible) {
      setSelectedIds(prev => {
        const next = new Set(prev)
        for (const a of visible) next.delete(a.id)
        return next
      })
    } else {
      setSelectedIds(prev => {
        const next = new Set(prev)
        for (const a of visible) next.add(a.id)
        return next
      })
    }
  }

  const activities = activitiesData?.pages.flatMap(page => page.activities) ?? []
  const filteredActivities = activities
  const tagsReprocessed = selectedScopeKeys.includes('tags')
  const reprocessPreservationCopy = t('workshopOperations.reprocess.preservationCopy')

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link
          to="/workshop"
          className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
        >
          &#8592; {t('workshopOperations.workshop.title')}
        </Link>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{t('workshopOperations.reprocess.title')}</h1>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {t('workshopOperations.reprocess.description', { preservation: reprocessPreservationCopy })}
        </p>

        {/* Mode toggle */}
        <div className="flex gap-2">
          <button
            onClick={() => setMode('all')}
            className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors cursor-pointer ${
              mode === 'all'
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            {t('workshopOperations.reprocess.modeAll')}
          </button>
          <button
            onClick={() => setMode('selection')}
            className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors cursor-pointer ${
              mode === 'selection'
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            {t('workshopOperations.reprocess.modeSelection')}
          </button>
        </div>

        {/* Filter mode */}
        {mode === 'all' && (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div>
              <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">{t('workshopOperations.reprocess.filters.sport')}</label>
              <select
                value={sportFilter}
                onChange={e => setSportFilter(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 px-2.5 py-1.5"
              >
                <option value="">{t('common.all')}</option>
                <option value="cycling">{t('workshopOperations.reprocess.sports.cycling')}</option>
                <option value="running">{t('workshopOperations.reprocess.sports.running')}</option>
                <option value="hiking">{t('workshopOperations.reprocess.sports.hiking')}</option>
                <option value="swimming">{t('workshopOperations.reprocess.sports.swimming')}</option>
                <option value="walking">{t('workshopOperations.reprocess.sports.walking')}</option>
                <option value="skiing">{t('workshopOperations.reprocess.sports.skiing')}</option>
                <option value="strength_training">{t('workshopOperations.reprocess.sports.strengthTraining')}</option>
              </select>
            </div>
            <div>
              <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">{t('workshopOperations.reprocess.filters.fileFormat')}</label>
              <select
                value={formatFilter}
                onChange={e => setFormatFilter(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 px-2.5 py-1.5"
              >
                <option value="">{t('common.all')}</option>
                <option value="fit">FIT</option>
                <option value="gpx">GPX</option>
                <option value="tcx">TCX</option>
              </select>
            </div>
            <div>
              <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">{t('workshopOperations.reprocess.filters.after')}</label>
              <input
                type="date"
                value={afterFilter}
                onChange={e => setAfterFilter(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 px-2.5 py-1.5"
              />
            </div>
            <div>
              <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">{t('workshopOperations.reprocess.filters.before')}</label>
              <input
                type="date"
                value={beforeFilter}
                onChange={e => setBeforeFilter(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 px-2.5 py-1.5"
              />
            </div>
          </div>
        )}

        {/* Selection mode */}
        {mode === 'selection' && (
          <div className="space-y-2">
            <div className="relative">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor"
                className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-neutral-400 pointer-events-none">
                <path fillRule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clipRule="evenodd" />
              </svg>
              <input
                type="text"
                value={search}
                onChange={e => setSearch(e.target.value)}
                placeholder={t('workshopOperations.reprocess.searchPlaceholder')}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 pl-8 pr-3 py-1.5
                           placeholder:text-neutral-400 dark:placeholder:text-neutral-500"
              />
            </div>

            <div className="max-h-64 overflow-y-auto rounded-lg border border-neutral-200 dark:border-neutral-700">
              {activitiesLoading ? (
                <div className="p-4 text-center text-sm text-neutral-400">
                  {t('workshopOperations.reprocess.loadingActivities')}
                </div>
              ) : filteredActivities.length === 0 ? (
                <div className="p-4 text-center text-sm text-neutral-400">
                  {activities.length === 0 ? t('workshopOperations.reprocess.noActivities') : t('workshopOperations.reprocess.noMatches')}
                </div>
              ) : (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
                      <th className="px-3 py-2 text-left">
                        <input
                          type="checkbox"
                          checked={filteredActivities.length > 0 && filteredActivities.every(a => selectedIds.has(a.id))}
                          onChange={() => toggleAll(filteredActivities)}
                          className="rounded"
                        />
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('workshopOperations.reprocess.table.date')}</th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('workshopOperations.reprocess.table.sport')}</th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('workshopOperations.reprocess.table.title')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredActivities.map((a, i) => (
                      <tr
                        key={a.id}
                        className={`border-b border-neutral-100 dark:border-neutral-800 cursor-pointer
                          hover:bg-neutral-50 dark:hover:bg-neutral-800/50
                          ${i % 2 === 0 ? '' : 'bg-neutral-50/50 dark:bg-neutral-800/20'}`}
                        onClick={() => toggleId(a.id)}
                      >
                        <td className="px-3 py-1.5" onClick={e => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            checked={selectedIds.has(a.id)}
                            onChange={() => toggleId(a.id)}
                            className="rounded"
                          />
                        </td>
                        <td className="px-3 py-1.5 text-neutral-600 dark:text-neutral-300 whitespace-nowrap">
                          {fmtDateCompact(a.started_at, a.timezone)}
                        </td>
                        <td className="px-3 py-1.5 text-neutral-600 dark:text-neutral-300">
                          {sportLabel(a.sport_type, a.sub_type, { language })}
                        </td>
                        <td className="px-3 py-1.5 text-neutral-900 dark:text-neutral-100 truncate max-w-[200px]">
                          {displayActivityTitle(a, t('workshopOperations.reprocess.untitled')) === t('workshopOperations.reprocess.untitled')
                            ? <span className="text-neutral-400 italic">{t('workshopOperations.reprocess.untitled')}</span>
                            : displayActivityTitle(a, t('workshopOperations.reprocess.untitled'))}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              {selectedIds.size > 0 && (
                <div className="px-3 py-2 text-xs text-neutral-500 dark:text-neutral-400 border-t border-neutral-200 dark:border-neutral-700">
                  {t('workshopOperations.reprocess.selectedCount', { count: selectedIds.size })}
                </div>
              )}
              {hasNextPage && (
                <div className="px-3 py-2 border-t border-neutral-200 dark:border-neutral-700">
                  <button
                    onClick={() => fetchNextPage()}
                    disabled={isFetchingNextPage}
                    className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 disabled:opacity-50 cursor-pointer"
                  >
                    {isFetchingNextPage ? t('common.loading') : t('workshopOperations.reprocess.loadMore')}
                  </button>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Scope selection */}
        <div>
          <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-2">{t('workshopOperations.reprocess.scopeLabel')}</label>
          <ScopeCheckboxList selected={scope} onToggle={toggleScope} layout="inline" capabilities={capabilities} />
        </div>

        {/* Action */}
        <div className="flex items-center gap-4">
          <button
            onClick={handleStart}
            disabled={mutation.isPending || selectedScopeKeys.length === 0 || (mode === 'selection' && selectedIds.size === 0)}
            className="px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT text-white
                       hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed
                       transition-colors cursor-pointer"
          >
            {mutation.isPending ? t('workshopOperations.reprocess.processing') : t('workshopOperations.reprocess.startButton')}
          </button>

          {result && (
            <span className="text-sm text-neutral-600 dark:text-neutral-300">
              {t('workshopOperations.reprocess.processedResult', { processed: result.processed, total: result.total })}
              {result.errors > 0 && (
                <span className="text-red-500 ml-2">({t('workshopOperations.reprocess.errorCount', { count: result.errors })})</span>
              )}
            </span>
          )}
        </div>

        {/* Confirmation dialog */}
        {showConfirm && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
            <div className="bg-white dark:bg-neutral-900 rounded-xl shadow-xl p-6 max-w-sm w-full mx-4 space-y-4">
              <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
                {t('workshopOperations.reprocess.confirmTitle')}
              </h3>
              <p className="text-sm text-neutral-500 dark:text-neutral-400">
                {mode === 'selection'
                  ? t('workshopOperations.reprocess.confirmSelection', { count: selectedIds.size })
                  : t('workshopOperations.reprocess.confirmAll')}
              </p>
              <p className="text-sm text-neutral-500 dark:text-neutral-400">
                {t('workshopOperations.reprocess.scopeSummary')}: {selectedScopeKeys.length === 0 ? t('workshopOperations.reprocess.noScopeSelected') : [
                  ...selectedScopeKeys.map(key => reprocessScopeLabel(key)),
                ].join(', ')}
              </p>
              <p className="text-sm text-neutral-500 dark:text-neutral-400">
                {tagsReprocessed
                  ? reprocessPreservationCopy
                  : t('workshopOperations.reprocess.preservationWithoutTags')}
              </p>
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setShowConfirm(false)}
                  className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400
                             hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                >
                  {t('common.cancel')}
                </button>
                <button
                  onClick={handleConfirm}
                  className="px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT text-white
                             hover:bg-accent-600 transition-colors cursor-pointer"
                >
                  {t('common.confirm')}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
