import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import RecursiveSubfieldRenderer from '@/components/gear/RecursiveSubfieldRenderer'
import type { Attribute, OptionValue } from '@/lib/generatedGearSchema'
import type { EvalContext } from '@/lib/gearConditionEval'

function noOptions(_key: string | undefined): OptionValue[] {
  return []
}

function makeOptions(keys: string[]): OptionValue[] {
  return keys.map(k => ({ key: k, labelDe: k }))
}

describe('RecursiveSubfieldRenderer', () => {
  it('TestRendersSimpleStringSubfield', () => {
    const subfields: Attribute[] = [
      { key: 'kartusche', labelDe: 'Kartusche', valueType: 'string' },
    ]
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{}}
        prefix="fork.dampfung"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
      />,
    )
    expect(screen.getByLabelText('Kartusche')).toBeInTheDocument()
  })

  it('TestRendersNumberSubfield', () => {
    const subfields: Attribute[] = [
      { key: 'lsc', labelDe: 'LSC (Clicks)', valueType: 'number' },
    ]
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{ lsc: 5 }}
        prefix="fork"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
      />,
    )
    expect(screen.getByDisplayValue('5')).toBeInTheDocument()
  })

  it('TestRendersBooleanSubfieldAsCheckbox', () => {
    const subfields: Attribute[] = [
      { key: 'lockout', labelDe: 'Lockout', valueType: 'boolean' },
    ]
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{ lockout: true }}
        prefix="fork"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
      />,
    )
    const checkbox = screen.getByRole('checkbox')
    expect(checkbox).toBeChecked()
  })

  it('TestRendersNestedObjectSubfield', () => {
    const subfields: Attribute[] = [
      {
        key: 'chassis',
        labelDe: 'Chassis',
        valueType: 'geometry',
        subfields: [
          { key: 'material', labelDe: 'Material', valueType: 'string' },
        ],
      },
    ]
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{ chassis: { material: 'carbon' } }}
        prefix="fork"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
      />,
    )
    // fieldset with legend 'Chassis' should contain 'material' input
    expect(screen.getByText('Chassis')).toBeInTheDocument()
    expect(screen.getByDisplayValue('carbon')).toBeInTheDocument()
  })

  it('TestConditionalFieldHidesFieldWhenSiblingMismatch', () => {
    const subfields: Attribute[] = [
      { key: 'typ', labelDe: 'Typ', valueType: 'string' },
      {
        key: 'luftfeder',
        labelDe: 'Luftfeder',
        valueType: 'string',
        showWhen: { field: 'typ', value: 'luft' },
      },
    ]
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{ typ: 'stahlfeder' }}
        prefix="fork.federung"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
      />,
    )
    // 'typ' is always visible
    expect(screen.getByLabelText('Typ')).toBeInTheDocument()
    // 'luftfeder' should be hidden because typ !== 'luft'
    expect(screen.queryByLabelText('Luftfeder')).not.toBeInTheDocument()
  })

  it('TestRendersEnumAsSearchableEnumInput', () => {
    const subfields: Attribute[] = [
      { key: 'kategorie', labelDe: 'Kategorie', valueType: 'enum', optionSetKey: 'test.kategorie' },
    ]
    const resolveOptions = (_key: string | undefined) => makeOptions(['federgabel', 'starr'])
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{}}
        prefix="fork"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={resolveOptions}
      />,
    )
    // SearchableEnumInput renders a combobox
    expect(screen.getByRole('combobox')).toBeInTheDocument()
  })

  it('TestRendersUnitNumberInput', () => {
    const subfields: Attribute[] = [
      {
        key: 'federweg',
        labelDe: 'Federweg',
        valueType: 'unit_number',
        allowedUnits: ['mm', 'in'],
      },
    ]
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{ federweg: { value: 140, unit: 'mm' } }}
        prefix="fork"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
      />,
    )
    expect(screen.getByDisplayValue('140')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Einheit' })).toHaveValue('mm')
  })

  it('TestFiresOnChangeForLeafInput', () => {
    const onChange = vi.fn()
    const subfields: Attribute[] = [
      { key: 'kartusche', labelDe: 'Kartusche', valueType: 'string' },
    ]
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{}}
        prefix="fork.dampfung"
        depth={1}
        onChange={onChange}
        resolveOptions={noOptions}
      />,
    )
    fireEvent.change(screen.getByLabelText('Kartusche'), { target: { value: 'Fox' } })
    expect(onChange).toHaveBeenCalledWith('kartusche', 'Fox')
  })

  it('TestDepthGuardPreventsInfiniteRecursion', () => {
    // Depth 6 should render nothing
    const subfields: Attribute[] = [
      { key: 'deep', labelDe: 'Deep', valueType: 'string' },
    ]
    const { container } = render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{}}
        prefix="x"
        depth={6}
        onChange={vi.fn()}
        resolveOptions={noOptions}
      />,
    )
    expect(container.querySelector('input')).toBeNull()
  })

  // R2-4: evalCtx-driven visible_when / applies_to_gear_when on subfields

  it('TestSubfieldVisibleWhenMatchingContext — shows when condition is met', () => {
    const subfields: Attribute[] = [
      {
        key: 'luftfeder_detail',
        labelDe: 'Luftfeder-Detail',
        valueType: 'string',
        visibleWhen: [{ ref: 'gear.bauart_typ', op: 'eq', value: 'Hardtail' }],
      },
    ]
    const evalCtx: EvalContext = { self: {}, gear: { bauart_typ: 'Hardtail' } }
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{}}
        prefix="fork"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
        evalCtx={evalCtx}
      />,
    )
    expect(screen.getByLabelText('Luftfeder-Detail')).toBeInTheDocument()
  })

  it('TestSubfieldVisibleWhenNotMatchingContext — hides when condition is not met', () => {
    const subfields: Attribute[] = [
      {
        key: 'luftfeder_detail',
        labelDe: 'Luftfeder-Detail',
        valueType: 'string',
        visibleWhen: [{ ref: 'gear.bauart_typ', op: 'eq', value: 'Hardtail' }],
      },
    ]
    const evalCtx: EvalContext = { self: {}, gear: { bauart_typ: 'Fully' } }
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{}}
        prefix="fork"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
        evalCtx={evalCtx}
      />,
    )
    expect(screen.queryByLabelText('Luftfeder-Detail')).not.toBeInTheDocument()
  })

  it('TestSubfieldWithoutVisibleWhenAlwaysShows — no evalCtx condition means always visible', () => {
    const subfields: Attribute[] = [
      { key: 'name_field', labelDe: 'Name', valueType: 'string' },
    ]
    const evalCtx: EvalContext = { self: {}, gear: { bauart_typ: 'Fully' } }
    render(
      <RecursiveSubfieldRenderer
        subfields={subfields}
        values={{}}
        prefix="fork"
        depth={1}
        onChange={vi.fn()}
        resolveOptions={noOptions}
        evalCtx={evalCtx}
      />,
    )
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
  })
})
