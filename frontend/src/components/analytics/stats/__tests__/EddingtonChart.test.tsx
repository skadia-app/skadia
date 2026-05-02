import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
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
  MarkLineComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: () => useRef<HTMLDivElement>(null),
}))

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
          data-testid="select-cycling"
          onClick={() => onChange({ ...value, selectedSports: ['cycling:road', 'cycling:mountain'] })}
        >
          Radfahren
        </button>
        <button
          data-testid="set-range"
          onClick={() => onChange({ ...value, from: '2024-01-01', to: '2024-12-31' })}
        >
          2024
        </button>
      </div>
    ),
  }
})

// ── API mocks ─────────────────────────────────────────────────────────────────

const mockFetchEddington = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchEddington: (params: string) => mockFetchEddington(params),
  }
})

// ── Component under test ──────────────────────────────────────────────────────

import EddingtonChart from '../EddingtonChart'

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeEddingtonResponse() {
  return {
    number: 5,
    top_values: [10, 9, 8, 7, 6, 5, 4, 3, 2, 1],
    next_levels: [{ level: 6, remaining: 1 }],
  }
}

function renderWithProviders(url = '/analytics/stats') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[url]}>
      <QueryClientProvider client={client}>
        <EddingtonChart />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('EddingtonChart — career-aggregate filter isolation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchEddington.mockResolvedValue(makeEddingtonResponse())
  })

  it('never calls useAnalyticsFilters', () => {
    renderWithProviders('/analytics/stats?tags=race&weather=sun')
    expect(mockUseAnalyticsFilters).not.toHaveBeenCalled()
  })

  it('renders SportOnlyFilterBar', () => {
    renderWithProviders()
    expect(screen.getByTestId('sport-only-filter-bar')).toBeInTheDocument()
  })

  it('URL params tags and weather have no effect on the API call', async () => {
    renderWithProviders('/analytics/stats?tags=foo&weather=rain')

    await waitFor(() => expect(mockFetchEddington).toHaveBeenCalled())

    for (const call of mockFetchEddington.mock.calls) {
      const rawParams = (call[0] as string).split('?').pop() ?? (call[0] as string)
      const params = new URLSearchParams(rawParams)
      expect(params.has('tags')).toBe(false)
      expect(params.has('weather')).toBe(false)
    }
  })

  it('sport selection from local picker appears expanded in API call', async () => {
    renderWithProviders()

    // Wait for initial render to settle
    await waitFor(() => expect(mockFetchEddington).toHaveBeenCalled())
    vi.clearAllMocks()
    mockFetchEddington.mockResolvedValue(makeEddingtonResponse())

    // Trigger cycling selection via the mock sport filter button
    fireEvent.click(screen.getByTestId('select-cycling'))

    await waitFor(() => expect(mockFetchEddington).toHaveBeenCalled())

    const lastCall = mockFetchEddington.mock.calls[mockFetchEddington.mock.calls.length - 1]
    const rawParams = (lastCall[0] as string)
    const params = new URLSearchParams(rawParams.includes('?') ? rawParams.split('?')[1] : rawParams)
    const sportType = params.get('sport_type') ?? ''
    expect(sportType).toContain('cycling:road')
    expect(sportType).toContain('cycling:mountain')
  })

  it('initial API call has no sport_type when no sport is selected', async () => {
    renderWithProviders()

    await waitFor(() => expect(mockFetchEddington).toHaveBeenCalled())

    const firstCall = mockFetchEddington.mock.calls[0]
    const rawParams = firstCall[0] as string
    const params = new URLSearchParams(rawParams.includes('?') ? rawParams.split('?')[1] : rawParams)
    expect(params.has('sport_type')).toBe(false)
  })
})
