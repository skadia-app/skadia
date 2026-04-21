/**
 * Test-Fixture-Factory für RouteStats — zentrale Quelle, damit neue RouteStats-
 * Felder nur an einer Stelle mit sinnvollen Defaults ergänzt werden müssen.
 *
 * Defaults sind "neutral": rendert weder Badges noch Overlays, liefert aber ein
 * minimales coordinates-Array, damit Profil-Build nicht leer läuft. Tests, die
 * Metriken validieren wollen, überschreiben gezielt via `overrides`.
 *
 * Verwendung:
 *
 * ```ts
 * import { makeFakeRouteStats } from '@/test-utils/routeStats'
 *
 * // Test prüft nur Waypoint-Logik — keine Metriken gewünscht
 * const stats = makeFakeRouteStats()
 *
 * // Test prüft Forest-Metrik
 * const stats = makeFakeRouteStats({
 *   forest_pct: 0.65,
 *   forest_ranges: [{ from: 0, to: 10 }],
 * })
 * ```
 */
import type { RouteStats } from '@/hooks/useRoutePlanner'

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
