import type { QueryClient } from '@tanstack/react-query'

export interface FusionInvalidationScope {
  recordingGroupId?: string | null
  activityIds?: string[]
}

// Invalidates every React-Query cache that can become stale after a fusion
// merge or revert. Centralising the key list prevents each new call site from
// missing one — e.g. the recording-group tab bar silently showing pre-fusion
// members until a window-focus refetch.
//
// Call sites: FusionPage.handleExecute (merge), ActivityHeader.revertFusion.
// Add future fusion-touching mutations here rather than inline.
export function invalidateAfterFusion(qc: QueryClient, scope: FusionInvalidationScope): void {
  qc.invalidateQueries({ queryKey: ['activities'] })
  qc.invalidateQueries({ queryKey: ['recording-groups'] })
  qc.invalidateQueries({ queryKey: ['recording-groups-badge'] })
  qc.invalidateQueries({ queryKey: ['fusion-queue'] })

  if (scope.recordingGroupId) {
    qc.invalidateQueries({ queryKey: ['recording-group', scope.recordingGroupId] })
    qc.invalidateQueries({ queryKey: ['recording-group-members', scope.recordingGroupId] })
  }

  // Per-activity keys used across the detail page. Kept explicit rather than
  // scanning prefixes: React Query has no wildcard at this level, and a typo
  // in a sibling component (e.g. 'activity-stream' vs 'activity-streams')
  // should fail loudly when a reader stops working, not silently via a prefix
  // that never matched.
  const perActivityKeys = [
    'activity',
    'activity-streams',
    'activity-sensors',
    'activity-climbs',
    'activity-stops',
    'activity-intervals',
    'activity-surfaces',
    'geo-tags',
    'best-efforts',
    'zone-distribution',
    'enrichment-status',
  ] as const

  for (const id of scope.activityIds ?? []) {
    for (const key of perActivityKeys) {
      qc.invalidateQueries({ queryKey: [key, id] })
    }
  }
}
