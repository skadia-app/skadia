export interface SplitSportSelection {
  sportTypes: string[]
  subTypes: string[]
}

export interface SportPredicate {
  sport: string
  sub_type?: string
}

export function sportPredicatesFromSelectionKeys(keys: Iterable<string>): SportPredicate[] {
  const predicates: SportPredicate[] = []
  const seen = new Set<string>()

  for (const key of keys) {
    const colonIdx = key.indexOf(':')
    const sport = colonIdx === -1 ? key : key.slice(0, colonIdx)
    const subType = colonIdx === -1 ? '' : key.slice(colonIdx + 1)
    if (!sport) continue

    const dedupeKey = `${sport}:${subType}`
    if (seen.has(dedupeKey)) continue
    seen.add(dedupeKey)

    const predicate: SportPredicate = { sport }
    if (subType) predicate.sub_type = subType
    predicates.push(predicate)
  }

  return predicates
}

export function splitSportSelectionKeys(keys: Iterable<string>): SplitSportSelection {
  const sportTypes = new Set<string>()
  const bareSports = new Set<string>()
  const subTypes = new Set<string>()

  for (const key of keys) {
    const colonIdx = key.indexOf(':')
    if (colonIdx === -1) {
      sportTypes.add(key)
      bareSports.add(key)
      continue
    }
    const sport = key.slice(0, colonIdx)
    const sub = key.slice(colonIdx + 1)
    sportTypes.add(sport)
    subTypes.add(sub)
  }

  return {
    sportTypes: [...sportTypes],
    subTypes: bareSports.size === 0 ? [...subTypes] : [],
  }
}
