import type { SegType } from './constants'
import type { SportPredicate } from '@/lib/sportSelection'
import {
  segmentPredicatesForSelection,
  segmentSportGroupForSelection,
} from '@/lib/segmentProjection'

export interface SegmentSportSavePatch {
  allowed_predicates?: SportPredicate[] | null
}

export interface NewSegmentSportSavePatch extends SegmentSportSavePatch {
  sport_group: string
}

export function buildNewSegmentSportPatch(segType: SegType, selection: readonly string[] | null): NewSegmentSportSavePatch {
  if (segType === 'lift') {
    return { allowed_predicates: null, sport_group: 'generic' }
  }
  return {
    allowed_predicates: segmentPredicatesForSelection(segType, selection),
    sport_group: segmentSportGroupForSelection(segType, selection),
  }
}

export function buildExistingSegmentSportPatch(args: {
  segType: SegType
  initialSegType: SegType
  selection: readonly string[] | null
  initialAllowedPredicates: SportPredicate[] | null | undefined
  touched: boolean
}): SegmentSportSavePatch {
  const eligibilityTouched = args.touched || args.segType !== args.initialSegType
  if (!eligibilityTouched) {
    return {}
  }
  if (args.segType === 'lift') {
    return { allowed_predicates: null }
  }
  return {
    allowed_predicates: segmentPredicatesForSelection(args.segType, args.selection),
  }
}
