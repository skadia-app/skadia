import { useState } from 'react'
import {
  DEFAULT_SCOPES,
  toBackendScope,
  toggleScopeKey,
  type AnyReprocessKey,
} from '@/lib/reprocessScopes'
import { ScopeCheckboxList } from '@/components/ScopeCheckboxList'

function ReprocessScopeModal({
  onConfirm,
  onClose,
  isPending,
}: {
  onConfirm: (scope: string[]) => void
  onClose: () => void
  isPending: boolean
}) {
  const [scope, setScope] = useState<Set<AnyReprocessKey>>(new Set(DEFAULT_SCOPES))

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

        <ScopeCheckboxList
          selected={scope}
          onToggle={key => setScope(prev => toggleScopeKey(prev, key))}
          layout="vertical"
          showDescriptions
        />

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
