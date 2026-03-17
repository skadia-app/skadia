import { apiClient, apiUpload } from './client'

// ---------------------------------------------------------------------------
// Tag Zone types
// ---------------------------------------------------------------------------

/** A geographic zone attached to a location tag. */
export interface TagZone {
  id: string
  tag_id: string
  /** 'circle' uses center + radius_m; 'polygon' uses polygon GeoJSON */
  zone_type: 'circle' | 'polygon'
  /** [lng, lat] — present for circle zones */
  center?: [number, number] | null
  radius_m?: number | null
  /** GeoJSON Polygon string — present for polygon zones */
  polygon?: string | null
  created_at: string
  updated_at: string
}

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
  has_automation?: boolean
  // New group-based structure (preferred). Falls back to flat conditions + conditions_logic.
  condition_groups?: TagConditionGroup[] | null
  // Legacy flat structure (still supported by API):
  conditions: TagCondition[] | null
  conditions_logic: 'and' | 'or'
  activity_count: number
  created_at: string
  // Optional cover image URL for the location tag popup.
  cover_image_url?: string | null
  // Optional thumbnail URL for the cover image (if available).
  cover_image_thumb_url?: string | null
  // Optional geo-coordinates — when set, the tag acts as a "location tag".
  lat?: number | null
  lng?: number | null
  // Optional zone attached to this location tag (included by ListTags).
  zone?: TagZone | null
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

export interface TagActivity {
  id: string
  title: string | null
  title_auto?: string | null
  started_at: string
  sport_type: string
  distance_m?: number | null
}

export async function fetchTagActivities(tagId: string): Promise<TagActivity[]> {
  return apiClient<TagActivity[]>(`/tags/${tagId}/activities`)
}

export async function assignActivitiesToTag(tagId: string, activityIds: string[]): Promise<void> {
  await apiClient<void>(`/tags/${tagId}/assign-activities`, {
    method: 'POST',
    body: { activity_ids: activityIds },
  })
}

/**
 * Sync location_lat/location_lng on all activities linked to tagId.
 * The tag must have lat/lng set. Fire-and-forget safe.
 */
export async function syncTagLocation(tagId: string): Promise<{ updated_count: number }> {
  return apiClient<{ updated_count: number }>(`/tags/${tagId}/sync-location`, { method: 'POST' })
}

/**
 * Merge sourceId into targetId.
 * All activities of sourceId are re-assigned to targetId.
 * sourceId is deleted afterwards.
 * Returns the surviving (target) tag.
 */
export async function uploadTagCoverImage(tagId: string, file: File): Promise<Tag> {
  const formData = new FormData()
  formData.append('image', file)
  const tag = await apiUpload<Tag>(`/tags/${tagId}/cover-image`, formData)
  return normaliseTag(tag)
}

export async function setTagCoverImageFromPhoto(tagId: string, photoUrl: string, photoId: string): Promise<Tag> {
  const tag = await apiClient<Tag>(`/tags/${tagId}/cover-image`, {
    method: 'POST',
    body: { activity_photo_url: photoUrl, activity_photo_id: photoId },
  })
  return normaliseTag(tag)
}

export async function clearTagCoverImage(tagId: string): Promise<Tag> {
  const tag = await apiClient<Tag>(`/tags/${tagId}/cover-image`, { method: 'DELETE' })
  return normaliseTag(tag)
}

export async function mergeTags(targetId: string, sourceId: string): Promise<Tag> {
  const tag = await apiClient<Tag>(`/tags/${targetId}/merge`, {
    method: 'POST',
    body: { source_tag_id: sourceId },
  })
  return normaliseTag(tag)
}

export async function unassignActivitiesFromTag(tagId: string, activityIds: string[]): Promise<void> {
  await apiClient<void>(`/tags/${tagId}/activities`, {
    method: 'DELETE',
    body: { activity_ids: activityIds },
  })
}

// ---------------------------------------------------------------------------
// Tag Zone API
// ---------------------------------------------------------------------------

export async function fetchTagZone(tagId: string): Promise<TagZone | null> {
  try {
    return await apiClient<TagZone>(`/tag-zones/${tagId}`)
  } catch (err: unknown) {
    // 404 means no zone exists — treat as null
    if (err instanceof Error && err.message.includes('404')) return null
    throw err
  }
}

export async function upsertTagZone(tagId: string, zone: Omit<TagZone, 'id' | 'tag_id' | 'created_at' | 'updated_at'>): Promise<TagZone> {
  return apiClient<TagZone>(`/tag-zones/${tagId}`, {
    method: 'PUT',
    body: zone,
  })
}

export async function deleteTagZone(tagId: string): Promise<void> {
  await apiClient<void>(`/tag-zones/${tagId}`, { method: 'DELETE' })
}

/**
 * Remove an orphan tag name from all activities.tags TEXT[] for the current user.
 * Orphan tags are tag names that appear in activities but have no corresponding tags table entry.
 */
export async function deleteOrphanTag(name: string): Promise<{ affected: number }> {
  return apiClient<{ affected: number }>('/tags/orphan', {
    method: 'DELETE',
    body: { name },
  })
}
