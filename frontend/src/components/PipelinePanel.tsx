import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

interface PipelinePanelProps {
  title: string
  description: string
  icon: LucideIcon
  buttonLabel: string
  buttonVariant?: 'accent' | 'blue'
  status?: ReactNode
  onStart: () => void
  isPending: boolean
  disabled?: boolean
  disabledHint?: string
  children: ReactNode
}

export default function PipelinePanel({
  title, description, icon: Icon, buttonLabel, buttonVariant = 'accent',
  status, onStart, isPending, disabled, disabledHint, children,
}: PipelinePanelProps) {
  const btnClass = buttonVariant === 'blue'
    ? 'bg-blue-500 hover:bg-blue-600'
    : 'bg-accent-DEFAULT hover:bg-accent-DEFAULT/90'

  return (
    <div className={`rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 ${disabled ? 'opacity-50' : ''}`}>
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-start gap-2">
          <Icon className="w-4 h-4 mt-0.5 text-neutral-500 dark:text-neutral-400 shrink-0" />
          <div>
            <h4 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">{title}</h4>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">{description}</p>
          </div>
        </div>
        <div className="flex items-center gap-3 shrink-0">
          {status}
          <button
            onClick={onStart}
            disabled={disabled || isPending}
            className={`text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${btnClass}`}
          >
            {isPending ? 'Läuft…' : buttonLabel}
          </button>
        </div>
      </div>
      {disabled && disabledHint ? (
        <p className="text-xs text-neutral-500 italic">{disabledHint}</p>
      ) : (
        children
      )}
    </div>
  )
}
