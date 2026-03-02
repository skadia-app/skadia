import { apiClient } from './client'

export interface TagCondition {
  field: string
  op: string
  value: any
}

export interface Tag {
  id: string
  name: string
  color: string
  text_color: string
  category: string
  description: string
  is_system: boolean
  is_auto: boolean
  conditions: TagCondition[] | null
  conditions_logic: 'and' | 'or'
  activity_count: number
  created_at: string
}

export async function fetchTags(): Promise<Tag[]> {
  return apiClient<Tag[]>('/tags')
}

export async function createTag(data: {
  name: string
  color?: string
  text_color?: string
  description?: string
  is_auto?: boolean
  conditions?: TagCondition[]
  conditions_logic?: 'and' | 'or'
}): Promise<Tag> {
  return apiClient<Tag>('/tags', { method: 'POST', body: data })
}

export async function updateTag(id: string, data: {
  name?: string
  color?: string
  text_color?: string
  description?: string
  is_auto?: boolean
  conditions?: TagCondition[]
  conditions_logic?: 'and' | 'or'
}): Promise<Tag> {
  return apiClient<Tag>(`/tags/${id}`, { method: 'PATCH', body: data })
}

export async function deleteTag(id: string): Promise<void> {
  await apiClient(`/tags/${id}`, { method: 'DELETE' })
}

export async function seedDefaultTags(): Promise<{ created: number }> {
  return apiClient<{ created: number }>('/tags/seed-defaults', { method: 'POST' })
}
