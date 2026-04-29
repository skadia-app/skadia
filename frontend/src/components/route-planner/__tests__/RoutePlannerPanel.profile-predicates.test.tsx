import { describe, expect, it } from 'vitest'
import { buildCreateProfilePayload } from '../RoutePlannerPanel'
import type { RoutingProfileEntry } from '@/api/routingProfiles'
import type { ProfileConfig } from '@/lib/routingProfiles'

function entry(overrides: Partial<RoutingProfileEntry>): RoutingProfileEntry {
  return {
    id: 'profile-1',
    name: 'profile',
    engine: 'badrouter',
    base_profile: 'road',
    sport_types: [],
    source: 'system',
    config: null,
    editable: false,
    ...overrides,
  }
}

describe('RoutePlannerPanel profile predicate save payload', () => {
  it('saves the MTB system profile with its tuple predicate', () => {
    const payload = buildCreateProfilePayload(
      'My MTB',
      { profile: 'mtb' } satisfies ProfileConfig,
      entry({
        name: 'mtb',
        base_profile: 'mtb',
        predicates: [{ sport: 'cycling', sub_type: 'mountain' }],
        sport_types: ['cycling'],
      }),
    )

    expect(payload).toMatchObject({
      name: 'My MTB',
      base_profile: 'mtb',
      predicates: [{ sport: 'cycling', sub_type: 'mountain' }],
      modifier_filters: {},
    })
    expect(payload.sport_types).toBeUndefined()
  })

  it('preserves the road system profile explicit no-subtype predicates', () => {
    const roadPredicates = [
      { sport: 'cycling', sub_type: '' },
      { sport: 'cycling', sub_type: 'generic' },
      { sport: 'cycling', sub_type: 'road' },
      { sport: 'cycling', sub_type: 'gravel_cycling' },
    ]

    const payload = buildCreateProfilePayload(
      'My Road',
      { profile: 'road' } satisfies ProfileConfig,
      entry({ name: 'road', base_profile: 'road', predicates: roadPredicates, sport_types: ['cycling'] }),
    )

    expect(payload.predicates).toEqual(roadPredicates)
    expect(payload.predicates).not.toEqual([{ sport: 'cycling' }])
  })

  it('preserves modifier filters from the selected profile', () => {
    const payload = buildCreateProfilePayload(
      'E MTB',
      { profile: 'mtb' } satisfies ProfileConfig,
      entry({
        predicates: [{ sport: 'cycling', sub_type: 'mountain' }],
        modifier_filters: { is_electric: true },
      }),
    )

    expect(payload.modifier_filters).toEqual({ is_electric: true })
  })

  it('falls back to legacy sport_types when predicates are absent', () => {
    const payload = buildCreateProfilePayload(
      'Legacy',
      { profile: 'hiking' } satisfies ProfileConfig,
      entry({ sport_types: ['hiking'] }),
    )

    expect(payload).toMatchObject({ sport_types: ['hiking'] })
    expect(payload.predicates).toBeUndefined()
  })
})
