import { apiClient, apiUpload } from './client'

export interface ActivityPhoto {
  id: string
  user_id: string
  activity_id?: string
  original_filename: string
  content_type: string
  file_size_bytes: number
  taken_at?: string
  lat?: number
  lng?: number
  location_source?: 'exif' | 'activity_track'
  created_at: string
  thumb_url: string
  full_url: string
}

export interface MapPhotoEntry {
  id: string
  activity_id?: string
  activity_title?: string
  lat: number
  lng: number
  taken_at?: string
  thumb_url: string
}

export async function listActivityPhotos(activityId: string): Promise<ActivityPhoto[]> {
  return apiClient<ActivityPhoto[]>(`/activities/${activityId}/photos`)
}

export async function uploadActivityPhoto(activityId: string, file: File): Promise<ActivityPhoto> {
  const formData = new FormData()
  formData.append('photo', file)
  return apiUpload<ActivityPhoto>(`/activities/${activityId}/photos`, formData)
}

export async function deleteActivityPhoto(activityId: string, photoId: string): Promise<void> {
  return apiClient<void>(`/activities/${activityId}/photos/${photoId}`, { method: 'DELETE' })
}

export async function listMapPhotos(): Promise<MapPhotoEntry[]> {
  return apiClient<MapPhotoEntry[]>('/map/photos')
}
