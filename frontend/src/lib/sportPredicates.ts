import { modifierKeysForCapability, type SportModifierKey } from './sportModifiers'

export interface SportPredicate {
  sport: string
  sub_type?: string
}

export interface ModifierFilters {
  is_electric?: boolean
  is_indoor?: boolean
  is_virtual?: boolean
  is_commute?: boolean
}

export interface ActivityModifierState {
  is_electric?: boolean
  is_indoor?: boolean
  is_virtual?: boolean
  is_commute?: boolean
}

export function predicatesMatch(
  predicates: SportPredicate[] | undefined | null,
  sportType: string,
  subType?: string | null,
): boolean {
  if (predicates == null) return true
  if (predicates.length === 0) return false

  const normalizedSubType = subType ?? ''
  return predicates.some(predicate => {
    if (predicate.sport !== sportType) return false
    if (predicate.sub_type === undefined) return true
    if (predicate.sub_type === normalizedSubType) return true
    return false
  })
}

export function modifiersMatch(
  filters: ModifierFilters | undefined | null,
  state: ActivityModifierState,
): boolean {
  if (!filters) return true
  return modifierKeysForCapability('modifier_filter')
    .every((key) => boolFilterMatches(filters[key as SportModifierKey], state[key as SportModifierKey]))
}

function boolFilterMatches(filter: boolean | undefined, state: boolean | undefined): boolean {
  return filter === undefined || filter === Boolean(state)
}
