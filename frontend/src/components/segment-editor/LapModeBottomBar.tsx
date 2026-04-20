// frontend/src/components/segment-editor/LapModeBottomBar.tsx
export interface LapModeBottomBarProps {
  detectedLaps: number
  onBack: () => void
  onPreviewOrSave: () => void
  isPreview: boolean
}

export function LapModeBottomBar({ detectedLaps, onBack, onPreviewOrSave, isPreview }: LapModeBottomBarProps) {
  return (
    <div className="flex items-center gap-2 border-t border-neutral-800 bg-neutral-900 p-3 text-sm">
      <p className="text-neutral-300">{detectedLaps} Runden erkannt</p>
      <div className="flex-1" />
      <button type="button" onClick={onBack} className="rounded bg-neutral-800 px-2 py-1 text-xs">Zurück</button>
      <button type="button" onClick={onPreviewOrSave} disabled={detectedLaps < 1} className="rounded bg-blue-600 px-3 py-1 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50">
        {isPreview ? 'Speichern' : 'Weiter'}
      </button>
    </div>
  )
}
