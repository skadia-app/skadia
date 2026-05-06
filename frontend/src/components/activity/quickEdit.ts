import type { Activity } from '@/api/types'
import type { updateActivity } from '@/api/activities'

export type ActivityQuickEditPatch = Parameters<typeof updateActivity>[1]

export interface ActivityQuickEditDraft {
  title: string
  description: string
  sportType: string
  subType: string
  isIndoor: boolean
  isElectric: boolean
  isVirtual: boolean
  visibility: Activity['visibility']
  workoutRpe: number | null
  workoutFeel: number | null
  tags: string[]
}

const COUPLED_TAGS = new Set(['indoor', 'e-bike', 'virtuell'])

export interface QuickEditTagCandidate {
  name: string
  activity_count?: number
}

function sameNullableString(a: string | null | undefined, b: string | null | undefined): boolean {
  return (a ?? '') === (b ?? '')
}

function normalizedTagName(name: string): string {
  return name.trim().toLowerCase()
}

function uniqueTags(tags: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of tags) {
    const tag = raw.trim()
    if (!tag) continue
    const key = normalizedTagName(tag)
    if (seen.has(key)) continue
    seen.add(key)
    out.push(tag)
  }
  return out
}

export function activityQuickEditDraftFromActivity(activity: Activity): ActivityQuickEditDraft {
  return {
    title: activity.title ?? '',
    description: activity.description ?? '',
    sportType: activity.sport_type,
    subType: activity.sub_type ?? '',
    isIndoor: activity.is_indoor,
    isElectric: activity.is_electric,
    isVirtual: activity.is_virtual,
    visibility: activity.visibility,
    workoutRpe: activity.workout_rpe ?? null,
    workoutFeel: activity.workout_feel ?? null,
    tags: uniqueTags(activity.tags ?? []),
  }
}

export function buildActivityQuickEditPatch(activity: Activity, draft: ActivityQuickEditDraft): ActivityQuickEditPatch {
  const patch: ActivityQuickEditPatch = {}
  const title = draft.title.trim()
  const description = draft.description.trim()
  const subType = draft.subType.trim()

  if ((activity.title ?? '') !== title) patch.title = title
  if ((activity.description ?? '') !== description) patch.description = description || null
  if (activity.sport_type !== draft.sportType) patch.sport_type = draft.sportType
  if (!sameNullableString(activity.sub_type, subType)) patch.sub_type = subType || null
  if (activity.is_indoor !== draft.isIndoor) patch.is_indoor = draft.isIndoor
  if (activity.is_electric !== draft.isElectric) patch.is_electric = draft.isElectric
  if (activity.is_virtual !== draft.isVirtual) patch.is_virtual = draft.isVirtual
  if (activity.visibility !== draft.visibility) patch.visibility = draft.visibility
  if ((activity.workout_rpe ?? null) !== draft.workoutRpe) patch.workout_rpe = draft.workoutRpe
  if ((activity.workout_feel ?? null) !== draft.workoutFeel) patch.workout_feel = draft.workoutFeel

  return patch
}

export function diffQuickEditTags(currentTags: string[], draftTags: string[]): { add: string[]; remove: string[] } {
  const current = uniqueTags(currentTags).filter(tag => !COUPLED_TAGS.has(normalizedTagName(tag)))
  const next = uniqueTags(draftTags).filter(tag => !COUPLED_TAGS.has(normalizedTagName(tag)))
  const currentKeys = new Set(current.map(normalizedTagName))
  const nextKeys = new Set(next.map(normalizedTagName))

  return {
    add: next.filter(tag => !currentKeys.has(normalizedTagName(tag))),
    remove: current.filter(tag => !nextKeys.has(normalizedTagName(tag))),
  }
}

export function quickEditTagSuggestions(
  allTags: QuickEditTagCandidate[],
  draftTags: string[],
  search: string,
): QuickEditTagCandidate[] {
  const selected = new Set(draftTags.map(normalizedTagName))
  const q = search.trim().toLowerCase()
  const limit = q ? 30 : 18
  return allTags
    .filter(tag => !selected.has(normalizedTagName(tag.name)))
    .filter(tag => !q || tag.name.toLowerCase().includes(q))
    .sort((a, b) => (b.activity_count ?? 0) - (a.activity_count ?? 0))
    .slice(0, limit)
}

export function quickEditTagDraftAfterToggle(
  draft: ActivityQuickEditDraft,
  tagName: string,
  selected: boolean,
): ActivityQuickEditDraft {
  const tag = tagName.trim()
  if (!tag) return draft
  const key = normalizedTagName(tag)
  if (key === 'virtuell' && draft.isVirtual && !selected) {
    return draft
  }

  const withoutTag = draft.tags.filter(t => normalizedTagName(t) !== key)
  const tags = selected ? uniqueTags([...withoutTag, tag]) : withoutTag
  const next: ActivityQuickEditDraft = { ...draft, tags }

  if (key === 'indoor') next.isIndoor = selected
  if (key === 'e-bike') next.isElectric = selected
  if (key === 'virtuell') next.isVirtual = selected

  return next
}

export function quickEditTagDraftAfterModifier(
  draft: ActivityQuickEditDraft,
  modifier: 'isIndoor' | 'isElectric' | 'isVirtual',
  selected: boolean,
): ActivityQuickEditDraft {
  const tagByModifier = {
    isIndoor: 'Indoor',
    isElectric: 'E-Bike',
    isVirtual: 'virtuell',
  } as const
  return quickEditTagDraftAfterToggle({ ...draft, [modifier]: selected }, tagByModifier[modifier], selected)
}
