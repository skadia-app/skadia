import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, it, expect, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import GearPage from './GearPage'

// Stub out API calls so tests don't reach the network.
vi.mock('@/api/gear', () => ({
  fetchGear: vi.fn(() => Promise.resolve([])),
  createGear: vi.fn(),
  fetchGearDocuments: vi.fn(() => Promise.resolve([])),
  getGearDocumentUrl: vi.fn(() => ''),
}))
vi.mock('@/api/hardware', () => ({
  fetchDevices: vi.fn(() => Promise.resolve([])),
}))
vi.mock('@/components/MaintenanceAlerts', () => ({
  default: () => null,
}))
vi.mock('@/components/GearMergePanel', () => ({
  default: () => null,
}))

function renderGearPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/gear']}>
        <GearPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('GearPage', () => {
  it('TestGearPageRendersAllEightScopes', async () => {
    renderGearPage()

    // The page renders 8 gear category tiles plus hardware tiles.
    // The 8 gear scopes are: bike, shoes, winter, water, diving, climb, apparel, other.
    const expectedTiles = [
      'Räder',
      'Schuhe',
      'Wintersport',
      'Wassersport',
      'Tauchen',
      'Klettern',
      'Bekleidung',
      'Sonstiges',
    ]

    for (const title of expectedTiles) {
      expect(screen.getByText(title)).toBeInTheDocument()
    }
  })
})
