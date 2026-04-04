import { fmtDuration, fmtDist } from '@/lib/format'

// ── Category label mapping ─────────────────────────────────────────────────────
// NOTE: dist_400m_pool / dist_800m_pool / dist_400m_swim / dist_800m_swim removed —
// the backend uses dist_400m / dist_800m for pool and running alike, distinguished
// by sport_group. These phantom keys never appear in the backend response.

export const CATEGORY_LABELS: Record<string, string> = {
  // Distance (running + pool shared keys)
  dist_400m: '400 m',
  dist_800m: '800 m',
  dist_1km: '1 km',
  dist_1mi: '1 Meile',
  dist_2mi: '2 Meilen',
  dist_5km: '5 km',
  dist_10km: '10 km',
  dist_15km: '15 km',
  dist_10mi: '10 Meilen',
  dist_20km: '20 km',
  dist_hm: 'Halbmarathon',
  dist_30km: '30 km',
  dist_marathon: 'Marathon',
  dist_50km: '50 km',
  dist_100km: '100 km',
  // Distance (cycling)
  dist_25mi: '25 Meilen',
  dist_40km: '40 km',
  dist_50mi: '50 Meilen',
  dist_100mi: '100 Meilen',
  dist_150km: '150 km',
  dist_200km: '200 km',
  dist_300km: '300 km',
  // Distance (swim)
  dist_50m: '50 m',
  dist_100m: '100 m',
  dist_200m: '200 m',
  dist_1500m: '1500 m',
  dist_2km: '2 km',
  // Elevation
  elev_500hm: '500 Hm',
  elev_1000hm: '1000 Hm',
  elev_1500hm: '1500 Hm',
  elev_2000hm: '2000 Hm',
  // Power
  power_5s: '5 Sek',
  power_30s: '30 Sek',
  power_1min: '1 Min',
  power_5min: '5 Min',
  power_10min: '10 Min',
  power_20min: '20 Min',
  power_30min: '30 Min',
  power_1h: '1 Std',
  power_2h: '2 Std',
  // Superlatives
  sup_longest_distance: 'Längste Distanz',
  sup_most_elevation: 'Meiste Höhenmeter',
  sup_biggest_climb: 'Größter Anstieg',
  sup_longest_duration: 'Längste Dauer',
}

export function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category] ?? category.replace(/_/g, ' ')
}

// ── Shared value formatter ─────────────────────────────────────────────────────

/**
 * Format a best effort value for display.
 * - dist_* / elev_* → fmtDuration(elapsed_time_s) — time taken to cover the distance/elevation
 * - power_* → rounded watts
 * - sup_longest_distance → fmtDist (metres input)
 * - sup_most_elevation / sup_biggest_climb → metres rounded
 * - sup_longest_duration → fmtDuration (seconds input)
 */
export function formatBestEffortValue(
  category: string,
  elapsed_time_s?: number | null,
  value_numeric?: number | null,
): string {
  if (category.startsWith('dist_') || category.startsWith('elev_')) {
    if (elapsed_time_s != null) return fmtDuration(elapsed_time_s)
    return '—'
  }
  if (category.startsWith('power_')) {
    if (value_numeric != null) return `${Math.round(value_numeric)} W`
    return '—'
  }
  if (category === 'sup_longest_distance') {
    if (value_numeric != null) return fmtDist(value_numeric)
    return '—'
  }
  if (category === 'sup_most_elevation' || category === 'sup_biggest_climb') {
    if (value_numeric != null) return `${Math.round(value_numeric)} m`
    return '—'
  }
  if (category === 'sup_longest_duration') {
    if (value_numeric != null) return fmtDuration(value_numeric)
    return '—'
  }
  // Fallback
  if (elapsed_time_s != null) return fmtDuration(elapsed_time_s)
  if (value_numeric != null) return value_numeric.toLocaleString('de-DE', { maximumFractionDigits: 1 })
  return '—'
}
