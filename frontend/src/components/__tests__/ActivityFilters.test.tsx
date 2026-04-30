import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import ActivityFilters, {
  EMPTY_FILTERS,
  filtersToApiParams,
  filtersToParams,
  paramsToFilters,
  type ActivityFilterState,
} from '../ActivityFilters'
import { GlobalSportSettingsProvider } from '@/lib/globalSportSettings'

vi.mock('@/api/stats', () => ({
  fetchSportTypeStats: vi.fn(async () => []),
}))

function renderFilters(filters: ActivityFilterState = EMPTY_FILTERS, onChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <GlobalSportSettingsProvider>
        <ActivityFilters filters={filters} onChange={onChange} />
      </GlobalSportSettingsProvider>
    </QueryClientProvider>,
  )
  return onChange
}

describe('ActivityFilters commute modifier', () => {
  it('serializes commute filters through URL and API params', () => {
    const filters: ActivityFilterState = { ...EMPTY_FILTERS, isCommute: true }

    expect(filtersToParams(filters).get('commute')).toBe('true')
    expect(paramsToFilters(new URLSearchParams('commute=false')).isCommute).toBe(false)
    expect(filtersToApiParams(filters)).toEqual(expect.objectContaining({ isCommute: true }))
  })

  it('shows commute as a sport modifier in the filter popover', () => {
    renderFilters()

    fireEvent.click(screen.getByRole('button', { name: /Filter/ }))

    expect(screen.getByLabelText(/Pendeln/)).toBeInTheDocument()
  })
})
