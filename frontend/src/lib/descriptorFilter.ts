import type { RouteSubSegment } from './routeSurfaceAggregation'

export type DescriptorKey =
  | 'mtb_technical' | 'alpine_hiking' | 'exposed' | 'low_visibility'
  | 'high_forest' | 'high_protected'

// Single source of truth — all consumers (RouteComparisonPanel, useStickyHighlight
// bridge in MapPage) import INTERACTIVE_DESCRIPTOR_KEYS from here.
export const INTERACTIVE_DESCRIPTOR_KEYS: ReadonlySet<string> = new Set<DescriptorKey>([
  'mtb_technical', 'alpine_hiking', 'exposed', 'low_visibility',
  'high_forest', 'high_protected',
])

export function isInteractiveDescriptorKey(key: string): key is DescriptorKey {
  return INTERACTIVE_DESCRIPTOR_KEYS.has(key)
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
    case 'high_forest':
    case 'high_protected':
      return []  // No SubSegment-Pfad — Ranges kommen aus dem Backend
  }
}
