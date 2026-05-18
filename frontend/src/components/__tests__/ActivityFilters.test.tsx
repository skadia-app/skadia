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

  it('serializes pending lift review filters through URL and API params', () => {
    const filters: ActivityFilterState = { ...EMPTY_FILTERS, liftReviewPending: true }

    expect(filtersToParams(filters).get('lift_review')).toBe('pending')
    expect(paramsToFilters(new URLSearchParams('lift_review=pending')).liftReviewPending).toBe(true)
    expect(filtersToApiParams(filters)).toEqual(expect.objectContaining({ liftReview: 'pending' }))
  })

  it('serializes country filters through URL and API params', () => {
    const filters: ActivityFilterState = { ...EMPTY_FILTERS, countryCodes: ['CH', 'IT'] }

    expect(filtersToParams(filters).get('countries')).toBe('CH,IT')
    expect(paramsToFilters(new URLSearchParams('countries=ch,IT,bad')).countryCodes).toEqual(['CH', 'IT'])
    expect(filtersToApiParams(filters)).toEqual(expect.objectContaining({ countryCodes: ['CH', 'IT'] }))
  })

  it('keeps partial country code input typeable while publishing valid codes', () => {
    const onChange = renderFilters()

    fireEvent.click(screen.getAllByRole('button', { name: /Filter/ })[0])
    const input = screen.getByPlaceholderText('CH, IT')

    fireEvent.change(input, { target: { value: 'C' } })
    expect(input).toHaveValue('C')
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ countryCodes: [] }))

    fireEvent.change(input, { target: { value: 'CH' } })
    expect(input).toHaveValue('CH')
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ countryCodes: ['CH'] }))
  })

  it('shows commute as a sport modifier in the filter popover', () => {
    renderFilters()

    fireEvent.click(screen.getAllByRole('button', { name: /Filter/ })[0])

    expect(screen.getByLabelText(/Pendeln/)).toBeInTheDocument()
  })

  it('labels the generic electric filter as E-Antrieb', () => {
    renderFilters({ ...EMPTY_FILTERS, isElectric: true })

    expect(screen.getByText('Nur E-Antrieb')).toBeInTheDocument()

    fireEvent.click(screen.getAllByRole('button', { name: /Filter/ })[0])
    expect(screen.getAllByLabelText(/E-Antrieb/).length).toBeGreaterThan(0)
    expect(screen.queryByLabelText(/E-Bike/)).not.toBeInTheDocument()
  })

  it('shows pending lift review as a direct filter option and chip', () => {
    const onChange = renderFilters({ ...EMPTY_FILTERS, liftReviewPending: true })

    expect(screen.getByText('Liftprüfung offen')).toBeInTheDocument()

    fireEvent.click(screen.getAllByRole('button', { name: /Filter/ })[0])
    fireEvent.click(screen.getByLabelText('Liftprüfung offen'))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ liftReviewPending: false }))
  })
})
