import type { Activity } from '@/api/types'
import { modifierFromTSField, modifierJSONKey, modifierKeysForCapability, modifierTagSync, modifierTagSyncOnlySportType, modifierTSField, type SportModifierKey, type SportModifierTSField } from '@/lib/sportModifiers'
import type { updateActivity } from '@/api/activities'

export type ActivityQuickEditPatch = Parameters<typeof updateActivity>[1]

export interface ActivityQuickEditDraft {
  title: string
  description: string
  sportType: string
  subType: string
  modifiers: Partial<Record<SportModifierTSField, boolean>>
  excludeFromHeatmap: boolean
  excludeFromStats: boolean
  visibility: Activity['visibility']
  workoutRpe: number | null
  workoutFeel: number | null
  tags: string[]
}

const COUPLED_TAGS = new Set(
  modifierKeysForCapability('quick_edit')
    .flatMap((key) => {
      const tag = modifierTagSync(key)?.systemTag
      return tag ? [normalizedTagName(tag)] : []
    }),
)

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
  const modifiers = quickEditModifiersFromActivity(activity)
  return {
    title: activity.title ?? activity.title_auto ?? '',
    description: activity.description ?? '',
    sportType: activity.sport_type,
    subType: activity.sub_type ?? '',
    modifiers,
    excludeFromHeatmap: activity.is_virtual || activity.exclude_from_heatmap,
    excludeFromStats: activity.exclude_from_stats,
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
  const originalTitle = activity.title ?? activity.title_auto ?? ''
  const draftVirtual = quickEditModifierSelected(draft, 'is_virtual')
  const effectiveExcludeFromHeatmap = draftVirtual ? true : draft.excludeFromHeatmap

  if (title) {
    if (originalTitle !== title) patch.title = title
  } else if (activity.title != null) {
    patch.clear_title = true
  }
  if (description) {
    if ((activity.description ?? '') !== description) patch.description = description
  } else if (activity.description) {
    patch.clear_description = true
  }
  if (activity.sport_type !== draft.sportType) patch.sport_type = draft.sportType
  if (!sameNullableString(activity.sub_type, subType)) patch.sub_type = subType || null
  for (const modifierKey of modifierKeysForCapability('quick_edit')) {
    const jsonKey = modifierJSONKey(modifierKey) as keyof ActivityQuickEditPatch & keyof Activity
    const selected = quickEditModifierSelected(draft, modifierKey)
    if (activity[jsonKey] !== selected) {
      ;(patch as Record<string, unknown>)[jsonKey] = selected
    }
  }
  if (activity.exclude_from_heatmap !== effectiveExcludeFromHeatmap) patch.exclude_from_heatmap = effectiveExcludeFromHeatmap
  if (activity.exclude_from_stats !== draft.excludeFromStats) patch.exclude_from_stats = draft.excludeFromStats
  if (activity.visibility !== draft.visibility) patch.visibility = draft.visibility
  if ((activity.workout_rpe ?? null) !== draft.workoutRpe) {
    if (draft.workoutRpe == null) patch.clear_workout_rpe = true
    else patch.workout_rpe = draft.workoutRpe
  }
  if ((activity.workout_feel ?? null) !== draft.workoutFeel) {
    if (draft.workoutFeel == null) patch.clear_workout_feel = true
    else patch.workout_feel = draft.workoutFeel
  }

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
  if (key === 'virtuell' && quickEditModifierSelected(draft, 'is_virtual') && !selected) {
    return draft
  }

  const withoutTag = draft.tags.filter(t => normalizedTagName(t) !== key)
  const tags = selected ? uniqueTags([...withoutTag, tag]) : withoutTag
  const next: ActivityQuickEditDraft = { ...draft, tags }

  for (const modifierKey of modifierKeysForCapability('quick_edit')) {
    const sync = modifierTagSync(modifierKey)
    if (!sync || normalizedTagName(sync.systemTag) !== key) continue
    const onlySportType = modifierTagSyncOnlySportType(modifierKey)
    if (onlySportType && onlySportType !== draft.sportType) continue
    setQuickEditModifier(next, modifierKey, selected)
  }

  return next
}

export function quickEditTagDraftAfterModifier(
  draft: ActivityQuickEditDraft,
  modifier: SportModifierKey | SportModifierTSField,
  selected: boolean,
): ActivityQuickEditDraft {
  const modifierKey = normalizeQuickEditModifierKey(modifier)
  if (!modifierKey) return draft
  const next = { ...draft }
  setQuickEditModifier(next, modifierKey, selected)
  const sync = modifierTagSync(modifierKey)
  const onlySportType = modifierTagSyncOnlySportType(modifierKey)
  if (sync && (!onlySportType || onlySportType === draft.sportType)) {
    return quickEditTagDraftAfterToggle(next, sync.systemTag, selected)
  }
  return next
}

function normalizeQuickEditModifierKey(modifier: SportModifierKey | SportModifierTSField): SportModifierKey | null {
  if (modifierKeysForCapability('quick_edit').includes(modifier as SportModifierKey)) {
    return modifier as SportModifierKey
  }
  const key = modifierFromTSField(modifier)
  return key && modifierKeysForCapability('quick_edit').includes(key) ? key : null
}

function setQuickEditModifier(draft: ActivityQuickEditDraft, modifier: SportModifierKey, selected: boolean) {
  if (!modifierKeysForCapability('quick_edit').includes(modifier)) return
  const field = modifierTSField(modifier)
  draft.modifiers = { ...draft.modifiers, [field]: selected }
  if (modifier === 'is_virtual') draft.excludeFromHeatmap = selected
}

export function quickEditModifierSelected(draft: ActivityQuickEditDraft, modifier: SportModifierKey): boolean {
  return Boolean(draft.modifiers[modifierTSField(modifier)])
}

function quickEditModifiersFromActivity(activity: Activity): Partial<Record<SportModifierTSField, boolean>> {
  const modifiers: Partial<Record<SportModifierTSField, boolean>> = {}
  for (const modifierKey of modifierKeysForCapability('quick_edit')) {
    const jsonKey = modifierJSONKey(modifierKey) as keyof Activity
    modifiers[modifierTSField(modifierKey)] = Boolean(activity[jsonKey])
  }
  return modifiers
}
