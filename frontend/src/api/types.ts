export type SportType =
  | 'cycling'
  | 'running'
  | 'hiking'
  | 'swimming'
  | 'e_biking'
  | 'skiing'
  | 'paddling'
  | 'walking'
  | 'strength'
  | 'rock_climbing'
  | 'stand_up_paddleboarding'
  | 'generic'
  | 'unknown'

export type Visibility = 'private' | 'bekannte' | 'instanz' | 'federation' | 'public'

export interface Activity {
  id: string
  user_id: string
  title: string | null
  sport_type: SportType
  sub_type: string | null
  is_ebike: boolean
  started_at: string
  elapsed_time_s: number
  distance_m: number | null
  elevation_gain_m: number | null
  elevation_loss_m: number | null
  is_indoor: boolean
  tags: string[]
  visibility: Visibility
  enrichment_status: string
  needs_classification: boolean
  has_gps: boolean
  created_at: string
  updated_at: string
}

export interface ActivitiesResponse {
  activities: Activity[]
  next_cursor: string | null
  total: number  // Gesamtzahl aller Aktivitäten des Nutzers
}
