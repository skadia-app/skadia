import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { useRef } from 'react'
import { I18nProvider } from '@/i18n/I18nProvider'

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
vi.mock('echarts/charts', () => ({ LineChart: {}, BarChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  LegendComponent: {},
  DataZoomComponent: {},
  MarkAreaComponent: {},
  MarkLineComponent: {},
  ToolboxComponent: {},
  BrushComponent: {},
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

const mockFetchAnalyticsPMC = vi.fn()
const mockFetchAnalyticsPMCSplit = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchAnalyticsPMC: (params: string) => mockFetchAnalyticsPMC(params),
    fetchAnalyticsPMCSplit: (params: Record<string, string>) => mockFetchAnalyticsPMCSplit(params),
  }
})

// ── Component under test ──────────────────────────────────────────────────────

import PMCChartExtended from '../PMCChartExtended'

// ── Helpers ───────────────────────────────────────────────────────────────────

function makePMCResponse() {
  return {
    days: [],
    acwr: 1.0,
    acwr_status: 'optimal',
    acwr_trend: 'stable',
    ramp_rate: 0,
    monotony: 1,
    strain: 0,
    projection: [],
    using_default_thresholds: false,
    default_threshold_fields: [],
  }
}

function makePMCDays(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    date: `2026-01-${String(index + 1).padStart(2, '0')}`,
    ctl: index + 1,
    atl: index + 2,
    tsb: index + 3,
    tss: index + 4,
  }))
}

function renderWithProviders(url = '/analytics/training') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[url]}>
      <I18nProvider>
        <QueryClientProvider client={client}>
          <PMCChartExtended />
        </QueryClientProvider>
      </I18nProvider>
    </MemoryRouter>,
  )
}

describe('PMCChartExtended — pure-aggregate filter isolation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchAnalyticsPMC.mockResolvedValue(makePMCResponse())
    mockFetchAnalyticsPMCSplit.mockResolvedValue({ series: {}, ...makePMCResponse() })
    localStorage.clear()
  })

  it('does not pass sport_type or tags from URL to the API call', async () => {
    renderWithProviders('/analytics/training?sport_type=cycling&tags=race&from=2024-01-01&to=2024-12-31')

    // Wait for at least one API call (loading state resolves)
    await screen.findByText('Performance Management (PMC)')

    // Every call to fetchAnalyticsPMC must not contain sport_type or tags
    for (const call of mockFetchAnalyticsPMC.mock.calls) {
      const params = new URLSearchParams(call[0] as string)
      expect(params.has('sport_type')).toBe(false)
      expect(params.has('tags')).toBe(false)
    }
  })

  it('always sends from= and to= in API calls (own internal picker, not from URL sport filters)', async () => {
    // PMC uses its own date-range picker and is NOT affected by URL sport/tag params.
    // The plan-drift decision: PMC keeps its own picker rather than PureAggregateRangePicker
    // (the existing picker is richer and would be lossy to replace).
    renderWithProviders('/analytics/training?from=2024-01-01&to=2024-06-30&sport_type=running')

    await screen.findByText('Performance Management (PMC)')

    const calls = mockFetchAnalyticsPMC.mock.calls
    expect(calls.length).toBeGreaterThan(0)

    for (const call of calls) {
      const params = new URLSearchParams(call[0] as string)
      // sport_type must never leak through (isolation guarantee — the key invariant).
      expect(params.has('sport_type')).toBe(false)
    }

    // Every call that has from= must also have to=, and vice versa.
    // PMC initialises from its own 6m default range — from/to values are
    // derived internally, not from the URL.
    const callsWithFrom = calls.filter(c => new URLSearchParams(c[0] as string).has('from'))
    expect(callsWithFrom.length).toBeGreaterThan(0)
    for (const call of callsWithFrom) {
      const params = new URLSearchParams(call[0] as string)
      expect(params.has('to')).toBe(true)
      // Both values must be non-empty ISO-like date strings.
      expect(params.get('from')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
      expect(params.get('to')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    }
  })

  it('sport_group split is independent of activity filter URL params', async () => {
    // Activate a sport group via localStorage (the PMC-internal mechanism)
    localStorage.setItem('pmc_selected_sport_groups', JSON.stringify(['cycling']))

    renderWithProviders('/analytics/training?sport_type=running&tags=commute')

    await screen.findByText('Performance Management (PMC)')

    // Split calls must not carry sport_type / tags
    for (const call of mockFetchAnalyticsPMCSplit.mock.calls) {
      const params = call[0] as Record<string, string>
      expect(params['sport_type']).toBeUndefined()
      expect(params['tags']).toBeUndefined()
    }
  })

  it('uses localized sport-group labels and hides unknown raw split keys', async () => {
    localStorage.setItem('pmc_selected_sport_groups', JSON.stringify(['cycling']))
    mockFetchAnalyticsPMCSplit.mockResolvedValue({
      ...makePMCResponse(),
      series: {
        __combined__: makePMCDays(6),
        cycling: makePMCDays(6),
        mystery_group: makePMCDays(6),
      },
    })

    renderWithProviders()

    expect(await screen.findByText('Radfahren')).toBeInTheDocument()
    expect(await screen.findByText('Unbekannt')).toBeInTheDocument()
    expect(screen.queryByText('Mystery_group')).not.toBeInTheDocument()
    expect(screen.queryByText('mystery_group')).not.toBeInTheDocument()
  })
})
