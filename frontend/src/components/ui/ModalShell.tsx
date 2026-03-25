import { useEscapeKey } from '@/hooks/useEscapeKey'
import { cn } from '@/lib/utils'

interface ModalShellProps {
  onClose: () => void
  maxWidth?: string
  children: React.ReactNode
  className?: string
}

export function ModalShell({ onClose, maxWidth = 'max-w-lg', children, className }: ModalShellProps) {
  useEscapeKey(onClose)

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true" aria-label="Dialog">
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={onClose} />
      <div className={cn('relative z-10 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 shadow-xl p-6', maxWidth, className)}>
        {children}
      </div>
    </div>
  )
}
