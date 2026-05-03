import { describe, expect, it } from 'vitest'
import {
  CATEGORY_KEYS,
  CATEGORY_LABELS,
  CATEGORY_METADATA,
  DIST_METERS,
  POWER_DURATIONS,
  categoryKeysForRecordGroup,
  categoryLabel,
} from '../bestEffortLabels'
import {
  GENERATED_RECORD_CATEGORIES,
  GENERATED_RECORD_CATEGORY_LABELS,
  GENERATED_RECORD_CATEGORY_ORDER,
  GENERATED_RECORD_POWER_CATEGORY_ORDER,
} from '../generatedRecordProjections'

describe('bestEffortLabels generated metadata', () => {
  it('reads labels and order from generated records projection', () => {
    expect(CATEGORY_LABELS).toEqual(GENERATED_RECORD_CATEGORY_LABELS)
    expect(CATEGORY_KEYS).toEqual([...GENERATED_RECORD_CATEGORY_ORDER])
    expect(POWER_DURATIONS).toEqual([...GENERATED_RECORD_POWER_CATEGORY_ORDER])
    expect(categoryLabel('dist_marathon')).toBe('Marathon')
  })

  it('exposes metric thresholds from generated category metadata', () => {
    expect(CATEGORY_METADATA).toEqual(GENERATED_RECORD_CATEGORIES)
    expect(CATEGORY_METADATA.find(category => category.key === 'dist_100mi')).toMatchObject({
      type: 'time_lower',
      distanceM: 160934.4,
    })
    expect(CATEGORY_METADATA.find(category => category.key === 'elev_1000hm')).toMatchObject({
      type: 'time_lower',
      elevationM: 1000,
    })
    expect(CATEGORY_METADATA.find(category => category.key === 'power_20min')).toMatchObject({
      type: 'value_higher',
      durationS: 1200,
    })
    expect(DIST_METERS.dist_1mi).toBe(1609.34)
    expect(DIST_METERS.dist_hm).toBe(21097.5)
    expect(DIST_METERS.dist_100mi).toBe(160934.4)
  })

  it('uses record-group category order for cycling-only distances', () => {
    const cycling = categoryKeysForRecordGroup('cycling')
    expect(cycling.indexOf('dist_25mi')).toBeGreaterThan(cycling.indexOf('dist_20km'))
    expect(cycling.indexOf('dist_25mi')).toBeLessThan(cycling.indexOf('elev_500hm'))
    expect(cycling.indexOf('dist_100mi')).toBeLessThan(cycling.indexOf('elev_500hm'))
  })
})
