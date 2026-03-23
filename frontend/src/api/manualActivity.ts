import { apiClient } from './client'

export interface CreateManualActivityRequest {
  sport_type: string
  sub_type?: string
  title?: string
  description?: string
  started_at: string
  elapsed_time_s: number
  distance_m?: number
  elevation_gain_m?: number
  elevation_loss_m?: number
  tags?: string[]
  gear_id?: string
  is_ebike?: boolean
  is_virtual?: boolean
  workout_feel?: number
  workout_rpe?: number
}

export async function createManualActivity(req: CreateManualActivityRequest) {
  return apiClient<{ id: string; status: string }>('/activities/manual', {
    method: 'POST',
    body: req,
  })
}
