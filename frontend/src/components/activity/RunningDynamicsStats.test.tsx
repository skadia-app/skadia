import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Activity } from '@/api/types'
import { i18n } from '@/i18n/i18n'
import { RunningDynamicsStats } from './RunningDynamicsStats'

const userPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' as 'metric' | 'imperial' }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

describe('RunningDynamicsStats unit preferences', () => {
  beforeEach(async () => {
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'
    await i18n.changeLanguage('en')
  })

  it('formats step length with selected user units', () => {
    render(<RunningDynamicsStats activity={{ avg_step_length_mm: 1000 } as Activity} />)

    expect(screen.getByText('Avg step length')).toBeInTheDocument()
    expect(screen.getByText('3.3 ft')).toBeInTheDocument()
    expect(screen.queryByText('1.00 m')).not.toBeInTheDocument()
  })

  it('preserves metric precision for short step lengths', () => {
    userPrefs.locale = 'de'
    userPrefs.units = 'metric'

    render(<RunningDynamicsStats activity={{ avg_step_length_mm: 850 } as Activity} />)

    expect(screen.getByText('0,85 m')).toBeInTheDocument()
    expect(screen.queryByText('1 m')).not.toBeInTheDocument()
  })
})
