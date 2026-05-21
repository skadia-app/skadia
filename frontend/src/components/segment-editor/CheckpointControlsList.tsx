import { X } from 'lucide-react'
import type { CheckpointControl } from '@/api/segments'

export function CheckpointControlsList(props: {
  controls: CheckpointControl[]
  onRemove: (index: number) => void
}) {
  if (props.controls.length === 0) return null
  return (
    <div className="border-t border-neutral-200 bg-white p-3 text-sm dark:border-neutral-800 dark:bg-neutral-900">
      <div className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
        Checkpoints
      </div>
      <div className="space-y-1.5">
        {props.controls.map((control, index) => (
          <div
            key={`${control.sequence_index}-${index}`}
            className="flex items-center gap-2 rounded-md border border-neutral-200 px-2 py-1.5 dark:border-neutral-700"
          >
            <span className="flex h-6 min-w-6 items-center justify-center rounded-full bg-orange-100 text-xs font-semibold text-orange-800 dark:bg-orange-950 dark:text-orange-200">
              {index === 0 ? 'S' : index === props.controls.length - 1 ? 'Z' : index + 1}
            </span>
            <span className="min-w-0 flex-1 truncate text-neutral-800 dark:text-neutral-100">
              {index === 0 ? 'Start' : index === props.controls.length - 1 ? 'Ziel' : `Checkpoint ${index + 1}`}
            </span>
            <button
              type="button"
              onClick={() => props.onRemove(index)}
              className="rounded p-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-white"
              aria-label="Checkpoint entfernen"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
