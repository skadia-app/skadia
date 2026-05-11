import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, it, expect, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import GearPage from './GearPage'
import { fetchGear } from '@/api/gear'
import type { Gear } from '@/api/types'

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

function renderGearPage(initialEntry = '/gear') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
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

  it('TestGearPageIncludesBikeLeafTypesInBikeScope', async () => {
    vi.mocked(fetchGear).mockResolvedValue([
      gearFixture({
        id: 'bike-1',
        name: 'Enduro',
        type: 'bike.mtb.enduro',
        scope: 'bike',
      }),
    ])

    renderGearPage('/gear?type=bike')

    expect(await screen.findByText('Enduro')).toBeInTheDocument()
    expect(screen.getByText('Fahrrad / Mountainbike / Enduro')).toBeInTheDocument()
  })
})

function gearFixture(overrides: Partial<Gear> = {}): Gear {
  return {
    id: 'gear-1',
    user_id: 'user-1',
    name: 'Rad',
    type: 'bike',
    total_distance_m: 0,
    total_elevation_gain_m: 0,
    total_elevation_loss_m: 0,
    total_time_s: 0,
    total_moving_time_s: 0,
    total_activities: 0,
    is_retired: false,
    status: 'active',
    applicable_sport_predicates: [],
    created_at: '2026-05-11T00:00:00Z',
    ...overrides,
  }
}
