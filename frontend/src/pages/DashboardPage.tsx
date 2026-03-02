import { useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import PMCChart from '@/components/PMCChart'
import { fetchActivities, reprocessAllActivities } from '@/api/activities'
import type { ReprocessFilter, ReprocessResult } from '@/api/activities'
import type { Activity } from '@/api/types'
import { sportLabel } from '@/lib/utils'

export default function DashboardPage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Dashboard</h1>
      <PMCChart />
      <MaintenanceSection />
    </div>
  )
}

// ── Maintenance Section ───────────────────────────────────────────────────────

type ReprocessMode = 'all' | 'selection'

function MaintenanceSection() {
  const [mode, setMode] = useState<ReprocessMode>('all')
  const [sportFilter, setSportFilter] = useState('')
  const [formatFilter, setFormatFilter] = useState('')
  const [afterFilter, setAfterFilter] = useState('')
  const [beforeFilter, setBeforeFilter] = useState('')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [showConfirm, setShowConfirm] = useState(false)
  const [result, setResult] = useState<ReprocessResult | null>(null)

  // Load activities for selection mode (first 100).
  const { data: activitiesData } = useQuery({
    queryKey: ['activities-for-reprocess'],
    queryFn: () => fetchActivities(undefined, 100),
    enabled: mode === 'selection',
  })

  const mutation = useMutation({
    mutationFn: (filter?: ReprocessFilter) => reprocessAllActivities(filter),
    onSuccess: (data) => {
      setResult(data)
      setShowConfirm(false)
    },
    onError: () => {
      setShowConfirm(false)
    },
  })

  function buildFilter(): ReprocessFilter | undefined {
    if (mode === 'selection') {
      if (selectedIds.size === 0) return undefined
      return { ids: Array.from(selectedIds) }
    }

    const filter: ReprocessFilter = {}
    if (sportFilter) filter.sport_types = [sportFilter]
    if (formatFilter) filter.format = formatFilter
    if (afterFilter) filter.after = new Date(afterFilter).toISOString()
    if (beforeFilter) filter.before = new Date(beforeFilter).toISOString()

    return Object.keys(filter).length > 0 ? filter : undefined
  }

  function handleStart() {
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

  function toggleAll(activities: Activity[]) {
    if (selectedIds.size === activities.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(activities.map(a => a.id)))
    }
  }

  const activities = activitiesData?.activities ?? []

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Wartung</h2>
      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        Aktivitäten neu verarbeiten: Die Originaldateien werden erneut geparst und alle abgeleiteten Daten (Streams, Track, Sensoren, Metriken) ersetzt. Nutzerdefinierte Felder (Titel, Beschreibung, Tags) bleiben erhalten.
      </p>

      {/* Mode toggle */}
      <div className="flex gap-2">
        <button
          onClick={() => setMode('all')}
          className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors cursor-pointer ${
            mode === 'all'
              ? 'bg-orange-500 text-white'
              : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
          }`}
        >
          Alle / Filter
        </button>
        <button
          onClick={() => setMode('selection')}
          className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors cursor-pointer ${
            mode === 'selection'
              ? 'bg-orange-500 text-white'
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
              <option value="e_biking">E-Bike</option>
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
        <div className="max-h-64 overflow-y-auto rounded-lg border border-neutral-200 dark:border-neutral-700">
          {activities.length === 0 ? (
            <div className="p-4 text-center text-sm text-neutral-400">Keine Aktivitäten vorhanden</div>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
                  <th className="px-3 py-2 text-left">
                    <input
                      type="checkbox"
                      checked={selectedIds.size === activities.length && activities.length > 0}
                      onChange={() => toggleAll(activities)}
                      className="rounded"
                    />
                  </th>
                  <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">Datum</th>
                  <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportart</th>
                  <th className="px-3 py-2 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400">Titel</th>
                </tr>
              </thead>
              <tbody>
                {activities.map((a, i) => (
                  <tr
                    key={a.id}
                    className={`border-b border-neutral-100 dark:border-neutral-800 cursor-pointer
                      hover:bg-neutral-50 dark:hover:bg-neutral-800/50
                      ${i % 2 === 0 ? '' : 'bg-neutral-50/50 dark:bg-neutral-800/20'}`}
                    onClick={() => toggleId(a.id)}
                  >
                    <td className="px-3 py-1.5">
                      <input
                        type="checkbox"
                        checked={selectedIds.has(a.id)}
                        onChange={() => toggleId(a.id)}
                        className="rounded"
                      />
                    </td>
                    <td className="px-3 py-1.5 text-neutral-600 dark:text-neutral-300 whitespace-nowrap">
                      {format(new Date(a.started_at), 'dd.MM.yyyy', { locale: de })}
                    </td>
                    <td className="px-3 py-1.5 text-neutral-600 dark:text-neutral-300">
                      {sportLabel(a.sport_type, a.sub_type)}
                    </td>
                    <td className="px-3 py-1.5 text-neutral-900 dark:text-neutral-100 truncate max-w-[200px]">
                      {a.title ?? <span className="text-neutral-400 italic">Kein Titel</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {mode === 'selection' && selectedIds.size > 0 && (
            <div className="px-3 py-2 text-xs text-neutral-500 dark:text-neutral-400 border-t border-neutral-200 dark:border-neutral-700">
              {selectedIds.size} ausgewählt
            </div>
          )}
        </div>
      )}

      {/* Action */}
      <div className="flex items-center gap-4">
        <button
          onClick={handleStart}
          disabled={mutation.isPending || (mode === 'selection' && selectedIds.size === 0)}
          className="px-4 py-2 rounded-lg text-sm font-medium bg-orange-500 text-white
                     hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed
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
                ? `${selectedIds.size} ausgewählte Aktivität${selectedIds.size === 1 ? '' : 'en'} werden neu geparst. Alle abgeleiteten Daten werden ersetzt.`
                : 'Alle passenden Aktivitäten werden neu geparst. Alle abgeleiteten Daten werden ersetzt.'}
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
                className="px-4 py-2 rounded-lg text-sm font-medium bg-orange-500 text-white
                           hover:bg-orange-600 transition-colors cursor-pointer"
              >
                Bestätigen
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
