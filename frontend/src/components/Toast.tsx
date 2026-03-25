import type { Toast, ToastType } from '@/lib/toast'
import { useToast } from '@/lib/toast'

interface ToastItemProps {
  toast: Toast
}

function getToastStyles(type: ToastType) {
  switch (type) {
    case 'success':
      return {
        bg: 'bg-emerald-50 dark:bg-emerald-900/20',
        border: 'border-emerald-200 dark:border-emerald-800',
        text: 'text-emerald-800 dark:text-emerald-300',
        icon: '✓',
        iconColor: 'text-emerald-600 dark:text-emerald-400',
      }
    case 'error':
      return {
        bg: 'bg-rose-50 dark:bg-rose-900/20',
        border: 'border-rose-200 dark:border-rose-800',
        text: 'text-rose-800 dark:text-rose-300',
        icon: '✕',
        iconColor: 'text-rose-600 dark:text-rose-400',
      }
    case 'warning':
      return {
        bg: 'bg-amber-50 dark:bg-amber-900/20',
        border: 'border-amber-200 dark:border-amber-800',
        text: 'text-amber-800 dark:text-amber-300',
        icon: '⚠',
        iconColor: 'text-amber-600 dark:text-amber-400',
      }
    case 'info':
    default:
      return {
        bg: 'bg-blue-50 dark:bg-blue-900/20',
        border: 'border-blue-200 dark:border-blue-800',
        text: 'text-blue-800 dark:text-blue-300',
        icon: 'ℹ',
        iconColor: 'text-blue-600 dark:text-blue-400',
      }
  }
}

export function ToastItem({ toast }: ToastItemProps) {
  const { removeToast } = useToast()
  const styles = getToastStyles(toast.type)

  return (
    <div
      className={`${styles.bg} ${styles.border} border rounded-lg p-4 flex items-start gap-3 min-w-[320px] max-w-[400px] shadow-lg animate-in slide-in-from-bottom-2 fade-in`}
      role="alert"
    >
      <span className={`${styles.iconColor} text-lg font-bold flex-shrink-0`}>
        {styles.icon}
      </span>
      <div className="flex-1">
        <div className={`${styles.text} font-semibold text-sm`}>
          {toast.title}
        </div>
        {toast.message && (
          <div className={`${styles.text} text-xs mt-1 opacity-90`}>
            {toast.message}
          </div>
        )}
      </div>
      <button
        onClick={() => removeToast(toast.id)}
        className={`${styles.iconColor} hover:opacity-70 flex-shrink-0 mt-0.5 cursor-pointer`}
        aria-label="Schließen"
      >
        ✕
      </button>
    </div>
  )
}

export function ToastContainer() {
  const { toasts } = useToast()

  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-3 pointer-events-auto max-w-[500px]">
      {toasts.map(toast => (
        <ToastItem key={toast.id} toast={toast} />
      ))}
    </div>
  )
}
