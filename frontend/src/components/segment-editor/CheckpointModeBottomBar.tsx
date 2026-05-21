import { Save, Trash2 } from 'lucide-react'

export function CheckpointModeBottomBar(props: {
  controlCount: number
  onClear: () => void
  onPreviewOrSave: () => void
}) {
  const canSave = props.controlCount >= 2
  return (
    <div className="flex items-center gap-2 border-t border-neutral-200 bg-white px-4 py-3 text-sm dark:border-neutral-800 dark:bg-neutral-900">
      <div className="text-neutral-600 dark:text-neutral-300">
        {props.controlCount === 1 ? '1 Checkpoint' : `${props.controlCount} Checkpoints`}
      </div>
      <div className="flex-1" />
      <button
        type="button"
        onClick={props.onClear}
        disabled={props.controlCount === 0}
        className="inline-flex items-center gap-1.5 rounded-md border border-neutral-300 px-3 py-1.5 text-neutral-700 disabled:opacity-40 dark:border-neutral-700 dark:text-neutral-200"
      >
        <Trash2 className="h-4 w-4" aria-hidden="true" />
        Leeren
      </button>
      <button
        type="button"
        onClick={props.onPreviewOrSave}
        disabled={!canSave}
        className="inline-flex items-center gap-1.5 rounded-md bg-accent-DEFAULT px-3 py-1.5 font-medium text-white disabled:opacity-40"
      >
        <Save className="h-4 w-4" aria-hidden="true" />
        Speichern
      </button>
    </div>
  )
}
