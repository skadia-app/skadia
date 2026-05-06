import { describe, expect, it } from 'vitest'
import {
  activityQuickEditDraftFromActivity,
  buildActivityQuickEditPatch,
  diffQuickEditTags,
  quickEditTagDraftAfterModifier,
  quickEditTagDraftAfterToggle,
  quickEditTagSuggestions,
} from '../quickEdit'
import type { ActivityQuickEditDraft } from '../quickEdit'
import type { Activity } from '@/api/types'

const baseActivity = {
  id: 'act-1',
  title: 'Old title',
  description: 'Old description',
  sport_type: 'running',
  sub_type: null,
  is_indoor: false,
  is_electric: false,
  is_virtual: true,
  exclude_from_heatmap: true,
  exclude_from_stats: true,
  visibility: 'private',
  workout_rpe: null,
  workout_feel: null,
  tags: ['virtuell', 'Alt'],
} as Activity

function draft(overrides: Partial<ActivityQuickEditDraft> = {}): ActivityQuickEditDraft {
  return {
    title: 'New title',
    description: '',
    sportType: 'cycling',
    subType: 'mountain',
    isIndoor: true,
    isElectric: true,
    isVirtual: false,
    excludeFromHeatmap: false,
    excludeFromStats: false,
    visibility: 'public',
    workoutRpe: 7,
    workoutFeel: 4,
    tags: ['Indoor', 'E-Bike', 'Neu'],
    ...overrides,
  }
}

describe('activity quick edit helpers', () => {
  it('builds the activity patch without mixing tag arrays into modifier changes', () => {
    const patch = buildActivityQuickEditPatch(baseActivity, draft())

    expect(patch).toEqual({
      title: 'New title',
      clear_description: true,
      sport_type: 'cycling',
      sub_type: 'mountain',
      is_indoor: true,
      is_electric: true,
      is_virtual: false,
      exclude_from_heatmap: false,
      exclude_from_stats: false,
      visibility: 'public',
      workout_rpe: 7,
      workout_feel: 4,
    })
    expect(patch).not.toHaveProperty('tags')
  })

  it('clears nullable quick-edit fields through explicit clear operations', () => {
    const patch = buildActivityQuickEditPatch(
      { ...baseActivity, title: 'Manual title', workout_rpe: 6, workout_feel: 2 },
      draft({ title: '', description: '', workoutRpe: null, workoutFeel: null }),
    )

    expect(patch).toMatchObject({
      clear_title: true,
      clear_description: true,
      clear_workout_rpe: true,
      clear_workout_feel: true,
    })
    expect(patch).not.toHaveProperty('title')
    expect(patch).not.toHaveProperty('description')
    expect(patch).not.toHaveProperty('workout_rpe')
    expect(patch).not.toHaveProperty('workout_feel')
  })

  it('shows the auto title without saving it back unchanged', () => {
    const activity = { ...baseActivity, title: null, title_auto: 'Auto title' }
    const initial = activityQuickEditDraftFromActivity(activity)

    expect(initial.title).toBe('Auto title')
    expect(buildActivityQuickEditPatch(activity, { ...initial })).not.toHaveProperty('title')
  })

  it('keeps virtual activities heatmap-excluded in the draft', () => {
    const initial = activityQuickEditDraftFromActivity({ ...baseActivity, is_virtual: true, exclude_from_heatmap: false })
    expect(initial.excludeFromHeatmap).toBe(true)

    const virtual = quickEditTagDraftAfterModifier(draft({ isVirtual: false, excludeFromHeatmap: false, tags: [] }), 'isVirtual', true)
    expect(virtual.isVirtual).toBe(true)
    expect(virtual.excludeFromHeatmap).toBe(true)

    const outdoor = quickEditTagDraftAfterModifier(virtual, 'isVirtual', false)
    expect(outdoor.isVirtual).toBe(false)
    expect(outdoor.excludeFromHeatmap).toBe(false)
  })

  it('diffs only normal tag assignments so coupled modifier tags stay trigger-owned', () => {
    const diff = diffQuickEditTags(baseActivity.tags, draft().tags)

    expect(diff).toEqual({
      add: ['Neu'],
      remove: ['Alt'],
    })
  })

  it('keeps virtual locked while the draft flag is true and maps system tags to flags', () => {
    const nextIndoor = quickEditTagDraftAfterToggle(draft({ tags: [] }), 'Indoor', true)
    expect(nextIndoor.isIndoor).toBe(true)
    expect(nextIndoor.tags).toContain('Indoor')

    const stillVirtual = quickEditTagDraftAfterToggle(draft({ isVirtual: true, tags: ['virtuell'] }), 'virtuell', false)
    expect(stillVirtual.isVirtual).toBe(true)
    expect(stillVirtual.tags).toContain('virtuell')

    const noElectric = quickEditTagDraftAfterToggle(draft({ isElectric: true, tags: ['E-Bike'] }), 'E-Bike', false)
    expect(noElectric.isElectric).toBe(false)
    expect(noElectric.tags).not.toContain('E-Bike')
  })

  it('limits tag suggestions and orders them by usage when search is empty', () => {
    const tags = Array.from({ length: 40 }, (_, i) => ({
      name: `Tag ${i}`,
      activity_count: i,
    }))

    const suggestions = quickEditTagSuggestions(tags, ['Tag 39'], '')

    expect(suggestions).toHaveLength(18)
    expect(suggestions[0].name).toBe('Tag 38')
    expect(suggestions.at(-1)?.name).toBe('Tag 21')
  })

  it('allows more filtered tag suggestions while still capping the list', () => {
    const tags = Array.from({ length: 40 }, (_, i) => ({
      name: `Urlaub ${i}`,
      activity_count: i,
    }))

    expect(quickEditTagSuggestions(tags, [], 'urlaub')).toHaveLength(30)
  })
})
