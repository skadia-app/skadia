export interface SplitSportSelection {
  sportTypes: string[]
  subTypes: string[]
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
