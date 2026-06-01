import { describe, expect, it } from 'vitest'
import {
  MAIN_CATEGORIES,
  MAIN_CATEGORY_ORDER,
  POWER_CURVE_GROUPS,
  RECORD_GROUP_BADGES,
  SPORT_GROUP_BADGE_LABELS,
  SPORT_GROUP_LABELS,
  SUB_FILTERS,
  getSportGroupBadgeLabel,
  getSportGroupLabel,
  getSubFilters,
  matchesSportGroup,
} from '../recordsConstants'
import { GENERATED_RECORD_GROUPS } from '../generatedRecordProjections'

describe('records projection constants', () => {
  it('exports labels and category grouping from generated records projection', () => {
    expect(SPORT_GROUP_LABELS).toEqual(Object.fromEntries(GENERATED_RECORD_GROUPS.map(group => [group.key, group.label])))
    expect(MAIN_CATEGORY_ORDER).toEqual(['laufen', 'radfahren', 'wandern', 'schwimmen'])
    expect(Object.keys(MAIN_CATEGORIES)).toEqual(['laufen', 'radfahren', 'wandern', 'schwimmen'])
    expect(SUB_FILTERS.radfahren.map(filter => filter.label)).toEqual(['Alle', 'Radfahren', 'E-Bike', 'Stationär'])
    expect(SUB_FILTERS.radfahren.map(filter => filter.key)).toEqual(['all', 'cycling', 'electric', 'stationary'])
    expect(POWER_CURVE_GROUPS.radfahren).toEqual(['cycling'])
    expect(SPORT_GROUP_BADGE_LABELS.cycling_electric).toBe('E-Bike')
    expect(SPORT_GROUP_BADGE_LABELS.swim_pool).toBe('Becken')
    expect(RECORD_GROUP_BADGES.swim_ow).toBe('FW')
  })

  it('localizes record labels without changing functional keys', () => {
    expect(SPORT_GROUP_LABELS.cycling).toBe('Radfahren')
    expect(SPORT_GROUP_LABELS.cycling).not.toBe('Cycling')
    expect(SPORT_GROUP_LABELS.cycling).toBe(getSportGroupLabel('cycling', 'de'))
    expect(getSportGroupLabel('cycling', 'en')).toBe('Cycling')
    expect(getSubFilters('radfahren', 'en').map(filter => [filter.key, filter.label])).toEqual([
      ['all', 'All'],
      ['cycling', 'Cycling'],
      ['electric', 'E-bike'],
      ['stationary', 'Stationary'],
    ])
    expect(getSportGroupBadgeLabel('cycling_virtual', 'en')).toBe('Stationary')
    expect(getSportGroupBadgeLabel('swim_pool', 'en')).toBe('Pool')
  })
})

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

  it('keeps snowshoeing in the hiking record group after category cleanup', () => {
    expect(matchesSportGroup('hiking', { sport_type: 'snowshoeing' })).toBe(true)
  })

  it('treats indoor and virtual modifiers symmetrically for stationary record groups', () => {
    expect(matchesSportGroup('running_indoor', { sport_type: 'running', is_indoor: true })).toBe(true)
    expect(matchesSportGroup('running_virtual', { sport_type: 'running', is_virtual: true })).toBe(true)
    expect(matchesSportGroup('cycling_virtual', { sport_type: 'cycling', is_indoor: true })).toBe(true)
    expect(matchesSportGroup('cycling_virtual', { sport_type: 'cycling', is_virtual: true })).toBe(true)
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', is_indoor: true })).toBe(false)
  })

  it('resolves modifier defaults and precedence from generated match rules', () => {
    expect(matchesSportGroup('cycling_virtual', { sport_type: 'cycling', sub_type: 'spin' })).toBe(true)
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', sub_type: 'spin' })).toBe(false)
    expect(matchesSportGroup('running_virtual', { sport_type: 'running', is_virtual: true, is_indoor: true })).toBe(true)
    expect(matchesSportGroup('running_indoor', { sport_type: 'running', is_virtual: true, is_indoor: true })).toBe(false)
    expect(matchesSportGroup('cycling_virtual', { sport_type: 'cycling', is_virtual: true, is_electric: true })).toBe(true)
    expect(matchesSportGroup('cycling_electric', { sport_type: 'cycling', is_virtual: true, is_electric: true })).toBe(false)
  })

  it('keeps unsupported and legacy top-level sport values out of records groups', () => {
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', is_electric: true })).toBe(false)
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', is_indoor: true })).toBe(false)
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', is_virtual: true })).toBe(false)
    expect(matchesSportGroup('cycling', { sport_type: 'cycling', sub_type: 'indoor_cycling' })).toBe(false)
    expect(matchesSportGroup('running_trail', { sport_type: 'trail_running' })).toBe(false)
    expect(matchesSportGroup('cycling', { sport_type: 'mountain_biking' })).toBe(false)
    expect(matchesSportGroup('swim_pool', { sport_type: 'lap_swimming' })).toBe(false)
  })
})
