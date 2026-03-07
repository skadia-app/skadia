import { apiClient } from './client'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ImportQueueItem {
  id: string
  item_type: 'image_no_exif' | 'image_ambiguous' | 'pdf_untyped' | 'duplicate' | 'import_error'
  status: 'pending' | 'resolved' | 'dismissed'
  filename: string
  file_size?: number
  created_at: string
  metadata?: {
    error?: string
    duplicate_of?: string
    duplicate_of_title?: string
    ambiguous_candidates?: Array<{ activity_id: string; activity_title: string; started_at: string }>
  }
}

export interface MediaUploadResultItem {
  filename: string
  status: 'assigned' | 'queued' | 'type_required'
  activity_id?: string
  activity_title?: string
  queue_id?: string
  queue_type?: string
}

export interface MediaUploadResult {
  results: MediaUploadResultItem[]
}

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

export async function fetchImportQueue(status?: string): Promise<ImportQueueItem[]> {
  const params = status ? `?status=${encodeURIComponent(status)}` : ''
  return apiClient<ImportQueueItem[]>(`/import-queue${params}`)
}

export async function fetchImportQueueCount(): Promise<number> {
  const res = await apiClient<{ count: number }>('/import-queue/count')
  return res.count
}

export async function assignQueueItem(id: string, activityId: string): Promise<void> {
  return apiClient<void>(`/import-queue/${id}/assign`, {
    method: 'POST',
    body: { activity_id: activityId },
  })
}

export async function patchQueueItem(id: string, updates: object): Promise<void> {
  return apiClient<void>(`/import-queue/${id}`, {
    method: 'PATCH',
    body: updates,
  })
}

export async function deleteQueueItem(id: string): Promise<void> {
  return apiClient<void>(`/import-queue/${id}`, { method: 'DELETE' })
}

export async function uploadMedia(files: File[]): Promise<MediaUploadResult> {
  const { apiUpload } = await import('./client')
  const fd = new FormData()
  for (const f of files) fd.append('files', f)
  return apiUpload<MediaUploadResult>('/upload/media', fd)
}
