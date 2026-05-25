import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { RouteElevationPoint } from '@/api/routes'

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
  DataZoomComponent: {},
  BrushComponent: {},
  ToolboxComponent: {},
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
import RouteProfileChart, { buildProfile, normalizeRouteProfileInput } from '../RouteProfileChart'
import { makeFakeRouteStats } from '@/test-utils/routeFixtures'

// Minimal RouteElevationPoint array — enough for buildProfile to produce ≥2 valid pts.
// dist_m is cumulative (0 → totalDistanceM). elevation uses ele_m field.
const FAKE_PROFILE = [
  { dist_m: 0,     ele_m: 500 },
  { dist_m: 5850,  ele_m: 510 },
  { dist_m: 11700, ele_m: 500 },
]

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

// ── buildProfile: null ele_m gap handling (Spec §6 Class A) ──────────────────

describe('buildProfile — null ele_m preserved as transparent gap', () => {
  it('TestBuildProfile_PreservesNullEleM_AsGap: null points kept in output with ele_m=null', () => {
    const raw: RouteElevationPoint[] = [
      { dist_m: 0,    ele_m: 500 },
      { dist_m: 3000, ele_m: null },  // DEM coverage gap
      { dist_m: 6000, ele_m: 550 },
    ]
    const result = buildProfile(raw, [])

    // All 3 points must be in the output — null not dropped
    expect(result).toHaveLength(3)
    // First and last have real elevation
    expect(result[0].elevation).toBe(500)
    expect(result[2].elevation).toBe(550)
    // Middle point must be null (gap), NOT 0 or interpolated
    expect(result[1].elevation).toBeNull()
    // Grade for gap point must be 0
    expect(result[1].grade).toBe(0)
  })

  it('TestBuildProfile_PreservesNullEleM_AsGap: indices are preserved so ECharts series data has null at correct index', () => {
    const raw: RouteElevationPoint[] = [
      { dist_m: 0,     ele_m: 400 },
      { dist_m: 5000,  ele_m: null },
      { dist_m: 10000, ele_m: 420 },
    ]
    const result = buildProfile(raw, [])
    // Verify null is at index 1, not shifted or dropped
    const elevationSeries = result.map(p => p.elevation)
    expect(elevationSeries[0]).toBe(400)
    expect(elevationSeries[1]).toBeNull()  // ECharts renders this as gap, not 0
    expect(elevationSeries[2]).toBe(420)
  })

  it('returns empty when fewer than 2 non-null points', () => {
    const raw: RouteElevationPoint[] = [
      { dist_m: 0,    ele_m: null },
      { dist_m: 3000, ele_m: null },
      { dist_m: 6000, ele_m: 500 },  // only 1 non-null — not renderable
    ]
    expect(buildProfile(raw, [])).toHaveLength(0)
  })
})

describe('normalizeRouteProfileInput — loaded route profile hardening', () => {
  it('converts legacy fraction/elevation route profiles to dist_m/ele_m points', () => {
    const legacyProfile = [
      { fraction: 0, elevation: 500 },
      { fraction: 0.5, elevation: null },
      { fraction: 1, elevation: 540 },
    ] as unknown as RouteElevationPoint[]

    expect(normalizeRouteProfileInput(legacyProfile, 12000)).toEqual([
      { dist_m: 0, ele_m: 500 },
      { dist_m: 6000, ele_m: null },
      { dist_m: 12000, ele_m: 540 },
    ])
  })

  it('drops incomplete or non-finite profile points before charting', () => {
    const corruptProfile = [
      { dist_m: 0, ele_m: 500 },
      { dist_m: Number.NaN, ele_m: 510 },
      { dist_m: 1000, ele_m: Number.POSITIVE_INFINITY },
      { fraction: 0.5, elevation: 520 },
      { dist_m: 2000, ele_m: 530 },
      { dist_m: undefined, ele_m: 540 },
    ] as unknown as RouteElevationPoint[]

    expect(normalizeRouteProfileInput(corruptProfile, 2000)).toEqual([
      { dist_m: 0, ele_m: 500 },
      { dist_m: 1000, ele_m: 520 },
      { dist_m: 2000, ele_m: 530 },
    ])
  })

  it('does not feed non-finite points into buildProfile', () => {
    const corruptProfile = [
      { fraction: 0, elevation: 500 },
      { dist_m: undefined, ele_m: 510 },
      { fraction: 1, elevation: 540 },
    ] as unknown as RouteElevationPoint[]

    const normalized = normalizeRouteProfileInput(corruptProfile, 12000)
    const points = buildProfile(normalized, [])

    expect(points).toHaveLength(2)
    for (const point of points) {
      expect(Number.isFinite(point.distanceKm)).toBe(true)
      expect(point.elevation === null || Number.isFinite(point.elevation)).toBe(true)
      expect(Number.isFinite(point.grade)).toBe(true)
    }
  })
})

// ── RouteProfileChart: null ele_m renders chart (not placeholder) ─────────────

describe('RouteProfileChart — null ele_m gap rendering (Spec §6 Class A)', () => {
  it('TestRouteProfileChart_RendersNullAsGap: chart renders (not placeholder) with partial null profile', () => {
    // Profile with a null gap in the middle — but ≥2 non-null points → chart renders.
    const profileWithGap: RouteElevationPoint[] = [
      { dist_m: 0,    ele_m: 500 },
      { dist_m: 3000, ele_m: null },
      { dist_m: 6000, ele_m: 550 },
    ]
    render(
      <RouteProfileChart
        profile={profileWithGap}
        totalDistanceM={6000}
        routeStats={null}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )
    // Chart renders — placeholder "nicht verfügbar" must NOT appear
    expect(screen.queryByText(/nicht verfügbar/i)).not.toBeInTheDocument()
  })

  it('shows placeholder when all ele_m are null (no renderable data)', () => {
    const allNullProfile: RouteElevationPoint[] = [
      { dist_m: 0,    ele_m: null },
      { dist_m: 5000, ele_m: null },
    ]
    render(
      <RouteProfileChart
        profile={allNullProfile}
        totalDistanceM={5000}
        routeStats={null}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )
    // No renderable points → placeholder
    expect(screen.getByText(/nicht verfügbar/i)).toBeInTheDocument()
  })
})
