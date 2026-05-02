import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

// ── Assert that useAnalyticsFilters is never called ───────────────────────────

const mockUseAnalyticsFilters = vi.fn()
vi.mock('@/lib/analyticsFilters', () => ({
  useAnalyticsFilters: mockUseAnalyticsFilters,
}))

// ── SportOnlyFilterBar mock ───────────────────────────────────────────────────

vi.mock('@/components/analytics/filters/SportOnlyFilterBar', async () => {
  const actual = await vi.importActual<typeof import('@/components/analytics/filters/SportOnlyFilterBar')>(
    '@/components/analytics/filters/SportOnlyFilterBar',
  )
  return {
    ...actual,
    default: ({
      value,
      onChange,
    }: {
      value: { selectedSports: string[]; from: string; to: string }
      onChange: (v: { selectedSports: string[]; from: string; to: string }) => void
    }) => (
      <div data-testid="sport-only-filter-bar">
        <button
          data-testid="select-running"
          onClick={() => onChange({ ...value, selectedSports: ['running:trail', 'running:road'] })}
        >
          Laufen
        </button>
      </div>
    ),
  }
})

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
    total_moving_time_s: 1_800_000,
    active_days: 300,
    countries: 10,
    moon_trips: 0,
    everest_count: 0,
    current_day_streak: 5,
    max_day_streak: 30,
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

describe('LifetimeStats — career-aggregate filter isolation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchLifetimeStats.mockResolvedValue(makeLifetimeResponse())
  })

  it('never calls useAnalyticsFilters', () => {
    renderWithProviders('/analytics/stats?tags=race&weather=sun')
    expect(mockUseAnalyticsFilters).not.toHaveBeenCalled()
  })

  it('renders SportOnlyFilterBar', async () => {
    renderWithProviders()
    // Filter bar is rendered immediately, even during loading state.
    expect(await screen.findByTestId('sport-only-filter-bar')).toBeInTheDocument()
  })

  it('URL params tags and weather have no effect on the API call', async () => {
    renderWithProviders('/analytics/stats?tags=foo&weather=rain&gear_id=42')

    await waitFor(() => expect(mockFetchLifetimeStats).toHaveBeenCalled())

    for (const call of mockFetchLifetimeStats.mock.calls) {
      const rawParams = call[0] as string
      const params = new URLSearchParams(rawParams)
      expect(params.has('tags')).toBe(false)
      expect(params.has('weather')).toBe(false)
      expect(params.has('gear_id')).toBe(false)
    }
  })

  it('sport selection from local picker appears in API call', async () => {
    renderWithProviders()

    // Wait for filter bar to be in DOM (rendered even during loading)
    await screen.findByTestId('select-running')
    await waitFor(() => expect(mockFetchLifetimeStats).toHaveBeenCalled())
    vi.clearAllMocks()
    mockFetchLifetimeStats.mockResolvedValue(makeLifetimeResponse())

    fireEvent.click(screen.getByTestId('select-running'))

    await waitFor(() => expect(mockFetchLifetimeStats).toHaveBeenCalled())

    const lastCall = mockFetchLifetimeStats.mock.calls[mockFetchLifetimeStats.mock.calls.length - 1]
    const params = new URLSearchParams(lastCall[0] as string)
    const sportType = params.get('sport_type') ?? ''
    expect(sportType).toContain('running:trail')
    expect(sportType).toContain('running:road')
  })

  it('initial API call has no sport_type when no sport is selected', async () => {
    renderWithProviders()

    await waitFor(() => expect(mockFetchLifetimeStats).toHaveBeenCalled())

    const firstCall = mockFetchLifetimeStats.mock.calls[0]
    const params = new URLSearchParams(firstCall[0] as string)
    expect(params.has('sport_type')).toBe(false)
  })
})
