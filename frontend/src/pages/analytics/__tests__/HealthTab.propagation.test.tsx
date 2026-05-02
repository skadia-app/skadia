import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

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
vi.mock('echarts/charts', () => ({
  BarChart: {},
  LineChart: {},
  ScatterChart: {},
}))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  LegendComponent: {},
  MarkPointComponent: {},
  MarkLineComponent: {},
  DataZoomComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))
vi.mock('@/hooks/useECharts', async () => {
  const { useRef } = await import('react')
  return { useECharts: () => useRef(null) }
})

// ── DateRangePicker mock (avoid complex calendar DOM) ─────────────────────────

vi.mock('@/components/DateRangePicker', () => ({
  default: () => null,
}))

// ── Changepoints mock ─────────────────────────────────────────────────────────

vi.mock('@/components/analytics/ChangepointMarkers', () => ({
  useChangepoints: () => [],
  changepointsToMarkLines: () => [],
}))

// ── HRVAnalysis + FitnessAge + HRVCorrelation — not part of the 7 daily components ──

vi.mock('@/components/analytics/health/HRVAnalysis', () => ({
  default: () => <div data-testid="hrv-analysis" />,
}))
vi.mock('@/components/analytics/health/FitnessAgeChart', () => ({
  default: () => <div data-testid="fitness-age" />,
}))
vi.mock('@/components/analytics/health/HRVCorrelationChart', () => ({
  default: () => <div data-testid="hrv-correlation" />,
}))

// ── API mocks ─────────────────────────────────────────────────────────────────

vi.mock('@/api/analytics', () => ({
  fetchSleepAnalysis: vi.fn().mockResolvedValue({ points: [], avg_duration_min: 0, avg_score: 0, trend: 'stable' }),
  fetchWeightAnalysis: vi.fn().mockResolvedValue({ points: [], current_kg: 0, change_30d: 0 }),
  fetchHealthStress: vi.fn().mockResolvedValue([]),
  fetchHealthBloodPressure: vi.fn().mockResolvedValue([]),
  fetchHealthSpO2: vi.fn().mockResolvedValue([]),
  fetchActivityGlucose: vi.fn().mockResolvedValue([]),
  fetchHealthMetrics: vi.fn().mockResolvedValue([]),
}))

// ── Component under test ──────────────────────────────────────────────────────

import HealthTab from '../HealthTab'
import * as analytics from '@/api/analytics'

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderHealthTab() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <HealthTab />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('HealthTab — range propagation to health-daily components', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default resolved values are already set in vi.mock above; re-apply after clearAllMocks
    vi.mocked(analytics.fetchSleepAnalysis).mockResolvedValue({
      points: [],
      avg_duration_min: 0,
      avg_score: 0,
      trend: 'stable',
    } as unknown as Awaited<ReturnType<typeof analytics.fetchSleepAnalysis>>)
    vi.mocked(analytics.fetchWeightAnalysis).mockResolvedValue({
      points: [],
      current_kg: 0,
      change_30d: 0,
    } as unknown as Awaited<ReturnType<typeof analytics.fetchWeightAnalysis>>)
    vi.mocked(analytics.fetchHealthStress).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthBloodPressure).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthSpO2).mockResolvedValue([])
    vi.mocked(analytics.fetchActivityGlucose).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthMetrics).mockResolvedValue([])
  })

  it('renders the preset bar with the default 30-day pill active', () => {
    renderHealthTab()
    // The "30 Tage" pill is rendered by HealthRangePresetBar
    expect(screen.getByText('30 Tage')).toBeInTheDocument()
    expect(screen.getByText('Letzte 7 Tage')).toBeInTheDocument()
  })

  it('clicking Letzte 7 Tage propagates a 7-day range to all 7 API calls', async () => {
    renderHealthTab()

    // Wait for initial render and default queries to fire
    await waitFor(() => {
      expect(analytics.fetchSleepAnalysis).toHaveBeenCalled()
    })

    vi.clearAllMocks()
    // Re-apply mocks after clearing
    vi.mocked(analytics.fetchSleepAnalysis).mockResolvedValue({
      points: [],
      avg_duration_min: 0,
      avg_score: 0,
      trend: 'stable',
    } as unknown as Awaited<ReturnType<typeof analytics.fetchSleepAnalysis>>)
    vi.mocked(analytics.fetchWeightAnalysis).mockResolvedValue({
      points: [],
      current_kg: 0,
      change_30d: 0,
    } as unknown as Awaited<ReturnType<typeof analytics.fetchWeightAnalysis>>)
    vi.mocked(analytics.fetchHealthStress).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthBloodPressure).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthSpO2).mockResolvedValue([])
    vi.mocked(analytics.fetchActivityGlucose).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthMetrics).mockResolvedValue([])

    // Click the 7-day pill
    fireEvent.click(screen.getByText('Letzte 7 Tage'))

    // All 7 daily components should re-query with a range spanning ~7 days
    await waitFor(() => {
      expect(analytics.fetchSleepAnalysis).toHaveBeenCalled()
      expect(analytics.fetchWeightAnalysis).toHaveBeenCalled()
      expect(analytics.fetchHealthStress).toHaveBeenCalled()
      expect(analytics.fetchHealthBloodPressure).toHaveBeenCalled()
      expect(analytics.fetchHealthSpO2).toHaveBeenCalled()
      expect(analytics.fetchActivityGlucose).toHaveBeenCalled()
      expect(analytics.fetchHealthMetrics).toHaveBeenCalled()
    })

    // Verify SleepAnalysis and WeightAnalysis (queryParams-based) carry the 7-day range
    const sleepCall = vi.mocked(analytics.fetchSleepAnalysis).mock.calls[0][0] as string
    const sleepParams = new URLSearchParams(sleepCall)
    const weightCall = vi.mocked(analytics.fetchWeightAnalysis).mock.calls[0][0] as string
    const weightParams = new URLSearchParams(weightCall)
    const metricsCall = vi.mocked(analytics.fetchHealthMetrics).mock.calls[0][0] as string
    const metricsParams = new URLSearchParams(metricsCall)

    // from/to must be present and the range must be approximately 7 days
    for (const [label, params] of [['sleep', sleepParams], ['weight', weightParams], ['metrics', metricsParams]] as [string, URLSearchParams][]) {
      const from = params.get('from')
      const to = params.get('to')
      expect(from, `${label} must have from param`).not.toBeNull()
      expect(to, `${label} must have to param`).not.toBeNull()
      const diffDays = (new Date(to!).getTime() - new Date(from!).getTime()) / (1000 * 60 * 60 * 24)
      expect(diffDays, `${label} range should be ~7 days`).toBeGreaterThanOrEqual(6)
      expect(diffDays, `${label} range should be ~7 days`).toBeLessThanOrEqual(8)
    }

    // Verify StressChart, BloodPressure, SpO2, Glucose (from/to as direct args)
    const stressCall = vi.mocked(analytics.fetchHealthStress).mock.calls[0]
    const bpCall = vi.mocked(analytics.fetchHealthBloodPressure).mock.calls[0]
    const spo2Call = vi.mocked(analytics.fetchHealthSpO2).mock.calls[0]
    const glucoseCall = vi.mocked(analytics.fetchActivityGlucose).mock.calls[0]

    for (const [label, call] of [
      ['stress', stressCall],
      ['blood-pressure', bpCall],
      ['spo2', spo2Call],
      ['glucose', glucoseCall],
    ] as [string, string[]][]) {
      const [from, to] = call
      const diffDays = (new Date(to).getTime() - new Date(from).getTime()) / (1000 * 60 * 60 * 24)
      expect(diffDays, `${label} range should be ~7 days`).toBeGreaterThanOrEqual(6)
      expect(diffDays, `${label} range should be ~7 days`).toBeLessThanOrEqual(8)
    }
  })

  it('all 7 components receive the same from/to on pill switch', async () => {
    renderHealthTab()

    await waitFor(() => {
      expect(analytics.fetchSleepAnalysis).toHaveBeenCalled()
    })

    vi.clearAllMocks()
    vi.mocked(analytics.fetchSleepAnalysis).mockResolvedValue({
      points: [],
      avg_duration_min: 0,
      avg_score: 0,
      trend: 'stable',
    } as unknown as Awaited<ReturnType<typeof analytics.fetchSleepAnalysis>>)
    vi.mocked(analytics.fetchWeightAnalysis).mockResolvedValue({
      points: [],
      current_kg: 0,
      change_30d: 0,
    } as unknown as Awaited<ReturnType<typeof analytics.fetchWeightAnalysis>>)
    vi.mocked(analytics.fetchHealthStress).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthBloodPressure).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthSpO2).mockResolvedValue([])
    vi.mocked(analytics.fetchActivityGlucose).mockResolvedValue([])
    vi.mocked(analytics.fetchHealthMetrics).mockResolvedValue([])

    fireEvent.click(screen.getByText('90 Tage'))

    await waitFor(() => {
      expect(analytics.fetchHealthStress).toHaveBeenCalled()
      expect(analytics.fetchHealthBloodPressure).toHaveBeenCalled()
      expect(analytics.fetchHealthSpO2).toHaveBeenCalled()
    })

    // All direct-arg calls must have the same from/to
    const stressFrom = vi.mocked(analytics.fetchHealthStress).mock.calls[0][0]
    const bpFrom = vi.mocked(analytics.fetchHealthBloodPressure).mock.calls[0][0]
    const spo2From = vi.mocked(analytics.fetchHealthSpO2).mock.calls[0][0]
    const glucoseFrom = vi.mocked(analytics.fetchActivityGlucose).mock.calls.length > 0
      ? vi.mocked(analytics.fetchActivityGlucose).mock.calls[0][0]
      : stressFrom

    expect(bpFrom).toBe(stressFrom)
    expect(spo2From).toBe(stressFrom)
    expect(glucoseFrom).toBe(stressFrom)

    // Params-based calls must also align
    const sleepFrom = new URLSearchParams(vi.mocked(analytics.fetchSleepAnalysis).mock.calls[0][0]).get('from')
    const weightFrom = new URLSearchParams(vi.mocked(analytics.fetchWeightAnalysis).mock.calls[0][0]).get('from')
    const metricsFrom = new URLSearchParams(vi.mocked(analytics.fetchHealthMetrics).mock.calls[0][0]).get('from')

    expect(sleepFrom).toBe(stressFrom)
    expect(weightFrom).toBe(stressFrom)
    expect(metricsFrom).toBe(stressFrom)
  })
})
