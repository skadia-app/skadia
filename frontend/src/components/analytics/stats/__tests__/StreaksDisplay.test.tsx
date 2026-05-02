import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { useRef } from 'react'

// ── ECharts mocks ─────────────────────────────────────────────────────────────

vi.mock('echarts/core', () => ({
  default: undefined,
  use: vi.fn(),
  init: vi.fn(() => ({
    setOption: vi.fn(),
    resize: vi.fn(),
    dispose: vi.fn(),
    on: vi.fn(),
    off: vi.fn(),
  })),
}))
vi.mock('echarts/charts', () => ({ BarChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: () => useRef<HTMLDivElement>(null),
}))

// ── Global filter mock — returns controllable params ──────────────────────────

const mockToQueryString = vi.fn(() => '')
const mockSetFilters = vi.fn()

vi.mock('@/lib/analyticsFilters', () => ({
  useAnalyticsFilters: () => ({
    toQueryString: mockToQueryString,
    setFilters: mockSetFilters,
    filters: {},
  }),
}))

// ── API mocks ─────────────────────────────────────────────────────────────────

const mockFetchStreaks = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchStreaks: (params: string) => mockFetchStreaks(params),
  }
})

// ── Component under test ──────────────────────────────────────────────────────

import StreaksDisplay from '../StreaksDisplay'

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeStreaksResponse() {
  return {
    current_streak: 7,
    max_streak: 30,
    current_week_streak: 3,
    max_week_streak: 12,
    streak_history: [],
  }
}

function renderWithProviders(url = '/analytics/stats') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[url]}>
      <QueryClientProvider client={client}>
        <StreaksDisplay />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('StreaksDisplay — global filter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockToQueryString.mockReturnValue('')
    mockFetchStreaks.mockResolvedValue(makeStreaksResponse())
  })

  it('calls useAnalyticsFilters to obtain query params', () => {
    renderWithProviders()
    expect(mockToQueryString).toHaveBeenCalled()
  })

  it('does NOT render a local sport filter or range picker', async () => {
    renderWithProviders()
    await waitFor(() => expect(mockFetchStreaks).toHaveBeenCalled())
    expect(screen.queryByTestId('sport-only-filter-bar')).not.toBeInTheDocument()
    expect(screen.queryByTestId('pure-range-picker')).not.toBeInTheDocument()
    expect(screen.queryByTestId('sport-filter')).not.toBeInTheDocument()
  })

  it('global params are forwarded to the API call', async () => {
    mockToQueryString.mockReturnValue('from=2024-01-01&to=2024-12-31')
    renderWithProviders()

    await waitFor(() => expect(mockFetchStreaks).toHaveBeenCalled())

    const lastCall = mockFetchStreaks.mock.calls[mockFetchStreaks.mock.calls.length - 1]
    const params = new URLSearchParams(lastCall[0] as string)
    expect(params.get('from')).toBe('2024-01-01')
    expect(params.get('to')).toBe('2024-12-31')
  })

  it('renders streak cards when data is loaded', async () => {
    renderWithProviders()
    await screen.findByText('Tages-Serie')
    await screen.findByText('Wochen-Serie')
  })

  it('shows empty state when no streaks exist', async () => {
    mockFetchStreaks.mockResolvedValue({
      current_streak: 0,
      max_streak: 0,
      current_week_streak: 0,
      max_week_streak: 0,
      streak_history: [],
    })
    renderWithProviders()
    await screen.findByText('Noch keine Serien')
  })
})
