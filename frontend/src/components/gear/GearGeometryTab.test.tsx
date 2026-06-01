import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GearGeometryTab from './GearGeometryTab'
import type { GearSchema } from '@/lib/generatedGearSchema'
import type { Gear } from '@/api/types'
import { i18n } from '@/i18n/i18n'

const schemaWithGeometry: GearSchema = {
  assignmentRoles: [],
  optionSets: [],
  attributes: [
    {
      key: 'bike.geometry',
      labelDe: 'Geometrie',
      valueType: 'geometry',
      subfields: [
        { key: 'reach_mm', labelDe: 'Reach (mm)', valueType: 'number' },
        { key: 'stack_mm', labelDe: 'Stack (mm)', valueType: 'number' },
      ],
    },
  ],
  gearTypes: [
    {
      key: 'bike',
      labelDe: 'Fahrrad',
      scope: 'bike',
      defaultAssignmentRoleKey: 'primary',
      attributeKeys: ['bike.geometry'],
      modifierKeys: [],
    },
  ],
}

function makeGear(overrides: Partial<Gear> = {}): Gear {
  return {
    id: 'g1',
    user_id: 'u1',
    name: 'Testrad',
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
    created_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('GearGeometryTab', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('TestRendersGeometrySubfieldsWhenAttributePresent', () => {
    const gear = makeGear({ attributes: { geometry: { reach_mm: 420, stack_mm: 610 } } })
    render(<GearGeometryTab item={gear} schema={schemaWithGeometry} onChange={vi.fn()} />)

    expect(screen.getByLabelText('Reach (mm)')).toBeInTheDocument()
    expect(screen.getByLabelText('Stack (mm)')).toBeInTheDocument()
    expect(screen.getByDisplayValue('420')).toBeInTheDocument()
    expect(screen.getByDisplayValue('610')).toBeInTheDocument()
  })

  it('renders empty inputs when attribute values are absent', () => {
    const gear = makeGear({ attributes: {} })
    render(<GearGeometryTab item={gear} schema={schemaWithGeometry} onChange={vi.fn()} />)

    expect(screen.getByLabelText('Reach (mm)')).toBeInTheDocument()
    expect(screen.getByLabelText('Stack (mm)')).toBeInTheDocument()
  })

  it('uses localized schema labels when available', async () => {
    await i18n.changeLanguage('en')
    const gear = makeGear({ attributes: {} })
    const schema: GearSchema = {
      ...schemaWithGeometry,
      attributes: [
        {
          key: 'bike.geometry',
          labelDe: 'Geometrie',
          labels: { de: 'Geometrie', en: 'Geometry' },
          valueType: 'geometry',
          subfields: [
            { key: 'reach_mm', labelDe: 'Reach (mm)', labels: { de: 'Reach (mm)', en: 'Reach' }, valueType: 'number' },
          ],
        },
      ],
    }

    render(<GearGeometryTab item={gear} schema={schema} onChange={vi.fn()} />)

    expect(screen.getByText('Geometry')).toBeInTheDocument()
    expect(screen.getByLabelText('Reach')).toBeInTheDocument()
    expect(screen.queryByText('Geometrie')).not.toBeInTheDocument()
  })

  it('renders nothing when gear type has no geometry attribute', () => {
    const schemaNoGeometry: GearSchema = {
      ...schemaWithGeometry,
      gearTypes: [
        {
          key: 'shoes',
          labelDe: 'Schuhe',
          scope: 'shoes',
          defaultAssignmentRoleKey: 'primary',
          attributeKeys: [],
          modifierKeys: [],
        },
      ],
    }
    const gear = makeGear({ type: 'shoes' })
    const { container } = render(
      <GearGeometryTab item={gear} schema={schemaNoGeometry} onChange={vi.fn()} />,
    )
    expect(container.querySelector('input')).toBeNull()
  })
})
