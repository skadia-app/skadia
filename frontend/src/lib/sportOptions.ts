// Shared sport-option types and constants used by MapPage filters and activity filters.
// Generated from sportTaxonomy.ts — single source of truth.

import { CATEGORIES, SPORT_DEFS, sportColor, sportsInCategory, type SportDef } from '@/lib/sportTaxonomy'

export interface SportOption {
  value: string
  label: string
  /** sub_type filter value (for sub-categories like cycling:mountain) */
  subType?: string
  children?: SportOption[]
  /** stable key for the group used in expand/collapse state */
  groupKey?: string
}

/** Default-Farben pro Sportart fuer den Tracks-Overlay */
export const DEFAULT_SPORT_COLORS: Record<string, string> = Object.fromEntries(
  SPORT_DEFS.map((s) => [s.sportType, sportColor(s.sportType)]),
)

/**
 * Returns the composite filter key for a leaf option.
 * Options without a subType use the plain sport value (e.g. "cycling").
 * Options with a subType use "sport:subType" (e.g. "cycling:mountain").
 */
export function getOptionKey(opt: SportOption): string {
  return opt.subType ? `${opt.value}:${opt.subType}` : opt.value
}

function defToOption(d: SportDef): SportOption {
  if (d.subTypes.length > 0) {
    return {
      value: d.sportType,
      label: d.label,
      groupKey: d.sportType,
      children: d.subTypes.map((st) => ({
        value: d.sportType,
        label: st.label,
        subType: st.subType,
      })),
    }
  }
  return { value: d.sportType, label: d.label }
}

function categoryGroup(catKey: string, label: string, sports: SportDef[]): SportOption {
  const children: SportOption[] = []
  for (const s of sports) {
    if (s.subTypes.length > 0) {
      for (const st of s.subTypes) {
        children.push({ value: s.sportType, label: st.label, subType: st.subType })
      }
    } else {
      children.push({ value: s.sportType, label: s.label })
    }
  }
  return { value: catKey, label, groupKey: catKey, children }
}

// ── GPS-relevant categories for map view ────────────────────────────────────────

const GPS_CATEGORIES = ['run', 'bike', 'winter', 'water', 'climb', 'outdoor', 'multi']
const INDOOR_CATEGORIES = ['fitness', 'game', 'other']

/**
 * Sport filter options for map view.
 * Only includes GPS-relevant categories + a combined "Indoor & Sonstiges" group.
 */
export function generateMapFilterOptions(): SportOption[] {
  const options: SportOption[] = []

  for (const catKey of GPS_CATEGORIES) {
    const cat = CATEGORIES.find((c) => c.key === catKey)
    if (!cat) continue
    const sports = sportsInCategory(catKey)
    if (sports.length === 0) continue

    if (sports.length === 1 && sports[0].subTypes.length === 0) {
      options.push({ value: sports[0].sportType, label: cat.label })
    } else {
      options.push(categoryGroup(catKey, cat.label, sports))
    }
  }

  // Collected indoor/non-GPS sports
  const indoorSports: SportDef[] = []
  for (const catKey of INDOOR_CATEGORIES) {
    indoorSports.push(...sportsInCategory(catKey))
  }
  if (indoorSports.length > 0) {
    options.push(categoryGroup('indoor_other', 'Indoor & Sonstiges', indoorSports))
  }

  return options
}

/**
 * Sport filter options for activity list view.
 * All 10 categories as expandable groups.
 */
export function generateActivityFilterOptions(): SportOption[] {
  const options: SportOption[] = []

  for (const cat of CATEGORIES) {
    const sports = sportsInCategory(cat.key)
    if (sports.length === 0) continue

    if (sports.length === 1 && sports[0].subTypes.length === 0) {
      options.push({ value: sports[0].sportType, label: cat.label })
    } else {
      options.push(categoryGroup(cat.key, cat.label, sports))
    }
  }

  return options
}

// Backwards-compatible static options (uses map filter as default)
export const SPORT_OPTIONS: SportOption[] = generateMapFilterOptions()

/**
 * Flat list of all selectable leaf keys across an option tree.
 * Used for "all selected" checks and default filter state.
 */
export function getAllSportKeys(options?: SportOption[]): string[] {
  const opts = options ?? SPORT_OPTIONS
  const keys: string[] = []
  for (const opt of opts) {
    if (opt.children) {
      for (const child of opt.children) {
        keys.push(getOptionKey(child))
      }
    } else {
      keys.push(opt.value)
    }
  }
  return keys
}

/**
 * Flat list of all sport types from the full taxonomy.
 * Useful for generating a list of all known sport type values.
 */
export function getAllSportTypes(): string[] {
  return SPORT_DEFS.map((s) => s.sportType)
}

/**
 * Generates sport options for GearForm (flat list of all sports, sorted by label).
 */
export function generateGearSportOptions(): { value: string; label: string }[] {
  return SPORT_DEFS
    .filter((s) => s.sportType !== 'unknown')
    .map((s) => ({ value: s.sportType, label: s.label }))
    .sort((a, b) => a.label.localeCompare(b.label, 'de'))
}
