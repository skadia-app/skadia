import { useQuery } from '@tanstack/react-query'
import { getImports, type ImportLogResponse } from '@/api/imports'
import { useCurrentUserId } from '@/providers/UserProvider'

export function useImportQueue() {
  // 'queued' is intentionally excluded — it's a terminal status for files
  // awaiting manual review in import_queue, not active import work.
  // user_id in the query key — defense-in-depth against cross-user cache hits
  // when the active user changes (e.g. logout/login or multi-tab session).
  const userId = useCurrentUserId()
  return useQuery<ImportLogResponse>({
    queryKey: ['imports', 'queue', userId],
    queryFn: () => getImports({ status: 'pending,processing', limit: 50 }),
    refetchInterval: 5000,
    staleTime: 0,
  })
}

export function useImportHistory(params: {
  source_platform?: string
  import_type?: string
  status?: string
  limit?: number
  offset?: number
} = {}) {
  const userId = useCurrentUserId()
  return useQuery<ImportLogResponse>({
    queryKey: ['imports', 'history', userId, params],
    queryFn: () =>
      getImports({
        ...params,
        limit: params.limit ?? 50,
      }),
    staleTime: 10000,
  })
}
