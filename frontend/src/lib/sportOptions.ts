// Shared sport-option types and constants used by MapPage filters and activity filters.
// Filter option trees come from the generated activity-filter projection.

import {
  SPORT_DEFS,
  sportColor,
} from '@/lib/sportTaxonomy'
import { GENERATED_ACTIVITY_FILTER_PRESETS } from '@/lib/generatedSportFilterProjections'
import { GENERATED_GEAR_DEFAULT_MANUAL_OPTIONS } from '@/lib/generatedGearDefaultProjections'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'

export interface SportOption {
  value: string
  label: string
  labels?: LocalizedString
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

function optionFromLeaf(leaf: GeneratedActivityFilterLeaf, language?: SupportedLanguage): SportOption {
  const subType = 'subType' in leaf ? leaf.subType : undefined
  return {
    value: leaf.sportType,
    label: localizeString(leaf.labels, language),
    labels: leaf.labels,
    ...(subType ? { subType } : {}),
  }
}

function optionsForPreset(presetKey: GeneratedActivityFilterPreset['key'], language?: SupportedLanguage): SportOption[] {
  const preset = GENERATED_ACTIVITY_FILTER_PRESETS.find((candidate) => candidate.key === presetKey)
  if (!preset) return []
  return preset.groups.map((group) => ({
    value: group.key,
    label: localizeString(group.labels, language),
    labels: group.labels,
    groupKey: group.key,
    children: group.leaves.map((leaf) => optionFromLeaf(leaf, language)),
  })).filter((group) => group.children.length > 0)
}

/**
 * Sport filter options for map view.
 * Only includes GPS-relevant categories + a combined "Fitness & Sonstiges" group.
 */
export function generateMapFilterOptions(language?: SupportedLanguage): SportOption[] {
  return optionsForPreset('map_panel_gps', language)
}

/**
 * Sport filter options for activity list view.
 */
export function generateActivityFilterOptions(language?: SupportedLanguage): SportOption[] {
  return optionsForPreset('activity_all', language)
}

// ── Unified filter option generator ─────────────────────────────────────────

export type FilterMode = 'map' | 'activity' | 'analytics'

/**
 * Unified sport filter option generator.
 *
 * - 'map': generated map-panel preset.
 * - 'activity' | 'analytics': generated full activity-filter preset.
 */
export function generateFilterOptions(mode: FilterMode, language?: SupportedLanguage): SportOption[] {
  if (mode === 'map') {
    return generateMapFilterOptions(language)
  }
  return generateActivityFilterOptions(language)
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
export function generateGearSportOptions(language?: SupportedLanguage): SportOption[] {
  return GENERATED_GEAR_DEFAULT_MANUAL_OPTIONS.map((option) => ({
    value: option.value,
    label: localizeString(option.labels, language),
    labels: option.labels,
  }))
}

/**
 * Filters sport options to tuples that exist in the user's data.
 * Category groups are removed when none of their children exists.
 * Children are filtered to existing sport tuples.
 *
 * @param options - All sport options
 * @param existingSportKeys - Sport keys currently present in the DB
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
