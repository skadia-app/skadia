import { describe, expect, it } from 'vitest'
import {
  GENERATED_RECORD_CATEGORY_LABELS,
  GENERATED_RECORD_CATEGORY_ORDER,
  GENERATED_RECORD_GROUPS,
  GENERATED_RECORD_MAIN_CATEGORIES,
  GENERATED_RECORD_MATCH_RULES,
  GENERATED_RECORD_POWER_CATEGORY_ORDER,
} from '../generatedRecordProjections'

describe('generatedRecordProjections', () => {
  it('contains all initial record groups and main categories', () => {
    expect(GENERATED_RECORD_GROUPS.map(group => group.key)).toEqual([
      'running',
      'running_trail',
      'running_indoor',
      'running_virtual',
      'cycling',
      'cycling_electric',
      'cycling_virtual',
      'hiking',
      'swim_pool',
      'swim_ow',
    ])
    expect(GENERATED_RECORD_MAIN_CATEGORIES.map(category => category.key)).toEqual([
      'laufen',
      'radfahren',
      'wandern',
      'schwimmen',
    ])
  })

  it('exports category labels, category order, power order, and sorted match rules', () => {
    expect(GENERATED_RECORD_CATEGORY_LABELS.dist_400m).toBe('400 m')
    expect(GENERATED_RECORD_CATEGORY_ORDER).toContain('dist_marathon')
    expect(GENERATED_RECORD_POWER_CATEGORY_ORDER).toEqual([
      'power_5s',
      'power_30s',
      'power_1min',
      'power_5min',
      'power_10min',
      'power_20min',
      'power_30min',
      'power_1h',
      'power_2h',
    ])
    expect(GENERATED_RECORD_MATCH_RULES[0]).toMatchObject({
      recordGroup: 'running_virtual',
      priority: 100,
      sportType: 'running',
      modifiers: { isVirtual: true },
    })
  })
})
