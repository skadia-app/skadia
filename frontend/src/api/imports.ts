import { apiClient } from './client'

export interface ImportLogEntry {
  id: string
  user_id: string
  source_platform: string
  filename: string
  file_hash?: string
  file_size_bytes?: number
  status: string
  activity_id?: string
  error_msg?: string
  duration_ms?: number
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface ImportLogResponse {
  entries: ImportLogEntry[]
  total: number
}

export interface ImportLogParams {
  status?: string
  source_platform?: string
  limit?: number
  offset?: number
}

export async function getImports(params: ImportLogParams = {}): Promise<ImportLogResponse> {
  const searchParams = new URLSearchParams()
  if (params.status) searchParams.set('status', params.status)
  if (params.source_platform) searchParams.set('source_platform', params.source_platform)
  if (params.limit !== undefined) searchParams.set('limit', String(params.limit))
  if (params.offset !== undefined) searchParams.set('offset', String(params.offset))

  const qs = searchParams.toString()
  return apiClient<ImportLogResponse>(`/imports${qs ? '?' + qs : ''}`)
}
