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
    <div className="flex h-[52px] items-center gap-2 border-t border-neutral-800 bg-neutral-900 px-3">
      {/* Point counter badge */}
      <span className="shrink-0 rounded bg-neutral-800 px-2 py-1 font-mono text-xs text-neutral-300">
        {pointCount} {pointCount === 1 ? 'Punkt' : 'Punkte'}
      </span>

      <div className="h-4 w-px bg-neutral-700 shrink-0" />

      {/* Undo */}
      <button
        type="button"
        onClick={onUndo}
        disabled={pointCount === 0}
        title="Letzten Punkt rückgängig"
        className={cn(
          'flex items-center gap-1.5 rounded px-2 py-1 text-xs text-neutral-300 transition-colors',
          'bg-neutral-800 hover:bg-neutral-700 hover:text-white',
          'disabled:pointer-events-none disabled:opacity-40',
        )}
      >
        <Undo2 className="size-3.5" />
        <span>Undo</span>
      </button>

      {/* Clear / Confirm clear */}
      {confirmClear ? (
        <span className="flex items-center gap-1 text-xs">
          <span className="shrink-0 text-neutral-300">Alle Punkte löschen?</span>
          <button
            type="button"
            onClick={() => { setConfirmClear(false); onClear() }}
            className="rounded bg-red-700 px-2 py-1 text-xs font-medium text-white transition-colors hover:bg-red-600"
          >
            Ja
          </button>
          <button
            type="button"
            onClick={() => setConfirmClear(false)}
            className="rounded bg-neutral-700 px-2 py-1 text-xs text-neutral-300 transition-colors hover:bg-neutral-600"
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
            'flex items-center gap-1.5 rounded px-2 py-1 text-xs text-red-400 transition-colors',
            'bg-neutral-800 hover:bg-red-900/60 hover:text-red-300',
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
          'rounded px-2 py-1 text-xs text-neutral-300 transition-colors',
          'bg-neutral-800 hover:bg-neutral-700 hover:text-white',
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
          'flex items-center gap-1.5 rounded px-2 py-1 text-xs text-neutral-300 transition-colors',
          'bg-neutral-800 hover:bg-neutral-700 hover:text-white',
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
          'rounded px-2 py-1 text-xs text-neutral-400 transition-colors',
          'bg-neutral-800 hover:bg-neutral-700 hover:text-neutral-300',
        )}
      >
        Reset
      </button>

      {/* Primary CTA */}
      <button
        type="button"
        onClick={onPreviewOrSave}
        disabled={pointCount < 2}
        className={cn(
          'rounded px-3 py-1 text-xs font-medium text-white transition-colors',
          'bg-blue-600 hover:bg-blue-500',
          'disabled:pointer-events-none disabled:opacity-40',
        )}
      >
        {isPreview ? 'Speichern' : 'Vorschau'}
      </button>
    </div>
  )
}
