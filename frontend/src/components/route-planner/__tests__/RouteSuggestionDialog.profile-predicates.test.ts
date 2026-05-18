import { describe, expect, it } from 'vitest'
import { requiredCyclingSubTypeForBroadSelection } from '../RouteSuggestionDialog'
import type { RoutingProfileEntry } from '@/api/routingProfiles'

function profile(predicates: RoutingProfileEntry['predicates']): RoutingProfileEntry {
  return {
    id: 'profile-1',
    name: 'road',
    engine: 'badrouter',
    base_profile: 'road',
    sport_types: ['cycling'],
    predicates,
    source: 'system',
    config: null,
    editable: false,
  }
}

describe('RouteSuggestionDialog profile predicate warnings', () => {
  it('does not require a specific cycling subtype when the profile explicitly supports broad cycling', () => {
    const road = profile([
      { sport: 'cycling', sub_type: '' },
      { sport: 'cycling', sub_type: 'road' },
    ])

    expect(requiredCyclingSubTypeForBroadSelection(road)).toBeNull()
  })

  it('requires the profile subtype when broad cycling would otherwise select an exact subtype profile', () => {
    const mtb = profile([{ sport: 'cycling', sub_type: 'mountain' }])

    expect(requiredCyclingSubTypeForBroadSelection(mtb)).toBe('mountain')
  })
})
