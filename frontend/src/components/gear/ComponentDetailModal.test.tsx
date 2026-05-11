import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import ComponentDetailModal from '@/components/gear/ComponentDetailModal'
import type { GearSchema, GearTypeSlot } from '@/lib/generatedGearSchema'
import type { GearComponent } from '@/api/types'

const schema: GearSchema = {
  assignmentRoles: [],
  optionSets: [
    {
      key: 'fork.material',
      options: [
        { key: 'carbon', labelDe: 'Carbon' },
        { key: 'aluminum', labelDe: 'Aluminium' },
      ],
    },
  ],
  attributes: [
    {
      key: 'bike.fork.material',
      labelDe: 'Material',
      valueType: 'enum',
      optionSetKey: 'fork.material',
    },
    {
      key: 'bike.fork.travel',
      labelDe: 'Federweg',
      valueType: 'unit_number',
      allowedUnits: ['mm'],
    },
    {
      key: 'bike.fork.features',
      labelDe: 'Features',
      valueType: 'geometry',
      subfields: [
        { key: 'lockout', labelDe: 'Lockout', valueType: 'boolean' },
      ],
    },
  ],
  gearTypes: [],
  componentTypes: [
    {
      key: 'bike.fork',
      labelDe: 'Gabel',
      attributeKeys: ['bike.fork.material', 'bike.fork.travel', 'bike.fork.features'],
    },
    {
      key: 'bike.saddle',
      labelDe: 'Sattel',
      attributeKeys: [],
    },
  ],
}

const forkSlot: GearTypeSlot = {
  key: 'fork',
  labelDe: 'Gabel',
  componentType: 'bike.fork',
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

describe('ComponentDetailModal', () => {
  it('TestRendersAllSubfieldsForComponentType', () => {
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schema}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    // Material enum renders a combobox
    expect(screen.getByLabelText('Material')).toBeInTheDocument()
    // Federweg renders a number input
    expect(screen.getByLabelText('Federweg')).toBeInTheDocument()
    // Features renders as fieldset with legend
    expect(screen.getByText('Features')).toBeInTheDocument()
  })

  it('TestUsesAppropriateInputComponentByValueType', () => {
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schema}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    // enum → SearchableEnumInput renders with label 'Material'
    expect(screen.getByLabelText('Material')).toBeInTheDocument()
    // unit_number → UnitNumberInput — the number input has id with suffix
    const federLabel = screen.getByLabelText('Federweg')
    expect(federLabel).toBeInTheDocument()
    // bool subfield inside Features → checkbox
    expect(screen.getByRole('checkbox')).toBeInTheDocument()
  })

  it('TestSaveTriggersOnSubmitAndClosesModal', () => {
    const onSave = vi.fn()
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schema}
        onSave={onSave}
        onCancel={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave.mock.calls[0][0]).toMatchObject({
      type: 'bike.fork',
      slot_key: 'fork',
    })
  })

  it('TestCancelDiscardsChanges', () => {
    const onCancel = vi.fn()
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={makeComponent({ attributes: { 'bike.fork.material': 'carbon' } })}
        schema={schema}
        onSave={vi.fn()}
        onCancel={onCancel}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Abbrechen' }))
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('TestConditionalFieldsHideWhenSiblingMismatch', () => {
    const schemaWithConditional: GearSchema = {
      ...schema,
      attributes: [
        { key: 'fork.typ', labelDe: 'Typ', valueType: 'string' },
        {
          key: 'fork.luftfeder',
          labelDe: 'Luftfeder',
          valueType: 'string',
          showWhen: { field: 'fork.typ', value: 'luft' },
        },
      ],
      componentTypes: [
        {
          key: 'bike.fork2',
          labelDe: 'Gabel2',
          attributeKeys: ['fork.typ', 'fork.luftfeder'],
        },
      ],
    }
    const slot2: GearTypeSlot = { key: 'fork2', labelDe: 'Gabel 2', componentType: 'bike.fork2' }
    render(
      <ComponentDetailModal
        slot={slot2}
        component={null}
        schema={schemaWithConditional}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    // Typ is always visible, Luftfeder is hidden (no value for fork.typ set)
    expect(screen.getByLabelText('Typ')).toBeInTheDocument()
    expect(screen.queryByLabelText('Luftfeder')).not.toBeInTheDocument()
  })

  it('TestEscapeKeyCallsOnCancel', () => {
    const onCancel = vi.fn()
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schema}
        onSave={vi.fn()}
        onCancel={onCancel}
      />,
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('TestEscapeKeyWithDirtyStateShowsConfirmBeforeClosing', () => {
    const onCancel = vi.fn()
    // Simulate user confirming the close dialog
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    // Render without slot so the type selector appears — changing it makes the form dirty
    render(
      <ComponentDetailModal
        slot={null}
        component={null}
        schema={schema}
        onSave={vi.fn()}
        onCancel={onCancel}
      />,
    )
    // Select a component type to make the form dirty
    const typeSelect = screen.getByLabelText('Bauteiltyp')
    fireEvent.change(typeSelect, { target: { value: 'bike.fork' } })
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(onCancel).toHaveBeenCalledTimes(1)
    ;(window.confirm as ReturnType<typeof vi.spyOn>).mockRestore()
  })

  it('TestEscapeKeyWithDirtyStateDoesNotCloseWhenConfirmCancelled', () => {
    const onCancel = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    render(
      <ComponentDetailModal
        slot={null}
        component={null}
        schema={schema}
        onSave={vi.fn()}
        onCancel={onCancel}
      />,
    )
    const typeSelect = screen.getByLabelText('Bauteiltyp')
    fireEvent.change(typeSelect, { target: { value: 'bike.fork' } })
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(onCancel).not.toHaveBeenCalled()
    ;(window.confirm as ReturnType<typeof vi.spyOn>).mockRestore()
  })

  it('TestBackdropClickClosesDirectlyWhenClean', () => {
    const onCancel = vi.fn()
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schema}
        onSave={vi.fn()}
        onCancel={onCancel}
      />,
    )
    // The backdrop is the outermost div with role=dialog
    const backdrop = screen.getByRole('dialog')
    fireEvent.click(backdrop, { target: backdrop })
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('TestSubmitsPresetGearTypeKeyWhenSelected', () => {
    const onSave = vi.fn()
    // Schema variant with bike-subtype gearTypes so the preset selector renders.
    const schemaWithGearTypes: GearSchema = {
      ...schema,
      gearTypes: [
        {
          key: 'bike',
          labelDe: 'Fahrrad',
          scope: 'bike',
          defaultAssignmentRoleKey: 'primary',
          attributeKeys: [],
          modifierKeys: [],
        },
        {
          key: 'bike.mtb',
          labelDe: 'MTB',
          scope: 'bike',
          parentKey: 'bike',
          defaultAssignmentRoleKey: 'primary',
          attributeKeys: [],
          modifierKeys: [],
        },
        {
          key: 'bike.mtb.enduro',
          labelDe: 'Enduro',
          scope: 'bike',
          parentKey: 'bike.mtb',
          defaultAssignmentRoleKey: 'primary',
          attributeKeys: [],
          modifierKeys: [],
        },
        {
          key: 'bike.road',
          labelDe: 'Rennrad',
          scope: 'bike',
          parentKey: 'bike',
          defaultAssignmentRoleKey: 'primary',
          attributeKeys: [],
          modifierKeys: [],
        },
      ],
    }
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schemaWithGearTypes}
        sourceGearTypeKey="bike.mtb"
        onSave={onSave}
        onCancel={vi.fn()}
      />,
    )
    // The selector must include bike.mtb and its descendant bike.mtb.enduro,
    // and must NOT include bike.road (sibling subtree) or bike (ancestor).
    const presetSelect = screen.getByLabelText('Geartyp-Voreinstellung') as HTMLSelectElement
    const optionValues = Array.from(presetSelect.options).map((o) => o.value).sort()
    expect(optionValues).toEqual(['', 'bike.mtb', 'bike.mtb.enduro'])

    fireEvent.change(presetSelect, { target: { value: 'bike.mtb' } })
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave.mock.calls[0][0]).toMatchObject({
      type: 'bike.fork',
      slot_key: 'fork',
      preset_gear_type_key: 'bike.mtb',
    })
  })

  it('TestPresetSelectorHiddenWhenNoGearTypesInSchema', () => {
    // The default test schema has gearTypes: [], so the selector should not render.
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schema}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    expect(screen.queryByLabelText('Geartyp-Voreinstellung')).not.toBeInTheDocument()
  })

  it('TestPresetClearsToNullWhenLeftEmpty', () => {
    const onSave = vi.fn()
    const schemaWithGearTypes: GearSchema = {
      ...schema,
      gearTypes: [
        {
          key: 'bike',
          labelDe: 'Fahrrad',
          scope: 'bike',
          defaultAssignmentRoleKey: 'primary',
          attributeKeys: [],
          modifierKeys: [],
        },
      ],
    }
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schemaWithGearTypes}
        sourceGearTypeKey="bike"
        onSave={onSave}
        onCancel={vi.fn()}
      />,
    )
    // Default value is '' (Keine Voreinstellung) — save should emit null.
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave.mock.calls[0][0]).toMatchObject({
      preset_gear_type_key: null,
    })
  })

  it('TestBackdropClickWithDirtyStateShowsConfirm', () => {
    const onCancel = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(
      <ComponentDetailModal
        slot={null}
        component={null}
        schema={schema}
        onSave={vi.fn()}
        onCancel={onCancel}
      />,
    )
    // Make form dirty by selecting a component type
    const typeSelect = screen.getByLabelText('Bauteiltyp')
    fireEvent.change(typeSelect, { target: { value: 'bike.fork' } })
    // Click on the backdrop div itself
    const backdrop = screen.getByRole('dialog')
    fireEvent.click(backdrop, { target: backdrop })
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(onCancel).toHaveBeenCalledTimes(1)
    ;(window.confirm as ReturnType<typeof vi.spyOn>).mockRestore()
  })
})

// ── Step 2Y tests ─────────────────────────────────────────────────────────────

describe('ComponentDetailModal — Step 2Y: visibility + custom fields', () => {
  // Schema with appliesToGearWhen: shock attr only visible when gear.frame_design = full_suspension
  const schemaWithConditions: GearSchema = {
    ...schema,
    attributes: [
      {
        key: 'bike.shock_eye',
        labelDe: 'Dämpfer-Einbaulänge',
        valueType: 'string',
        appliesToGearWhen: [
          { ref: 'gear.bike.frame_design', op: 'eq', value: 'full_suspension' },
        ],
      },
      {
        key: 'bike.shock_stroke',
        labelDe: 'Dämpfer-Hub',
        valueType: 'string',
      },
    ],
    componentTypes: [
      {
        key: 'bike.shock',
        labelDe: 'Dämpfer',
        attributeKeys: ['bike.shock_eye', 'bike.shock_stroke'],
      },
    ],
  }

  const shockSlot: GearTypeSlot = {
    key: 'shock',
    labelDe: 'Dämpfer',
    componentType: 'bike.shock',
  }

  it('TestComponentAttributeHiddenByAppliesToGearWhen — rigid gear hides shock_eye_to_eye_mm', () => {
    render(
      <ComponentDetailModal
        slot={shockSlot}
        component={null}
        schema={schemaWithConditions}
        gearAttributes={{ 'bike.frame_design': 'rigid' }}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    // shock_eye should be hidden (gear is rigid, not full_suspension)
    expect(screen.queryByLabelText('Dämpfer-Einbaulänge')).not.toBeInTheDocument()
    // shock_stroke has no appliesToGearWhen so it remains visible
    expect(screen.getByLabelText('Dämpfer-Hub')).toBeInTheDocument()
  })

  it('TestComponentAttributeShownByAppliesToGearWhen — full_suspension shows shock field', () => {
    render(
      <ComponentDetailModal
        slot={shockSlot}
        component={null}
        schema={schemaWithConditions}
        gearAttributes={{ 'bike.frame_design': 'full_suspension' }}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    expect(screen.getByLabelText('Dämpfer-Einbaulänge')).toBeInTheDocument()
  })

  // Schema with a nested object attribute to test sub-attribute rendering
  const schemaWithObject: GearSchema = {
    assignmentRoles: [],
    optionSets: [],
    gearTypes: [],
    attributes: [
      {
        key: 'bike.fork.geo',
        labelDe: 'Geometrie',
        valueType: 'object',
        subfields: [
          { key: 'offset', labelDe: 'Offset', valueType: 'number' },
          { key: 'axle_to_crown', labelDe: 'Achse-Krone', valueType: 'number' },
        ],
      },
    ],
    componentTypes: [
      {
        key: 'bike.fork3',
        labelDe: 'Gabel 3',
        attributeKeys: ['bike.fork.geo'],
      },
    ],
  }

  const fork3Slot: GearTypeSlot = { key: 'fork3', labelDe: 'Gabel 3', componentType: 'bike.fork3' }

  it('TestComponentSubAttributesRenderRecursive — object attr with subfields renders nested', () => {
    render(
      <ComponentDetailModal
        slot={fork3Slot}
        component={null}
        schema={schemaWithObject}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    // The fieldset legend and both subfield labels should appear
    expect(screen.getByText('Geometrie')).toBeInTheDocument()
    expect(screen.getByLabelText('Offset')).toBeInTheDocument()
    expect(screen.getByLabelText('Achse-Krone')).toBeInTheDocument()
  })

  it('TestCustomFieldRoundTrip — add custom field, save, check onSave called with custom key', () => {
    const onSave = vi.fn()
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={null}
        schema={schema}
        onSave={onSave}
        onCancel={vi.fn()}
      />,
    )

    // Click "+ Feld hinzufügen"
    fireEvent.click(screen.getByText('+ Feld hinzufügen'))

    // Fill in Feldname and Wert
    const inputs = screen.getAllByRole('textbox')
    const nameInput = inputs.find(i => (i as HTMLInputElement).placeholder === 'Feldname')
    const valueInput = inputs.find(i => (i as HTMLInputElement).placeholder === 'Wert')
    expect(nameInput).toBeDefined()
    expect(valueInput).toBeDefined()

    fireEvent.change(nameInput!, { target: { value: 'mein_feld' } })
    fireEvent.change(valueInput!, { target: { value: '42' } })

    // Click Hinzufügen
    fireEvent.click(screen.getByText('Hinzufügen'))

    // Save the component
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(onSave).toHaveBeenCalledTimes(1)
    const saved = onSave.mock.calls[0][0] as { attributes: Record<string, unknown> }
    // Number-parsed value
    expect(saved.attributes['mein_feld']).toBe(42)
  })

  it('TestExistingCustomAttributeAppearsInCustomSection — unknown key shown as custom field', () => {
    render(
      <ComponentDetailModal
        slot={forkSlot}
        component={makeComponent({
          type: 'bike.fork',
          attributes: {
            'bike.fork.material': 'carbon',  // schema key
            'mein_custom_key': 'hello',       // NOT in schema
          },
        })}
        schema={schema}
        onSave={vi.fn()}
        onCancel={vi.fn()}
      />,
    )

    // Schema key should be in the schema-driven fields area
    expect(screen.getByLabelText('Material')).toBeInTheDocument()

    // Custom key should appear in the "Eigene Felder" section
    expect(screen.getByText('Eigene Felder')).toBeInTheDocument()
    expect(screen.getByText('mein_custom_key')).toBeInTheDocument()
  })
})
