import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { BestEffortBadges } from './BestEffortBadges'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

describe('BestEffortBadges unit formatting', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('formats near-miss power differences with locale-aware fixed units', () => {
    render(
      <BestEffortBadges
        activityId="activity-1"
        bestEfforts={[]}
        nearMisses={[{
          category: 'power_20min',
          sport_group: 'cycling',
          diff_watts: 12345.6,
        }]}
      />,
    )

    expect(screen.getByText('12,346 W off 20 min PR')).toBeInTheDocument()
    expect(screen.queryByText('12346 W off 20 min PR')).not.toBeInTheDocument()
  })
})
