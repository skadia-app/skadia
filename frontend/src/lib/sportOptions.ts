// Shared sport-option types and constants used by MapPage filters and activity filters.
// Generated from sportTaxonomy.ts — single source of truth.

import {
  CATEGORIES,
  SPORT_DEFS,
  PROMOTE_SUB_TYPE,
  sportColor,
  sportsInCategory,
  type SportDef,
} from '@/lib/sportTaxonomy'

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

/**
 * Sub-type labels that are too generic without a parent-sport prefix.
 * For these, the filter label becomes "Sport: SubType" (e.g. "Fußball: Aufwärmen").
 */
const GENERIC_SUBTYPES = new Set(['warm_up', 'exercise', 'match'])

/**
 * Builds the children array for a category group.
 *
 * Rules:
 * - Promoted sports (training, fitness_equipment, racket): only sub-type entries,
 *   no parent entry (sub-type IS the label).
 * - Sports with sub-types but NOT promoted: parent entry first (catches activities
 *   without any sub_type), then each sub-type entry.
 * - Generic sub-type labels (warm_up, exercise, match): prefixed with parent sport
 *   label for context (e.g. "Fußball: Aufwärmen").
 * - Sports without sub-types: plain entry.
 */
function categoryGroup(catKey: string, label: string, sports: SportDef[]): SportOption {
  const children: SportOption[] = []
  for (const s of sports) {
    if (s.subTypes.length > 0) {
      if (!PROMOTE_SUB_TYPE.has(s.sportType)) {
        // Parent entry first — catches activities stored with no sub_type
        children.push({ value: s.sportType, label: s.label })
      }
      for (const st of s.subTypes) {
        const stLabel = GENERIC_SUBTYPES.has(st.subType)
          ? `${s.label}: ${st.label}`
          : st.label
        children.push({ value: s.sportType, label: stLabel, subType: st.subType })
      }
    } else {
      children.push({ value: s.sportType, label: s.label })
    }
  }
  return { value: catKey, label, groupKey: catKey, children }
}

// ── GPS-relevant categories for map view ────────────────────────────────────────

const GPS_CATEGORIES = ['run', 'bike', 'winter', 'water', 'climb', 'outdoor', 'multi']
const NON_GPS_CATEGORIES = ['fitness', 'game', 'other']

/**
 * Sport filter options for map view.
 * Only includes GPS-relevant categories + a combined "Fitness & Sonstiges" group.
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

  // Collected non-GPS sports
  const nonGpsSports: SportDef[] = []
  for (const catKey of NON_GPS_CATEGORIES) {
    nonGpsSports.push(...sportsInCategory(catKey))
  }
  if (nonGpsSports.length > 0) {
    options.push(categoryGroup('fitness_other', 'Fitness & Sonstiges', nonGpsSports))
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

// ── Unified filter option generator ─────────────────────────────────────────

export type FilterMode = 'map' | 'activity' | 'analytics'

/**
 * Unified sport filter option generator.
 *
 * - 'map': GPS-relevant categories (run/bike/winter/water/climb/outdoor) +
 *          a combined "Indoor & Sonstiges" group for fitness + other.
 * - 'activity' | 'analytics': All 8 categories as expandable groups.
 */
export function generateFilterOptions(mode: FilterMode): SportOption[] {
  if (mode === 'map') {
    return generateMapFilterOptions()
  }
  return generateActivityFilterOptions()
}

// Backwards-compatible static options (uses map filter as default)
export const SPORT_OPTIONS: SportOption[] = generateFilterOptions('map')

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

/**
 * Filtert Sport-Optionen auf nur vorhandene Sport-Types.
 * Kategorie-Gruppen werden entfernt wenn keine ihrer Kinder vorhandene Sport-Types enthaelt.
 * Kinder werden gefiltert auf vorhandene Sport-Types.
 *
 * @param options - Alle Sport-Optionen
 * @param existingSportTypes - Set der Sport-Types die in der DB vorhanden sind
 */
export function filterToExistingSports(
  options: SportOption[],
  existingSportTypes: Set<string>,
): SportOption[] {
  const filtered: SportOption[] = []
  for (const opt of options) {
    if (opt.children) {
      const filteredChildren = opt.children.filter((child) =>
        existingSportTypes.has(child.value),
      )
      if (filteredChildren.length > 0) {
        filtered.push({ ...opt, children: filteredChildren })
      }
    } else {
      if (existingSportTypes.has(opt.value)) {
        filtered.push(opt)
      }
    }
  }
  return filtered
}
