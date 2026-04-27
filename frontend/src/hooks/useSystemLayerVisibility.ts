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
 */
export function useSystemLayerVisibility(): {
  isLoading: boolean
  visibility: (key: string) => SystemLayerState
} {
  const { systemVisibility, isLoading } = useMapLayers()

  return {
    isLoading,
    visibility: (key: string): SystemLayerState => {
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
    },
  }
}
