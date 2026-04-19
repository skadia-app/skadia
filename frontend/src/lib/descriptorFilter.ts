import type { RouteSubSegment } from './routeSurfaceAggregation'

export type DescriptorKey = 'mtb_technical' | 'alpine_hiking' | 'exposed' | 'low_visibility'

// Single source of truth — all consumers (RouteComparisonPanel, useStickyHighlight
// bridge in MapPage) import F26_KEYS from here. Adding a 5th key updates all
// three consumers in one place.
export const F26_KEYS: ReadonlySet<string> = new Set<DescriptorKey>([
  'mtb_technical', 'alpine_hiking', 'exposed', 'low_visibility',
])

export function isF26Key(key: string): key is DescriptorKey {
  return F26_KEYS.has(key)
}

export function filterSubSegmentsByDescriptor(
  segments: RouteSubSegment[],
  descriptorKey: DescriptorKey,
): RouteSubSegment[] {
  switch (descriptorKey) {
    case 'mtb_technical':
      return segments.filter(s =>
        ['S2', 'S3', 'S4', 'S5'].includes(s.mtb_scale ?? '') ||
        ['S2', 'S3', 'S4', 'S5'].includes(s.mtb_scale_uphill ?? '')
      )
    case 'alpine_hiking':
      return segments.filter(s => ['T4', 'T5', 'T6'].includes(s.sac_scale ?? ''))
    case 'exposed':
      return segments.filter(s => s.highway === 'via_ferrata' || !!s.via_ferrata_scale)
    case 'low_visibility':
      return segments.filter(s =>
        ['intermediate', 'bad', 'horrible', 'no'].includes(s.trail_visibility ?? '')
      )
  }
}
