import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import type { SummitVisit } from '@/api/summits'
import SummitBadges from './SummitBadges'
import SummitMapFullscreen from './SummitMapFullscreen'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

vi.mock('@/hooks/useEscapeKey', () => ({
  useEscapeKey: vi.fn(),
}))

vi.mock('@/hooks/useMapLayers', () => ({
  useMapLayers: () => ({ base: [] }),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: { features: {} } }),
}))

vi.mock('@/hooks/useSummitCollection', () => ({
  useSummitCollection: () => ({
    displaySummits: [],
    availableCountries: [],
    availableYears: [],
  }),
}))

vi.mock('./SummitActivityList', () => ({
  default: () => <div data-testid="summit-activity-list" />,
}))

function summit(overrides: Partial<SummitVisit> = {}): SummitVisit {
  return {
    osm_id: 1,
    name: 'Synthetic Summit',
    elevation_m: 1500,
    visit_count: 1,
    first_visit: '2026-05-01T00:00:00Z',
    last_visit: '2026-05-01T00:00:00Z',
    lat: 47.1,
    lng: 11.3,
    country_codes: ['XX'],
    ...overrides,
  }
}

describe('summit map and badges unit preferences', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('formats badge elevation classes with the selected units', () => {
    render(<SummitBadges summits={[summit()]} />)

    expect(screen.getByText('3,281-6,558 ft')).toBeInTheDocument()
    expect(screen.queryByText('1000er')).not.toBeInTheDocument()
  })

  it('formats the fullscreen selected summit elevation with the selected units', () => {
    render(<SummitMapFullscreen initialSelected={summit()} onClose={vi.fn()} />)

    expect(screen.getByRole('button', { name: '3,281-6,558 ft' })).toBeInTheDocument()
    expect(screen.getByText('4,921 ft')).toBeInTheDocument()
    expect(screen.queryByText('1500 m')).not.toBeInTheDocument()
  })
})
