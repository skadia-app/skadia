import { describe, expect, it } from 'vitest'
import {
  matchesSportGroup,
  recordFilterSelection,
  recordSportFilterValue,
} from '../recordsConstants'

describe('matchesSportGroup', () => {
  it('matches canonical cycling and swimming subtypes without legacy sport_type values', () => {
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', sub_type: 'mountain' })).toBe(true)
    expect(matchesSportGroup('cycling_virtual', { sport_type: 'cycling', sub_type: 'indoor_cycling' })).toBe(true)
    expect(matchesSportGroup('swim_pool', { sport_type: 'swimming', sub_type: 'lap_swimming' })).toBe(true)
    expect(matchesSportGroup('swim_ow', { sport_type: 'swimming', sub_type: 'open_water' })).toBe(true)
    expect(matchesSportGroup('cycling', { sport_type: 'mountain_biking' })).toBe(false)
    expect(matchesSportGroup('swim_pool', { sport_type: 'lap_swimming' })).toBe(false)
  })

  it('keeps road running separate from canonical trail and indoor running', () => {
    expect(matchesSportGroup('running', { sport_type: 'running' })).toBe(true)
    expect(matchesSportGroup('running', { sport_type: 'running', sub_type: 'trail' })).toBe(false)
    expect(matchesSportGroup('running', { sport_type: 'running', sub_type: 'indoor_running' })).toBe(false)
    expect(matchesSportGroup('running_trail', { sport_type: 'running', sub_type: 'trail' })).toBe(true)
    expect(matchesSportGroup('running_indoor', { sport_type: 'running', sub_type: 'indoor_running' })).toBe(true)
  })

  it('treats indoor and virtual modifiers symmetrically for stationary record groups', () => {
    expect(matchesSportGroup('running_indoor', { sport_type: 'running', is_indoor: true })).toBe(true)
    expect(matchesSportGroup('running_virtual', { sport_type: 'running', is_virtual: true })).toBe(true)
    expect(matchesSportGroup('cycling_virtual', { sport_type: 'cycling', is_indoor: true })).toBe(true)
    expect(matchesSportGroup('cycling_virtual', { sport_type: 'cycling', is_virtual: true })).toBe(true)
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', is_indoor: true })).toBe(false)
  })

  it('round-trips stationary record filters through tuple plus modifier state', () => {
    expect(recordSportFilterValue('laufen', 'Stationär')).toEqual({
      sportTypes: ['running:indoor_running'],
      modifiers: { is_virtual: true },
    })
    expect(recordFilterSelection({
      sportTypes: ['running:indoor_running'],
      modifiers: { is_virtual: true },
    })).toEqual({ mainCategory: 'laufen', subFilter: 'Stationär' })

    expect(recordSportFilterValue('radfahren', 'Stationär')).toEqual({
      sportTypes: ['cycling:indoor_cycling'],
      modifiers: { is_virtual: true },
    })
    expect(recordFilterSelection({
      sportTypes: ['cycling:indoor_cycling'],
      modifiers: { is_virtual: true },
    })).toEqual({ mainCategory: 'radfahren', subFilter: 'Stationär' })
  })
})
