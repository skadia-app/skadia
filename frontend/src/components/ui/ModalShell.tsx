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
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={onClose} />
      <div className={cn('relative z-10 rounded-xl border bg-card shadow-xl p-6', maxWidth, className)}>
        {children}
      </div>
    </div>
  )
}
