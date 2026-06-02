import {
  GENERATED_RECORD_GROUPS,
  GENERATED_RECORD_MAIN_CATEGORIES,
  GENERATED_RECORD_MATCH_RULES,
} from './generatedRecordProjections'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'
import { modifierDefaultsForTuple } from './sportTaxonomy'

export const SPORT_GROUP_LABELS: Record<string, string> = Object.fromEntries(
  GENERATED_RECORD_GROUPS.map(group => [group.key, group.label]),
)

const SPORT_GROUP_LABEL_MAPS: Record<string, LocalizedString> = Object.fromEntries(
  GENERATED_RECORD_GROUPS.map(group => [group.key, group.labels]),
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

const MAIN_CATEGORY_LABEL_MAPS: Record<MainCategory, LocalizedString> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.map(category => [category.key, category.labels]),
) as Record<MainCategory, LocalizedString>

export const MAIN_CATEGORY_COLORS: Record<MainCategory, string> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.map(category => [category.key, category.color]),
) as Record<MainCategory, string>

export interface SubFilter {
  key: string
  label: string
  labels: LocalizedString
  sportGroups: string[]
}

export const SUB_FILTERS: Record<MainCategory, SubFilter[]> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.map(category => [
    category.key,
    category.subFilters.map(filter => ({
      key: filter.key,
      label: filter.label,
      labels: filter.labels,
      sportGroups: [...filter.recordGroups],
    })),
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

const SPORT_GROUP_BADGE_LABEL_MAPS: Record<string, LocalizedString> = Object.fromEntries(
  GENERATED_RECORD_MAIN_CATEGORIES.flatMap(category =>
    category.subFilters.flatMap(filter => {
      if (filter.label === 'Alle') return []
      return filter.recordGroups.map(group => [group, filter.labels] as const)
    }),
  ),
)

export function getSportGroupLabel(sportGroup: string, language?: SupportedLanguage): string {
  return localizeString(SPORT_GROUP_LABEL_MAPS[sportGroup], language) || sportGroup
}

export function getSportGroupBadgeLabel(sportGroup: string, language?: SupportedLanguage): string {
  return localizeString(SPORT_GROUP_BADGE_LABEL_MAPS[sportGroup], language) || getSportGroupLabel(sportGroup, language)
}

export function getMainCategoryLabel(category: MainCategory, language?: SupportedLanguage): string {
  return localizeString(MAIN_CATEGORY_LABEL_MAPS[category], language) || category
}

export function getSubFilters(category: MainCategory, language?: SupportedLanguage): SubFilter[] {
  return SUB_FILTERS[category].map(filter => ({
    ...filter,
    label: localizeString(filter.labels, language),
  }))
}

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

type RecordRuleModifiers = {
  readonly isIndoor?: boolean
  readonly isVirtual?: boolean
  readonly isElectric?: boolean
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
    const subTypes = rule.subTypes as readonly string[]
    const excludeSubTypes = rule.excludeSubTypes as readonly string[]
    const modifiers = rule.modifiers as RecordRuleModifiers
    if (subTypes.length > 0 && !subTypes.includes(subType)) continue
    if (excludeSubTypes.includes(subType)) continue
    if (modifiers.isIndoor !== undefined && modifiers.isIndoor !== isIndoor) continue
    if (modifiers.isVirtual !== undefined && modifiers.isVirtual !== isVirtual) continue
    if (modifiers.isElectric !== undefined && modifiers.isElectric !== isElectric) continue
    return rule.recordGroup
  }
  return null
}
