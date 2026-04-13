import { ModalShell } from '@/components/ui/ModalShell'

interface ConfirmModalProps {
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  /** Tailwind classes for the confirm button (default: red destructive) */
  confirmClass?: string
  isPending?: boolean
  onConfirm: () => void
  onClose: () => void
}

/**
 * Generic confirmation modal for destructive or important actions.
 * Uses ModalShell for consistent ARIA, focus-trapping, and escape handling.
 */
export default function ConfirmModal({
  title,
  message,
  confirmLabel = 'Bestätigen',
  cancelLabel = 'Abbrechen',
  confirmClass,
  isPending = false,
  onConfirm,
  onClose,
}: ConfirmModalProps) {
  return (
    <ModalShell onClose={onClose} maxWidth="max-w-sm">
      <div className="space-y-4">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">{title}</h2>
        <p className="text-sm text-neutral-600 dark:text-neutral-400">{message}</p>
        <div className="flex justify-end gap-2 pt-2">
          <button
            onClick={onClose}
            disabled={isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
          >
            {cancelLabel}
          </button>
          <button
            onClick={onConfirm}
            disabled={isPending}
            className={`cursor-pointer flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors ${confirmClass ?? 'bg-red-600 hover:bg-red-700'}`}
          >
            {isPending && (
              <svg className="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
            )}
            {confirmLabel}
          </button>
        </div>
      </div>
    </ModalShell>
  )
}
