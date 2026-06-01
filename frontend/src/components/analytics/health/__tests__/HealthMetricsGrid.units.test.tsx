import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import HealthMetricsGrid from '../HealthMetricsGrid'
import { fetchHealthMetrics } from '@/api/analytics'

const mocks = vi.hoisted(() => ({
  userPrefs: { locale: 'en', units: 'imperial' as 'metric' | 'imperial' },
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mocks.userPrefs,
}))

vi.mock('@/components/analytics/filters/HealthRangeContext', () => ({
  useHealthRange: () => ({ from: '2026-05-01', to: '2026-05-31' }),
}))

vi.mock('@/hooks/useECharts', async () => {
  const { useRef } = await import('react')
  return { useECharts: () => useRef(null) }
})

vi.mock('echarts/core', () => ({ use: vi.fn() }))
vi.mock('echarts/charts', () => ({ LineChart: {} }))
vi.mock('echarts/components', () => ({ GridComponent: {}, TooltipComponent: {} }))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('@/api/analytics', () => ({
  fetchHealthMetrics: vi.fn(),
}))

const fetchHealthMetricsMock = vi.mocked(fetchHealthMetrics)

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe('HealthMetricsGrid unit preferences', () => {
  beforeEach(async () => {
    fetchHealthMetricsMock.mockReset()
    mocks.userPrefs.locale = 'en'
    mocks.userPrefs.units = 'imperial'
    await i18n.changeLanguage('en')
  })

  it('formats backend weight metrics with the active unit preference', async () => {
    fetchHealthMetricsMock.mockResolvedValue([
      {
        name: 'weight',
        current: 70,
        unit: 'kg',
        trend: 0,
        points: [{ date: '2026-05-31', value: 70 }],
      },
    ])

    render(<HealthMetricsGrid />, { wrapper })

    expect(await screen.findByText('154.3')).toBeInTheDocument()
    expect(screen.getByText('lb')).toBeInTheDocument()
    expect(screen.queryByText('kg')).not.toBeInTheDocument()
  })
})
