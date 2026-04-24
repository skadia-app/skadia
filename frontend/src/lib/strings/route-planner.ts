import type { Toast } from '../toast'

export const lazyFillMessages = {
  noticeTitle:      'Höhenprofil wird beim nächsten Speichern aktualisiert',
  noticeMessage:    'Das Höhenprofil dieser Route wurde neu berechnet. Drücke Speichern, um die Änderung zu sichern.',
  inProgress:       'Höhenprofil wird berechnet',
  inProgressDetail: 'Bitte kurz warten — Route wird gerade aktualisiert.',
  saveButtonHint:   'Höhenprofil wurde automatisch ergänzt — bitte speichern',
} as const

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
    // Quota / Inkognito / SecurityError — ignorieren
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
): void {
  if (readLazyFillNoticeFlag()) return
  addToast({
    type: 'info',
    title: lazyFillMessages.noticeTitle,
    message: lazyFillMessages.noticeMessage,
    duration: 8000,
  })
  writeLazyFillNoticeFlag()
}
