import { useQuery } from '@tanstack/react-query'
import { fetchSystemLayerKeys, type SystemLayerDescriptor } from '@/api/mapLayers'

/**
 * Fetch and cache system layer descriptors (code constants, change only on
 * backend release). Cached for 24h. No auth required — public endpoint.
 */
export function useSystemLayerKeys(): {
  isLoading: boolean
  keys: SystemLayerDescriptor[]
  byKey: Record<string, SystemLayerDescriptor>
} {
  const { data, isLoading } = useQuery({
    queryKey: ['system-layer-keys'],
    queryFn: fetchSystemLayerKeys,
    staleTime: 24 * 60 * 60_000,
  })

  return {
    isLoading,
    keys: data ?? [],
    byKey: Object.fromEntries((data ?? []).map((d) => [d.key, d])),
  }
}
