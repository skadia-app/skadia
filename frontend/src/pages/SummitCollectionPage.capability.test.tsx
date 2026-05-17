import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
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

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

vi.mock('@/hooks/useSummitCollection', () => ({
  useSummitCollection: () => mocks.useSummitCollection(),
}))

vi.mock('@/components/summit/SummitMap', () => ({ default: () => null }))
vi.mock('@/components/summit/SummitMapFullscreen', () => ({ default: () => null }))

function renderPage() {
  render(
    <MemoryRouter initialEntries={['/analytics/summits']}>
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
  })

  it('does not query or render the summit collection when geodata enrichment is unavailable', () => {
    renderPage()

    expect(mocks.useSummitCollection).not.toHaveBeenCalled()
    expect(screen.queryByRole('heading', { name: 'Gipfelsammlung' })).not.toBeInTheDocument()
    expect(screen.getByText(/Gipfeldaten sind in dieser Installation nicht verfügbar/i)).toBeInTheDocument()
  })
})
