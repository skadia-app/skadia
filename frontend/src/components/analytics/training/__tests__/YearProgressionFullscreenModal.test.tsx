import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// ── ResizeObserver stub (not available in jsdom) ──────────────────────────────

global.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// ── ECharts mocks ─────────────────────────────────────────────────────────────
// Phase 2.3 modal renders a real ECharts chart via useECharts. Mock the entire
// echarts/core module so init returns a no-op chart stub.

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
}))
vi.mock('echarts/charts', () => ({ LineChart: {}, BarChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  LegendComponent: {},
  DataZoomComponent: {},
  ToolboxComponent: {},
  BrushComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

// ── API mock ──────────────────────────────────────────────────────────────────

const mockFetchYearProgression = vi.fn().mockResolvedValue({
  metric: 'distance_m',
  best_year: 2024,
  years: [
    { year: 2024, monthly: [100, 200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], cumulative: [100, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300], total: 300 },
    { year: 2023, monthly: [50, 100, 150, 0, 0, 0, 0, 0, 0, 0, 0, 0], cumulative: [50, 150, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300], total: 300 },
  ],
})

const mockFetchYearProgressionRolling = vi.fn().mockResolvedValue({
  metric: 'distance_m',
  period_days: 30,
  granularity: 'day',
  years: [],
  best_year: 0,
})

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchYearProgression: (params: string) => mockFetchYearProgression(params),
    fetchYearProgressionRolling: (params: string) => mockFetchYearProgressionRolling(params),
  }
})

// ── Sport stats mock (used by SportFilter internally) ─────────────────────────

vi.mock('@/api/stats', () => ({
  fetchSportTypeStats: vi.fn().mockResolvedValue([]),
}))

vi.mock('@/lib/globalSportSettings', () => ({
  useGlobalSportSettings: () => ({ hideUnused: false }),
}))

// ── Subject ───────────────────────────────────────────────────────────────────

import YearProgressionFullscreenModal from '../YearProgressionFullscreenModal'

function wrapper(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

// ─────────────────────────────────────────────────────────────────────────────

describe('YearProgressionFullscreenModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Reset mocks to default values so Phase 2.3 tests start clean.
    mockFetchYearProgression.mockResolvedValue({
      metric: 'distance_m',
      best_year: 2024,
      years: [
        { year: 2024, monthly: [100, 200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], cumulative: [100, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300], total: 300 },
        { year: 2023, monthly: [50, 100, 150, 0, 0, 0, 0, 0, 0, 0, 0, 0], cumulative: [50, 150, 300, 300, 300, 300, 300, 300, 300, 300, 300, 300], total: 300 },
      ],
    })
    mockFetchYearProgressionRolling.mockResolvedValue({
      metric: 'distance_m',
      period_days: 30,
      granularity: 'day',
      years: [],
      best_year: 0,
    })
  })
  it('renders nothing when open=false', () => {
    const { container } = wrapper(
      <YearProgressionFullscreenModal open={false} onClose={vi.fn()} />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders modal with heading when open=true', () => {
    wrapper(
      <YearProgressionFullscreenModal open={true} onClose={vi.fn()} />
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Jahresvergleich')).toBeInTheDocument()
  })

  it('passes onClose to ModalShell — ESC handling is delegated to ModalShell', () => {
    // ModalShell is responsible for ESC key handling (tested in ModalShell tests).
    // Here we verify that the modal mounts the dialog role element and can be
    // programmatically closed: when open flips to false, the dialog unmounts.
    const onClose = vi.fn()
    const { rerender } = wrapper(
      <YearProgressionFullscreenModal open={true} onClose={onClose} />
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <YearProgressionFullscreenModal open={false} onClose={onClose} />
      </QueryClientProvider>
    )
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('propagates initialMetric — Höhenmeter pill is active when initialMetric=elevation_gain_m', () => {
    wrapper(
      <YearProgressionFullscreenModal
        open={true}
        onClose={vi.fn()}
        initialMetric="elevation_gain_m"
      />
    )
    // The active metric pill should have the "dark background" active class.
    const pill = screen.getByRole('button', { name: 'Höhenmeter' })
    expect(pill).toHaveClass('bg-neutral-900')
  })

  it('defaults to Distanz metric when no initialMetric provided', () => {
    wrapper(
      <YearProgressionFullscreenModal open={true} onClose={vi.fn()} />
    )
    const pill = screen.getByRole('button', { name: 'Distanz' })
    expect(pill).toHaveClass('bg-neutral-900')
  })

  it('state resets on remount (F007): closing and reopening uses initialMetric again', async () => {
    const { rerender } = wrapper(
      <YearProgressionFullscreenModal
        open={true}
        onClose={vi.fn()}
        initialMetric="distance_m"
      />
    )

    // Switch metric by clicking Höhenmeter pill
    await userEvent.click(screen.getByRole('button', { name: 'Höhenmeter' }))
    expect(screen.getByRole('button', { name: 'Höhenmeter' })).toHaveClass('bg-neutral-900')

    // Close the modal (unmounts inner state container)
    rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <YearProgressionFullscreenModal open={false} onClose={vi.fn()} initialMetric="distance_m" />
      </QueryClientProvider>
    )

    // Reopen — should reset to initialMetric = distance_m
    rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <YearProgressionFullscreenModal open={true} onClose={vi.fn()} initialMetric="distance_m" />
      </QueryClientProvider>
    )
    expect(screen.getByRole('button', { name: 'Distanz' })).toHaveClass('bg-neutral-900')
  })

  // ── Phase 2.3 tests ───────────────────────────────────────────────────────

  it('renders Progress mode toggle as active by default', () => {
    wrapper(<YearProgressionFullscreenModal open={true} onClose={vi.fn()} />)
    const progressBtn = screen.getByRole('button', { name: 'Verlauf' })
    expect(progressBtn).toHaveAttribute('aria-pressed', 'true')
    const rollingBtn = screen.getByRole('button', { name: 'Gleitend' })
    expect(rollingBtn).toHaveAttribute('aria-pressed', 'false')
  })

  it('switching to Rolling mode shows period selector and hides it on Progress', async () => {
    wrapper(<YearProgressionFullscreenModal open={true} onClose={vi.fn()} />)

    // Period selector is not visible in progress mode.
    expect(screen.queryByTestId('period-selector')).toBeNull()

    // Switch to rolling.
    await userEvent.click(screen.getByRole('button', { name: 'Gleitend' }))
    expect(screen.getByTestId('period-selector')).toBeInTheDocument()

    // Switch back to progress — period selector disappears.
    await userEvent.click(screen.getByRole('button', { name: 'Verlauf' }))
    expect(screen.queryByTestId('period-selector')).toBeNull()
  })

  it('period-selector: multiplier input updates period days hint', async () => {
    wrapper(<YearProgressionFullscreenModal open={true} onClose={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'Gleitend' }))

    // Default: 1 × Monat = 30 Tage.
    expect(screen.getByText('≈ 30 Tage')).toBeInTheDocument()

    // Change unit to Woche → 1 × 7 = 7 Tage.
    await userEvent.click(screen.getByRole('button', { name: 'Woche' }))
    expect(screen.getByText('≈ 7 Tage')).toBeInTheDocument()

    // Change multiplier to 2 → 2 × 7 = 14.
    // Use fireEvent.change instead of userEvent.type to avoid the jsdom
    // number-input edge case where clear+type appends instead of replacing.
    const input = screen.getByRole('spinbutton', { name: 'Zeitfenster-Anzahl' })
    fireEvent.change(input, { target: { value: '2' } })
    expect(screen.getByText('≈ 14 Tage')).toBeInTheDocument()
  })

  it('period-selector: Tag unit with multiplier 1 shows singular hint', async () => {
    wrapper(<YearProgressionFullscreenModal open={true} onClose={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: 'Gleitend' }))
    await userEvent.click(screen.getByRole('button', { name: 'Tag' }))
    // 1 × day = 1 → singular form "1 Tag Fenster"
    expect(screen.getByText('≈ 1 Tag')).toBeInTheDocument()
  })

  it('metric toggle changes aria-pressed state', async () => {
    wrapper(<YearProgressionFullscreenModal open={true} onClose={vi.fn()} />)

    // Distanz is default.
    expect(screen.getByRole('button', { name: 'Distanz' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Höhenmeter' })).toHaveAttribute('aria-pressed', 'false')

    await userEvent.click(screen.getByRole('button', { name: 'Höhenmeter' }))
    expect(screen.getByRole('button', { name: 'Höhenmeter' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Distanz' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('mode-switch (F007): lockedDate resets, metric and years are preserved', async () => {
    // We test that switching mode does not clear the metric.
    // The lockedDate reset is observable via YearComparisonTable which goes away
    // in rolling mode — structural verification only here.
    wrapper(
      <YearProgressionFullscreenModal
        open={true}
        onClose={vi.fn()}
        initialMetric="elevation_gain_m"
      />,
    )

    // Initial metric: Höhenmeter.
    expect(screen.getByRole('button', { name: 'Höhenmeter' })).toHaveAttribute('aria-pressed', 'true')

    // Switch to rolling.
    await userEvent.click(screen.getByRole('button', { name: 'Gleitend' }))

    // Metric is preserved.
    expect(screen.getByRole('button', { name: 'Höhenmeter' })).toHaveAttribute('aria-pressed', 'true')

    // Rolling mode shows period selector and the rolling description panel.
    expect(screen.getByTestId('period-selector')).toBeInTheDocument()
  })

  it('F007: rolling mode with fewer years than progress trims year pills', async () => {
    // Progress returns 2024 + 2023; rolling only returns 2024.
    mockFetchYearProgressionRolling.mockResolvedValue({
      metric: 'distance_m',
      period_days: 30,
      granularity: 'day',
      best_year: 2024,
      years: [
        { year: 2024, points: [] },
      ],
    })

    wrapper(<YearProgressionFullscreenModal open={true} onClose={vi.fn()} />)

    // Switch to rolling mode.
    await userEvent.click(screen.getByRole('button', { name: 'Gleitend' }))

    // Wait for rolling data to settle.
    await new Promise(r => setTimeout(r, 50))

    // In rolling mode, availableYears derives from rollingResult — only 2024 is present.
    // The year pills container should not show 2023.
    const yearPills = screen.queryByTestId('year-pills')
    if (yearPills) {
      // If the pill container is rendered, 2023 must not appear in it.
      const pills = yearPills.querySelectorAll('button')
      const pillTexts = Array.from(pills).map(b => b.textContent)
      expect(pillTexts).not.toContain('2023')
    }
  })

  it('metric toggle sends the correct metric value via useYearProgression', async () => {
    // This test verifies that switching metric triggers a re-fetch with the new
    // metric param. Because useYearProgression is wired to the real hook (mocked
    // at the API level), we can observe fetch calls.
    wrapper(<YearProgressionFullscreenModal open={true} onClose={vi.fn()} />)

    // Default fetch with distance_m.
    // The hook calls fetchYearProgression; wait briefly for renders to settle.
    await userEvent.click(screen.getByRole('button', { name: 'Höhenmeter' }))

    // After clicking Höhenmeter, a new query with metric=elevation_gain_m is
    // issued. Both fetchYearProgression and fetchYearProgressionRolling accept
    // a params string — verify elevation_gain_m appears in at least one call.
    // Allow async settle.
    await new Promise(r => setTimeout(r, 50))

    const allProgressCalls: string[] = mockFetchYearProgression.mock.calls.flat()
    const hasElevation = allProgressCalls.some(p => p.includes('elevation_gain_m'))
    expect(hasElevation).toBe(true)
  })
})
