import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import GearForm from './GearForm'
import type { Gear } from '@/api/types'

describe('GearForm', () => {
  it('preserves storage-only gear default sports when saving an edited gear', () => {
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
    expect(onSave.mock.calls[0][0].default_for_sports).toEqual(['fitness_equipment', 'cycling'])
    expect(onSave.mock.calls[0][0].default_sport_predicates).toEqual([
      { sport_type: 'fitness_equipment' },
      { sport_type: 'cycling' },
    ])
  })

  it('edits precise sport predicate defaults including subtype and modifiers', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          default_for_sports: ['cycling'],
          default_sport_predicates: [
            {
              sport_type: 'cycling',
              sub_type: 'mountain',
              modifiers: { is_electric: true },
            },
          ],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    expect(screen.getByLabelText('Standard 1 Sportart')).toHaveValue('cycling:mountain')
    expect(screen.getByLabelText('Standard 1 E-Antrieb')).toHaveValue('true')

    fireEvent.change(screen.getByLabelText('Standard 1 Sportart'), { target: { value: 'diving:ccr_diving' } })
    fireEvent.change(screen.getByLabelText('Standard 1 E-Antrieb'), { target: { value: 'any' } })
    fireEvent.change(screen.getByLabelText('Standard 1 Indoor'), { target: { value: 'false' } })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave.mock.calls[0][0].default_for_sports).toEqual(['diving'])
    expect(onSave.mock.calls[0][0].default_sport_predicates).toEqual([
      {
        sport_type: 'diving',
        sub_type: 'ccr_diving',
        modifiers: { is_indoor: false },
      },
    ])
  })

  it('can clear imported predicate defaults explicitly', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          default_for_sports: ['cycling'],
          default_sport_predicates: [{ sport_type: 'cycling', sub_type: 'mountain' }],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Standard 1 entfernen' }))
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave.mock.calls[0][0].default_for_sports).toEqual([])
    expect(onSave.mock.calls[0][0].default_sport_predicates).toEqual([])
  })

  it('preserves the complete stored subtype when a legacy predicate key contains a colon', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          default_sport_predicates: [{ sport_type: 'cycling', sub_type: 'legacy:subtype' }],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave.mock.calls[0][0].default_sport_predicates).toEqual([
      { sport_type: 'cycling', sub_type: 'legacy:subtype' },
    ])
  })

  it('blocks duplicate precise predicate defaults instead of silently dropping one', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          default_sport_predicates: [
            { sport_type: 'cycling', sub_type: 'mountain', modifiers: { is_electric: true } },
            { sport_type: 'cycling', sub_type: 'mountain', modifiers: { is_electric: true } },
          ],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    expect(screen.getAllByText('Doppelter Standard: gleiche Sportart und Modifier bereits vorhanden.')).toHaveLength(2)
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave).not.toHaveBeenCalled()
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
    default_sport_predicates: [],
    created_at: '2026-05-03T00:00:00Z',
    ...overrides,
  }
}
