import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import FilterActiveBadge from '../FilterActiveBadge'
import type { AnalyticsFilterState } from '@/lib/analyticsFilters'

// useAnalyticsFilters reads from URL search params; mock it for isolation.
const mockFilters: AnalyticsFilterState = {
  from: '', to: '',
  sportTypes: [], tags: [],
  weather: '', tempMin: '', tempMax: '',
  timeOfDay: [], intensity: [],
  distMin: '', distMax: '',
  elevMin: '', elevMax: '',
  gearId: '', isCommute: '', isRace: '',
  paceMin: '', paceMax: '',
  countries: [], seasons: [], weekdays: [],
  hrMin: '', hrMax: '', powerMin: '', powerMax: '',
  includeTags: [], excludeTags: [],
  includeGear: [], excludeGear: [],
  isElectric: null, isIndoor: null, isVirtual: null,
}

vi.mock('@/lib/analyticsFilters', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/analyticsFilters')>()
  return {
    ...actual,
    useAnalyticsFilters: () => ({
      filters: mockFilters,
      setFilters: vi.fn(),
      toQueryString: vi.fn(),
    }),
  }
})

describe('FilterActiveBadge', () => {
  beforeEach(() => {
    // Reset filters to all-empty state before each test.
    Object.assign(mockFilters, {
      from: '', to: '',
      sportTypes: [], tags: [],
      weather: '', tempMin: '', tempMax: '',
      timeOfDay: [], intensity: [],
      distMin: '', distMax: '',
      elevMin: '', elevMax: '',
      gearId: '', isCommute: '', isRace: '',
      paceMin: '', paceMax: '',
      countries: [], seasons: [], weekdays: [],
      hrMin: '', hrMax: '', powerMin: '', powerMax: '',
      includeTags: [], excludeTags: [],
      includeGear: [], excludeGear: [],
      isElectric: null, isIndoor: null, isVirtual: null,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders null when no filters are active', () => {
    const { container } = render(<FilterActiveBadge />)
    expect(container.firstChild).toBeNull()
  })

  it('renders badge with count when one filter is active', () => {
    mockFilters.from = '2024-01-01'
    render(<FilterActiveBadge />)
    expect(screen.getByText(/1 Filter aktiv/)).toBeInTheDocument()
  })

  it('renders badge with count when multiple filters are active', () => {
    mockFilters.from = '2024-01-01'
    mockFilters.sportTypes = ['cycling:road']
    mockFilters.distMin = '10'
    render(<FilterActiveBadge />)
    expect(screen.getByText(/3 Filter aktiv/)).toBeInTheDocument()
  })

  it('hides badge when N === 0', () => {
    const { container } = render(<FilterActiveBadge />)
    expect(container.firstChild).toBeNull()
  })

  it('clicking badge calls scrollIntoView on the filter panel element', () => {
    mockFilters.from = '2024-01-01'

    const mockScrollIntoView = vi.fn()
    const mockElement = { scrollIntoView: mockScrollIntoView }
    vi.spyOn(document, 'getElementById').mockReturnValue(mockElement as unknown as HTMLElement)

    render(<FilterActiveBadge />)
    fireEvent.click(screen.getByRole('button'))

    expect(document.getElementById).toHaveBeenCalledWith('analytics-filter-panel')
    expect(mockScrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth' })
  })

  it('clicking badge dispatches analytics:open-filter-panel custom event', () => {
    mockFilters.from = '2024-01-01'

    const receivedEvents: Event[] = []
    const listener = (e: Event) => receivedEvents.push(e)
    window.addEventListener('analytics:open-filter-panel', listener)

    render(<FilterActiveBadge />)
    fireEvent.click(screen.getByRole('button'))

    expect(receivedEvents).toHaveLength(1)
    expect(receivedEvents[0].type).toBe('analytics:open-filter-panel')

    window.removeEventListener('analytics:open-filter-panel', listener)
  })

  it('does not throw when filter panel element is not in DOM', () => {
    mockFilters.from = '2024-01-01'
    vi.spyOn(document, 'getElementById').mockReturnValue(null)

    render(<FilterActiveBadge />)
    expect(() => fireEvent.click(screen.getByRole('button'))).not.toThrow()
  })
})
