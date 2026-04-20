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
import type { RouteSuggestion } from '@/api/routes'

// Minimal ElevationPoint array — enough for buildProfile to produce ≥2 valid pts.
const FAKE_PROFILE = [
  { fraction: 0.0, elevation: 500 },
  { fraction: 0.5, elevation: 510 },
  { fraction: 1.0, elevation: 500 },
]

/** Factory for RouteSuggestion test fixtures. */
function makeFakeSuggestion(overrides: Partial<RouteSuggestion> = {}): RouteSuggestion {
  return {
    variant: 0,
    profile: 'hiking',
    via_point: [12.0, 49.0],
    variant_id: 'shortest',
    variant_label_key: 'shortest',
    distance_m: 11700,
    ascent_m: 208,
    descent_m: 208,
    avg_flat: 0.5,
    loop_quality: 'loop',
    overlap_pct: 0.1,
    distance_target_achieved: true,
    ascent_target_achieved: true,
    is_fallback: false,
    forest_pct: 0,
    protected_pct: 0,
    geojson: {
      type: 'FeatureCollection',
      features: [{
        type: 'Feature',
        geometry: {
          type: 'LineString',
          coordinates: [
            [12.0, 49.0, 500],
            [12.01, 49.01, 510],
            [12.0, 49.0, 500],
          ],
        },
        properties: {},
      }],
    },
    ...overrides,
  }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('RouteProfileChart — K?-Chip-Reset (LF-#11)', () => {
  /**
   * Invariant (I9 / LF-#11):
   * Route A mit vf_unknown_ranges → K?-Chip sichtbar.
   * Route B ohne vf_unknown_ranges → K?-Chip verschwindet.
   *
   * Der K?-Chip ist eine reine Ableitung aus suggestion.vf_unknown_ranges —
   * keine manuelle Invalidierung nötig. Das Parent (MapPage) hält das
   * aktive RouteSuggestion-Objekt und reicht es als `suggestion`-Prop durch.
   * Ein Wechsel von Suggestion A zu B löst automatisch einen Re-Render aus,
   * bei dem das neue Objekt (ohne vf_unknown_ranges) die Render-Bedingung
   * (suggestion.vf_unknown_ranges?.length > 0) auf false setzt.
   */
  it('K?-Chip-Reset (LF-#11): Route B ohne vf_unknown_ranges verliert den K?-Zustand', () => {
    const routeA = makeFakeSuggestion({
      vf_unknown_ranges: [{ from: 0, to: 3 }],
    })

    const { rerender } = render(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        suggestion={routeA}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    // Route A: K?-Chip must be present
    expect(
      screen.getByRole('status', { name: /Klettersteig ohne bestätigte Schwierigkeit/i })
    ).toBeInTheDocument()

    // Switch to Route B: no vf_unknown_ranges
    const routeB = makeFakeSuggestion({
      vf_unknown_ranges: undefined,
    })

    rerender(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        suggestion={routeB}
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
    const suggestion = makeFakeSuggestion({
      vf_unknown_ranges: [],
    })

    render(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        suggestion={suggestion}
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    expect(
      screen.queryByRole('status', { name: /Klettersteig ohne bestätigte Schwierigkeit/i })
    ).not.toBeInTheDocument()
  })

  it('K?-Chip absent when no suggestion prop passed (pre-v4 state)', () => {
    render(
      <RouteProfileChart
        profile={FAKE_PROFILE}
        totalDistanceM={11700}
        // suggestion intentionally omitted
        scalePin={null}
        onTogglePin={() => {}}
      />
    )

    expect(
      screen.queryByRole('status', { name: /Klettersteig ohne bestätigte Schwierigkeit/i })
    ).not.toBeInTheDocument()
  })
})
