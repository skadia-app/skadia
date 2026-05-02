// Shared sport-option types and constants used by MapPage filters and activity filters.
// Filter option trees come from the generated activity-filter projection.

import {
  SPORT_DEFS,
  sportColor,
  selectableParentSportOptions,
} from '@/lib/sportTaxonomy'
import { GENERATED_ACTIVITY_FILTER_PRESETS } from '@/lib/generatedSportFilterProjections'

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

type GeneratedActivityFilterPreset = (typeof GENERATED_ACTIVITY_FILTER_PRESETS)[number]
type GeneratedActivityFilterGroup = GeneratedActivityFilterPreset['groups'][number]
type GeneratedActivityFilterLeaf = GeneratedActivityFilterGroup['leaves'][number]

function optionFromLeaf(leaf: GeneratedActivityFilterLeaf): SportOption {
  return {
    value: leaf.sportType,
    label: leaf.label,
    ...(leaf.subType ? { subType: leaf.subType } : {}),
  }
}

function optionsForPreset(presetKey: GeneratedActivityFilterPreset['key']): SportOption[] {
  const preset = GENERATED_ACTIVITY_FILTER_PRESETS.find((candidate) => candidate.key === presetKey)
  if (!preset) return []
  return preset.groups.map((group) => ({
    value: group.key,
    label: group.label,
    groupKey: group.key,
    children: group.leaves.map(optionFromLeaf),
  }))
}

/**
 * Sport filter options for map view.
 * Only includes GPS-relevant categories + a combined "Fitness & Sonstiges" group.
 */
export function generateMapFilterOptions(): SportOption[] {
  return optionsForPreset('map_panel_gps')
}

/**
 * Sport filter options for activity list view.
 * All 10 categories as expandable groups.
 */
export function generateActivityFilterOptions(): SportOption[] {
  return optionsForPreset('activity_all')
}

// ── Unified filter option generator ─────────────────────────────────────────

export type FilterMode = 'map' | 'activity' | 'analytics'

/**
 * Unified sport filter option generator.
 *
 * - 'map': generated map-panel preset.
 * - 'activity' | 'analytics': generated full activity-filter preset.
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
  return selectableParentSportOptions()
    .filter((s) => s.value !== 'unknown')
    .sort((a, b) => a.label.localeCompare(b.label, 'de'))
}

/**
 * Filtert Sport-Optionen auf nur vorhandene Sport-Tuples.
 * Kategorie-Gruppen werden entfernt wenn keine ihrer Kinder vorhandene Sport-Tuples enthaelt.
 * Kinder werden gefiltert auf vorhandene Sport-Tuples.
 *
 * @param options - Alle Sport-Optionen
 * @param existingSportKeys - Set der Sport-Keys die in der DB vorhanden sind
 */
export function filterToExistingSports(
  options: SportOption[],
  existingSportKeys: Set<string>,
): SportOption[] {
  const filtered: SportOption[] = []
  for (const opt of options) {
    if (opt.children) {
      const filteredChildren = opt.children.filter((child) =>
        existingSportKeys.has(getOptionKey(child)),
      )
      if (filteredChildren.length > 0) {
        filtered.push({ ...opt, children: filteredChildren })
      }
    } else {
      if (existingSportKeys.has(getOptionKey(opt))) {
        filtered.push(opt)
      }
    }
  }
  return filtered
}
