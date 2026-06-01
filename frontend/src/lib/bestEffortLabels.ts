import { fmtDuration, fmtDist } from '@/lib/format'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'
import {
  formatDistance as formatUnitDistance,
  formatElevation as formatUnitElevation,
  formatPower as formatUnitPower,
  formatSpeed as formatUnitSpeed,
  type UnitFormatOptions,
} from '@/lib/units'
import {
  GENERATED_RECORD_CATEGORIES,
  GENERATED_RECORD_CATEGORY_LABELS,
  GENERATED_RECORD_CATEGORY_ORDER,
  GENERATED_RECORD_GROUPS,
  GENERATED_RECORD_POWER_CATEGORY_ORDER,
} from '@/lib/generatedRecordProjections'

export const CATEGORY_LABELS: Record<string, string> = Object.fromEntries(
  Object.entries(GENERATED_RECORD_CATEGORY_LABELS).map(([key, label]) => [
    key,
    typeof label === 'string' ? label : label.de,
  ]),
)
const CATEGORY_LABEL_MAPS: Record<string, LocalizedString> = Object.fromEntries(
  Object.entries(GENERATED_RECORD_CATEGORY_LABELS).map(([key, label]) => [
    key,
    typeof label === 'string' ? { de: label } : label,
  ]),
)
export const CATEGORY_METADATA = GENERATED_RECORD_CATEGORIES
export const CATEGORY_KEYS: string[] = [...GENERATED_RECORD_CATEGORY_ORDER]
export const POWER_DURATIONS: string[] = [...GENERATED_RECORD_POWER_CATEGORY_ORDER]

const CATEGORY_KEYS_BY_RECORD_GROUP: Record<string, string[]> = Object.fromEntries(
  GENERATED_RECORD_GROUPS.map(group => [group.key, group.categories.map(category => category.key)]),
)

export function categoryLabel(category: string, language?: SupportedLanguage): string {
  return localizeString(CATEGORY_LABEL_MAPS[category], language) || category
}

export function categoryKeysForRecordGroup(recordGroup?: string): string[] {
  if (!recordGroup) return CATEGORY_KEYS
  return CATEGORY_KEYS_BY_RECORD_GROUP[recordGroup] ?? CATEGORY_KEYS
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
  unitOptions?: UnitFormatOptions,
): string {
  if (category.startsWith('dist_') || category.startsWith('elev_')) {
    if (elapsed_time_s != null) return fmtDuration(elapsed_time_s)
    return '—'
  }
  if (category.startsWith('power_')) {
    if (value_numeric != null) {
      return unitOptions
        ? formatUnitPower(value_numeric, unitOptions)
        : `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(Math.round(value_numeric))} W`
    }
    return '—'
  }
  if (category === 'sup_longest_distance') {
    if (value_numeric != null) return unitOptions ? formatUnitDistance(value_numeric, unitOptions) : fmtDist(value_numeric)
    return '—'
  }
  if (category === 'sup_most_elevation' || category === 'sup_biggest_climb') {
    if (value_numeric != null) return unitOptions ? formatUnitElevation(value_numeric, unitOptions) : `${Math.round(value_numeric)} m`
    return '—'
  }
  if (category === 'sup_longest_duration') {
    if (value_numeric != null) return fmtDuration(value_numeric)
    return '—'
  }
  // Fallback
  if (elapsed_time_s != null) return fmtDuration(elapsed_time_s)
  if (value_numeric != null) return value_numeric.toLocaleString(unitOptions?.locale, { maximumFractionDigits: 1 })
  return '—'
}

// ── Distance lookup for speed calculation ─────────────────────────────────────

export const DIST_METERS: Record<string, number> = Object.fromEntries(
  CATEGORY_METADATA
    .filter(category => 'distanceM' in category)
    .map(category => [category.key, category.distanceM]),
)

export interface BestEffortDisplay {
  primary: string
  secondary?: string
}

/**
 * Format a best effort for display with sport-aware formatting.
 * Cycling distance categories show km/h as primary, time as secondary.
 */
export function formatBestEffortDisplay(
  category: string,
  elapsed_time_s?: number | null,
  value_numeric?: number | null,
  sportGroup?: string,
  unitOptions?: UnitFormatOptions,
): BestEffortDisplay {
  // Cycling distance: show km/h as primary, time as secondary
  if (
    sportGroup?.startsWith('cycling') &&
    category.startsWith('dist_') &&
    elapsed_time_s != null &&
    elapsed_time_s > 0
  ) {
    const distM = DIST_METERS[category]
    if (distM) {
      return {
        primary: unitOptions
          ? formatUnitSpeed(distM / elapsed_time_s, unitOptions)
          : ((distM / 1000) / (elapsed_time_s / 3600)).toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) + ' km/h',
        secondary: fmtDuration(elapsed_time_s),
      }
    }
  }

  // Default: use existing formatter
  return { primary: formatBestEffortValue(category, elapsed_time_s, value_numeric, unitOptions) }
}
