import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
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
    dispatchAction: vi.fn(),
  })),
  graphic: { LinearGradient: class { constructor() {} } },
}))
vi.mock('echarts/charts', () => ({ LineChart: {}, ScatterChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  LegendComponent: {},
  MarkPointComponent: {},
  MarkLineComponent: {},
  DataZoomComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: () => useRef<HTMLDivElement>(null),
}))

// ── Changepoints mock ─────────────────────────────────────────────────────────

vi.mock('@/components/analytics/ChangepointMarkers', () => ({
  useChangepoints: () => [],
  changepointsToMarkLines: () => [],
}))

// ── API mocks ─────────────────────────────────────────────────────────────────

const mockFetchHRVAnalysis = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchHRVAnalysis: (params: string) => mockFetchHRVAnalysis(params),
  }
})

// ── Component under test ──────────────────────────────────────────────────────

import HRVAnalysisChart from '../HRVAnalysis'

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeHRVResponse() {
  return {
    points: [],
    baseline_mean: 50,
    baseline_std: 10,
    seven_day_avg: 48,
    anomalies: [],
  }
}

function renderWithProviders(url = '/analytics/health') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[url]}>
      <QueryClientProvider client={client}>
        <HRVAnalysisChart />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('HRVAnalysis — pure-aggregate filter isolation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchHRVAnalysis.mockResolvedValue(makeHRVResponse())
  })

  it('does not pass sport_type or tags from URL to the API call', async () => {
    renderWithProviders('/analytics/health?sport_type=cycling&tags=race&from=2024-01-01&to=2024-12-31')

    // Wait for component to mount and fire initial query
    await screen.findByText('HRV-Analyse')

    for (const call of mockFetchHRVAnalysis.mock.calls) {
      const params = new URLSearchParams(call[0] as string)
      expect(params.has('sport_type')).toBe(false)
      expect(params.has('tags')).toBe(false)
    }
  })

  it('only sends from and to query params', async () => {
    renderWithProviders('/analytics/health?sport_type=running&is_indoor=true')

    await screen.findByText('HRV-Analyse')

    for (const call of mockFetchHRVAnalysis.mock.calls) {
      const params = new URLSearchParams(call[0] as string)
      // Only from and to are permitted
      for (const key of params.keys()) {
        expect(['from', 'to']).toContain(key)
      }
    }
  })

  it('renders without useAnalyticsFilters', () => {
    // The component must render without a global analytics filter context.
    // If it still called useAnalyticsFilters it would throw (no provider).
    // This test passes if render does not throw.
    expect(() =>
      renderWithProviders('/analytics/health'),
    ).not.toThrow()
  })
})
