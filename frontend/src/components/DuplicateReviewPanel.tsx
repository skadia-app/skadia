import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { type FusionQueueEntry } from '@/api/importWizard'
import { sportLabel } from '@/lib/sportTaxonomy'

interface DuplicateReviewPanelProps {
  entries: FusionQueueEntry[]
  onDone: () => void
  doneLabel?: string
}

export default function DuplicateReviewPanel({ entries, onDone, doneLabel = 'Weiter zum Import' }: DuplicateReviewPanelProps) {
  const navigate = useNavigate()
  const [dismissed, setDismissed] = useState<Set<string>>(new Set())

  // selectedSports: wenn sport_type_a !== sport_type_b muss der Nutzer eine wählen
  const [selectedSports, setSelectedSports] = useState<Record<string, string>>({})

  function pairKey(a: string, b: string) {
    return [a, b].sort().join(':')
  }

  function formatDist(m: number) {
    return m >= 1000 ? `${(m / 1000).toFixed(1)} km` : `${Math.round(m)} m`
  }

  function formatTimeDiff(sec: number) {
    if (sec < 60) return `${Math.round(sec)}s`
    if (sec < 3600) return `${Math.round(sec / 60)} min`
    return `${(sec / 3600).toFixed(1)} h`
  }

  const visible = entries.filter(e => {
    const key = pairKey(e.activity_id_a, e.activity_id_b)
    return !dismissed.has(key)
  })

  if (visible.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-8 text-center">
        <p className="text-neutral-500 dark:text-neutral-400 text-sm">
          Keine Fusionskandidaten gefunden.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        {visible.length} mögliche{visible.length > 1 ? ' Duplikate' : 's Duplikat'} aus zwei verschiedenen Geräten erkannt.
        Wähle für jedes Paar ob die Aktivitäten zusammengeführt werden sollen.
      </p>

      {visible.map(entry => {
        const key = pairKey(entry.activity_id_a, entry.activity_id_b)
        const richerIsA = entry.field_richness_a >= entry.field_richness_b
        const richerId = richerIsA ? entry.activity_id_a : entry.activity_id_b
        const leanerId = richerIsA ? entry.activity_id_b : entry.activity_id_a

        return (
          <div key={key} className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 space-y-3">
            {/* Header */}
            <div className="flex items-center gap-2 text-xs text-neutral-400">
              <span className="bg-accent-100 dark:bg-orange-900/30 text-accent-600 dark:text-orange-400 px-2 py-0.5 rounded-full font-medium">
                {Math.round(entry.overlap_percent)}% Übereinstimmung
              </span>
              <span>·</span>
              <span>Zeitdifferenz: {formatTimeDiff(entry.time_diff_sec)}</span>
            </div>

            {/* Side-by-side */}
            <div className="grid grid-cols-2 gap-3">
              {[
                { id: entry.activity_id_a, title: entry.title_a, startedAt: entry.started_at_a, dist: entry.distance_m_a, richness: entry.field_richness_a, isRicher: richerIsA },
                { id: entry.activity_id_b, title: entry.title_b, startedAt: entry.started_at_b, dist: entry.distance_m_b, richness: entry.field_richness_b, isRicher: !richerIsA },
              ].map(side => (
                <div
                  key={side.id}
                  className={`rounded-lg border p-3 text-sm space-y-1 ${
                    side.isRicher
                      ? 'border-accent-DEFAULT/40 bg-accent-100 dark:bg-accent-DEFAULT/10'
                      : 'border-neutral-200 dark:border-neutral-700'
                  }`}
                >
                  {side.isRicher && (
                    <span className="text-xs text-accent-DEFAULT font-medium">Mehr Daten</span>
                  )}
                  <div className="font-medium text-neutral-900 dark:text-neutral-100 truncate">{side.title}</div>
                  <div className="text-neutral-500 dark:text-neutral-400">
                    {new Date(side.startedAt).toLocaleString('de-DE', { dateStyle: 'short', timeStyle: 'short' })}
                  </div>
                  <div className="text-neutral-500 dark:text-neutral-400">
                    {formatDist(side.dist)} · {side.richness} Felder
                  </div>
                </div>
              ))}
            </div>

            {/* Sport-Auswahl (nur wenn unterschiedliche Sport-Typen) */}
            {entry.sport_type_a !== entry.sport_type_b && (
              <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-3">
                <p className="text-xs font-medium text-amber-700 dark:text-amber-400 mb-2">
                  Unterschiedliche Sportarten — welche soll verwendet werden?
                </p>
                <div className="flex gap-2">
                  {[
                    { id: entry.activity_id_a, sport: entry.sport_type_a, subType: entry.sub_type_a, isEbike: entry.is_ebike_a },
                    { id: entry.activity_id_b, sport: entry.sport_type_b, subType: entry.sub_type_b, isEbike: entry.is_ebike_b },
                  ].map(opt => (
                    <button
                      key={opt.id}
                      onClick={() => setSelectedSports(prev => ({ ...prev, [key]: opt.id }))}
                      className={`flex-1 rounded-lg px-3 py-1.5 text-xs font-medium border transition-colors ${
                        selectedSports[key] === opt.id
                          ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                          : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400'
                      }`}
                    >
                      {sportLabel(opt.sport, opt.subType)}
                      {opt.isEbike && <span className="ml-1 opacity-75">⚡</span>}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {/* Actions */}
            <div className="flex items-center gap-2 pt-1">
              <button
                onClick={() => {
                  navigate(`/fusion?target=${richerId}&source=${leanerId}`)
                }}
                disabled={entry.sport_type_a !== entry.sport_type_b && !selectedSports[key]}
                title={
                  entry.sport_type_a !== entry.sport_type_b && !selectedSports[key]
                    ? 'Bitte zuerst eine Sportart auswählen'
                    : undefined
                }
                className="flex-1 bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed text-white rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
              >
                Editor öffnen
              </button>
              <button
                onClick={() => setDismissed(prev => new Set([...prev, key]))}
                className="flex-1 border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 rounded-lg px-3 py-1.5 text-sm font-medium text-neutral-600 dark:text-neutral-400 transition-colors"
              >
                Separate Aktivitäten
              </button>
            </div>
          </div>
        )
      })}

      {/* Continue button always visible at bottom */}
      <div className="pt-2">
        <button
          onClick={onDone}
          className="w-full border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 rounded-lg px-4 py-2 text-sm font-medium text-neutral-600 dark:text-neutral-400 transition-colors"
        >
          {visible.length > 0 ? 'Restliche überspringen und weiter' : doneLabel}
        </button>
      </div>
    </div>
  )
}
