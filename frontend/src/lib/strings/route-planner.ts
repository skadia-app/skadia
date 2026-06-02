import type { Toast } from '../toast'

const LAZY_FILL_NOTICE_KEY = 'skadia.lazy_fill_notice_shown'

export function readLazyFillNoticeFlag(): boolean {
  try {
    return sessionStorage.getItem(LAZY_FILL_NOTICE_KEY) === '1'
  } catch {
    return false
  }
}

export function writeLazyFillNoticeFlag(): void {
  try {
    sessionStorage.setItem(LAZY_FILL_NOTICE_KEY, '1')
  } catch {
    // Quota / private browsing / SecurityError: ignore.
  }
}

/**
 * Shows a one-time-per-session info toast when a lazy-fill completes.
 * Accepts addToast as a parameter (not imported directly) because addToast
 * is only available via the useToast() React hook — this module is a pure
 * utility, not a hook. The caller (useRoutePlanner) passes its own addToast.
 *
 * Spec §7.4, §8
 */
export function showLazyFillNoticeOncePerSession(
  addToast: (toast: Omit<Toast, 'id'>) => unknown,
  messages: { title: string; message: string },
): void {
  if (readLazyFillNoticeFlag()) return
  addToast({
    type: 'info',
    title: messages.title,
    message: messages.message,
    duration: 8000,
  })
  writeLazyFillNoticeFlag()
}
