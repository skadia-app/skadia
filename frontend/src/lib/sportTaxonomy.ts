// Sport Taxonomy — Single Source of Truth for all sport-related display logic
// in the frontend. Must match the backend Go taxonomy exactly (same sport_type
// keys, category keys).

import { GENERATED_ACTIVITY_FILTER_PRESETS } from './generatedSportFilterProjections'
import { GENERATED_LEGACY_ALIASES, GENERATED_LEGACY_SUBTYPE_ALIASES } from './generatedSportAliases'
import { GENERATED_CATEGORIES, GENERATED_SPORT_DEFS } from './generatedSportTaxonomy'
import { GENERATED_SEGMENT_PROJECTION } from './generatedSegmentProjections'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'
import type { SportModifierTSField } from './generatedSportModifiers'

// ── Types ────────────────────────────────────────────────────────────────────────

export interface SportCategory {
  key: string
  label: string
  labels: LocalizedString
  color: string
}

export interface SportTraits {
  environments: string[]
  movementFamilies: string[]
  equipmentFamilies: string[]
  activityForm: 'single' | 'family' | 'composite' | 'transition' | 'mode' | 'fallback' | 'system'
  propulsionModes: string[]
}

interface GeneratedTraits {
  readonly environments: readonly string[]
  readonly movementFamilies: readonly string[]
  readonly equipmentFamilies: readonly string[]
  readonly activityForm: SportTraits['activityForm']
  readonly propulsionModes: readonly string[]
}

type GeneratedModifierDefaults = Partial<Record<SportModifierTSField, boolean>>

export type ModifierDefaults = Partial<Record<SportModifierTSField, boolean>>

export interface SubTypeDef {
  subType: string
  label: string
  labels: LocalizedString
  detail: string
  details: LocalizedString
  movingThreshold?: number
  selectable: boolean
  traits: SportTraits
  modifierDefaults: ModifierDefaults
}

export interface SportDef {
  sportType: string
  categoryKey: string
  segmentGroup: 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic'
  label: string
  labels: LocalizedString
  typicallyHasGPS: boolean
  isIndoor: boolean
  lifecycle: 'canonical' | 'broad_fallback' | 'deprecated'
  selectable: boolean
  traits: SportTraits
  modifierDefaults: ModifierDefaults
  subTypes: SubTypeDef[]
}

export interface SportTuple {
  sportType: string
  subType: string
  modifiers: ModifierDefaults
}

export interface SportLabelOptions {
  mode?: 'compact' | 'withParent'
  language?: SupportedLanguage
}

export interface SportOption {
  value: string
  label: string
  subType?: string
}

export interface RouteSportOption {
  sportType: string
  subType?: string
  label: string
}

// ── Categories ──────────────────────────────────────────────────────────────────

export const CATEGORIES: SportCategory[] = GENERATED_CATEGORIES.map((c) => ({
  ...c,
  labels: c.labels,
}))

const categoryMap = new Map<string, SportCategory>(CATEGORIES.map((c) => [c.key, c]))
const UNKNOWN_CATEGORY: SportCategory = {
  key: 'other',
  label: 'Sonstiges',
  labels: {
    de: 'Sonstiges',
    en: 'Other',
    fr: 'Autre',
    it: 'Altro',
    es: 'Otros',
  },
  color: '#6B7280',
}

// ── Sport Definitions ───────────────────────────────────────────────────────────

const generatedSportDefs = new Map<string, (typeof GENERATED_SPORT_DEFS)[number]>(GENERATED_SPORT_DEFS.map((s) => [s.sportType, s]))
const projectedSportOrder = sportOrderFromActivityFilterProjection()
const projectedSportOrderSet = new Set<string>(projectedSportOrder)

export const SPORT_DEFS: SportDef[] = [
  ...projectedSportOrder.map((sportType) => generatedSportDefs.get(sportType)).filter((s): s is (typeof GENERATED_SPORT_DEFS)[number] => Boolean(s)),
  ...GENERATED_SPORT_DEFS.filter((s) => !projectedSportOrderSet.has(s.sportType)),
].map((s) => ({
  sportType: s.sportType,
  categoryKey: s.categoryKey,
  segmentGroup: s.segmentGroup,
  label: s.label,
  labels: s.labels,
  typicallyHasGPS: s.typicallyHasGPS,
  isIndoor: s.isIndoor,
  lifecycle: s.lifecycle,
  selectable: s.selectable,
  traits: copyTraits(s.traits),
  modifierDefaults: copyModifierDefaults('modifierDefaults' in s ? s.modifierDefaults : undefined),
  subTypes: s.subTypes.map((st) => ({
    ...st,
    labels: st.labels,
    details: st.details,
    traits: copyTraits(st.traits),
    modifierDefaults: copyModifierDefaults('modifierDefaults' in st ? st.modifierDefaults : undefined),
  })),
}))

function copyTraits(traits: GeneratedTraits): SportTraits {
  return {
    environments: [...traits.environments],
    movementFamilies: [...traits.movementFamilies],
    equipmentFamilies: [...traits.equipmentFamilies],
    activityForm: traits.activityForm,
    propulsionModes: [...traits.propulsionModes],
  }
}

function copyModifierDefaults(defaults: GeneratedModifierDefaults | undefined): ModifierDefaults {
  return defaults ? { ...defaults } : {}
}

function sportOrderFromActivityFilterProjection(): string[] {
  const preset = GENERATED_ACTIVITY_FILTER_PRESETS.find((candidate) => candidate.key === 'activity_all')
  const order: string[] = []
  const seen = new Set<string>()
  for (const group of preset?.groups ?? []) {
    for (const leaf of group.leaves) {
      if (seen.has(leaf.sportType)) continue
      seen.add(leaf.sportType)
      order.push(leaf.sportType)
    }
  }
  return order
}

type GeneratedLegacyAlias = (typeof GENERATED_LEGACY_ALIASES)[number] | (typeof GENERATED_LEGACY_SUBTYPE_ALIASES)[number]

function tupleFromGeneratedAlias(alias: GeneratedLegacyAlias): SportTuple {
  return {
    sportType: alias.sportType,
    subType: 'subType' in alias ? alias.subType : '',
    modifiers: 'modifiers' in alias ? { ...alias.modifiers } : {},
  }
}

const legacyAliasMap = new Map<string, SportTuple>(
  GENERATED_LEGACY_ALIASES.map((alias) => [alias.raw, tupleFromGeneratedAlias(alias)])
)

const legacySubtypeAliasMap = new Map<string, SportTuple>(
  GENERATED_LEGACY_SUBTYPE_ALIASES.map((alias) => [tupleKey(alias.raw, alias.rawSub), tupleFromGeneratedAlias(alias)])
)

function tupleKey(sportType: string, subType: string): string {
  return `${sportType}\u0000${subType}`
}

// ── Lookup Maps ─────────────────────────────────────────────────────────────────

const sportMap = new Map<string, SportDef>()
const catSportsMap = new Map<string, SportDef[]>()

for (const s of SPORT_DEFS) {
  sportMap.set(s.sportType, s)
  const list = catSportsMap.get(s.categoryKey)
  if (list) {
    list.push(s)
  } else {
    catSportsMap.set(s.categoryKey, [s])
  }
}

// ── Sub-type promotion set ──────────────────────────────────────────────────────

export const PROMOTE_SUB_TYPE = new Set(['training', 'fitness_equipment', 'racket'])

// ── Fallback humanizer ──────────────────────────────────────────────────────────

function humanize(raw: string): string {
  return raw
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

// ── Lookup Functions ────────────────────────────────────────────────────────────

export function normalizeSportTuple(sportType: string, subType: string | null = null): SportTuple {
  const trimmedSport = sportType.trim()
  const trimmedSub = subType?.trim() ?? ''
  const subtypeAlias = legacySubtypeAliasMap.get(tupleKey(trimmedSport, trimmedSub))
  if (subtypeAlias) {
    return {
      sportType: subtypeAlias.sportType,
      subType: subtypeAlias.subType,
      modifiers: { ...modifierDefaultsForTuple(subtypeAlias.sportType, subtypeAlias.subType), ...subtypeAlias.modifiers },
    }
  }
  const alias = legacyAliasMap.get(trimmedSport)
  const tuple: SportTuple = alias
    ? {
        sportType: alias.sportType,
        subType: alias.subType || (trimmedSub && trimmedSub !== 'generic' ? trimmedSub : ''),
        modifiers: { ...alias.modifiers },
      }
    : {
        sportType: trimmedSport,
        subType: trimmedSub,
        modifiers: {},
      }

  if (tuple.subType === 'generic') {
    tuple.subType = ''
  }
  tuple.modifiers = { ...modifierDefaultsForTuple(tuple.sportType, tuple.subType), ...tuple.modifiers }

  return tuple
}

export function modifierDefaultsForTuple(sportType: string, subType = ''): ModifierDefaults {
  const sport = sportMap.get(sportType.trim())
  if (!sport) return {}

  const defaults: ModifierDefaults = { ...sport.modifierDefaults }
  const cleanSubType = subType.trim()
  if (cleanSubType && cleanSubType !== 'generic') {
    const subtype = sport.subTypes.find((st) => st.subType === cleanSubType)
    if (!subtype) return defaults
    Object.assign(defaults, subtype.modifierDefaults)
  }
  return defaults
}

/**
 * Get sport definition. Returns undefined for unknown sport types.
 */
export function getSportDef(sportType: string): SportDef | undefined {
  return sportMap.get(sportType.trim())
}

/**
 * Get the segment group for a sport type. Returns 'generic' for unknown types.
 */
export function getSegmentGroup(sportType: string): 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic' {
  return getSportDef(sportType)?.segmentGroup ?? 'generic'
}

/**
 * Display label with sub-type promotion for training, fitness_equipment,
 * and racket sport types.
 */
export function sportLabel(sportType: string, subType?: string | null, opts?: SportLabelOptions): string {
  const trimmedSport = sportType.trim()
  if (legacyAliasMap.has(trimmedSport)) {
    return opts?.language === 'en' ? `Legacy sport (${humanize(trimmedSport)})` : `Veraltete Sportart (${humanize(trimmedSport)})`
  }
  const tuple = { sportType: trimmedSport, subType: subType?.trim() ?? '' }
  const d = sportMap.get(tuple.sportType)
  if (!d) return humanize(sportType)

  if (tuple.subType && tuple.subType !== 'generic') {
    const st = d.subTypes.find((s) => s.subType === tuple.subType)
    if (st) {
      if (opts?.mode === 'withParent' && !subTypeLabelCanStandAlone(d)) {
        return `${localizeString(d.labels, opts?.language)} (${localizeString(st.labels, opts?.language)})`
      }
      return localizeString(st.labels, opts?.language)
    }
  }

  return localizeString(d.labels, opts?.language)
}

/**
 * Full detail label including both sport and sub-type when applicable.
 */
export function sportDetailLabel(sportType: string, subType?: string | null, language?: SupportedLanguage): string {
  return sportLabel(sportType, subType, { mode: 'withParent', language })
}

function subTypeLabelCanStandAlone(d: SportDef): boolean {
  return d.sportType === 'training' || d.sportType === 'fitness_equipment' || d.sportType === 'racket'
}

/**
 * Category color for a sport type. Returns gray (#6B7280) for unknown types.
 */
export function sportColor(sportType: string): string {
  const d = getSportDef(sportType)
  if (!d) return '#6B7280'
  return categoryMap.get(d.categoryKey)?.color ?? '#6B7280'
}

/**
 * Get category for a sport type. Defaults to the 'other' category for unknowns.
 */
export function categoryForSport(sportType: string): SportCategory {
  const d = getSportDef(sportType)
  const cat = d ? categoryMap.get(d.categoryKey) : undefined
  return cat ?? UNKNOWN_CATEGORY
}

/**
 * Get all sport definitions in a category.
 */
export function sportsInCategory(categoryKey: string): SportDef[] {
  return catSportsMap.get(categoryKey) ?? []
}

export function canonicalSportOptions(language?: SupportedLanguage): SportOption[] {
  return SPORT_DEFS.map((s) => ({ value: s.sportType, label: localizeString(s.labels, language) }))
}

export function selectableParentSportOptions(language?: SupportedLanguage): SportOption[] {
  return SPORT_DEFS
    .filter((s) => s.selectable && s.sportType !== 'unknown')
    .map((s) => ({ value: s.sportType, label: localizeString(s.labels, language) }))
}

export function selectableSportTupleOptions(language?: SupportedLanguage): SportOption[] {
  const options: SportOption[] = []
  for (const s of SPORT_DEFS) {
    if (s.selectable && s.sportType !== 'unknown') {
      options.push({ value: s.sportType, label: localizeString(s.labels, language) })
    }
    for (const st of s.subTypes) {
      if (st.selectable) {
        options.push({ value: s.sportType, subType: st.subType, label: sportLabel(s.sportType, st.subType, { language }) })
      }
    }
  }
  return options
}

export function manualActivitySportOptions(): SportDef[] {
  return SPORT_DEFS.filter((s) => s.sportType !== 'unknown' && (s.selectable || s.subTypes.some((st) => st.selectable)))
}

export function routeSportOptions(language?: SupportedLanguage): RouteSportOption[] {
  return GENERATED_SEGMENT_PROJECTION.routeOptions
    .map((option) => ({
      sportType: option.sportType,
      subType: 'subType' in option ? option.subType : undefined,
      label: sportLabel(option.sportType, 'subType' in option ? option.subType : undefined, { language }),
    }))
}
