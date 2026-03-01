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
