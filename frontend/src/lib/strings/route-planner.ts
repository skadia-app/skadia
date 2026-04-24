export const lazyFillMessages = {
  noticeTitle:      'Höhenprofil wird beim nächsten Speichern aktualisiert',
  noticeMessage:    'Das Höhenprofil dieser Route wurde neu berechnet. Drücke Speichern, um die Änderung zu sichern.',
  inProgress:       'Höhenprofil wird berechnet',
  inProgressDetail: 'Bitte kurz warten — Route wird gerade aktualisiert.',
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
