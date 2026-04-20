// frontend/src/components/segment-editor/DrawModeBottomBar.tsx
import { useState } from 'react'
import { Undo2, ArrowLeftRight, Trash2 } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface DrawModeBottomBarProps {
  pointCount: number
  onUndo: () => void
  onClear: () => void
  onFinish: () => void
  onReverse: () => void
  onReset: () => void
  onPreviewOrSave: () => void
  isPreview: boolean
}

export function DrawModeBottomBar(props: DrawModeBottomBarProps) {
  const { pointCount, onUndo, onClear, onFinish, onReverse, onReset, onPreviewOrSave, isPreview } = props
  const [confirmClear, setConfirmClear] = useState(false)

  function handleClearClick() {
    if (pointCount >= 3) {
      setConfirmClear(true)
    } else {
      onClear()
    }
  }

  return (
    <div className="flex h-[52px] items-center gap-2 border-t border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900 px-3">
      {/* Point counter badge */}
      <span className="shrink-0 rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-2 py-1 font-mono text-xs text-neutral-600 dark:text-neutral-300">
        {pointCount} {pointCount === 1 ? 'Punkt' : 'Punkte'}
      </span>

      <div className="h-4 w-px bg-neutral-200 dark:bg-neutral-700 shrink-0" />

      {/* Undo */}
      <button
        type="button"
        onClick={onUndo}
        disabled={pointCount === 0}
        title="Letzten Punkt rückgängig"
        className={cn(
          'flex items-center gap-1.5 rounded-md px-2 py-1 text-xs',
          'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300',
          'border border-neutral-200 dark:border-neutral-700',
          'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-white',
          'disabled:pointer-events-none disabled:opacity-40',
        )}
      >
        <Undo2 className="size-3.5" />
        <span>Undo</span>
      </button>

      {/* Clear / Confirm clear */}
      {confirmClear ? (
        <span className="flex items-center gap-1 text-xs">
          <span className="shrink-0 text-neutral-600 dark:text-neutral-300">Alle Punkte löschen?</span>
          <button
            type="button"
            onClick={() => { setConfirmClear(false); onClear() }}
            className="rounded-md border border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/40 px-2 py-1 text-xs font-medium text-red-700 dark:text-red-300 transition-colors hover:bg-red-100 dark:hover:bg-red-900/60"
          >
            Ja
          </button>
          <button
            type="button"
            onClick={() => setConfirmClear(false)}
            className="rounded-md border border-neutral-200 dark:border-neutral-700 bg-neutral-100 dark:bg-neutral-800 px-2 py-1 text-xs text-neutral-600 dark:text-neutral-300 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700"
          >
            Nein
          </button>
        </span>
      ) : (
        <button
          type="button"
          onClick={handleClearClick}
          disabled={pointCount === 0}
          title="Alle Punkte löschen"
          className={cn(
            'flex items-center gap-1.5 rounded-md px-2 py-1 text-xs',
            'bg-neutral-100 dark:bg-neutral-800 text-red-500 dark:text-red-400',
            'border border-neutral-200 dark:border-neutral-700',
            'transition-colors hover:bg-red-50 dark:hover:bg-red-900/40 hover:text-red-700 dark:hover:text-red-300 hover:border-red-200 dark:hover:border-red-800',
            'disabled:pointer-events-none disabled:opacity-40',
          )}
        >
          <Trash2 className="size-3.5" />
          <span>Löschen</span>
        </button>
      )}

      {/* Finish */}
      <button
        type="button"
        onClick={onFinish}
        disabled={pointCount < 2}
        title="Zeichnen abschließen"
        className={cn(
          'rounded-md px-2 py-1 text-xs',
          'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300',
          'border border-neutral-200 dark:border-neutral-700',
          'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-white',
          'disabled:pointer-events-none disabled:opacity-40',
        )}
      >
        Abschließen
      </button>

      {/* Reverse */}
      <button
        type="button"
        onClick={onReverse}
        disabled={pointCount < 2}
        title="Richtung umkehren"
        className={cn(
          'flex items-center gap-1.5 rounded-md px-2 py-1 text-xs',
          'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300',
          'border border-neutral-200 dark:border-neutral-700',
          'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-white',
          'disabled:pointer-events-none disabled:opacity-40',
        )}
      >
        <ArrowLeftRight className="size-3.5" />
        <span>Umkehren</span>
      </button>

      <div className="flex-1" />

      {/* Reset */}
      <button
        type="button"
        onClick={onReset}
        title="Auf gespeicherten Stand zurücksetzen"
        className={cn(
          'rounded-md px-2 py-1 text-xs',
          'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400',
          'border border-neutral-200 dark:border-neutral-700',
          'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-700 dark:hover:text-neutral-300',
        )}
      >
        Reset
      </button>

      {/* Primary CTA — orange accent */}
      <button
        type="button"
        onClick={onPreviewOrSave}
        disabled={pointCount < 2}
        className={cn(
          'rounded-md px-3 py-1 text-xs font-medium text-white transition-colors shadow-sm',
          'bg-accent-DEFAULT hover:bg-accent-600',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1',
          'disabled:pointer-events-none disabled:opacity-40',
        )}
      >
        Speichern
      </button>
    </div>
  )
}
