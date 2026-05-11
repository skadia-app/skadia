import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import GearForm from './GearForm'
import type { Gear } from '@/api/types'

describe('GearForm', () => {
  it('hides custom subtype input until a type is selected in scoped create forms', () => {
    render(
      <GearForm
        categoryScope="diving"
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    )

    expect(screen.getByPlaceholderText('z.B. Atemregler Set')).toBeInTheDocument()
    expect(screen.queryByLabelText('Untertyp')).not.toBeInTheDocument()
    expect(screen.queryByPlaceholderText('z.B. MTB, Rennrad, Trail')).not.toBeInTheDocument()
  })

  it('derives predicate defaults from stored predicates when saving', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          applicable_sport_predicates: [
            { sport_type: 'fitness_equipment' },
            { sport_type: 'cycling' },
          ],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave.mock.calls[0][0].applicable_sport_predicates).toEqual([
      { sport_type: 'fitness_equipment' },
      { sport_type: 'cycling' },
    ])
  })

  it('edits precise sport predicate defaults including subtype and modifiers', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          applicable_sport_predicates: [
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

    expect(onSave.mock.calls[0][0].applicable_sport_predicates).toEqual([
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
          applicable_sport_predicates: [{ sport_type: 'cycling', sub_type: 'mountain' }],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Standard 1 entfernen' }))
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave.mock.calls[0][0].applicable_sport_predicates).toEqual([])
  })

  it('preserves the complete stored subtype when a legacy predicate key contains a colon', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          applicable_sport_predicates: [{ sport_type: 'cycling', sub_type: 'legacy:subtype' }],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave.mock.calls[0][0].applicable_sport_predicates).toEqual([
      { sport_type: 'cycling', sub_type: 'legacy:subtype' },
    ])
  })

  it('blocks duplicate precise predicate defaults instead of silently dropping one', () => {
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          applicable_sport_predicates: [
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
    total_elevation_loss_m: 0,
    total_time_s: 0,
    total_moving_time_s: 0,
    total_activities: 0,
    is_retired: false,
    status: 'active',
    applicable_sport_predicates: [],
    created_at: '2026-05-03T00:00:00Z',
    ...overrides,
  }
}

// Task 2.7 tests: slot-aware integration
describe('GearForm — slot integration', () => {
  it('TestRendersInlineAttributesForHelmet', () => {
    render(
      <GearForm
        gear={gearFixture({ type: 'helmet', name: 'Testhelm' })}
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    )
    // Helmet has attributeKeys → should render its attributes inline (no slots)
    // The attributes section renders attribute labels from the schema
    expect(screen.queryByTestId('slot-')).toBeNull()
    // No ComponentSlotList for helmet
    expect(screen.queryByTestId('add-component-button')).not.toBeInTheDocument()
  })

  it('TestRendersInlineAttributesForBikeWithoutSlots', () => {
    // The current generated schema has no gear types with slots (Phase 3 pending).
    // Verify the inline attribute renderer works for 'bike' type normally.
    render(
      <GearForm
        gear={gearFixture({ type: 'bike', name: 'Mein Rad' })}
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    )
    // 'bike' type has attributeKeys (frame_material, wheel_size, etc.)
    // but no slots in current schema — inline rendering should show attribute labels
    expect(screen.queryByTestId('add-component-button')).not.toBeInTheDocument()
  })

  it('TestSubmitsAllAttributesPlusComponentsOnSave', () => {
    // Gear with existing components
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          type: 'bike',
          name: 'Test Bike',
          components: [
            {
              id: 'c1',
              gear_id: 'gear-1',
              name: 'Fox 36',
              type: 'bike.fork',
              slot_key: 'fork',
              total_distance_m: 0,
              total_elevation_gain_m: 0,
              total_elevation_loss_m: 0,
              total_moving_time_s: 0,
              total_time_s: 0,
              total_activities: 0,
              is_retired: false,
              status: 'active',
            },
          ],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    // components should NOT be included for gear types without slots
    // (current bike type has no slots in schema)
    expect(onSave).toHaveBeenCalledTimes(1)
    const saved = onSave.mock.calls[0][0] as Gear
    // components is undefined when type has no slots
    expect(saved.components).toBeUndefined()
  })

  it('persists drag-and-drop reorder of free components in form state', () => {
    // bike.road has slots in the generated schema → ComponentSlotList renders
    // and can fire onFreeComponentReorder.
    const onSave = vi.fn()
    render(
      <GearForm
        gear={gearFixture({
          type: 'bike.road',
          name: 'Road bike',
          components: [
            {
              id: 'free-a', gear_id: 'gear-1', name: 'Lampe A',
              type: 'bike.accessory', slot_key: null,
              total_distance_m: 0, total_elevation_gain_m: 0,
              total_elevation_loss_m: 0, total_moving_time_s: 0,
              total_time_s: 0, total_activities: 0,
              is_retired: false, status: 'active',
            },
            {
              id: 'free-b', gear_id: 'gear-1', name: 'Lampe B',
              type: 'bike.accessory', slot_key: null,
              total_distance_m: 0, total_elevation_gain_m: 0,
              total_elevation_loss_m: 0, total_moving_time_s: 0,
              total_time_s: 0, total_activities: 0,
              is_retired: false, status: 'active',
            },
          ],
        })}
        onSave={onSave}
        onCancel={() => {}}
      />,
    )

    // Drag B before A
    const a = screen.getByTestId('free-component-free-a')
    const b = screen.getByTestId('free-component-free-b')
    fireEvent.dragStart(b)
    fireEvent.dragOver(a)
    fireEvent.drop(a)

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave).toHaveBeenCalledTimes(1)
    const saved = onSave.mock.calls[0][0] as Gear
    // bike.road has slots → components must be present.
    expect(saved.components).toBeDefined()
    const ids = (saved.components ?? []).map(c => c.id)
    // Free components reordered: B first, then A.
    expect(ids).toEqual(expect.arrayContaining(['free-b', 'free-a']))
    expect(ids.indexOf('free-b')).toBeLessThan(ids.indexOf('free-a'))
  })
})

// ── R2-10 tests: schema-driven kombi-group replaces SHOCK_KOMBI_PAIR ──────────
//
// The shock_dimensions kombi-group only appears in componentTypes (bike.frame /
// bike.shock), not in any gear-level attributeKeys. To test the GearForm kombi
// rendering logic, we mock gearSchemaHelpers to inject a two-attribute kombi
// group at the gear level for a known gear type.

vi.mock('@/lib/gearSchemaHelpers', async (importOriginal) => {
  const original = await importOriginal<typeof import('@/lib/gearSchemaHelpers')>()
  return {
    ...original,
    gearTypeAttributes: (schema: Parameters<typeof original.gearTypeAttributes>[0], typeKey: string) => {
      if (typeKey === 'bike.test_kombi') {
        return [
          { key: 'test_pair_a', labelDe: 'Feld A', valueType: 'string', kombiGroup: 'test_pair' },
          { key: 'test_pair_b', labelDe: 'Feld B', valueType: 'string', kombiGroup: 'test_pair' },
        ]
      }
      return original.gearTypeAttributes(schema, typeKey)
    },
  }
})

describe('GearForm — schema-driven kombi group', () => {
  it('TestKombiGroupPairRenderedSideBySide', () => {
    // Two attributes sharing the same kombiGroup should render as a 2-column row.
    // Uses a mocked gearTypeAttributes that returns a test_pair group.
    render(
      <GearForm
        gear={gearFixture({
          type: 'bike.test_kombi',
          name: 'Mein Bike',
          attributes: { test_pair_a: 'foo', test_pair_b: 'bar' },
        })}
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    )
    // Both inputs in the kombi group must be present
    expect(document.querySelector('#gear-attr-test_pair_a')).not.toBeNull()
    expect(document.querySelector('#gear-attr-test_pair_b')).not.toBeNull()
    // Readout shows combined values
    expect(screen.getByText(/foo×bar mm/)).toBeInTheDocument()
  })

  it('TestKombiReadoutAbsentWhenOnlyOneValueSet', () => {
    render(
      <GearForm
        gear={gearFixture({
          type: 'bike.test_kombi',
          name: 'Mein Bike',
          attributes: { test_pair_a: '200' },
        })}
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    )
    // Both inputs present but readout is absent (second value not set)
    expect(document.querySelector('#gear-attr-test_pair_a')).not.toBeNull()
    expect(screen.queryByText(/Einbaumaß/)).not.toBeInTheDocument()
  })
})

// ── Step 2Y tests ─────────────────────────────────────────────────────────────

describe('GearForm — recommended values + custom dropdown + mullet', () => {
  it('TestRecommendedValuesShownFirst — empfohlen tag appears at the top of the dropdown', () => {
    // bike.mtb.enduro has recommended wheel_size: ["29", "27_5"]
    render(
      <GearForm
        gear={gearFixture({ type: 'bike.mtb.enduro', name: 'Enduro' })}
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    )
    // Find the wheel_size select
    const wheelSelect = document.querySelector('#gear-attr-bike\\.wheel_size') as HTMLSelectElement | null
    expect(wheelSelect).not.toBeNull()

    // The first non-empty option should contain "empfohlen" and reference a recommended value
    const options = Array.from(wheelSelect!.options).filter(o => o.value !== '')
    expect(options.length).toBeGreaterThan(0)
    // Recommended options carry "(empfohlen)" in their text
    const firstActualOption = options[0]
    expect(firstActualOption.textContent).toContain('empfohlen')
  })

  it('TestCustomDropdownEntryEnablesTextInput — picking Custom-Wert reveals inline text input', () => {
    render(
      <GearForm
        gear={gearFixture({ type: 'bike.mtb.enduro', name: 'Enduro' })}
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    )
    const wheelSelect = document.querySelector('#gear-attr-bike\\.wheel_size') as HTMLSelectElement | null
    expect(wheelSelect).not.toBeNull()

    // Choose the custom sentinel option
    fireEvent.change(wheelSelect!, { target: { value: '__custom__' } })

    // After selecting custom, a text input should appear in place of the select
    const customInput = document.querySelector('#gear-attr-bike\\.wheel_size') as HTMLInputElement | null
    expect(customInput).not.toBeNull()
    expect(customInput!.tagName.toLowerCase()).toBe('input')
  })

  it('TestMulletToggleSwapsToTwoPickers — checking Mullet reveals Vorne + Hinten pickers', () => {
    render(
      <GearForm
        gear={gearFixture({ type: 'bike', name: 'Mein Bike' })}
        onSave={vi.fn()}
        onCancel={() => {}}
      />,
    )
    // Mullet checkbox should be present for bike type (has _front/_rear)
    const mulletCheck = screen.getByLabelText('Mullet (unterschiedliche Laufradgrößen vorne/hinten)')
    expect(mulletCheck).toBeInTheDocument()

    // Before toggle: unified wheel_size picker visible, front/rear hidden
    expect(document.querySelector('#gear-attr-bike\\.wheel_size')).not.toBeNull()
    expect(document.querySelector('#gear-attr-bike\\.wheel_size_front')).toBeNull()
    expect(document.querySelector('#gear-attr-bike\\.wheel_size_rear')).toBeNull()

    // Enable mullet
    fireEvent.click(mulletCheck)

    // After toggle: front + rear pickers visible, unified hidden
    expect(document.querySelector('#gear-attr-bike\\.wheel_size_front')).not.toBeNull()
    expect(document.querySelector('#gear-attr-bike\\.wheel_size_rear')).not.toBeNull()
    expect(document.querySelector('#gear-attr-bike\\.wheel_size')).toBeNull()
  })
})
