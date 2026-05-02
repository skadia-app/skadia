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

// ── Assert that useAnalyticsFilters is never called ───────────────────────────

const mockUseAnalyticsFilters = vi.fn()
vi.mock('@/lib/analyticsFilters', () => ({
  useAnalyticsFilters: mockUseAnalyticsFilters,
}))

// ── PureAggregateRangePicker mock ─────────────────────────────────────────────

const mockRangePickerOnChange = vi.fn()

vi.mock('@/components/analytics/filters/PureAggregateRangePicker', () => ({
  default: ({ onChange, defaultPreset }: { onChange: (r: { from: string; to: string }) => void; defaultPreset?: string }) => {
    // Capture onChange so tests can invoke it.
    mockRangePickerOnChange.mockImplementation(onChange)
    return (
      <div data-testid="pure-range-picker" data-default-preset={defaultPreset}>
        <button
          data-testid="set-range-7d"
          onClick={() => onChange({ from: '2024-06-01', to: '2024-06-07' })}
        >
          7 Tage
        </button>
      </div>
    )
  },
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

describe('StreaksDisplay — career-exception filter isolation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchStreaks.mockResolvedValue(makeStreaksResponse())
  })

  it('never calls useAnalyticsFilters', () => {
    renderWithProviders('/analytics/stats?tags=race&sport_type=cycling')
    expect(mockUseAnalyticsFilters).not.toHaveBeenCalled()
  })

  it('renders local range picker with 1y default preset', async () => {
    renderWithProviders()
    // Range picker is rendered unconditionally, even during loading state.
    const picker = await screen.findByTestId('pure-range-picker')
    expect(picker).toBeInTheDocument()
    expect(picker).toHaveAttribute('data-default-preset', '1y')
  })

  it('does NOT render a sport filter toggle', async () => {
    renderWithProviders()
    await waitFor(() => expect(mockFetchStreaks).toHaveBeenCalled())
    expect(screen.queryByTestId('sport-only-filter-bar')).not.toBeInTheDocument()
    expect(screen.queryByTestId('sport-filter')).not.toBeInTheDocument()
  })

  it('range picker change propagates to API call', async () => {
    renderWithProviders()

    // Wait for picker to be available
    const btn = await screen.findByTestId('set-range-7d')

    const callsBefore = mockFetchStreaks.mock.calls.length

    // Trigger range change via mock picker button (rendered unconditionally)
    btn.click()

    await waitFor(() => expect(mockFetchStreaks.mock.calls.length).toBeGreaterThan(callsBefore))

    const lastCall = mockFetchStreaks.mock.calls[mockFetchStreaks.mock.calls.length - 1]
    const params = new URLSearchParams(lastCall[0] as string)
    expect(params.get('from')).toBe('2024-06-01')
    expect(params.get('to')).toBe('2024-06-07')
    // No sport_type in streaks queries — career-exception.
    expect(params.has('sport_type')).toBe(false)
  })
})
