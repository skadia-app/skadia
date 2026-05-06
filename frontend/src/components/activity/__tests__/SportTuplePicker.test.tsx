import { fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import SportTuplePicker from '../SportTuplePicker'
import { sportDetailLabel } from '@/lib/sportTaxonomy'

describe('SportTuplePicker', () => {
  it('filters tuple options and returns sport plus sub-sport', () => {
    const onChange = vi.fn()
    render(<SportTuplePicker sportType="cycling" subType="mountain" onChange={onChange} />)

    expect(screen.getByRole('button', { name: sportDetailLabel('cycling', 'mountain') })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: sportDetailLabel('cycling', 'mountain') }))
    fireEvent.change(screen.getByPlaceholderText('Sportart suchen...'), { target: { value: 'trail' } })
    fireEvent.click(screen.getByRole('button', { name: sportDetailLabel('running', 'trail') }))

    expect(onChange).toHaveBeenCalledWith('running', 'trail')
    expect(screen.queryByPlaceholderText('Sportart suchen...')).not.toBeInTheDocument()
  })
})
