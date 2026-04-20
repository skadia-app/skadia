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
    <div className="flex h-[52px] items-center gap-2 border-t border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900 px-3">
      {/* Lap count indicator */}
      <div className="flex items-center gap-1.5">
        <Flag className={cn('size-3.5 shrink-0', hasLaps ? 'text-accent-DEFAULT' : 'text-neutral-400 dark:text-neutral-600')} />
        {hasLaps ? (
          <span className="text-xs font-medium text-neutral-700 dark:text-neutral-300">
            {detectedLaps} {detectedLaps === 1 ? 'Runde erkannt' : 'Runden erkannt'}
          </span>
        ) : (
          <span className="text-xs text-neutral-400 dark:text-neutral-500">Auf Track klicken — Start/Ziel setzen</span>
        )}
      </div>

      <div className="flex-1" />

      {/* Back */}
      <button
        type="button"
        onClick={onBack}
        className={cn(
          'rounded-md px-2 py-1 text-xs',
          'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400',
          'border border-neutral-200 dark:border-neutral-700',
          'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-700 dark:hover:text-neutral-300',
        )}
      >
        Zurück
      </button>

      {/* Primary CTA — orange accent */}
      <button
        type="button"
        onClick={onPreviewOrSave}
        disabled={!hasLaps}
        className={cn(
          'rounded-md px-3 py-1 text-xs font-medium text-white transition-colors shadow-sm',
          'bg-accent-DEFAULT hover:bg-accent-600',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1',
          'disabled:pointer-events-none disabled:opacity-40',
        )}
      >
        {isPreview ? 'Speichern' : 'Weiter →'}
      </button>
    </div>
  )
}
