import { useQuery } from '@tanstack/react-query'
import { getImports, type ImportLogResponse } from '@/api/imports'
import { useCurrentUserId } from '@/providers/UserProvider'

export function useImportQueue() {
  // user_id in the query key — defense-in-depth against cross-user cache hits.
  const userId = useCurrentUserId()
  return useQuery<ImportLogResponse>({
    queryKey: ['imports', 'queue', userId],
    queryFn: () => getImports({ status: 'pending,processing', limit: 50 }),
    refetchInterval: 5000,
    staleTime: 0,
  })
}

export function useImportHistory(params: { source_platform?: string; limit?: number; offset?: number } = {}) {
  const userId = useCurrentUserId()
  return useQuery<ImportLogResponse>({
    queryKey: ['imports', 'history', userId, params],
    queryFn: () => getImports({ ...params, limit: params.limit ?? 50 }),
    staleTime: 10000,
  })
}
