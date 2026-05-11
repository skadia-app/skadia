import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import SearchableEnumInput from '@/components/gear/inputs/SearchableEnumInput'
import type { OptionValue } from '@/lib/generatedGearSchema'

const OPTIONS: OptionValue[] = [
  { key: 'aluminum', labelDe: 'Aluminium' },
  { key: 'carbon', labelDe: 'Carbon' },
  { key: 'steel', labelDe: 'Stahl' },
  { key: 'titanium', labelDe: 'Titan' },
]

describe('SearchableEnumInput', () => {
  it('TestRendersAllOptionsInitially', () => {
    render(
      <SearchableEnumInput id="test" value="" options={OPTIONS} onChange={vi.fn()} />,
    )
    fireEvent.focus(screen.getByRole('combobox'))
    expect(screen.getByText('Aluminium')).toBeInTheDocument()
    expect(screen.getByText('Carbon')).toBeInTheDocument()
    expect(screen.getByText('Stahl')).toBeInTheDocument()
    expect(screen.getByText('Titan')).toBeInTheDocument()
  })

  it('TestFiltersOptionsByQuery', () => {
    render(
      <SearchableEnumInput id="test" value="" options={OPTIONS} onChange={vi.fn()} />,
    )
    const input = screen.getByRole('combobox')
    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'al' } })
    // 'al' matches 'Aluminium' (contains 'al')
    expect(screen.getByText('Aluminium')).toBeInTheDocument()
    // 'Carbon', 'Stahl', 'Titan' do not contain 'al' (case-insensitive: Stahl does contain 'al' in German)
    // Actually 'Stahl' contains 'ahl' and not 'al' - verify: S-t-a-h-l, yes 'ta' then 'ah' — 'Stahl'.toLowerCase() = 'stahl' contains 'al'? 'stahl'.includes('al') = true (st-a-hl, no: s-t-a-h-l → 'sta','tah','ahl' — 'al' is not in 'stahl'). Let's use 'arb' to match Carbon
    // Rerun check: filter on 'arb' - matches Carbon
    fireEvent.change(input, { target: { value: 'arb' } })
    expect(screen.getByText('Carbon')).toBeInTheDocument()
    expect(screen.queryByText('Aluminium')).not.toBeInTheDocument()
    expect(screen.queryByText('Stahl')).not.toBeInTheDocument()
  })

  it('TestSelectingOptionFiresOnChangeWithKey', () => {
    const onChange = vi.fn()
    render(
      <SearchableEnumInput id="test" value="" options={OPTIONS} onChange={onChange} />,
    )
    fireEvent.focus(screen.getByRole('combobox'))
    fireEvent.click(screen.getByText('Carbon'))
    expect(onChange).toHaveBeenCalledWith('carbon')
  })

  it('TestFreeTextSubmittedAsCustomValueIfAllowed', () => {
    const onChange = vi.fn()
    render(
      <SearchableEnumInput
        id="test"
        value=""
        options={OPTIONS}
        onChange={onChange}
        allowFreeText
      />,
    )
    const input = screen.getByRole('combobox')
    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'Bamboo' } })
    // Custom option should appear
    expect(screen.getByText('"Bamboo" verwenden')).toBeInTheDocument()
    fireEvent.click(screen.getByText('"Bamboo" verwenden'))
    expect(onChange).toHaveBeenCalledWith('Bamboo')
  })

  it('TestFreeTextRejectedWhenStrictEnum', () => {
    const onChange = vi.fn()
    render(
      <SearchableEnumInput
        id="test"
        value=""
        options={OPTIONS}
        onChange={onChange}
        allowFreeText={false}
      />,
    )
    const input = screen.getByRole('combobox')
    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'Bamboo' } })
    // No custom option should appear
    expect(screen.queryByText('"Bamboo" verwenden')).not.toBeInTheDocument()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('TestKeyboardNavigationArrowsAndEnter', () => {
    const onChange = vi.fn()
    render(
      <SearchableEnumInput id="test" value="" options={OPTIONS} onChange={onChange} />,
    )
    const input = screen.getByRole('combobox')
    fireEvent.focus(input)
    // Arrow down to first option
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    // Arrow down again to second option
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    // Press Enter — should select second option (index 1 = carbon)
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith('carbon')
  })
})
