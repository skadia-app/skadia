import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import MultiEnumInput from '@/components/gear/inputs/MultiEnumInput'
import type { OptionValue } from '@/lib/generatedGearSchema'

const OPTIONS: OptionValue[] = [
  { key: 'positive', labelDe: 'Positiv' },
  { key: 'negative', labelDe: 'Negativ' },
  { key: 'ramp_control', labelDe: 'Rampensteuerung' },
  { key: 'volume_spacer', labelDe: 'Volumenspacer' },
]

describe('MultiEnumInput', () => {
  it('TestRendersChipsForSelected', () => {
    render(
      <MultiEnumInput
        id="test"
        value={['positive', 'negative']}
        options={OPTIONS}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByText('Positiv')).toBeInTheDocument()
    expect(screen.getByText('Negativ')).toBeInTheDocument()
  })

  it('TestRemovesChipOnClick', () => {
    const onChange = vi.fn()
    render(
      <MultiEnumInput
        id="test"
        value={['positive', 'negative']}
        options={OPTIONS}
        onChange={onChange}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Positiv entfernen' }))
    expect(onChange).toHaveBeenCalledWith(['negative'])
  })

  it('TestAddsOptionFromSearchOnSelect', () => {
    const onChange = vi.fn()
    render(
      <MultiEnumInput
        id="test"
        value={['positive']}
        options={OPTIONS}
        onChange={onChange}
      />,
    )
    const input = screen.getByRole('combobox')
    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'Ram' } })
    // 'Ram' should match 'Rampensteuerung'
    const btn = screen.getByText('Rampensteuerung')
    fireEvent.click(btn)
    expect(onChange).toHaveBeenCalledWith(['positive', 'ramp_control'])
  })

  it('TestRejectsDuplicates', () => {
    const onChange = vi.fn()
    render(
      <MultiEnumInput
        id="test"
        value={['positive']}
        options={OPTIONS}
        onChange={onChange}
      />,
    )
    const input = screen.getByRole('combobox')
    fireEvent.focus(input)
    // 'positive' is already selected — should not appear in the dropdown
    expect(screen.queryByRole('option', { name: 'Positiv' })).not.toBeInTheDocument()
  })

  it('TestShowsNoOptionsWhenAllSelected', () => {
    render(
      <MultiEnumInput
        id="test"
        value={['positive', 'negative', 'ramp_control', 'volume_spacer']}
        options={OPTIONS}
        onChange={vi.fn()}
      />,
    )
    const input = screen.getByRole('combobox')
    fireEvent.focus(input)
    expect(screen.getByText('Keine weiteren Optionen')).toBeInTheDocument()
  })
})
