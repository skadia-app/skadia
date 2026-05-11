// Schema-driven lookup helpers operating on the generated GearSchema type.
// All functions accept a GearSchema instance (pass generated directly or via getActiveSchema()).

import type { GearSchema, GearType, Attribute, OptionValue } from './generatedGearSchema'
import type { GearDefaultSportPredicate } from '@/api/types'
import type { GearCategoryScope, GearTypeOption } from './gearSchemaTypes'
import { isGearCategoryScope } from './gearSchemaTypes'

// ── Path label helpers ────────────────────────────────────────────────────────

function gearTypeAncestors(schema: GearSchema, typeKey: string): GearType[] {
  const byKey = new Map(schema.gearTypes.map(gt => [gt.key, gt]))
  const path: GearType[] = []
  const seen = new Set<string>()
  for (let current = typeKey; current && !seen.has(current);) {
    seen.add(current)
    const item = byKey.get(current)
    if (!item) break
    path.push(item)
    current = item.parentKey ?? ''
  }
  return path.reverse()
}

export function gearTypePathLabel(schema: GearSchema, typeKey: string): string {
  const path = gearTypeAncestors(schema, typeKey)
  if (path.length === 0) return typeKey
  return path.map(item => item.labelDe).join(' / ')
}

const LEGACY_GEAR_TYPE_LABELS: Record<string, string> = {
  bike: 'Fahrrad',
  shoes: 'Schuhe',
  goggles: 'Brille/Goggle',
  ski: 'Ski',
  paddle: 'Paddel',
  other: 'Sonstiges',
}

export function gearTypeLabel(schema: GearSchema, typeKey: string): string {
  const path = gearTypePathLabel(schema, typeKey)
  return path !== typeKey ? path : (LEGACY_GEAR_TYPE_LABELS[typeKey] ?? typeKey)
}

export function gearTypeScope(schema: GearSchema, typeKey: string, fallbackScope?: string | null): GearCategoryScope {
  if (isGearCategoryScope(fallbackScope)) return fallbackScope
  const scope = schema.gearTypes.find(t => t.key === typeKey)?.scope
  return isGearCategoryScope(scope) ? scope : 'other'
}

export function gearMatchesCategoryScope(
  gear: { type: string; scope?: string | null },
  scope: GearCategoryScope,
  schema: GearSchema,
): boolean {
  return gearTypeScope(schema, gear.type, gear.scope) === scope
}

// ── Scope-aware type options ──────────────────────────────────────────────────

// Checks whether a gear type (by its scope field) belongs to a category scope.
// The generated GearType has a 'scope' field set by the YAML generator.
export function gearTypeMatchesScope(typeKey: string, scope: GearCategoryScope, schema: GearSchema): boolean {
  return gearTypeScope(schema, typeKey) === scope
}

function hasManualChild(schema: GearSchema, typeKey: string): boolean {
  // A parent type that has at least one selectable child should not appear itself
  // as a selectable option (user selects the leaf, not the parent).
  // The generated schema does not have a manual_selectable flag; we use the
  // heuristic that if any gear type has parentKey === typeKey it is a parent.
  return schema.gearTypes.some(gt => gt.parentKey === typeKey)
}

export function gearTypeOptionsForScope(schema: GearSchema, scope: GearCategoryScope): GearTypeOption[] {
  return schema.gearTypes
    .filter(gt => gearTypeMatchesScope(gt.key, scope, schema))
    .filter(gt => gt.defaultAssignmentRoleKey !== 'consumable')
    .filter(gt => !hasManualChild(schema, gt.key))
    .map(gt => ({ value: gt.key, label: gearTypePathLabel(schema, gt.key) }))
    .sort((a, b) => a.label.localeCompare(b.label, 'de'))
}

// ── Attribute helpers ─────────────────────────────────────────────────────────

export function gearTypeAttributes(schema: GearSchema, typeKey: string): Attribute[] {
  const attrByKey = new Map(schema.attributes.map(a => [a.key, a]))
  const seen = new Set<string>()
  const out: Attribute[] = []
  for (const ancestor of gearTypeAncestors(schema, typeKey)) {
    for (const key of ancestor.attributeKeys) {
      if (seen.has(key)) continue
      const attr = attrByKey.get(key)
      if (!attr) continue
      seen.add(key)
      out.push(attr)
    }
  }
  return out
}

// ── Option set resolution (supports future extends chain) ─────────────────────

export function optionSetOptions(schema: GearSchema, optionSetKey: string | undefined): OptionValue[] {
  if (!optionSetKey) return []
  const os = schema.optionSets.find(o => o.key === optionSetKey)
  return os?.options ?? []
}

// ── Subcategory → default sport predicates ───────────────────────────────────
//
// Maps a (typeKey, subcategoryValue) pair to a list of GearDefaultSportPredicate
// entries that make sense as defaults for that subcategory.  Callers use this to
// pre-populate the sports tab when applicable_sport_predicates is empty and the
// item already carries a subcategory attribute.
//
// Returns [] for unknown combinations so callers can fall back to a type-level
// default or leave the list empty.

type SubcategoryMap = Record<string, GearDefaultSportPredicate[]>

function predicates(...sportTypes: string[]): GearDefaultSportPredicate[] {
  return sportTypes.map(sport_type => ({ sport_type }))
}

// Indexed by typeKey prefix.  All apparel.* variants share one key ('apparel.*').
// Note: 'skiing' (sport_type) has lifecycle=broad_fallback and selectable=false in the taxonomy.
// The selectable canonical type for resort skiing is 'alpine_skiing'.
// 'backcountry_skiing' and 'cross_country_skiing' are intentionally omitted here to keep
// defaults simple; users can add them manually.
const HELMET_MAP: SubcategoryMap = {
  road_bike:    predicates('cycling'),
  mtb:          predicates('cycling'),
  full_face_dh: predicates('cycling'),
  ski:          predicates('alpine_skiing', 'snowboarding'),
  climb:        predicates('rock_climbing'),
  equestrian:   predicates('horseback_riding'),
}

const GOGGLES_MAP: SubcategoryMap = {
  ski:  predicates('alpine_skiing', 'snowboarding'),
  swim: predicates('swimming'),
  dh:   predicates('cycling'),
  mx:   predicates('motorcycling'),
  // 'sun' intentionally omitted — general-purpose, no specific sport
}

const APPAREL_MAP: SubcategoryMap = {
  cycling:  predicates('cycling'),
  running:  predicates('running'),
  hiking:   predicates('hiking'),
  // mirrors HELMET_MAP/GOGGLES_MAP: ski apparel covers alpine_skiing and snowboarding
  ski:      predicates('alpine_skiing', 'snowboarding'),
  climbing: predicates('rock_climbing'),
  swimming: predicates('swimming'),
  // 'general' intentionally omitted — no specific sport
}

export function defaultSportsForSubcategory(
  typeKey: string,
  subcategoryValue: string | undefined,
): GearDefaultSportPredicate[] {
  if (!subcategoryValue) return []

  if (typeKey === 'helmet') {
    return HELMET_MAP[subcategoryValue] ?? []
  }
  if (typeKey === 'goggles') {
    return GOGGLES_MAP[subcategoryValue] ?? []
  }
  // All apparel.* leaf types share the same subcategory option set.
  if (typeKey.startsWith('apparel.')) {
    return APPAREL_MAP[subcategoryValue] ?? []
  }

  return []
}
