import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GearWearTab from './GearWearTab'
import { listServiceEvents, listServiceIntervals } from '@/api/gear'
import type { Gear, GearComponent } from '@/api/types'

const userPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' as 'metric' | 'imperial' }))

vi.mock('@/api/gear', () => ({
  listServiceEvents: vi.fn(),
  listServiceIntervals: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

function makeGear(overrides: Partial<Gear> = {}): Gear {
  return {
    id: 'g1',
    user_id: 'u1',
    name: 'Testrad',
    type: 'bike',
    total_distance_m: 5000_000,
    total_elevation_gain_m: 50_000,
    total_elevation_loss_m: 0,
    total_time_s: 200_000,
    total_moving_time_s: 200_000,
    total_activities: 42,
    is_retired: false,
    status: 'active',
    applicable_sport_predicates: [],
    created_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeComponent(overrides: Partial<GearComponent> = {}): GearComponent {
  return {
    id: 'c1',
    gear_id: 'g1',
    name: 'Kette',
    type: 'chain',
    total_distance_m: 450_000,
    total_elevation_gain_m: 10_000,
    total_elevation_loss_m: 8_000,
    total_moving_time_s: 7_200,
    total_time_s: 8_000,
    total_activities: 10,
    is_retired: false,
    status: 'active',
    ...overrides,
  }
}

function renderWearTab(item: Gear) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <GearWearTab item={item} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('GearWearTab', () => {
  beforeEach(() => {
    vi.mocked(listServiceIntervals).mockResolvedValue([])
    vi.mocked(listServiceEvents).mockResolvedValue([])
  })

  it('TestRendersWearAggregatesAndComponentServiceOverview', async () => {
    renderWearTab(makeGear())

    // Total aggregates are shown.
    expect(screen.getByText(/Distanz/i)).toBeInTheDocument()
    expect(screen.getByText(/Höhenmeter/i)).toBeInTheDocument()
    expect(screen.getByText(/Zeit/i)).toBeInTheDocument()
    expect(screen.getByText(/Aktivitäten/i)).toBeInTheDocument()

    expect(screen.getByTestId('service-interval-overview')).toHaveTextContent('Aktive Service-Intervalle')
    expect(screen.getByTestId('service-events-overview')).toHaveTextContent('Letzte Service-Ereignisse')
    expect(await screen.findByText('Keine aktiven Service-Intervalle.')).toBeInTheDocument()
    expect(screen.queryByText(/zukünftigen Version/i)).not.toBeInTheDocument()
  })

  it('TestRendersIntervalsByProgressAndRecentServiceEvents', async () => {
    vi.mocked(listServiceIntervals).mockImplementation(async (componentId: string) => {
      if (componentId === 'c1') {
        return [{
          id: 'i1',
          component_id: 'c1',
          name: 'Tauschen',
          interval_type: 'distance_m',
          metric_key: 'distance_m',
          interval_value: 500_000,
          is_recurring: true,
        }]
      }
      return [{
        id: 'i2',
        component_id: 'c2',
        name: 'Prüfen',
        interval_type: 'moving_time_s',
        metric_key: 'moving_time_s',
        interval_value: 10_000,
        is_recurring: true,
      }]
    })
    vi.mocked(listServiceEvents).mockImplementation(async (componentId: string) => (
      componentId === 'c1'
        ? [{
          id: 'e1',
          component_id: 'c1',
          performed_at: '2026-05-10T12:00:00Z',
          description: 'Kette gewechselt',
          created_at: '2026-05-10T12:00:00Z',
        }]
        : []
    ))

    renderWearTab(makeGear({
      components: [
        makeComponent({ id: 'c1', name: 'Kette', total_distance_m: 450_000 }),
        makeComponent({ id: 'c2', name: 'Dämpfer', total_moving_time_s: 2_000 }),
      ],
    }))

    expect(await screen.findByText('Kette - Tauschen')).toBeInTheDocument()
    expect(screen.getByText('279.6 mi / 310.7 mi')).toBeInTheDocument()
    expect(screen.getByText('90%')).toBeInTheDocument()
    expect(screen.getByText('Kette gewechselt')).toBeInTheDocument()
  })
})
