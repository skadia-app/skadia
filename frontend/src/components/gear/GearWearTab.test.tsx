import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it } from 'vitest'

import GearWearTab from './GearWearTab'
import type { Gear } from '@/api/types'

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

describe('GearWearTab', () => {
  it('TestRendersWearAggregatesAndServicePlaceholder', () => {
    render(<GearWearTab item={makeGear()} />)

    // Total aggregates are shown.
    expect(screen.getByText(/Distanz/i)).toBeInTheDocument()
    expect(screen.getByText(/Höhenmeter/i)).toBeInTheDocument()
    expect(screen.getByText(/Zeit/i)).toBeInTheDocument()
    expect(screen.getByText(/Aktivitäten/i)).toBeInTheDocument()

    // Service trigger placeholder section is present.
    expect(screen.getByTestId('service-events-placeholder')).toBeInTheDocument()
  })
})
