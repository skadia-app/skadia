import { apiClient } from './client'

export interface PMCDataPoint {
  date: string
  ctl: number
  atl: number
  tsb: number
  tss: number
}

export async function fetchPMC(from: string, to: string): Promise<PMCDataPoint[]> {
  const params = new URLSearchParams({ from, to })
  return apiClient<PMCDataPoint[]>(`/stats/pmc?${params}`)
}

export interface SportTypeStatItem {
  sport_type: string
  sub_type?: string | null
  count: number
}

export interface SportTypeStatsRange {
  from?: string  // YYYY-MM-DD or RFC 3339
  to?: string
}

export async function fetchSportTypeStats(range?: SportTypeStatsRange): Promise<SportTypeStatItem[]> {
  const params = new URLSearchParams()
  if (range?.from) params.set('from', range.from)
  if (range?.to)   params.set('to',   range.to)
  const qs = params.toString()
  const path = qs ? `/activities/sport-type-stats?${qs}` : '/activities/sport-type-stats'
  const data = await apiClient<{ sport_types: SportTypeStatItem[] }>(path)
  return data.sport_types
}
