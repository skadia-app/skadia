import {
  GENERATED_RECORD_GROUPS,
  GENERATED_RECORD_MAIN_CATEGORIES,
  GENERATED_RECORD_MATCH_RULES,
} from './generatedRecordProjections'
import { modifierDefaultsForTuple } from './sportTaxonomy'

export const SPORT_GROUP_LABELS: Record<string, string> = Object.fromEntries(
  GENERATED_RECORD_GROUPS.map(group => [group.key, group.label]),
)

export const RECORD_GROUP_BADGES: Record<string, string> = Object.fromEntries(
  GENERATED_RECORD_GROUPS
    .filter(group => 'badge' in group)
    .map(group => [group.key, group.badge]),
)

export type MainCategory = (typeof GENERATED_RECORD_MAIN_CATEGORIES)[number]['key']

export const MAIN_CATEGORY_ORDER = GENERATED_RECORD_MAIN_CATEGORIES.map(category => category.key) as MainCategory[]

export const MAIN_CATEGORIES: Record<MainCategory, string[]> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.map(category => [category.key, [...category.recordGroups]]),
) as Record<MainCategory, string[]>

export const MAIN_CATEGORY_LABELS: Record<MainCategory, string> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.map(category => [category.key, category.label]),
) as Record<MainCategory, string>

export const MAIN_CATEGORY_COLORS: Record<MainCategory, string> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.map(category => [category.key, category.color]),
) as Record<MainCategory, string>

export interface SubFilter {
  label: string
  sportGroups: string[]
}

export const SUB_FILTERS: Record<MainCategory, SubFilter[]> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.map(category => [
    category.key,
    category.subFilters.map(filter => ({ label: filter.label, sportGroups: [...filter.recordGroups] })),
  ]),
) as Record<MainCategory, SubFilter[]>

export const SPORT_GROUP_BADGE_LABELS: Record<string, string> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.flatMap(category =>
    category.subFilters.flatMap(filter => {
      if (filter.label === 'Alle') return []
      return filter.recordGroups.map(group => [group, filter.label] as const)
    }),
  ),
)

export const POWER_CURVE_GROUPS: Partial<Record<MainCategory, string[]>> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES
    .filter(category => 'powerCurve' in category)
    .map(category => [category.key, [...category.powerCurve!.recordGroups]]),
) as Partial<Record<MainCategory, string[]>>

export const SPORT_GROUP_ORDER = GENERATED_RECORD_GROUPS.map(group => group.key)

export const CATEGORY_PREFIXES = {
  dist: 'dist_',
  elev: 'elev_',
  power: 'power_',
  sup: 'sup_',
} as const

type RecordEffort = {
  sport_type: string
  sub_type?: string
  is_electric?: boolean
  is_indoor?: boolean
  is_virtual?: boolean
}

export function matchesSportGroup(sportGroup: string, effort: RecordEffort): boolean {
  return recordGroupForEffort(effort) === sportGroup
}

function recordGroupForEffort(effort: RecordEffort): string | null {
  const subType = effort.sub_type === 'generic' ? '' : (effort.sub_type ?? '')
  const defaults = modifierDefaultsForTuple(effort.sport_type, subType)
  const isIndoor = effort.is_indoor ?? defaults.isIndoor ?? false
  const isVirtual = effort.is_virtual ?? false
  const isElectric = effort.is_electric ?? false

  for (const rule of GENERATED_RECORD_MATCH_RULES) {
    if (rule.sportType !== effort.sport_type) continue
    const subTypes = 'subTypes' in rule ? rule.subTypes as readonly string[] : null
    const excludeSubTypes = 'excludeSubTypes' in rule ? rule.excludeSubTypes as readonly string[] : null
    if (subTypes && !subTypes.includes(subType)) continue
    if (excludeSubTypes?.includes(subType)) continue
    if ('modifiers' in rule) {
      if ('isIndoor' in rule.modifiers && rule.modifiers.isIndoor !== isIndoor) continue
      if ('isVirtual' in rule.modifiers && rule.modifiers.isVirtual !== isVirtual) continue
      if ('isElectric' in rule.modifiers && rule.modifiers.isElectric !== isElectric) continue
    }
    return rule.recordGroup
  }
  return null
}
