import { useQuery } from '@tanstack/react-query'
import {
  fetchAdminMapLayers,
  fetchMapLayers,
  fetchUserMapLayers,
  type MapLayer,
  type SystemLayerVisibility,
  type UserMapLayer,
} from '@/api/mapLayers'
import { useCapabilities } from './useCapabilities'
import { useCurrentUserId } from '@/providers/UserProvider'
import { featureAvailable, validateCapabilityKey } from '@/lib/deploymentCapabilities'

// Module-level set so capability-warning fires once per session per missing key.
const warnedCaps = new Set<string>()

/** A layer that is enabled but blocked by a missing capability. */
export interface DisabledLayer {
  layer: MapLayer
  missingCapability: string
}

export interface UseMapLayersResult {
  isLoading: boolean
  /** role='base', enabled=true, capability-passing layers */
  base: MapLayer[]
  /** role='overlay', enabled=true, capability-passing layers */
  overlay: MapLayer[]
  /**
   * Enabled layers blocked by a missing capability — shown grayed-out in the
   * picker so users know the feature exists but requires a capability toggle.
   */
  disabled: DisabledLayer[]
  /** Unfiltered list — admin views need this to show grayed-out entries. */
  raw: MapLayer[]
  systemVisibility: Record<string, SystemLayerVisibility>
  capabilitiesLoading: boolean
}

interface MapLayersQueryResult {
  layers: MapLayer[]
  system_visibility: Record<string, SystemLayerVisibility>
}

function buildResult(
  data: MapLayersQueryResult | undefined,
  isLoading: boolean,
  caps: ReturnType<typeof useCapabilities>['data'],
  capsLoading: boolean,
): UseMapLayersResult {
  const passesCap = (l: MapLayer): boolean => {
    if (!l.requires_capability) return true
    if (!caps) return false
    if (!validateCapabilityKey(l.requires_capability)) {
      if (!warnedCaps.has(l.requires_capability)) {
        console.warn(
          `map_layers.${l.key}: requires_capability=${l.requires_capability} but capability is not exposed by /capabilities`,
        )
        warnedCaps.add(l.requires_capability)
      }
      return false
    }
    return featureAvailable(caps, l.requires_capability)
  }

  const enabledLayers = (data?.layers ?? []).filter((l) => l.enabled)
  const filtered = enabledLayers.filter((l) => passesCap(l))

  const disabled: DisabledLayer[] = caps
    ? enabledLayers
        .filter((l) => !passesCap(l) && !!l.requires_capability)
        .map((l) => ({ layer: l, missingCapability: l.requires_capability! }))
    : []

  return {
    isLoading: isLoading || capsLoading,
    base: filtered.filter((l) => l.role === 'base'),
    overlay: filtered.filter((l) => l.role === 'overlay'),
    disabled,
    raw: data?.layers ?? [],
    systemVisibility: data?.system_visibility ?? {},
    capabilitiesLoading: capsLoading,
  }
}

export function useMapLayers(): UseMapLayersResult {
  const { data: caps, isLoading: capsLoading } = useCapabilities()

  const { data, isLoading } = useQuery({
    queryKey: ['map-layers'],
    queryFn: fetchMapLayers,
    staleTime: 5 * 60_000,
  })

  return buildResult(data, isLoading, caps, capsLoading)
}

// ── User-layer merging helpers ────────────────────────────────────────────────

/**
 * Maps a UserMapLayer to a MapLayer-compatible object so it can flow through
 * the same rendering pipeline as system layers.
 *
 * Keys follow the convention:
 *   base    → `user-base-${id}`
 *   overlay → `user-overlay-${id}`
 */
function userLayerToMapLayer(ul: UserMapLayer): MapLayer {
  return {
    id: ul.id,
    key: ul.role === 'base' ? `user-base-${ul.id}` : `user-overlay-${ul.id}`,
    role: ul.role,
    name: ul.name,
    source_kind: ul.source_kind,
    url_template: ul.url_template,
    style_url: ul.style_url,
    attribution: ul.attribution,
    min_zoom: ul.min_zoom,
    max_zoom: ul.max_zoom,
    default_opacity: ul.default_opacity,
    is_system: false,
    enabled: ul.enabled,
    sort_order: ul.sort_order,
    proxy_through_backend: false,
    // User layers have no capability gate.
    requires_capability: undefined,
    created_at: ul.created_at,
    updated_at: ul.updated_at,
  }
}

export interface UseMapLayersMergedResult extends UseMapLayersResult {
  /** System-only base layers (no user layers). Use in admin/non-MapPage contexts. */
  systemBase: MapLayer[]
  /** System-only overlay layers (no user layers). Use in admin/non-MapPage contexts. */
  systemOverlay: MapLayer[]
}

/**
 * Extends useMapLayers() with user-owned custom layers merged into `base` and
 * `overlay`. Sort order: system layers first (by sort_order), user layers after
 * (by their own sort_order). The original `systemBase` / `systemOverlay` fields
 * expose the system-only lists for consumers that need them.
 *
 * Only raster_tiles user overlays are surfaced in `overlay` — vector_style user
 * layers with role='overlay' are excluded (MapLibre style-composition is out of
 * scope). vector_style user base layers ARE included (they switch the whole style).
 */
export function useMapLayersMerged(): UseMapLayersMergedResult {
  const systemResult = useMapLayers()
  const userId = useCurrentUserId()

  const { data: userLayersRaw } = useQuery<UserMapLayer[] | null, Error, UserMapLayer[]>({
    queryKey: ['user-map-layers', userId],
    queryFn: fetchUserMapLayers,
    staleTime: 60_000,
    select: (d) => d ?? [],
  })

  const userLayers = userLayersRaw ?? []

  // Only include enabled user layers; for overlays restrict to raster_tiles.
  const userBase: MapLayer[] = userLayers
    .filter((ul) => ul.enabled && ul.role === 'base')
    .map(userLayerToMapLayer)
    .sort((a, b) => a.sort_order - b.sort_order)

  const userOverlay: MapLayer[] = userLayers
    .filter((ul) => ul.enabled && ul.role === 'overlay' && ul.source_kind === 'raster_tiles')
    .map(userLayerToMapLayer)
    .sort((a, b) => a.sort_order - b.sort_order)

  return {
    ...systemResult,
    base: [...systemResult.base, ...userBase],
    overlay: [...systemResult.overlay, ...userOverlay],
    systemBase: systemResult.base,
    systemOverlay: systemResult.overlay,
  }
}

/**
 * Admin variant of useMapLayers — calls GET /api/v1/admin/map-layers, which
 * includes layers with enabled=false. Use this in admin UIs that need to show
 * (and re-enable) disabled layers; the public hook would filter them out at the
 * backend before they ever reach the client.
 */
export function useAdminMapLayers(): UseMapLayersResult {
  const { data: caps, isLoading: capsLoading } = useCapabilities()

  const { data, isLoading } = useQuery({
    queryKey: ['admin-map-layers'],
    queryFn: fetchAdminMapLayers,
    staleTime: 5 * 60_000,
  })

  return buildResult(data, isLoading, caps, capsLoading)
}
