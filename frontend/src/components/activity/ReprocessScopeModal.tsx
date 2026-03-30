import { useState } from 'react'
import { GEO_SCOPE_KEYS, GEO_SCOPE_LABELS } from '@/components/RecomputeModal'

const REPROCESS_SCOPES = [
  ['streams', 'Streams & Track'],
  ['sensors', 'Sensoren'],
  ['metrics', 'Metriken'],
  ['details', 'Runden/Splits'],
  ['segments', 'Segmente & Runden'],
  ['sport_type', 'Sportart'],
  ['tags', 'Tags'],
] as const

type ReprocessScopeKey = typeof REPROCESS_SCOPES[number][0]
export type AnyReprocessKey = ReprocessScopeKey | typeof GEO_SCOPE_KEYS[number]

// Geo-Keys in "geo" Backend-Scope übersetzen (Stage 2 läuft immer als Ganzes).
export function toBackendScope(sel: Set<AnyReprocessKey>): string[] {
  const result = new Set<string>()
  for (const s of sel) {
    if ((GEO_SCOPE_KEYS as readonly string[]).includes(s)) {
      result.add('geo')
    } else {
      result.add(s)
    }
  }
  return [...result]
}

function ReprocessScopeModal({
  onConfirm,
  onClose,
  isPending,
}: {
  onConfirm: (scope: string[]) => void
  onClose: () => void
  isPending: boolean
}) {
  const [scope, setScope] = useState<Set<AnyReprocessKey>>(
    new Set(['streams', 'sensors', 'metrics', 'details'])
  )

  function toggleScope(key: AnyReprocessKey) {
    setScope(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="bg-white dark:bg-neutral-900 rounded-xl shadow-xl p-6 max-w-sm w-full mx-4 space-y-4">
        <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
          Neu verarbeiten — Details auswählen
        </h3>
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Wähle, welche Teile dieser Aktivität neu verarbeitet werden sollen.
          Nutzerdefinierte Felder (Titel, Beschreibung, Sichtbarkeit) bleiben immer erhalten.
        </p>

        <div className="space-y-2">
          {REPROCESS_SCOPES.map(([key, label]) => (
            <label
              key={key}
              className="flex items-center gap-2.5 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer select-none"
            >
              <input
                type="checkbox"
                checked={scope.has(key)}
                onChange={() => toggleScope(key)}
                className="rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT"
              />
              {label}
            </label>
          ))}
          <p className="text-xs text-neutral-400 dark:text-neutral-500 pt-1">Geo-Anreicherung (ML, asynchron)</p>
          {GEO_SCOPE_KEYS.map(key => (
            <label
              key={key}
              className="flex items-center gap-2.5 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer select-none"
            >
              <input
                type="checkbox"
                checked={scope.has(key)}
                onChange={() => toggleScope(key)}
                className="rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT"
              />
              {GEO_SCOPE_LABELS[key]}
            </label>
          ))}
        </div>

        <div className="flex justify-end gap-3 pt-1">
          <button
            onClick={onClose}
            disabled={isPending}
            className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400
                       hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer
                       disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Abbrechen
          </button>
          <button
            onClick={() => onConfirm(toBackendScope(scope))}
            disabled={isPending || scope.size === 0}
            className="px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT text-white
                       hover:bg-accent-600 transition-colors cursor-pointer
                       disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isPending ? 'Verarbeite...' : 'Starten'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default ReprocessScopeModal
