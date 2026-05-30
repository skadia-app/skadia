/**
 * Test fixture factories for the route-planner domain. Keep default values in
 * one place so new route fields do not drift across tests.
 *
 * Defaults are neutral: tests that validate metrics should override values via
 * `overrides`. Compile-only coverage is not enough for newly added fields;
 * add a focused assertion wherever the field is read or merged.
 *
 * When extending RouteStats / RouteSuggestion:
 * 1. Add a neutral default in the matching factory below.
 * 2. Add a focused test that proves the field is actively consumed:
 *    - extraction from feature.properties
 *    - merge behavior across route segments
 *    - rendering by chart and map consumers
 * 3. A factory default without a focused assertion can hide silent key or scale
 *    regressions.
 *
 * Verwendung:
 *
 * ```ts
 * import { makeFakeRouteStats, makeFakeSuggestion } from '@/test-utils/routeFixtures'
 *
 * // Neutral fixture (no metrics expected)
 * const stats = makeFakeRouteStats()
 *
 * // Focused override
 * const stats = makeFakeRouteStats({
 *   forest_pct: 0.65,
 *   forest_ranges: [{ from: 0, to: 10 }],
 * })
 * ```
 */
import type { RouteStats } from '@/hooks/useRoutePlanner'
import type { RouteSuggestion } from '@/api/routes'

/** Minimal coordinate array (3 points), enough for buildProfile + buildHighlight. */
const DEFAULT_COORDS: number[][] = [
  [12.0, 49.0, 500],
  [12.01, 49.01, 510],
  [12.0, 49.0, 500],
]

export function makeFakeRouteStats(overrides: Partial<RouteStats> = {}): RouteStats {
  // Defaults use "unknown/not computed" (null) instead of "route has 0%" (0).
  // Tests that need an explicit 0% value should override it directly.
  return {
    forest_pct: null,
    protected_pct: null,
    forest_ranges: null,
    protected_ranges: null,
    sac_ranges: null,
    mtb_ranges: null,
    vf_ranges: null,
    vf_unknown_ranges: null,
    summits_passed: null,
    coordinates: DEFAULT_COORDS,
    ...overrides,
  }
}

export function makeFakeSuggestion(overrides: Partial<RouteSuggestion> = {}): RouteSuggestion {
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
          coordinates: DEFAULT_COORDS,
        },
        properties: {},
      }],
    },
    ...overrides,
  }
}
