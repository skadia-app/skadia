export type SegmentSportSelection = string[] | null

export function removeSegmentSportSelectionKeys(
  current: SegmentSportSelection,
  keys: readonly string[],
): SegmentSportSelection {
  if (current === null) return null
  const remove = new Set(keys)
  return current.filter(key => !remove.has(key))
}

export function filterSegmentSportSelection(
  current: SegmentSportSelection,
  whitelist: readonly string[],
): SegmentSportSelection {
  if (current === null) return null
  const allowed = new Set(whitelist)
  return current.filter(key => allowed.has(key))
}
