import { apiClient } from './client'
import type { ActivityPhoto } from './activityPhotos'

export interface PhotoFeedActivitySummary {
  id: string
  title: string
  sport_type: string
  started_at?: string
  distance_m?: number
  moving_time_s?: number
  elevation_gain_m?: number
  track_geojson?: string   // GeoJSON LineString als String (JSON.parse nötig)
  lat?: number
  lng?: number
  timezone?: string | null
}

export interface PhotoFeedGroup {
  group_key: string
  group_type: 'activity' | 'day'
  activity?: PhotoFeedActivitySummary
  photos: ActivityPhoto[]
}

export interface PhotoFeedPage {
  groups: PhotoFeedGroup[]
  next_page?: number
  total: number
}

export interface PhotoSearchResult {
  photo: ActivityPhoto
  activity?: PhotoFeedActivitySummary
  similarity: number
  match_type: 'clip' | 'title'
}

export type PhotoSortMode = 'date' | 'added'

export async function listPhotoFeed(sort: PhotoSortMode, page: number): Promise<PhotoFeedPage> {
  return apiClient<PhotoFeedPage>(`/photos?sort=${sort}&page=${page}`)
}

export async function searchPhotos(q: string, limit = 20): Promise<PhotoSearchResult[]> {
  return apiClient<PhotoSearchResult[]>(`/photos/search?q=${encodeURIComponent(q)}&limit=${limit}`)
}
