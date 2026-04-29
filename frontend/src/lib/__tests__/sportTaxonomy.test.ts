import { describe, expect, it } from 'vitest'

import { SPORT_DEFS, normalizeSportTuple, routeSportOptions, sportLabel } from '../sportTaxonomy'

describe('sport taxonomy aliases', () => {
  it('normalizes legacy sport types to canonical tuples', () => {
    expect(normalizeSportTuple('mountain_biking')).toEqual({
      sportType: 'cycling',
      subType: 'mountain',
      modifiers: {},
    })
    expect(normalizeSportTuple('treadmill_running')).toEqual({
      sportType: 'running',
      subType: 'indoor_running',
      modifiers: { isIndoor: true },
    })
    expect(normalizeSportTuple('e_mountain_biking')).toEqual({
      sportType: 'cycling',
      subType: 'mountain',
      modifiers: { isElectric: true },
    })
  })

  it('uses canonical subtype labels', () => {
    expect(sportLabel('cycling', 'mountain')).toBe('MTB')
    expect(sportLabel('running', 'indoor_running')).toBe('Indoor')
    expect(sportLabel('running', 'indoor_running', { mode: 'withParent' })).toBe('Laufen (Indoor)')
    expect(sportLabel('paddling')).toBe('Paddeln (Generisch)')
  })

  it('keeps canonical sport options free of deprecated top-level sports', () => {
    const sportTypes = SPORT_DEFS.map((s) => s.sportType)
    expect(sportTypes).not.toContain('mountain_biking')
    expect(sportTypes).not.toContain('trail_running')
    expect(sportTypes).not.toContain('gravel_cycling')
    expect(sportTypes).not.toContain('indoor_cycling')
    expect(sportTypes).not.toContain('treadmill_running')
    expect(sportTypes).not.toContain('lap_swimming')
    expect(sportTypes).not.toContain('open_water_swimming')
  })

  it('still displays legacy input values through their canonical labels', () => {
    expect(sportLabel('mountain_biking')).toBe('MTB')
    expect(sportLabel('trail_running')).toBe('Trail')
    expect(sportLabel('gravel_cycling')).toBe('Gravel')
    expect(sportLabel('indoor_cycling')).toBe('Indoor')
    expect(sportLabel('treadmill_running')).toBe('Indoor')
    expect(sportLabel('lap_swimming')).toBe('Hallenbad')
    expect(sportLabel('open_water_swimming')).toBe('Freiwasser')
  })

  it('builds route sport options from canonical GPS-capable sports and subtypes', () => {
    const values = routeSportOptions().map(opt => `${opt.sportType}/${opt.subType ?? ''}`)

    expect(values).toContain('cycling/mountain')
    expect(values).toContain('cycling/gravel_cycling')
    expect(values).toContain('running/trail')
    expect(values).toContain('hiking/')
    expect(values).not.toContain('mountain_biking/')
    expect(values).not.toContain('trail_running/')
  })
})
