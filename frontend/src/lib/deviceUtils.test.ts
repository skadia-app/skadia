import { describe, expect, it, beforeEach } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { deviceTypeBadgeLabel } from './deviceUtils'

describe('deviceTypeBadgeLabel', () => {
  beforeEach(() => i18n.changeLanguage('de'))

  it('localizes known device types', () => {
    expect(deviceTypeBadgeLabel('heart_rate', i18n.t)).toBe(i18n.t('devicePages.deviceTypes.heart_rate'))
  })

  it('does not expose unknown backend device type tokens', () => {
    expect(deviceTypeBadgeLabel('new_backend_device_type', i18n.t)).toBe(i18n.t('devicePages.deviceTypes.unknown'))
  })
})
