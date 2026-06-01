import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { useRef } from 'react'
import { i18n } from '@/i18n/i18n'

// ── ECharts mocks ─────────────────────────────────────────────────────────────

// Capture click handlers registered on the chart instance so tests can invoke them.
const chartClickHandlers: Map<string, ((params: unknown) => void)[]> = new Map()
const chartOptionBuilders: Array<() => any> = []

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
  useECharts: (buildOption: () => any, _deps: unknown, onMount?: (chart: unknown) => void) => {
    const ref = useRef<HTMLDivElement>(null)
    chartOptionBuilders.push(buildOption)
    if (onMount) onMount(mockChartInstance)
    return ref
  },
}))

// ── Global filter mock — returns controllable params ──────────────────────────

const mockToQueryString = vi.fn(() => '')
const mockSetFilters = vi.fn()
const mockUserPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' }))

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

describe('DistributionCharts — global filter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    chartClickHandlers.clear()
    chartOptionBuilders.length = 0
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    void i18n.changeLanguage('en')
    mockToQueryString.mockReturnValue('')
    mockFetchDistributions.mockResolvedValue(makeDistributionResponse())
  })

  it('calls useAnalyticsFilters to obtain query params', () => {
    renderWithProviders()
    expect(mockToQueryString).toHaveBeenCalled()
  })

  it('does NOT render a local SportOnlyFilterBar', () => {
    renderWithProviders()
    expect(screen.queryByTestId('sport-only-filter-bar')).not.toBeInTheDocument()
  })

  it('pie click on sport segment calls global setFilters, not local state', async () => {
    renderWithProviders()

    await waitFor(() => expect(mockFetchDistributions).toHaveBeenCalled())

    const handlers = chartClickHandlers.get('click') ?? []
    expect(handlers.length).toBeGreaterThan(0)

    // Simulate pie click on a sport segment carrying canonical sport_type token.
    handlers[0]({ name: 'cycling', data: { sport_type: 'cycling' } })

    expect(mockSetFilters).toHaveBeenCalledWith(
      expect.objectContaining({ sportTypes: ['cycling'] }),
    )
  })

  it('pie click on unknown sport label is ignored', async () => {
    renderWithProviders()

    await waitFor(() => expect(mockFetchDistributions).toHaveBeenCalled())

    const handlers = chartClickHandlers.get('click') ?? []
    handlers[0]({ name: 'Unknown', data: { sport_type: 'unknown' } })

    expect(mockSetFilters).not.toHaveBeenCalled()
  })

  it('pie click on country segment calls setFilters for country', async () => {
    renderWithProviders()

    await waitFor(() => expect(mockFetchDistributions).toHaveBeenCalled())

    // The second chart card is 'country'; click handlers are registered per-card.
    // At least two handlers are expected (sport + country).
    const handlers = chartClickHandlers.get('click') ?? []
    expect(handlers.length).toBeGreaterThan(1)

    handlers[1]({ name: 'Deutschland', data: {} })

    expect(mockSetFilters).toHaveBeenCalledWith(
      expect.objectContaining({ countries: ['Deutschland'] }),
    )
  })

  it('global params are forwarded to the API call', async () => {
    mockToQueryString.mockReturnValue('sport_type=cycling%3Aroad')
    renderWithProviders()

    await waitFor(() => expect(mockFetchDistributions).toHaveBeenCalled())

    for (const call of mockFetchDistributions.mock.calls) {
      expect(call[0] as string).toContain('sport_type=cycling%3Aroad')
    }
  })

  it('formats distribution distances with the user unit preference', async () => {
    const user = userEvent.setup()
    mockFetchDistributions.mockResolvedValue([{ label: 'cycling', value: 1609.344 }])
    renderWithProviders()

    await user.click(await screen.findByRole('button', { name: 'Distance' }))
    await waitFor(() => {
      expect(mockFetchDistributions.mock.calls.some(call => String(call[0]).includes('metric=distance_m'))).toBe(true)
    })

    const option = chartOptionBuilders.at(-1)?.()
    const html = option.tooltip.formatter({ name: 'Cycling', value: 1609.344, percent: 100 })
    expect(html).toContain('1.0 mi')
    expect(html).not.toContain('km')
  })
})
