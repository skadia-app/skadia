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
  location_source?: 'exif' | 'activity_track' | 'manual'
  created_at: string
  thumb_url: string
  preview_url: string  // large preview (2048/4K) — same as full_url when no preview was generated
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

export interface UploadPhotoResult {
  status: 'created' | 'duplicate'
  photo?: ActivityPhoto
}

export async function uploadActivityPhoto(activityId: string, file: File): Promise<UploadPhotoResult> {
  const formData = new FormData()
  formData.append('photo', file)
  const result = await apiUpload<ActivityPhoto | { status: string }>(`/activities/${activityId}/photos`, formData)
  if ('status' in result && result.status === 'duplicate') {
    return { status: 'duplicate' }
  }
  return { status: 'created', photo: result as ActivityPhoto }
}

export async function deleteActivityPhoto(activityId: string, photoId: string): Promise<void> {
  return apiClient<void>(`/activities/${activityId}/photos/${photoId}`, { method: 'DELETE' })
}

export async function listMapPhotos(): Promise<MapPhotoEntry[]> {
  return apiClient<MapPhotoEntry[]>('/map/photos')
}

export interface PatchPhotoLocationPayload {
  lat: number
  lng: number
  taken_at?: string
  location_source: 'manual'
}

export async function patchActivityPhotoLocation(
  activityId: string,
  photoId: string,
  payload: PatchPhotoLocationPayload,
): Promise<ActivityPhoto> {
  return apiClient<ActivityPhoto>(`/activities/${activityId}/photos/${photoId}`, {
    method: 'PATCH',
    body: payload,
  })
}

export async function recomputePhotoPreviews(): Promise<{ job_id: string }> {
  return apiClient('/photos/recompute-previews', { method: 'POST' })
}

export async function recomputePhotoEmbeddings(): Promise<{ job_id: string; queued: number }> {
  return apiClient('/photos/recompute-embeddings', { method: 'POST' })
}
