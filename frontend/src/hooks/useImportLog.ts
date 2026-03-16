import { useQuery } from '@tanstack/react-query'
import { getImports, type ImportLogResponse } from '@/api/imports'

export function useImportQueue() {
  return useQuery<ImportLogResponse>({
    queryKey: ['imports', 'queue'],
    queryFn: () => getImports({ status: 'pending,processing', limit: 50 }),
    refetchInterval: 5000,
    staleTime: 0,
  })
}

export function useImportHistory(params: { source_platform?: string; limit?: number; offset?: number } = {}) {
  return useQuery<ImportLogResponse>({
    queryKey: ['imports', 'history', params],
    queryFn: () => getImports({ ...params, limit: params.limit ?? 50 }),
    staleTime: 10000,
  })
}
