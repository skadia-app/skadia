import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import ComponentSlotList from '@/components/gear/ComponentSlotList'
import type { GearSchema, GearType } from '@/lib/generatedGearSchema'
import type { GearComponent } from '@/api/types'

const schema: GearSchema = {
  assignmentRoles: [],
  optionSets: [],
  attributes: [],
  gearTypes: [],
  componentTypes: [
    { key: 'bike.fork', labelDe: 'Gabel', attributeKeys: [] },
    { key: 'bike.saddle', labelDe: 'Sattel', attributeKeys: [] },
  ],
}

const bikeGearType: GearType = {
  key: 'bike.mtb',
  labelDe: 'Mountainbike',
  scope: 'bike',
  defaultAssignmentRoleKey: 'primary',
  attributeKeys: [],
  modifierKeys: [],
  slots: [
    { key: 'fork', labelDe: 'Gabel', componentType: 'bike.fork' },
    { key: 'saddle', labelDe: 'Sattel', componentType: 'bike.saddle', optional: true },
  ],
}

function makeComponent(overrides: Partial<GearComponent> = {}): GearComponent {
  return {
    id: 'c1',
    gear_id: 'g1',
    name: 'Fox 36',
    type: 'bike.fork',
    total_distance_m: 0,
    total_elevation_gain_m: 0,
    total_elevation_loss_m: 0,
    total_moving_time_s: 0,
    total_time_s: 0,
    total_activities: 0,
    is_retired: false,
    status: 'active',
    ...overrides,
  }
}

describe('ComponentSlotList', () => {
  it('TestListsAllSlotsFromGearType', () => {
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[]}
        onComponentSave={vi.fn()}
      />,
    )
    expect(screen.getByTestId('slot-fork')).toBeInTheDocument()
    expect(screen.getByTestId('slot-saddle')).toBeInTheDocument()
  })

  it('TestShowsBriefSummaryPerSlot', () => {
    const fork = makeComponent({
      slot_key: 'fork',
      brand: 'Fox',
      model: '36 Factory',
    })
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[fork]}
        onComponentSave={vi.fn()}
      />,
    )
    expect(screen.getByText('Fox 36 Factory')).toBeInTheDocument()
  })

  it('TestOpensModalOnSlotClick', () => {
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[]}
        onComponentSave={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByTestId('slot-fork'))
    // Modal should appear with the slot title
    expect(screen.getByRole('dialog', { name: 'Gabel' })).toBeInTheDocument()
  })

  it('TestAddsCustomComponentViaPlusButton', () => {
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[]}
        onComponentSave={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByTestId('add-component-button'))
    // Free-component modal opens (slot is null — shows component type selector)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByLabelText('Bauteiltyp')).toBeInTheDocument()
  })

  it('TestSlotMarkedOptionalIsLabeledOptional', () => {
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[]}
        onComponentSave={vi.fn()}
      />,
    )
    // saddle slot has optional: true
    const saddleSlot = screen.getByTestId('slot-saddle')
    expect(saddleSlot).toHaveTextContent('(optional)')
  })

  it('TestSaveCallsOnComponentSave', () => {
    const onComponentSave = vi.fn()
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[]}
        onComponentSave={onComponentSave}
      />,
    )
    fireEvent.click(screen.getByTestId('slot-fork'))
    // Click save in modal
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    expect(onComponentSave).toHaveBeenCalledTimes(1)
    expect(onComponentSave.mock.calls[0][0]).toMatchObject({ slot_key: 'fork' })
  })

  it('TestFreeComponentsAreDraggable', () => {
    const compA = makeComponent({ id: 'ca', name: 'Dynamo', slot_key: null })
    const compB = makeComponent({ id: 'cb', name: 'Licht', slot_key: null })
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[compA, compB]}
        onComponentSave={vi.fn()}
      />,
    )
    const itemA = screen.getByTestId('free-component-ca')
    const itemB = screen.getByTestId('free-component-cb')
    expect(itemA).toHaveAttribute('draggable', 'true')
    expect(itemB).toHaveAttribute('draggable', 'true')
  })

  it('TestDragAndDropReordersFreeComponents', () => {
    const onFreeComponentReorder = vi.fn()
    const compA = makeComponent({ id: 'ca', name: 'Dynamo', slot_key: null })
    const compB = makeComponent({ id: 'cb', name: 'Licht', slot_key: null })
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[compA, compB]}
        onComponentSave={vi.fn()}
        onFreeComponentReorder={onFreeComponentReorder}
      />,
    )
    const itemA = screen.getByTestId('free-component-ca')
    const itemB = screen.getByTestId('free-component-cb')

    // Simulate drag of item A onto item B
    fireEvent.dragStart(itemA)
    fireEvent.dragOver(itemB)
    fireEvent.drop(itemB)

    expect(onFreeComponentReorder).toHaveBeenCalledTimes(1)
    // After drag A over B: B should come first, A second
    expect(onFreeComponentReorder).toHaveBeenCalledWith(['cb', 'ca'])
  })

  it('TestSlotItemsAreNotDraggable', () => {
    render(
      <ComponentSlotList
        gearType={bikeGearType}
        schema={schema}
        components={[]}
        onComponentSave={vi.fn()}
      />,
    )
    const slotFork = screen.getByTestId('slot-fork')
    // Slot items must not have draggable attribute
    expect(slotFork).not.toHaveAttribute('draggable', 'true')
  })
})
