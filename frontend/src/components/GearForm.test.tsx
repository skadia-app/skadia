import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import GearForm from './GearForm'
import type { Gear } from '@/api/types'

describe('GearForm', () => {
  it('does not submit storage-only gear default sports as manual choices', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({ default_for_sports: ['fitness_equipment', 'cycling'] })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave.mock.calls[0][0].default_for_sports).toEqual(['cycling'])
  })
})

function gearFixture(overrides: Partial<Gear> = {}): Gear {
  return {
    id: 'gear-1',
    user_id: 'user-1',
    name: 'Road Bike',
    type: 'bike',
    total_distance_m: 0,
    total_elevation_gain_m: 0,
    total_time_s: 0,
    total_activities: 0,
    is_retired: false,
    status: 'active',
    default_for_sports: [],
    created_at: '2026-05-03T00:00:00Z',
    ...overrides,
  }
}
