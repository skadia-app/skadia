import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { useRef } from 'react'
import { i18n } from '@/i18n/i18n'
import { HealthRangeProvider } from '@/components/analytics/filters/HealthRangeContext'

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
vi.mock('echarts/charts', () => ({ LineChart: {}, BarChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  LegendComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: () => useRef<HTMLDivElement>(null),
}))

const mockUserPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

const mockFetchWeightAnalysis = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchWeightAnalysis: (params: string) => mockFetchWeightAnalysis(params),
  }
})

import WeightChart from '../WeightChart'

function renderWithProviders() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={['/analytics/health']}>
      <QueryClientProvider client={client}>
        <HealthRangeProvider>
          <WeightChart />
        </HealthRangeProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('WeightChart unit preferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    void i18n.changeLanguage('en')
    mockFetchWeightAnalysis.mockResolvedValue({
      points: [
        { date: '2024-01-01', weight_kg: 70, body_fat_pct: null },
      ],
      current_kg: 70,
      change_30d: -1,
    })
  })

  it('formats current weight and 30-day change with the user unit preference', async () => {
    renderWithProviders()

    expect(await screen.findByText('154.3 lb')).toBeInTheDocument()
    expect(screen.getByText('-2.2 lb (30 days)')).toBeInTheDocument()
    expect(screen.queryByText(/kg/)).not.toBeInTheDocument()
    expect(screen.queryByText(/30T/)).not.toBeInTheDocument()
  })
})
