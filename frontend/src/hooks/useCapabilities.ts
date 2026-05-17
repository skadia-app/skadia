import { useQuery } from '@tanstack/react-query'
import { getCapabilities, type Capabilities } from '@/api/routes'

export const CAPABILITIES_QUERY_KEY = ['capabilities'] as const
export const CAPABILITIES_STALE_TIME_MS = 30_000

/**
 * Single source of truth for capability lookups across the app.
 * Keep this short enough for artifact availability changes to settle without a
 * page reload after admins add/remove local data files.
 */
export function useCapabilities() {
  return useQuery<Capabilities>({
    queryKey: CAPABILITIES_QUERY_KEY,
    queryFn: getCapabilities,
    staleTime: CAPABILITIES_STALE_TIME_MS,
  })
}
