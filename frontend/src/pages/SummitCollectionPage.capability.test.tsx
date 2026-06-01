import '@testing-library/jest-dom/vitest'
import '@/i18n/i18n'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Capabilities } from '@/api/routes'
import SummitCollectionPage from './SummitCollectionPage'

const mocks = vi.hoisted(() => ({
  capabilities: {
    features: {
      geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
    },
  } as Capabilities,
  useSummitCollection: vi.fn(),
}))
const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'de',
  units: 'metric',
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

vi.mock('@/hooks/useSummitCollection', () => ({
  useSummitCollection: (options: unknown) => mocks.useSummitCollection(options),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/components/summit/SummitMap', () => ({ default: () => null }))
vi.mock('@/components/summit/SummitMapFullscreen', () => ({ default: () => null }))

function renderPage(initialEntry = '/analytics/summits') {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <SummitCollectionPage />
    </MemoryRouter>,
  )
}

describe('SummitCollectionPage deployment capabilities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.capabilities = {
      features: {
        geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
      },
    } as Capabilities
    mocks.useSummitCollection.mockReturnValue({
      allSummits: [],
      displaySummits: [],
      isLoading: false,
      isFiltered: false,
      availableCountries: [],
      availableYears: [],
    })
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
  })

  it('does not query or render the summit collection when geodata enrichment is unavailable', () => {
    renderPage()

    expect(mocks.useSummitCollection).not.toHaveBeenCalled()
    expect(screen.queryByRole('heading', { name: 'Gipfelsammlung' })).not.toBeInTheDocument()
    expect(screen.getByText(/Gipfeldaten sind in dieser Installation nicht verfügbar/i)).toBeInTheDocument()
  })

  it('reads the elevation filter from stable URL ids instead of visible labels', () => {
    mocks.capabilities = {
      features: {
        geo_poi_enrichment: { configured: true, available: true },
      },
    } as Capabilities

    renderPage('/analytics/summits?summit_elev=elevation_2000')

    expect(mocks.useSummitCollection).toHaveBeenLastCalledWith(expect.objectContaining({
      elevClass: expect.objectContaining({
        id: 'elevation_2000',
        min: 2000,
        max: 2999,
      }),
    }))
  })

  it('writes stable elevation ids when changing the summit elevation filter', async () => {
    const user = userEvent.setup()
    mocks.capabilities = {
      features: {
        geo_poi_enrichment: { configured: true, available: true },
      },
    } as Capabilities

    renderPage('/analytics/summits')
    await user.click(screen.getByRole('button', { name: '2000er' }))

    expect(mocks.useSummitCollection).toHaveBeenLastCalledWith(expect.objectContaining({
      elevClass: expect.objectContaining({
        id: 'elevation_2000',
        min: 2000,
        max: 2999,
      }),
    }))
  })

  it('formats summit elevations with the selected user units', () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    mocks.capabilities = {
      features: {
        geo_poi_enrichment: { configured: true, available: true },
      },
    } as Capabilities
    mocks.useSummitCollection.mockReturnValue({
      allSummits: [],
      displaySummits: [
        {
          osm_id: 1,
          name: 'Imperial Peak',
          elevation_m: 1000,
          first_visit: '2026-05-01T00:00:00Z',
          last_visit: '2026-05-02T00:00:00Z',
          visit_count: 1,
        },
      ],
      isLoading: false,
      isFiltered: false,
      availableCountries: [],
      availableYears: [],
    })

    renderPage('/analytics/summits')

    expect(screen.getAllByText('Imperial Peak')).toHaveLength(2)
    expect(screen.getAllByText('3,281 ft')).toHaveLength(2)
    expect(screen.getByText('Über 9,843 ft')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('1000 m')
  })
})
