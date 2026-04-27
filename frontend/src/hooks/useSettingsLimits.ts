import { useQuery } from '@tanstack/react-query'
import { getSettingsLimits, type SettingsLimits } from '@/api/settings'

const FALLBACK_LIMITS: SettingsLimits = {
  user_map_layer_limit: 50,
  user_map_layer_rate_limit_per_10min: 60,
}

/**
 * Fetch and cache user-facing system limits from the public
 * /api/v1/settings/limits endpoint. Cached for 60s. Falls back to
 * hardcoded defaults when the request fails so the UI stays functional.
 */
export function useSettingsLimits(): {
  isLoading: boolean
  limits: SettingsLimits
} {
  const { data, isLoading } = useQuery({
    queryKey: ['settings-limits'],
    queryFn: getSettingsLimits,
    staleTime: 60_000,
    refetchOnMount: false,
  })

  return {
    isLoading,
    limits: data ?? FALLBACK_LIMITS,
  }
}
