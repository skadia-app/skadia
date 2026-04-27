import { useQuery } from '@tanstack/react-query'
import { fetchMapLayers, type MapLayer, type SystemLayerVisibility } from '@/api/mapLayers'
import { useCapabilities } from './useCapabilities'

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

export function useMapLayers(): UseMapLayersResult {
  const { data: caps, isLoading: capsLoading } = useCapabilities()

  const { data, isLoading } = useQuery({
    queryKey: ['map-layers'],
    queryFn: fetchMapLayers,
    staleTime: 5 * 60_000,
  })

  const passesCap = (l: MapLayer): boolean => {
    if (!l.requires_capability) return true
    // Restrictive default while capabilities are still loading.
    if (!caps) return false
    const key = l.requires_capability as keyof typeof caps
    if (!(key in caps)) {
      // An admin set requires_capability to a key the backend never exposes.
      // Warn once per layer key per session rather than silently filtering forever.
      if (!warnedCaps.has(l.requires_capability)) {
        console.warn(
          `map_layers.${l.key}: requires_capability=${l.requires_capability} but capability is not exposed by /capabilities`,
        )
        warnedCaps.add(l.requires_capability)
      }
      return false
    }
    return caps[key] === true
  }

  const enabledLayers = (data?.layers ?? []).filter((l) => l.enabled)
  const filtered = enabledLayers.filter((l) => passesCap(l))

  // Layers that are enabled but blocked by a missing/false capability.
  // Only populated once caps have loaded — while loading, we keep the
  // restrictive default (no grayed-out entries yet) to avoid flicker.
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
