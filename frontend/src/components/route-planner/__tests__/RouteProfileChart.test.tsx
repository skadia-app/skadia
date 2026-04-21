import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'

// ── ECharts mocks ─────────────────────────────────────────────────────────────
// RouteProfileChart calls echarts.use([...]) at module level and echarts.init()
// inside useECharts. jsdom has no Canvas — mock the whole ECharts layer.

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
  graphic: {
    LinearGradient: class {
      constructor() {}
    },
  },
}))

vi.mock('echarts/charts', () => ({ LineChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  AxisPointerComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

// ── useECharts mock ────────────────────────────────────────────────────────────
// Return a stable ref that points to a <div> so containerRef.current is never null.
// The real hook calls echarts.init(containerRef.current) — we skip that entirely.
import { useRef } from 'react'
vi.mock('@/hooks/useECharts', () => ({
  useECharts: () => {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    return useRef<HTMLDivElement>(null)
  },
}))

// ── scaleAggregation mock ─────────────────────────────────────────────────────
// Return empty breakdowns so no SAC/MTB/VF ScaleBadges render — isolates K?-chip.
vi.mock('@/lib/scaleAggregation', () => ({
  aggregateScaleLevels: vi.fn(() => ({ breakdown: [], maxLevel: null })),
}))

// ── Component under test ──────────────────────────────────────────────────────
import RouteProfileChart from '../RouteProfileChart'
import type { RouteStats } from '@/hooks/useRoutePlanner'

// Minimal ElevationPoint array — enough for buildProfile to produce ≥2 valid pts.
const FAKE_PROFILE = [
  { fraction: 0.0, elevation: 500 },
  { fraction: 0.5, elevation: 510 },
  { fraction: 1.0, elevation: 500 },
]

/** Factory for RouteStats test fixtures (V6-Retrofit shape). */
function makeFakeRouteStats(overrides: Partial<RouteStats> = {}): RouteStats {
  return {
    forest_pct: 0,
    protected_pct: 0,
    sac_ranges: null,
    mtb_ranges: null,
    vf_ranges: null,
    vf_unknown_ranges: null,
    summits_passed: null,
    coordinates: [
      [12.0, 49.0, 500],
      [12.01, 49.01, 510],
      [12.0, 49.0, 500],
    ],
    ...overrides,
  }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('RouteProfileChart — K?-Chip-Reset (LF-#11, V6-Retrofit)', () => {
  /**
   * Invariant (I9 / LF-#11):
   * Route A mit vf_unknown_ranges → K?-Chip sichtbar.
   * Route B ohne vf_unknown_ranges → K?-Chip verschwindet.
   *
   * Nach V6-Retrofit liest der Chart routeStats statt suggestion.
   * Das Parent (MapPage) hält planner.routeStats und reicht es als Prop durch.
   */
  it('K?-Chip-Reset (LF-#11): routeStats B ohne vf_unknown_ranges verliert den K?-Zustand', () => {
    const routeStatsA = makeFakeRouteStats({
      vf_unknown_ranges: [{ from: 0, to: 3 }],
    })

    const { rerender } = render(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        routeStats={routeStatsA}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    // Route A: K?-Chip must be present
    expect(
      screen.getByRole('status', { name: /Klettersteig ohne bestätigte Schwierigkeit/i })
    ).toBeInTheDocument()

    // Switch to Route B: no vf_unknown_ranges
    const routeStatsB = makeFakeRouteStats({
      vf_unknown_ranges: null,
    })

    rerender(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        routeStats={routeStatsB}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    // Route B: K?-Chip must be gone
    expect(
      screen.queryByRole('status', { name: /Klettersteig ohne bestätigte Schwierigkeit/i })
    ).not.toBeInTheDocument()
  })

  it('K?-Chip absent when vf_unknown_ranges is empty array', () => {
    const routeStats = makeFakeRouteStats({
      vf_unknown_ranges: [],
    })

    render(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        routeStats={routeStats}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    expect(
      screen.queryByRole('status', { name: /Klettersteig ohne bestätigte Schwierigkeit/i })
    ).not.toBeInTheDocument()
  })

  it('K?-Chip absent when routeStats is null', () => {
    render(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        routeStats={null}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    expect(
      screen.queryByRole('status', { name: /Klettersteig ohne bestätigte Schwierigkeit/i })
    ).not.toBeInTheDocument()
  })

  it('Stats-Zeile zeigt Forest/Protected/Summits wenn routeStats nicht null', () => {
    const routeStats = makeFakeRouteStats({
      forest_pct: 0.42,
      protected_pct: 0.15,
      summits_passed: [
        { osm_id: 1, name: 'Testgipfel', ele_m: 1200, category: 'summit', lat: 47.5, lng: 11.5, dist_m: 5 },
      ],
    })

    render(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        routeStats={routeStats}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    // Forest label (42%) should render — use getAllByText to allow multiple matches
    expect(screen.getByText(/42\s*%/)).toBeInTheDocument()
    // Protected label (15%)
    expect(screen.getByText(/15\s*%/)).toBeInTheDocument()
    // Summit label — check the summits-specific text "1 Gipfel" or similar pattern
    expect(screen.getByText(/Gipfel/i)).toBeInTheDocument()
  })

  it('Stats-Zeile zeigt keine Forest/Protected-Chips wenn routeStats null', () => {
    render(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        routeStats={null}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    // No forest/protected/summits chips — the stats section should not render those labels
    // (the gradient legend also has "%" labels but no forest/summits-specific text)
    expect(screen.queryByText(/Wald|Forest|Gipfel|Naturschutz|Protected/i)).not.toBeInTheDocument()
  })
})
