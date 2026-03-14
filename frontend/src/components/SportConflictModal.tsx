import { useState, useMemo, useEffect } from 'react'
import { Activity } from '@/api/types'
import { apiFetch } from '@/lib/auth'
import { resolveSportConflict } from '@/api/sportConflict'
import { useToast } from '@/lib/toast'
import { CATEGORIES, SPORT_DEFS, sportLabel } from '@/lib/sportTaxonomy'

interface SportConflictModalProps {
  activity: Activity
  open: boolean
  onClose: () => void
  onResolved: () => void
}

export default function SportConflictModal({ activity, open, onClose, onResolved }: SportConflictModalProps) {
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const { addToast } = useToast()

  // If opened from ActivitiesPage (list), sport_conflict meta may be null
  // (ListActivities doesn't JOIN import_queue). Fetch full activity if needed.
  const [fullActivity, setFullActivity] = useState<Activity | null>(null)
  const effectiveActivity = fullActivity ?? activity

  useEffect(() => {
    const abortCtrl = new AbortController()
    if (open && !activity.sport_conflict && activity.sport_conflict_pending) {
      apiFetch(`/api/v1/activities/${activity.id}`, { signal: abortCtrl.signal })
        .then(r => r.json())
        .then(a => { if (!abortCtrl.signal.aborted) setFullActivity(a) })
        .catch(() => {}) // graceful — modal works without meta
    }
    if (!open) setFullActivity(null)
    return () => abortCtrl.abort()
  }, [open, activity.id, activity.sport_conflict, activity.sport_conflict_pending])

  const conflict = effectiveActivity.sport_conflict
  const hasConflictMeta = conflict && conflict.file_sport && conflict.platform_sport

  // Both generic = no useful quick-pick
  const bothGeneric = hasConflictMeta &&
    conflict.file_sport === 'generic' && conflict.platform_sport === 'generic'

  // Filtered sport list — uses CATEGORIES + SPORT_DEFS from sportTaxonomy.ts
  const allSports = useMemo(() => {
    return SPORT_DEFS.map(sd => {
      const cat = CATEGORIES.find(c => c.key === sd.categoryKey)
      return { key: sd.sportType, label: sd.label, category: cat?.label ?? sd.categoryKey }
    })
  }, [])

  const filtered = useMemo(() => {
    if (!search) return allSports
    const q = search.toLowerCase()
    return allSports.filter(s =>
      s.label.toLowerCase().includes(q) || s.key.includes(q)
    )
  }, [allSports, search])

  const handleResolve = async (sportType?: string) => {
    setLoading(true)
    try {
      await resolveSportConflict(activity.id, sportType)
      addToast({ type: 'success', title: 'Sport-Typ aktualisiert' })
      onResolved()
      onClose()
    } catch {
      addToast({ type: 'error', title: 'Fehler beim Auflösen', duration: 0 })
    } finally {
      setLoading(false)
    }
  }

  if (!open) return null

  const sourceLabel = (src: string) => {
    switch (src) {
      case 'strava': return 'Strava'
      case 'komoot': return 'Komoot'
      case 'fit': return 'FIT-Datei'
      case 'gpx': return 'GPX-Datei'
      case 'tcx': return 'TCX-Datei'
      default: return src
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-zinc-900 border border-zinc-700 rounded-xl max-w-md w-full mx-4 max-h-[80vh] flex flex-col" onClick={e => e.stopPropagation()}>
        {/* Header */}
        <div className="p-4 border-b border-zinc-700">
          <h3 className="text-lg font-semibold text-zinc-100">Sport-Typ-Konflikt</h3>
          <p className="text-sm text-zinc-400 mt-1">
            {bothGeneric
              ? 'Keine Sportart erkannt — bitte manuell wählen.'
              : 'Die Plattform und die Aktivitätsdatei erkennen unterschiedliche Sportarten.'}
          </p>
        </div>

        <div className="overflow-y-auto flex-1 p-4 space-y-4">
          {/* Conflict info */}
          {hasConflictMeta && !bothGeneric && (
            <div className="grid grid-cols-2 gap-3">
              <div className="bg-zinc-800 rounded-lg p-3 text-center">
                <div className="text-xs text-zinc-500 mb-1">{sourceLabel(conflict.platform_source)}</div>
                <div className="text-sm font-medium text-zinc-200">{sportLabel(conflict.platform_sport)}</div>
              </div>
              <div className="bg-zinc-800 rounded-lg p-3 text-center">
                <div className="text-xs text-zinc-500 mb-1">{sourceLabel(conflict.file_source)}</div>
                <div className="text-sm font-medium text-zinc-200">{sportLabel(conflict.file_sport)}</div>
              </div>
            </div>
          )}

          {/* Quick pick buttons */}
          {hasConflictMeta && !bothGeneric && (
            <div className="flex gap-2">
              <button
                disabled={loading}
                onClick={() => handleResolve(conflict.platform_sport)}
                className="flex-1 px-3 py-2 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-sm text-zinc-200 transition-colors disabled:opacity-50"
              >
                {sportLabel(conflict.platform_sport)}
              </button>
              <button
                disabled={loading}
                onClick={() => handleResolve(conflict.file_sport)}
                className="flex-1 px-3 py-2 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-sm text-zinc-200 transition-colors disabled:opacity-50"
              >
                {sportLabel(conflict.file_sport)}
              </button>
            </div>
          )}

          {/* Search + full list */}
          <div>
            <input
              type="text"
              placeholder="Sportart suchen..."
              value={search}
              onChange={e => setSearch(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-zinc-800 border border-zinc-600 text-sm text-zinc-200 placeholder:text-zinc-500 focus:outline-none focus:border-zinc-500"
            />
          </div>

          <div className="space-y-1 max-h-48 overflow-y-auto">
            {filtered.map(s => (
              <button
                key={s.key}
                disabled={loading}
                onClick={() => handleResolve(s.key)}
                className="w-full text-left px-3 py-1.5 rounded hover:bg-zinc-800 text-sm text-zinc-300 transition-colors disabled:opacity-50"
              >
                <span>{s.label}</span>
                <span className="text-xs text-zinc-500 ml-2">{s.category}</span>
              </button>
            ))}
          </div>
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-zinc-700 flex justify-between">
          <button
            disabled={loading}
            onClick={() => handleResolve()}
            className="px-4 py-2 text-sm text-zinc-400 hover:text-zinc-200 transition-colors"
          >
            Importierte Sportart behalten
          </button>
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-zinc-400 hover:text-zinc-200 transition-colors"
          >
            Abbrechen
          </button>
        </div>
      </div>
    </div>
  )
}
