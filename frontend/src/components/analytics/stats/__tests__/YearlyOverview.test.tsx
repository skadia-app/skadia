import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import type { YearlyOverviewRow } from '@/api/analytics'
import '@/i18n/i18n'

const mockToQueryString = vi.fn(() => '')
const mockUserPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/lib/analyticsFilters', () => ({
  useAnalyticsFilters: () => ({
    toQueryString: mockToQueryString,
    setFilters: vi.fn(),
    filters: {},
  }),
}))

const mockFetchYearlyOverview = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchYearlyOverview: (params: string) => mockFetchYearlyOverview(params),
  }
})

import YearlyOverview from '../YearlyOverview'

function renderWithProviders() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={['/analytics/stats']}>
      <QueryClientProvider client={client}>
        <YearlyOverview />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('YearlyOverview unit preferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    const rows: YearlyOverviewRow[] = [
      {
        year: new Date().getFullYear(),
        month: new Date().getMonth() + 1,
        count: 1,
        distance_m: 1609.344,
        elevation_m: 100,
        duration_s: 3600,
      },
      {
        year: 2024,
        count: 1,
        distance_m: 1609.344,
        elevation_m: 100,
        duration_s: 3600,
      },
    ]
    mockFetchYearlyOverview.mockResolvedValue(rows)
  })

  it('formats year, month, and fallback summaries with imperial user units', async () => {
    renderWithProviders()

    await waitFor(() => expect(mockFetchYearlyOverview).toHaveBeenCalled())
    expect(await screen.findAllByText('1.0 mi')).toHaveLength(5)
    expect(screen.getAllByText('328 ft')).toHaveLength(5)
  })
})
