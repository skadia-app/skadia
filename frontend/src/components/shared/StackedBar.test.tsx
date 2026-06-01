import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import StackedBar from './StackedBar'

const userPrefs = vi.hoisted((): { locale: 'de' | 'en'; units: 'metric' | 'imperial' } => ({
  locale: 'en',
  units: 'imperial',
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

describe('StackedBar', () => {
  it('localizes the other bucket and formats lengths with user units', async () => {
    await i18n.changeLanguage('en')
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'

    render(
      <StackedBar
        aggregated={[{ surface_type: '__other__', length_m: 1609.344, percentage: 100 }]}
        colorMap={{}}
        title="Surfaces"
      />,
    )

    expect(screen.getByText('Other')).toBeInTheDocument()
    expect(screen.getByText('(1.0 mi)')).toBeInTheDocument()
    expect(screen.queryByText('Sonstige')).not.toBeInTheDocument()
    expect(screen.queryByText('(1.6 km)')).not.toBeInTheDocument()
  })
})
