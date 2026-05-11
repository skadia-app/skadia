import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import UnitNumberInput from '@/components/gear/inputs/UnitNumberInput'

describe('UnitNumberInput', () => {
  it('TestRendersValueAndUnitDropdown', () => {
    render(
      <UnitNumberInput
        id="test"
        value={{ value: 140, unit: 'mm' }}
        allowedUnits={['mm', 'in']}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByDisplayValue('140')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Einheit' })).toHaveValue('mm')
  })

  it('TestSingleUnitRendersAsLabelOnly', () => {
    render(
      <UnitNumberInput
        id="test"
        value={{ value: 100, unit: 'mm' }}
        allowedUnits={['mm']}
        onChange={vi.fn()}
      />,
    )
    // No combobox for unit — it's a span
    expect(screen.queryByRole('combobox', { name: 'Einheit' })).not.toBeInTheDocument()
    expect(screen.getByText('mm')).toBeInTheDocument()
  })

  it('TestEmitsValueObjectOnChange', () => {
    const onChange = vi.fn()
    render(
      <UnitNumberInput
        id="test"
        value={{ value: 100, unit: 'mm' }}
        allowedUnits={['mm', 'in']}
        onChange={onChange}
      />,
    )
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '150' } })
    expect(onChange).toHaveBeenCalledWith({ value: 150, unit: 'mm' })
  })

  it('TestRejectsNonNumericInput', () => {
    const onChange = vi.fn()
    render(
      <UnitNumberInput
        id="test"
        value={{ value: 100, unit: 'mm' }}
        allowedUnits={['mm']}
        onChange={onChange}
      />,
    )
    // jsdom sanitizes non-numeric input on type="number" inputs to '' — so
    // clearing the field (empty string) results in { value: null }
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '' } })
    expect(onChange).toHaveBeenCalledWith({ value: null, unit: 'mm' })
  })

  it('TestUnitChangeEmitsNewUnit', () => {
    const onChange = vi.fn()
    render(
      <UnitNumberInput
        id="test"
        value={{ value: 100, unit: 'mm' }}
        allowedUnits={['mm', 'in']}
        onChange={onChange}
      />,
    )
    fireEvent.change(screen.getByRole('combobox', { name: 'Einheit' }), { target: { value: 'in' } })
    expect(onChange).toHaveBeenCalledWith({ value: 100, unit: 'in' })
  })
})
