// frontend/src/components/segment-editor/LapModeBottomBar.tsx
import { Flag } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface LapModeBottomBarProps {
  detectedLaps: number
  onBack: () => void
  onPreviewOrSave: () => void
  isPreview: boolean
}

export function LapModeBottomBar({ detectedLaps, onBack, onPreviewOrSave, isPreview }: LapModeBottomBarProps) {
  const hasLaps = detectedLaps >= 1

  return (
    <div className="flex h-[52px] items-center gap-2 border-t border-neutral-800 bg-neutral-900 px-3">
      {/* Lap count indicator */}
      <div className="flex items-center gap-1.5">
        <Flag className={cn('size-3.5 shrink-0', hasLaps ? 'text-blue-400' : 'text-neutral-600')} />
        {hasLaps ? (
          <span className="text-xs text-neutral-300">
            {detectedLaps} {detectedLaps === 1 ? 'Runde erkannt' : 'Runden erkannt'}
          </span>
        ) : (
          <span className="text-xs text-neutral-500">Keine Runden erkannt</span>
        )}
      </div>

      <div className="flex-1" />

      {/* Back */}
      <button
        type="button"
        onClick={onBack}
        className={cn(
          'rounded px-2 py-1 text-xs text-neutral-400 transition-colors',
          'bg-neutral-800 hover:bg-neutral-700 hover:text-neutral-300',
        )}
      >
        Zurück
      </button>

      {/* Primary CTA */}
      <button
        type="button"
        onClick={onPreviewOrSave}
        disabled={!hasLaps}
        className={cn(
          'rounded px-3 py-1 text-xs font-medium text-white transition-colors',
          'bg-blue-600 hover:bg-blue-500',
          'disabled:pointer-events-none disabled:opacity-40',
        )}
      >
        {isPreview ? 'Speichern' : 'Weiter'}
      </button>
    </div>
  )
}
