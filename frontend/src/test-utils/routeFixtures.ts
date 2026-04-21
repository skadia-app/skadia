/**
 * Test-Fixture-Factories für Route-Planner-Domain — zentrale Quelle, damit neue
 * Type-Felder nur an einer Stelle mit sinnvollen Defaults ergänzt werden müssen.
 *
 * Defaults sind "neutral": Tests, die Metriken validieren wollen, überschreiben
 * gezielt via `overrides`. Compile-grün allein reicht nicht — jedes neu
 * hinzugefügte Feld braucht einen gezielten Test der es aktiv prüft, sonst
 * "dark code" (siehe Plan LF-#21).
 *
 * Verwendung:
 *
 * ```ts
 * import { makeFakeRouteStats, makeFakeSuggestion } from '@/test-utils/routeFixtures'
 *
 * // Neutrale Fixture (keine Metriken gewünscht)
 * const stats = makeFakeRouteStats()
 *
 * // Gezielter Override
 * const stats = makeFakeRouteStats({
 *   forest_pct: 0.65,
 *   forest_ranges: [{ from: 0, to: 10 }],
 * })
 * ```
 */
import type { RouteStats } from '@/hooks/useRoutePlanner'
import type { RouteSuggestion } from '@/api/routes'

/** Minimaler Coordinate-Array (3 Punkte), genug für buildProfile + buildHighlight. */
const DEFAULT_COORDS: number[][] = [
  [12.0, 49.0, 500],
  [12.01, 49.01, 510],
  [12.0, 49.0, 500],
]

export function makeFakeRouteStats(overrides: Partial<RouteStats> = {}): RouteStats {
  return {
    forest_pct: 0,
    protected_pct: 0,
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
