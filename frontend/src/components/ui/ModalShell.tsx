import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

// Selectors for all natively focusable elements
const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

interface ModalShellProps {
  onClose: () => void
  maxWidth?: string
  children: React.ReactNode
  className?: string
}

export function ModalShell({ onClose, maxWidth = 'max-w-lg', children, className }: ModalShellProps) {
  const panelRef = useRef<HTMLDivElement>(null)
  // Save the element that had focus before the modal opened
  const returnFocusRef = useRef<Element | null>(null)

  useEffect(() => {
    // Capture the trigger element before we move focus away
    returnFocusRef.current = document.activeElement

    // Focus the first focusable element inside the modal (or the panel itself)
    const panel = panelRef.current
    if (panel) {
      const first = panel.querySelector<HTMLElement>(FOCUSABLE)
      if (first) {
        first.focus()
      } else {
        panel.focus()
      }
    }

    return () => {
      // Return focus to the element that triggered the modal
      const target = returnFocusRef.current
      if (target instanceof HTMLElement) {
        target.focus()
      }
    }
  }, [])

  function handleKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
      return
    }

    if (e.key !== 'Tab') return

    const panel = panelRef.current
    if (!panel) return

    const focusable = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE))
    if (focusable.length === 0) return

    const first = focusable[0]
    const last = focusable[focusable.length - 1]

    if (e.shiftKey) {
      // Shift+Tab at first element → wrap to last
      if (document.activeElement === first) {
        e.preventDefault()
        last.focus()
      }
    } else {
      // Tab at last element → wrap to first
      if (document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      role="dialog"
      aria-modal="true"
      aria-label="Dialog"
    >
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={onClose} />
      <div
        ref={panelRef}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        className={cn(
          'relative z-10 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 shadow-xl p-6 outline-none',
          maxWidth,
          className,
        )}
      >
        {children}
      </div>
    </div>
  )
}
