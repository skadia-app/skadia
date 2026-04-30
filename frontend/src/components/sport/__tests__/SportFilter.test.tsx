import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import { GlobalSportSettingsProvider } from '@/lib/globalSportSettings'
import { SportFilter, type SportFilterValue } from '../SportFilter'

vi.mock('@/api/stats', () => ({
  fetchSportTypeStats: vi.fn(async () => [{ sport_type: 'cycling', sub_type: 'mountain', count: 1 }]),
}))

function renderFilter(value: SportFilterValue, onChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <GlobalSportSettingsProvider>
        <SportFilter preset="ACTIVITY_ALL" value={value} onChange={onChange} />
      </GlobalSportSettingsProvider>
    </QueryClientProvider>,
  )
  return onChange
}

describe('SportFilter', () => {
  it('renders canonical top sport groups without deprecated top-level options', () => {
    renderFilter({ sportTypes: null })
    expect(screen.getByRole('button', { name: 'Radfahrten' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Radfahrten' }))
    expect(screen.getByText('MTB')).toBeInTheDocument()
    expect(screen.queryByText('mountain_biking')).not.toBeInTheDocument()
  })

  it('emits sport keys and modifier filters together', () => {
    const onChange = renderFilter({ sportTypes: null, modifiers: {} })
    fireEvent.change(screen.getByLabelText(/E-Bike/), { target: { value: 'true' } })
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({
      modifiers: expect.objectContaining({ is_electric: true }),
    }))
  })

  it('hide-unused keeps only existing sport tuples', async () => {
    localStorage.setItem('skadia-hide-unused-sports', 'true')
    renderFilter({ sportTypes: null })

    fireEvent.click(await screen.findByRole('button', { name: 'Radfahrten' }))
    expect(screen.getByText('MTB')).toBeInTheDocument()
    expect(screen.queryByText('Gravel')).not.toBeInTheDocument()
    expect(screen.queryByText('Radfahren')).not.toBeInTheDocument()
    localStorage.removeItem('skadia-hide-unused-sports')
  })
})
