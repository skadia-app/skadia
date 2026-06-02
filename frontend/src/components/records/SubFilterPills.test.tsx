import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { getSubFilters } from '@/lib/recordsConstants'
import { SubFilterPills } from './SubFilterPills'

describe('SubFilterPills', () => {
  it('displays localized labels but emits stable filter keys', () => {
    const onChange = vi.fn()

    render(
      <SubFilterPills
        filters={getSubFilters('radfahren', 'en')}
        activeKey="electric"
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'E-bike' }))
    fireEvent.click(screen.getByRole('button', { name: 'All' }))

    expect(onChange).toHaveBeenNthCalledWith(1, 'electric')
    expect(onChange).toHaveBeenNthCalledWith(2, null)
    expect(screen.queryByRole('button', { name: 'Radfahren' })).toBeNull()
  })
})
