import { describe, expect, it, beforeEach } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { getAuthErrorMessage } from './errorMessages'

describe('getAuthErrorMessage', () => {
  beforeEach(() => i18n.changeLanguage('de'))

  it('localizes known auth error codes', () => {
    expect(getAuthErrorMessage('missing_token')).toBe(i18n.t('auth.errors.missing_token'))
  })

  it('does not expose unknown backend auth error codes', () => {
    expect(getAuthErrorMessage('backend_added_new_error')).toBe(i18n.t('auth.errors.unknown_error'))
  })
})
