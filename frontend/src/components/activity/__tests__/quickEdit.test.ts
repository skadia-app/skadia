import { describe, expect, it } from 'vitest'
import {
  buildActivityQuickEditPatch,
  diffQuickEditTags,
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
      description: null,
      sport_type: 'cycling',
      sub_type: 'mountain',
      is_indoor: true,
      is_electric: true,
      is_virtual: false,
      visibility: 'public',
      workout_rpe: 7,
      workout_feel: 4,
    })
    expect(patch).not.toHaveProperty('tags')
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
