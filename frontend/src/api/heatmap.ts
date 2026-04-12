import { apiClient } from './client'

const MARTIN_BASE = import.meta.env.VITE_MARTIN_URL ?? '/api/v1/tiles'

export function getHeatmapTileUrl(params?: {
  user_id?: string
  sport_type?: string
  date_from?: string
  date_to?: string
}): string {
  const base = `${MARTIN_BASE}/heatmap_lines/{z}/{x}/{y}`
  if (!params) return base

  const qs = new URLSearchParams()
  if (params.user_id) qs.set('user_id', params.user_id)
  if (params.sport_type) qs.set('sport_type', params.sport_type)
  if (params.date_from) qs.set('date_from', params.date_from)
  if (params.date_to) qs.set('date_to', params.date_to)

  const str = qs.toString()
  return str ? `${base}?${str}` : base
}

export async function backfillHeatmap(): Promise<{ processed: number }> {
  return apiClient<{ processed: number }>('/admin/backfill-heatmap', { method: 'POST' })
}
