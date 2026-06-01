import { i18n } from '@/i18n/i18n'

/** Auth error code -> user-facing message lookup. */
export function getAuthErrorMessage(code: string): string {
  const key = `auth.errors.${code}`
  return i18n.exists(key) ? i18n.t(key) : i18n.t('auth.errors.unknown_error')
}
