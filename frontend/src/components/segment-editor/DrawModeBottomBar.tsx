// frontend/src/components/segment-editor/DrawModeBottomBar.tsx
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
  return (
    <div className="flex items-center gap-2 border-t border-neutral-800 bg-neutral-900 p-3 text-sm">
      <span className="rounded bg-neutral-800 px-2 py-1 text-xs text-neutral-300">
        Punkte: {pointCount}
      </span>
      <button type="button" onClick={onUndo} disabled={pointCount === 0} className="rounded bg-neutral-800 px-2 py-1 text-xs disabled:opacity-50">↶ Undo</button>
      <button
        type="button"
        onClick={() => {
          if (pointCount >= 3 && !window.confirm('Alle Punkte löschen?')) return
          onClear()
        }}
        disabled={pointCount === 0}
        className="rounded bg-neutral-800 px-2 py-1 text-xs disabled:opacity-50"
      >Clear</button>
      <button type="button" onClick={onFinish} disabled={pointCount < 2} className="rounded bg-neutral-800 px-2 py-1 text-xs disabled:opacity-50">Finish</button>
      <button type="button" onClick={onReverse} disabled={pointCount < 2} className="rounded bg-neutral-800 px-2 py-1 text-xs disabled:opacity-50">⇄ Umkehren</button>
      <div className="flex-1" />
      <button type="button" onClick={onReset} className="rounded bg-neutral-800 px-2 py-1 text-xs">Reset</button>
      <button type="button" onClick={onPreviewOrSave} disabled={pointCount < 2} className="rounded bg-blue-600 px-3 py-1 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50">
        {isPreview ? 'Speichern' : 'Vorschau'}
      </button>
    </div>
  )
}
