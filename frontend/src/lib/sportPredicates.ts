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
  return boolFilterMatches(filters.is_electric, state.is_electric) &&
    boolFilterMatches(filters.is_indoor, state.is_indoor) &&
    boolFilterMatches(filters.is_virtual, state.is_virtual) &&
    boolFilterMatches(filters.is_commute, state.is_commute)
}

function boolFilterMatches(filter: boolean | undefined, state: boolean | undefined): boolean {
  return filter === undefined || filter === Boolean(state)
}
