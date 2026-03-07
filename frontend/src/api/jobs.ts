import { apiClient } from './client'

export interface ActiveJob {
  id: string
  job_type: 'segment_recompute' | 'activity_recompute_all' | 'osm_import'
  entity_id?: string
  entity_name?: string
  status: 'running' | 'completed' | 'failed'
  phase?: 'download' | 'import' | 'index'
  progress_current: number
  progress_total: number
  started_at: string
  finished_at?: string
}

export interface ActiveJobsResponse {
  jobs: ActiveJob[]
  pending_enrichments: number
}

export async function fetchActiveJobs(): Promise<ActiveJobsResponse> {
  return apiClient<ActiveJobsResponse>('/jobs/active')
}
