import { useQuery } from '@tanstack/react-query'
import { getCapabilities, type Capabilities } from '@/api/routes'

/**
 * Single source of truth for capability lookups across the app.
 * staleTime 5min — capabilities only change when the admin restarts the API
 * with new env vars or PMTiles files appear/disappear.
 */
export function useCapabilities() {
  return useQuery<Capabilities>({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 5 * 60_000,
  })
}
