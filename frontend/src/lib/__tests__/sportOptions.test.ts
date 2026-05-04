import { describe, expect, it } from 'vitest'

import { filterToExistingSports, generateActivityFilterOptions, generateGearSportOptions, generateMapFilterOptions, getOptionKey, type SportOption } from '../sportOptions'

describe('sport filter options', () => {
  it('renders activity projection groups in deterministic order', () => {
    const options = generateActivityFilterOptions()

    expect(options.map((group) => group.groupKey)).toEqual([
      'run',
      'bike',
      'winter',
      'water_swim',
      'water_paddle',
      'water_board',
      'water_boat',
      'water_dive',
      'water_other',
      'fitness',
      'game',
      'climb',
      'outdoor',
      'multi',
      'other',
    ])
  })

  it('renders map projection groups in deterministic order', () => {
    const options = generateMapFilterOptions()

    expect(options.map((group) => group.groupKey)).toEqual([
      'run',
      'bike',
      'winter',
      'water_swim',
      'water_paddle',
      'water_board',
      'water_boat',
      'water_dive',
      'water_other',
      'climb',
      'outdoor',
      'multi',
      'fitness_other',
    ])
  })

  it('renders six water groups from generated activity projection', () => {
    const options = generateActivityFilterOptions()
    const waterGroups = options.filter((group) => group.groupKey?.startsWith('water_'))

    expect(waterGroups.map((group) => group.groupKey)).toEqual([
      'water_swim',
      'water_paddle',
      'water_board',
      'water_boat',
      'water_dive',
      'water_other',
    ])
    expect(options.map((group) => group.groupKey)).not.toContain('water_surf')
    expect(options.map((group) => group.label)).not.toContain('Surfen & Segeln')
  })

  it('renders water leaves in the intended groups', () => {
    const options = generateActivityFilterOptions()

    expect(keysInGroup(options, 'water_swim')).toContain('swimming')
    expect(keysInGroup(options, 'water_paddle')).toEqual(expect.arrayContaining([
      'paddling',
      'kayaking',
      'stand_up_paddleboarding',
      'rowing',
      'rafting',
      'canoeing',
    ]))
    expect(keysInGroup(options, 'water_board')).toEqual(expect.arrayContaining([
      'surfing',
      'kitesurfing',
      'windsurfing',
      'wakeboarding',
      'wakesurfing',
      'water_skiing',
      'water_tubing',
    ]))
    expect(keysInGroup(options, 'water_boat')).toEqual(expect.arrayContaining(['boating', 'sailing']))
    expect(keysInGroup(options, 'water_dive')).toEqual(expect.arrayContaining(['diving', 'snorkeling']))
    expect(keysInGroup(options, 'water_other')).toEqual(['fishing'])
    expect(groupByKey(options, 'water_other').label).toBe('Angeln')
  })

  it('renders map preset from generated projection data', () => {
    const options = generateMapFilterOptions()
    const waterGroups = options.filter((group) => group.groupKey?.startsWith('water_'))

    expect(waterGroups.map((group) => group.groupKey)).toEqual([
      'water_swim',
      'water_paddle',
      'water_board',
      'water_boat',
      'water_dive',
      'water_other',
    ])
    expect(groupByKey(options, 'fitness_other').children?.map(getOptionKey)).toEqual(expect.arrayContaining([
      'strength_training',
      'racket:squash',
      'racket:badminton',
      'generic',
      'unknown',
    ]))
    expect(groupByKey(options, 'fitness_other').children?.map(getOptionKey)).not.toContain('racket')
    expect(options.map((group) => group.label)).not.toContain('Wassersport')
  })

  it('preserves representative non-water filter leaves', () => {
    const options = generateActivityFilterOptions()

    expect(keysInGroup(options, 'run').slice(0, 8)).toEqual([
      'running',
      'running:street',
      'running:trail',
      'running:track',
      'running:treadmill',
      'running:indoor_running',
      'running:ultra',
      'running:obstacle',
    ])
    expect(keysInGroup(options, 'winter')).toEqual(expect.arrayContaining([
      'alpine_skiing',
      'alpine_skiing:resort',
      'snowboarding:resort',
      'snowshoeing',
      'skiing',
    ]))
    expect(keysInGroup(options, 'fitness')).toEqual(expect.arrayContaining([
      'training:strength_training',
      'fitness_equipment:treadmill',
      'strength_training',
      'hiit',
      'yoga',
      'pilates',
      'mobility',
    ]))
    expect(keysInGroup(options, 'game')).toEqual(expect.arrayContaining([
      'tennis',
      'tennis:match',
      'racket:squash',
      'racket:badminton',
      'golf',
    ]))
    expect(keysInGroup(options, 'outdoor')).toEqual(expect.arrayContaining([
      'inline_skating',
      'skateboarding',
      'geocaching',
      'archery',
      'shooting',
    ]))
  })

  it('preserves representative map compact leaves', () => {
    const options = generateMapFilterOptions()

    expect(keysInGroup(options, 'outdoor')).toEqual(expect.arrayContaining([
      'geocaching',
      'archery',
      'shooting',
    ]))
    expect(keysInGroup(options, 'fitness_other')).toEqual(expect.arrayContaining([
      'training:strength_training',
      'fitness_equipment:treadmill',
      'strength_training',
      'mobility',
      'racket:squash',
      'racket:badminton',
      'generic',
      'unknown',
    ]))
  })

  it('keeps explicitly projected broad fallback leaves filterable', () => {
    const options = generateActivityFilterOptions()

    expect(keysInGroup(options, 'water_paddle')).toContain('paddling')
    expect(keysInGroup(options, 'winter')).toContain('skiing')
  })

  it('does not duplicate tuple keys within a preset', () => {
    for (const options of [generateActivityFilterOptions(), generateMapFilterOptions()]) {
      const leaves = flattenLeaves(options)
      expect(new Set(leaves.map(getOptionKey)).size).toBe(leaves.length)
    }
  })

  it('disambiguates duplicate subtype labels with parent context', () => {
    const options = generateActivityFilterOptions()
    const leaves = flattenLeaves(options)

    expect(leaves.find((leaf) => getOptionKey(leaf) === 'pilates')?.label).toBe('Pilates')
    for (const group of options) {
      const labels = (group.children ?? [group]).map((leaf) => leaf.label)
      expect(new Set(labels).size).toBe(labels.length)
    }

    for (const key of ['training:strength_training', 'training:yoga', 'training:pilates', 'training:hiit']) {
      const leaf = leaves.find((candidate) => getOptionKey(candidate) === key)
      expect(leaf).toBeDefined()
      expect(leaf?.label).not.toBe(leaves.find((candidate) => getOptionKey(candidate) === key.split(':')[1])?.label)
    }
    expect(leaves.find((leaf) => getOptionKey(leaf) === 'fitness_equipment:pilates')?.label).not.toBe('Pilates')
    expect(leaves.find((leaf) => getOptionKey(leaf) === 'running:indoor_running')?.label).not.toBe(
      leaves.find((leaf) => getOptionKey(leaf) === 'walking:indoor_walking')?.label,
    )
    expect(leaves.find((leaf) => getOptionKey(leaf) === 'alpine_skiing:resort')?.label).not.toBe(
      leaves.find((leaf) => getOptionKey(leaf) === 'snowboarding:resort')?.label,
    )
    expect(leaves.find((leaf) => getOptionKey(leaf) === 'kayaking:whitewater')?.label).not.toBe(
      leaves.find((leaf) => getOptionKey(leaf) === 'rafting:whitewater')?.label,
    )
  })

  it('keeps non-selectable stored subtype tuples filterable', () => {
    const leaves = flattenLeaves(generateActivityFilterOptions())
    const keys = leaves.map(getOptionKey)

    expect(keys).toEqual(expect.arrayContaining([
      'training:strength_training',
      'training:yoga',
      'training:pilates',
      'training:hiit',
      'fitness_equipment:pilates',
      'strength_training',
      'yoga',
      'pilates',
      'hiit',
    ]))
  })

  it('renders generic and unknown activity filters unchanged by taxonomy traits', () => {
    const options = generateActivityFilterOptions()
    const leaves = flattenLeaves(options)

    expect(leaves.map((leaf) => getOptionKey(leaf))).toContain('generic')
    expect(leaves.map((leaf) => getOptionKey(leaf))).toContain('unknown')
    expect(leaves.find((leaf) => getOptionKey(leaf) === 'generic')?.label).toBe('Aktivität')
    expect(leaves.find((leaf) => getOptionKey(leaf) === 'unknown')?.label).toBe('Unbekannt')
  })

  it('filters existing generic and unknown sports without trait-specific behavior', () => {
    const filtered = filterToExistingSports(generateActivityFilterOptions(), new Set(['generic', 'unknown']))
    const leaves = flattenLeaves(filtered)

    expect(leaves.map((leaf) => getOptionKey(leaf))).toEqual(['generic', 'unknown'])
    expect(leaves.map((leaf) => leaf.label)).toEqual(['Aktivität', 'Unbekannt'])
  })

  it('renders generated manual gear default sport options', () => {
    const options = generateGearSportOptions()
    const values = options.map((option) => option.value)

    expect(values).toContain('archery')
    expect(values).toContain('cycling')
    expect(values).toContain('geocaching')
    expect(values).toContain('generic')
    expect(values).toContain('mobility')
    expect(values).toContain('shooting')
    expect(values).not.toContain('fitness_equipment')
    expect(values).not.toContain('training')
    expect(values).not.toContain('unknown')
    expect(values).not.toContain('cycling:mountain')
  })
})

function flattenLeaves(options: SportOption[]): SportOption[] {
  return options.flatMap((option) => option.children ?? [option])
}

function groupByKey(options: SportOption[], key: string): SportOption {
  const group = options.find((option) => option.groupKey === key)
  if (!group) throw new Error(`missing group ${key}`)
  return group
}

function keysInGroup(options: SportOption[], key: string): string[] {
  return (groupByKey(options, key).children ?? []).map(getOptionKey)
}
