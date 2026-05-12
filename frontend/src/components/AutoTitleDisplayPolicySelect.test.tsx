import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AutoTitleDisplayPolicySelect } from './AutoTitleDisplayPolicySelect'

describe('AutoTitleDisplayPolicySelect', () => {
  it('shows compact choices and emits the selected policy', () => {
    const onChange = vi.fn()
    render(
      <AutoTitleDisplayPolicySelect
        value="source_unless_generic"
        onChange={onChange}
        disabled={false}
      />,
    )

    const select = screen.getByLabelText('Titelanzeige') as HTMLSelectElement
    expect(select.value).toBe('source_first')
    expect(screen.queryByRole('option', { name: /Smart/ })).toBeNull()
    expect(screen.getByRole('option', { name: 'Importtitel immer bevorzugen' })).toBeTruthy()
    expect(screen.getByRole('option', { name: 'Auto-Titel immer bevorzugen' })).toBeTruthy()

    fireEvent.change(select, { target: { value: 'auto_first' } })

    expect(onChange).toHaveBeenCalledWith('auto_first')
  })
})
