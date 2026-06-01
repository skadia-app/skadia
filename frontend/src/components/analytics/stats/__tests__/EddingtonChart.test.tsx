import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { useRef } from 'react'
import '@/i18n/i18n'

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

describe('EddingtonChart — global filter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockToQueryString.mockReturnValue('')
    mockFetchEddington.mockResolvedValue(makeEddingtonResponse())
  })

  it('calls useAnalyticsFilters to obtain query params', () => {
    renderWithProviders()
    expect(mockToQueryString).toHaveBeenCalled()
  })

  it('does NOT render a local SportOnlyFilterBar', () => {
    renderWithProviders()
    expect(screen.queryByTestId('sport-only-filter-bar')).not.toBeInTheDocument()
  })

  it('global sport_type param is forwarded to the API call', async () => {
    mockToQueryString.mockReturnValue('sport_type=cycling%3Aroad')
    renderWithProviders()

    await waitFor(() => expect(mockFetchEddington).toHaveBeenCalled())

    const lastCall = mockFetchEddington.mock.calls[mockFetchEddington.mock.calls.length - 1]
    expect(lastCall[0] as string).toContain('sport_type=cycling%3Aroad')
  })

  it('initial API call with empty global filter has no sport_type', async () => {
    mockToQueryString.mockReturnValue('')
    renderWithProviders()

    await waitFor(() => expect(mockFetchEddington).toHaveBeenCalled())

    const firstCall = mockFetchEddington.mock.calls[0]
    const params = new URLSearchParams(firstCall[0] as string)
    expect(params.has('sport_type')).toBe(false)
  })

  it('metric toggle triggers a new API call', async () => {
    renderWithProviders()

    await waitFor(() => expect(mockFetchEddington).toHaveBeenCalled())
    vi.clearAllMocks()
    mockFetchEddington.mockResolvedValue(makeEddingtonResponse())

    fireEvent.click(screen.getByText('Höhenmeter'))

    await waitFor(() => expect(mockFetchEddington).toHaveBeenCalled())

    const lastCall = mockFetchEddington.mock.calls[mockFetchEddington.mock.calls.length - 1]
    expect(lastCall[0] as string).toContain('metric=elevation')
  })
})
