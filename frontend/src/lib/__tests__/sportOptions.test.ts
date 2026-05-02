import { describe, expect, it } from 'vitest'

import { filterToExistingSports, generateActivityFilterOptions, getOptionKey, type SportOption } from '../sportOptions'

describe('sport filter options', () => {
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
})

function flattenLeaves(options: SportOption[]): SportOption[] {
  return options.flatMap((option) => option.children ?? [option])
}
