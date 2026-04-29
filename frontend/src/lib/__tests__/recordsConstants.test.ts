import { describe, expect, it } from 'vitest'
import { matchesSportGroup } from '../recordsConstants'

describe('matchesSportGroup', () => {
  it('matches canonical cycling and swimming subtypes without legacy sport_type values', () => {
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', sub_type: 'mountain' })).toBe(true)
    expect(matchesSportGroup('cycling_virtual', { sport_type: 'cycling', sub_type: 'indoor_cycling' })).toBe(true)
    expect(matchesSportGroup('swim_pool', { sport_type: 'swimming', sub_type: 'lap_swimming' })).toBe(true)
    expect(matchesSportGroup('swim_ow', { sport_type: 'swimming', sub_type: 'open_water' })).toBe(true)
  })

  it('keeps road running separate from canonical trail and indoor running', () => {
    expect(matchesSportGroup('running', { sport_type: 'running' })).toBe(true)
    expect(matchesSportGroup('running', { sport_type: 'running', sub_type: 'trail' })).toBe(false)
    expect(matchesSportGroup('running', { sport_type: 'running', sub_type: 'indoor_running' })).toBe(false)
    expect(matchesSportGroup('running_trail', { sport_type: 'running', sub_type: 'trail' })).toBe(true)
    expect(matchesSportGroup('running_indoor', { sport_type: 'running', sub_type: 'indoor_running' })).toBe(true)
  })
})
