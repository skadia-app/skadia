import { describe, expect, it } from 'vitest'

import type { FusionCandidate, FusionQueueEntry } from '../importWizard'
import { modifierJSONKey, modifierKeysForCapability } from '@/lib/sportModifiers'

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

    const candidatePayload = Object.fromEntries(fusionCandidateModifierKeys.map((key) => [modifierJSONKey(key), true]))
    const queuePayload = Object.fromEntries(fusionQueueModifierPairs.flatMap(([left, right]) => [[left, true], [right, false]]))
    for (const key of expected) {
      expect(candidatePayload).toHaveProperty(modifierJSONKey(key))
      expect(queuePayload).toHaveProperty(`${modifierJSONKey(key)}_a`)
      expect(queuePayload).toHaveProperty(`${modifierJSONKey(key)}_b`)
    }
  })
})
