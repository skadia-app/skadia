import { describe, expect, it } from 'vitest'

import type { FusionCandidate, FusionQueueEntry } from '../importWizard'
import { modifierKeysForCapability } from '@/lib/sportModifiers'

const fusionCandidateModifierKeys = [
  'is_electric',
  'is_indoor',
  'is_virtual',
] satisfies Array<keyof FusionCandidate>

const fusionQueueModifierPairs = [
  ['is_electric_a', 'is_electric_b'],
  ['is_indoor_a', 'is_indoor_b'],
  ['is_virtual_a', 'is_virtual_b'],
] satisfies Array<[keyof FusionQueueEntry, keyof FusionQueueEntry]>

describe('import wizard modifier wire shape', () => {
  it('keeps fusion activity-state modifier fields aligned with generated metadata', () => {
    const expected = modifierKeysForCapability('activity_state').filter((key) => key !== 'is_commute')
    expect(fusionCandidateModifierKeys).toEqual(expected)
    expect(fusionQueueModifierPairs.map(([left]) => left.replace(/_a$/, ''))).toEqual(expected)
  })
})
