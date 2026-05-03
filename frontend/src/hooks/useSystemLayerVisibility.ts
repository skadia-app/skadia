import { useCallback } from 'react'
import { useMapLayers } from './useMapLayers'

export interface SystemLayerState {
  enabled: boolean
  defaultEnabledForNewUsers: boolean
  defaultOpacity: number | null
  label: string | null
}

/**
 * Returns a per-key visibility lookup for system overlay layers.
 *
 * Falls back to `enabled=true` with null label/opacity when no admin-configured
 * DB row exists for a key — i.e. a system layer with no admin override is on
 * by default.
 *
 * `visibility` is memoized over `systemVisibility` so consumers that put it in
 * a useEffect/useCallback dep array (e.g. useCoverageOverlays' `adminEnabled`)
 * don't re-fire on every render. Without the memoization a coverage toggle
 * triggers a re-render → new `visibility` ref → `adminEnabled` ref churn →
 * the coverage source-management effect tears down and re-adds every coverage
 * layer at `addLayer(id, anchor)`, which ends up immediately before the
 * vector anchor and visually above any user overlays the user has positioned
 * on top of coverage.
 */
export function useSystemLayerVisibility(): {
  isLoading: boolean
  visibility: (key: string) => SystemLayerState
} {
  const { systemVisibility, isLoading } = useMapLayers()

  const visibility = useCallback((key: string): SystemLayerState => {
    const row = systemVisibility[key]
    if (!row) {
      return {
        enabled: true,
        defaultEnabledForNewUsers: false,
        defaultOpacity: null,
        label: null,
      }
    }
    return {
      enabled: row.enabled,
      defaultEnabledForNewUsers: row.default_enabled_for_new_users,
      defaultOpacity: row.default_opacity ?? null,
      label: row.label_override ?? null,
    }
  }, [systemVisibility])

  return { isLoading, visibility }
}
