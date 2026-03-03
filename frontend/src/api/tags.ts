import { apiClient } from './client'

export interface TagCondition {
  field: string
  op: string
  value: any
  // Geofencing params (only when field === 'geofence')
  geofence?: {
    lat: number
    lon: number
    radius_m: number
    mode: 'inside' | 'outside'
  }
}

/** A group of conditions joined by AND or OR internally.
 *  Multiple groups are always joined by AND between groups. */
export interface TagConditionGroup {
  logic: 'and' | 'or'
  conditions: TagCondition[]
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
  // New group-based structure (preferred). Falls back to flat conditions + conditions_logic.
  condition_groups?: TagConditionGroup[] | null
  // Legacy flat structure (still supported by API):
  conditions: TagCondition[] | null
  conditions_logic: 'and' | 'or'
  activity_count: number
  created_at: string
  // Optional geo-coordinates — when set, the tag acts as a "location tag".
  lat?: number | null
  lng?: number | null
}

/**
 * Normalise a tag returned from the API: if the backend only stored the
 * legacy flat `conditions` array (migration < 000022 or old data), wrap it
 * in a single condition_group so callers always see the new structure.
 */
function normaliseTag(tag: Tag): Tag {
  if (!tag.condition_groups && tag.conditions && tag.conditions.length > 0) {
    tag.condition_groups = [
      {
        logic: tag.conditions_logic ?? 'and',
        conditions: tag.conditions,
      },
    ]
  }
  // Ensure condition_groups is never undefined (null is fine, means no groups yet)
  if (tag.condition_groups === undefined) {
    tag.condition_groups = null
  }
  return tag
}

export async function fetchTags(): Promise<Tag[]> {
  const tags = await apiClient<Tag[]>('/tags')
  return tags.map(normaliseTag)
}

export async function createTag(data: {
  name: string
  color?: string
  text_color?: string
  description?: string
  is_auto?: boolean
  // Prefer condition_groups; falls back to flat conditions+logic if groups absent
  condition_groups?: TagConditionGroup[]
  conditions?: TagCondition[]
  conditions_logic?: 'and' | 'or'
  lat?: number | null
  lng?: number | null
}): Promise<Tag> {
  // Derive flat conditions from first group for backwards compatibility with
  // any backend that does not yet know condition_groups (should not happen
  // after 000022, but kept as a safety net).
  const payload: Record<string, unknown> = { ...data }
  if (data.condition_groups && data.condition_groups.length > 0 && !data.conditions) {
    payload.conditions = data.condition_groups[0].conditions
    payload.conditions_logic = data.condition_groups[0].logic
  }
  const tag = await apiClient<Tag>('/tags', { method: 'POST', body: payload })
  return normaliseTag(tag)
}

export async function updateTag(id: string, data: {
  name?: string
  color?: string
  text_color?: string
  description?: string
  is_auto?: boolean
  condition_groups?: TagConditionGroup[]
  conditions?: TagCondition[]
  conditions_logic?: 'and' | 'or'
  lat?: number | null
  lng?: number | null
}): Promise<Tag> {
  // Derive flat conditions from first group for backwards compatibility.
  const payload: Record<string, unknown> = { ...data }
  if (data.condition_groups && data.condition_groups.length > 0 && !data.conditions) {
    payload.conditions = data.condition_groups[0].conditions
    payload.conditions_logic = data.condition_groups[0].logic
  }
  const tag = await apiClient<Tag>(`/tags/${id}`, { method: 'PATCH', body: payload })
  return normaliseTag(tag)
}

export async function deleteTag(id: string): Promise<void> {
  await apiClient(`/tags/${id}`, { method: 'DELETE' })
}

export async function seedDefaultTags(): Promise<{ created: number }> {
  return apiClient<{ created: number }>('/tags/seed-defaults', { method: 'POST' })
}
