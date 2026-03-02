import { apiClient } from './client'

export interface Tag {
  id: string
  name: string
  color: string
  text_color: string
  category: string
  is_system: boolean
  activity_count: number
  created_at: string
}

export async function fetchTags(): Promise<Tag[]> {
  return apiClient<Tag[]>('/tags')
}

export async function createTag(data: { name: string; color?: string; text_color?: string }): Promise<Tag> {
  return apiClient<Tag>('/tags', { method: 'POST', body: data })
}

export async function updateTag(id: string, data: { name?: string; color?: string; text_color?: string }): Promise<Tag> {
  return apiClient<Tag>(`/tags/${id}`, { method: 'PATCH', body: data })
}

export async function deleteTag(id: string): Promise<void> {
  await apiClient(`/tags/${id}`, { method: 'DELETE' })
}
