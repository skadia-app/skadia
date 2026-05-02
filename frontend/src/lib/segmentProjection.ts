import { GENERATED_SEGMENT_PROJECTION } from './generatedSegmentProjections'
import type { SegType } from '@/components/segment-editor/constants'
import type { SportPredicate } from './sportSelection'

type SegmentProjectionEntry = (typeof GENERATED_SEGMENT_PROJECTION)['eligible'][number]

function keyFor(entry: Pick<SegmentProjectionEntry, 'sportType'> & { subType?: string }): string {
  return entry.subType ? `${entry.sportType}:${entry.subType}` : entry.sportType
}

function entrySubType(entry: SegmentProjectionEntry): string | undefined {
  const subType = (entry as { subType?: unknown }).subType
  return typeof subType === 'string' ? subType : undefined
}

export function segmentProjectionEntriesForType(segType: SegType): SegmentProjectionEntry[] {
  return GENERATED_SEGMENT_PROJECTION.eligible.filter(entry => entry.segmentType === segType)
}

export function segmentEligibleSelectionKeys(segType: SegType): string[] {
  return segmentProjectionEntriesForType(segType).map(keyFor)
}

export function segmentPredicatesForSelection(segType: SegType, keys: readonly string[] | null): SportPredicate[] {
  const selected = keys === null ? segmentEligibleSelectionKeys(segType) : keys
  return selected.map(key => {
    const [sport, subType] = key.split(':', 2)
    return subType ? { sport, sub_type: subType } : { sport }
  })
}

export function segmentSportGroupForSelection(segType: SegType, keys: readonly string[] | null): string {
  const selected = new Set(keys === null ? segmentEligibleSelectionKeys(segType) : keys)
  if (selected.size === 0) return 'generic'

  const groups = new Set<string>()
  const entries = segmentProjectionEntriesForType(segType)
  for (const key of selected) {
    const [sport, subType] = key.split(':', 2)
    const exact = entries.find(entry => entry.sportType === sport && entrySubType(entry) === subType)
    const parent = entries.find(entry => entry.sportType === sport && entrySubType(entry) == null)
    const entry = exact ?? parent
    if (!entry) return 'generic'
    groups.add(entry.sportGroup)
  }
  return groups.size === 1 ? [...groups][0] : 'generic'
}
