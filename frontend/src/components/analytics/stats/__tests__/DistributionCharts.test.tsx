import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { useRef } from 'react'

// ── ECharts mocks ─────────────────────────────────────────────────────────────

// Capture click handlers registered on the chart instance so tests can invoke them.
const chartClickHandlers: Map<string, ((params: unknown) => void)[]> = new Map()

const mockChartInstance = {
  setOption: vi.fn(),
  resize: vi.fn(),
  dispose: vi.fn(),
  on: vi.fn((event: string, handler: (params: unknown) => void) => {
    if (!chartClickHandlers.has(event)) chartClickHandlers.set(event, [])
    chartClickHandlers.get(event)!.push(handler)
  }),
  off: vi.fn(),
}

vi.mock('echarts/core', () => ({
  default: undefined,
  use: vi.fn(),
  init: vi.fn(() => mockChartInstance),
}))
vi.mock('echarts/charts', () => ({ PieChart: {}, BarChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  LegendComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: (_buildOption: unknown, _deps: unknown, onMount?: (chart: unknown) => void) => {
    const ref = useRef<HTMLDivElement>(null)
    // Register click handlers via the passed onMount callback, using our mock chart.
    if (onMount) onMount(mockChartInstance)
    return ref
  },
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
        <span data-testid="selected-sports">{value.selectedSports.join(',')}</span>
        <button
          data-testid="clear-sports"
          onClick={() => onChange({ ...value, selectedSports: [] })}
        >
          Alle Sportarten
        </button>
      </div>
    ),
  }
})

// ── API mocks ─────────────────────────────────────────────────────────────────

const mockFetchDistributions = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchDistributions: (params: string) => mockFetchDistributions(params),
  }
})

// ── Component under test ──────────────────────────────────────────────────────

import DistributionCharts from '../DistributionCharts'

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeDistributionResponse() {
  return [
    { label: 'Rennrad', value: 100 },
    { label: 'Laufen', value: 80 },
  ]
}

function renderWithProviders(url = '/analytics/stats') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[url]}>
      <QueryClientProvider client={client}>
        <DistributionCharts />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('DistributionCharts — career-aggregate filter isolation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    chartClickHandlers.clear()
    mockFetchDistributions.mockResolvedValue(makeDistributionResponse())
  })

  it('never calls useAnalyticsFilters', () => {
    renderWithProviders('/analytics/stats?tags=race&weather=sun')
    expect(mockUseAnalyticsFilters).not.toHaveBeenCalled()
  })

  it('renders SportOnlyFilterBar', () => {
    renderWithProviders()
    expect(screen.getByTestId('sport-only-filter-bar')).toBeInTheDocument()
  })

  it('pie click on sport segment sets local sport state, not global setFilters', async () => {
    renderWithProviders()

    await waitFor(() => expect(mockFetchDistributions).toHaveBeenCalled())

    const selectedBefore = screen.getByTestId('selected-sports').textContent
    expect(selectedBefore).toBe('')

    // Simulate a pie click on a sport segment by invoking registered click handlers.
    const handlers = chartClickHandlers.get('click') ?? []
    expect(handlers.length).toBeGreaterThan(0)

    // Invoke the first handler with a sport-type segment click event.
    handlers[0]({ name: 'Rennrad' })

    // The local selected-sports display should update — not the global URL.
    await waitFor(() => {
      const selected = screen.getByTestId('selected-sports').textContent
      expect(selected).toContain('Rennrad')
    })

    // setFilters from useAnalyticsFilters must not have been called.
    expect(mockUseAnalyticsFilters).not.toHaveBeenCalled()
  })

  it('URL params tags and weather have no effect on the API call', async () => {
    renderWithProviders('/analytics/stats?tags=foo&weather=rain')

    await waitFor(() => expect(mockFetchDistributions).toHaveBeenCalled())

    for (const call of mockFetchDistributions.mock.calls) {
      const rawParams = call[0] as string
      const params = new URLSearchParams(rawParams.includes('?') ? rawParams.split('?')[1] : rawParams)
      expect(params.has('tags')).toBe(false)
      expect(params.has('weather')).toBe(false)
    }
  })
})
