import { useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { Link } from 'react-router-dom'
import { fmtDateCompact } from '@/lib/format'
import { fetchActivities, reprocessAllActivities } from '@/api/activities'
import type { ReprocessFilter, ReprocessResult } from '@/api/activities'
import type { Activity } from '@/api/types'
import { sportLabel } from '@/lib/sportTaxonomy'
import { GEO_SCOPE_KEYS, GEO_SCOPE_LABELS, REPROCESS_SCOPES, DEFAULT_SCOPES, toBackendScope, toggleScopeKey, type AnyReprocessKey } from '@/lib/reprocessScopes'

type ReprocessMode = 'all' | 'selection'

export default function WorkshopReprocessPage() {
  const [mode, setMode] = useState<ReprocessMode>('all')
  const [sportFilter, setSportFilter] = useState('')
  const [formatFilter, setFormatFilter] = useState('')
  const [afterFilter, setAfterFilter] = useState('')
  const [beforeFilter, setBeforeFilter] = useState('')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState('')
  const [showConfirm, setShowConfirm] = useState(false)
  const [result, setResult] = useState<ReprocessResult | null>(null)
  const [scope, setScope] = useState<Set<AnyReprocessKey>>(new Set(DEFAULT_SCOPES))

  const { data: activitiesData } = useQuery({
    queryKey: ['activities-for-reprocess'],
    queryFn: () => fetchActivities(undefined, 100),
    enabled: mode === 'selection',
  })

  const { addToast } = useToast()

  const mutation = useMutation({
    mutationFn: (filter?: ReprocessFilter) => reprocessAllActivities(filter),
    onSuccess: (data) => {
      setShowConfirm(false)
      if (data.status === 'already_running') {
        addToast({ type: 'warning', title: 'Job läuft bereits', message: 'Ein Recompute-Job ist bereits aktiv.' })
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
    const allScopes = REPROCESS_SCOPES.length + GEO_SCOPE_KEYS.length
    const converted = scope.size > 0 && scope.size < allScopes ? toBackendScope(scope) : undefined
    const scopeArr = converted && converted.length > 0 ? converted : undefined

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
    if (scope.size === 0) return
    if (mode === 'selection' && selectedIds.size === 0) return
    setShowConfirm(true)
  }

  function handleConfirm() {
    setResult(null)
    mutation.mutate(buildFilter())
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

  const activities = activitiesData?.activities ?? []
  const searchLower = search.toLowerCase()
  const filteredActivities = search
    ? activities.filter(a =>
        (a.title ?? '').toLowerCase().includes(searchLower) ||
        sportLabel(a.sport_type, a.sub_type).toLowerCase().includes(searchLower))
    : activities

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link
          to="/workshop"
          className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
        >
          &#8592; Werkstatt
        </Link>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Neu verarbeiten</h1>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Aktivitäten neu verarbeiten: Die Originaldateien werden erneut geparst und alle abgeleiteten Daten (Streams, Track, Sensoren, Metriken) ersetzt. Nutzerdefinierte Felder (Titel, Beschreibung, Tags) bleiben erhalten.
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
            Alle / Filter
          </button>
          <button
            onClick={() => setMode('selection')}
            className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors cursor-pointer ${
              mode === 'selection'
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            Auswahl
          </button>
        </div>

        {/* Filter mode */}
        {mode === 'all' && (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div>
              <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">Sportart</label>
              <select
                value={sportFilter}
                onChange={e => setSportFilter(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 px-2.5 py-1.5"
              >
                <option value="">Alle</option>
                <option value="cycling">Radfahren</option>
                <option value="running">Laufen</option>
                <option value="hiking">Wandern</option>
                <option value="swimming">Schwimmen</option>
                <option value="walking">Gehen</option>
                <option value="skiing">Ski</option>
                <option value="strength">Kraft</option>
              </select>
            </div>
            <div>
              <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">Dateiformat</label>
              <select
                value={formatFilter}
                onChange={e => setFormatFilter(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 px-2.5 py-1.5"
              >
                <option value="">Alle</option>
                <option value="fit">FIT</option>
                <option value="gpx">GPX</option>
                <option value="tcx">TCX</option>
              </select>
            </div>
            <div>
              <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">Nach</label>
              <input
                type="date"
                value={afterFilter}
                onChange={e => setAfterFilter(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 px-2.5 py-1.5"
              />
            </div>
            <div>
              <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">Vor</label>
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
                placeholder="Titel oder Sportart suchen..."
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900
                           text-sm text-neutral-900 dark:text-neutral-100 pl-8 pr-3 py-1.5
                           placeholder:text-neutral-400 dark:placeholder:text-neutral-500"
              />
            </div>

            <div className="max-h-64 overflow-y-auto rounded-lg border border-neutral-200 dark:border-neutral-700">
              {filteredActivities.length === 0 ? (
                <div className="p-4 text-center text-sm text-neutral-400">
                  {activities.length === 0 ? 'Keine Aktivitäten vorhanden' : 'Keine Treffer'}
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
                      <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">Datum</th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportart</th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">Titel</th>
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
                          {sportLabel(a.sport_type, a.sub_type)}
                        </td>
                        <td className="px-3 py-1.5 text-neutral-900 dark:text-neutral-100 truncate max-w-[200px]">
                          {a.title ?? a.title_auto ?? <span className="text-neutral-400 italic">Kein Titel</span>}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              {selectedIds.size > 0 && (
                <div className="px-3 py-2 text-xs text-neutral-500 dark:text-neutral-400 border-t border-neutral-200 dark:border-neutral-700">
                  {selectedIds.size} ausgewählt
                </div>
              )}
            </div>
          </div>
        )}

        {/* Scope selection */}
        <div>
          <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-2">Was neu berechnen?</label>
          <div className="flex flex-wrap gap-x-5 gap-y-1.5">
            {REPROCESS_SCOPES.map(([key, label]) => (
              <label key={key} className="flex items-center gap-1.5 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer">
                <input
                  type="checkbox"
                  checked={scope.has(key)}
                  onChange={() => toggleScope(key)}
                  className="rounded"
                />
                {label}
              </label>
            ))}
            <span className="text-xs text-neutral-400 dark:text-neutral-500 w-full pt-1">Geo-Anreicherung (ML, asynchron)</span>
            {GEO_SCOPE_KEYS.map(key => (
              <label key={key} className="flex items-center gap-1.5 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer">
                <input
                  type="checkbox"
                  checked={scope.has(key)}
                  onChange={() => toggleScope(key)}
                  className="rounded"
                />
                {GEO_SCOPE_LABELS[key]}
              </label>
            ))}
          </div>
        </div>

        {/* Action */}
        <div className="flex items-center gap-4">
          <button
            onClick={handleStart}
            disabled={mutation.isPending || scope.size === 0 || (mode === 'selection' && selectedIds.size === 0)}
            className="px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT text-white
                       hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed
                       transition-colors cursor-pointer"
          >
            {mutation.isPending ? 'Verarbeite...' : 'Neu verarbeiten'}
          </button>

          {result && (
            <span className="text-sm text-neutral-600 dark:text-neutral-300">
              {result.processed} / {result.total} verarbeitet
              {result.errors > 0 && (
                <span className="text-red-500 ml-2">({result.errors} Fehler)</span>
              )}
            </span>
          )}
        </div>

        {/* Confirmation dialog */}
        {showConfirm && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
            <div className="bg-white dark:bg-neutral-900 rounded-xl shadow-xl p-6 max-w-sm w-full mx-4 space-y-4">
              <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
                Aktivitäten neu verarbeiten?
              </h3>
              <p className="text-sm text-neutral-500 dark:text-neutral-400">
                {mode === 'selection'
                  ? `${selectedIds.size} ausgewählte Aktivität${selectedIds.size === 1 ? '' : 'en'} werden neu verarbeitet.`
                  : 'Alle passenden Aktivitäten werden neu verarbeitet.'}
              </p>
              <p className="text-sm text-neutral-500 dark:text-neutral-400">
                Umfang: {scope.size === 0 ? 'Nichts ausgewählt' : [
                  ...REPROCESS_SCOPES.filter(([key]) => scope.has(key)).map(([, label]) => label),
                  ...GEO_SCOPE_KEYS.filter(k => scope.has(k)).map(k => GEO_SCOPE_LABELS[k]),
                ].join(', ')}
              </p>
              <p className="text-sm text-neutral-500 dark:text-neutral-400">
                Nutzerdefinierte Felder (Titel, Beschreibung, Tags, Sichtbarkeit) bleiben erhalten.
              </p>
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setShowConfirm(false)}
                  className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400
                             hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                >
                  Abbrechen
                </button>
                <button
                  onClick={handleConfirm}
                  className="px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT text-white
                             hover:bg-accent-600 transition-colors cursor-pointer"
                >
                  Bestätigen
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
