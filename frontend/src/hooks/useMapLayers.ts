import { useQuery } from '@tanstack/react-query'
import { fetchMapLayers, type MapLayer, type SystemLayerVisibility } from '@/api/mapLayers'
import { useCapabilities } from './useCapabilities'
import { _setBaseStylesShim } from '@/lib/mapStyles'

// Module-level set so capability-warning fires once per session per missing key.
const warnedCaps = new Set<string>()

export interface UseMapLayersResult {
  isLoading: boolean
  /** role='base', enabled=true, capability-passing layers */
  base: MapLayer[]
  /** role='overlay', enabled=true, capability-passing layers */
  overlay: MapLayer[]
  /** Unfiltered list — admin views need this to show grayed-out entries. */
  raw: MapLayer[]
  systemVisibility: Record<string, SystemLayerVisibility>
  capabilitiesLoading: boolean
}

export function useMapLayers(): UseMapLayersResult {
  const { data: caps, isLoading: capsLoading } = useCapabilities()

  const { data, isLoading } = useQuery({
    queryKey: ['map-layers'],
    queryFn: async () => {
      const res = await fetchMapLayers()
      // Sync deprecation-shim ONCE per fetch (not in useEffect — that would
      // fan out across N consumers and create stale-closure windows during
      // Phase 1.5 mixed-code rendering). Removed entirely in Task 37.
      _setBaseStylesShim(res.layers)
      return res
    },
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

  const filtered = (data?.layers ?? []).filter((l) => l.enabled && passesCap(l))

  return {
    isLoading: isLoading || capsLoading,
    base: filtered.filter((l) => l.role === 'base'),
    overlay: filtered.filter((l) => l.role === 'overlay'),
    raw: data?.layers ?? [],
    systemVisibility: data?.system_visibility ?? {},
    capabilitiesLoading: capsLoading,
  }
}
