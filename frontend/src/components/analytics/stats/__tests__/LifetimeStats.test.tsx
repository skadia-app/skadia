import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import '@/i18n/i18n'

// ── Global filter mock — returns controllable params ──────────────────────────

const mockToQueryString = vi.fn(() => '')
const mockSetFilters = vi.fn()
const mockUserPrefs = vi.hoisted(() => ({ locale: 'de', units: 'metric' }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/lib/analyticsFilters', () => ({
  useAnalyticsFilters: () => ({
    toQueryString: mockToQueryString,
    setFilters: mockSetFilters,
    filters: {},
  }),
}))

// ── API mocks ─────────────────────────────────────────────────────────────────

const mockFetchLifetimeStats = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchLifetimeStats: (params: string) => mockFetchLifetimeStats(params),
  }
})

// ── Component under test ──────────────────────────────────────────────────────

import LifetimeStats from '../LifetimeStats'

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeLifetimeResponse() {
  return {
    total_activities: 500,
    total_distance_m: 5_000_000,
    total_elevation_m: 100_000,
    total_elevation_loss_m: 80_000,
    total_moving_time_s: 1_800_000,
    total_elapsed_time_s: 2_000_000,
    total_calories: 250_000,
    active_days: 300,
    countries: 10,
    earth_laps: 0,
    moon_trips: 0,
    everest_count: 0,
    everest_count_down: 0,
    current_day_streak: 5,
    max_day_streak: 30,
    lift_ride_count: 0,
    lift_ascent_m: 0,
    lift_descent_m: 0,
    lift_distance_m: 0,
  }
}

function renderWithProviders(url = '/analytics/stats') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[url]}>
      <QueryClientProvider client={client}>
        <LifetimeStats />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('LifetimeStats — global filter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
    mockToQueryString.mockReturnValue('')
    mockFetchLifetimeStats.mockResolvedValue(makeLifetimeResponse())
  })

  it('calls useAnalyticsFilters to obtain query params', () => {
    renderWithProviders()
    expect(mockToQueryString).toHaveBeenCalled()
  })

  it('does NOT render a local SportOnlyFilterBar', async () => {
    renderWithProviders()
    await waitFor(() => expect(mockFetchLifetimeStats).toHaveBeenCalled())
    expect(screen.queryByTestId('sport-only-filter-bar')).not.toBeInTheDocument()
  })

  it('global sport_type param is forwarded to the API call', async () => {
    mockToQueryString.mockReturnValue('sport_type=running%3Atrail')
    renderWithProviders()

    await waitFor(() => expect(mockFetchLifetimeStats).toHaveBeenCalled())

    const lastCall = mockFetchLifetimeStats.mock.calls[mockFetchLifetimeStats.mock.calls.length - 1]
    expect(lastCall[0] as string).toContain('sport_type=running%3Atrail')
  })

  it('initial API call with empty global filter has no sport_type', async () => {
    mockToQueryString.mockReturnValue('')
    renderWithProviders()

    await waitFor(() => expect(mockFetchLifetimeStats).toHaveBeenCalled())

    const firstCall = mockFetchLifetimeStats.mock.calls[0]
    const params = new URLSearchParams(firstCall[0] as string)
    expect(params.has('sport_type')).toBe(false)
  })

  it('renders stat cards when data is loaded', async () => {
    renderWithProviders()
    await screen.findByText('500')
  })

  it('renders net ascent and descent lifetime elevation cards', async () => {
    renderWithProviders()

    expect(await screen.findByText('Höhenmeter ↑')).toBeInTheDocument()
    expect(screen.getByText('Höhenmeter ↓')).toBeInTheDocument()
    expect(screen.getByText('100.000 m')).toBeInTheDocument()
    expect(screen.getByText('80.000 m')).toBeInTheDocument()
  })

  it('formats distance and elevation with imperial user units', async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    mockFetchLifetimeStats.mockResolvedValue({
      ...makeLifetimeResponse(),
      total_distance_m: 1609.344,
      total_elevation_m: 100,
      total_elevation_loss_m: 50,
      lift_ride_count: 1,
      lift_ascent_m: 100,
      lift_descent_m: 50,
      lift_distance_m: 1609.344,
    })

    renderWithProviders()

    expect(await screen.findAllByText('1.0 mi')).toHaveLength(2)
    expect(screen.getAllByText('328 ft')).toHaveLength(2)
    expect(screen.getAllByText('164 ft')).toHaveLength(2)
  })
})
