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
      subType: 'treadmill',
      modifiers: { isIndoor: true },
    })
    expect(normalizeSportTuple('lap_swimming')).toEqual({
      sportType: 'swimming',
      subType: 'lap_swimming',
      modifiers: {},
    })
    expect(normalizeSportTuple('e_mountain_biking')).toEqual({
      sportType: 'cycling',
      subType: 'mountain',
      modifiers: { isElectric: true },
    })
    expect(normalizeSportTuple('e_bike')).toEqual({
      sportType: 'cycling',
      subType: '',
      modifiers: { isElectric: true },
    })
    expect(normalizeSportTuple('cycling', 'generic')).toEqual({
      sportType: 'cycling',
      subType: '',
      modifiers: {},
    })
  })

  it('keeps broad canonical sport classes distinct from legacy aliases', () => {
    expect(normalizeSportTuple('skiing')).toEqual({ sportType: 'skiing', subType: '', modifiers: {} })
    expect(normalizeSportTuple('strength')).toEqual({ sportType: 'strength', subType: '', modifiers: {} })
    expect(normalizeSportTuple('canoeing')).toEqual({ sportType: 'canoeing', subType: '', modifiers: {} })
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

  it('marks legacy input values as invalid at runtime display boundaries', () => {
    expect(sportLabel('mountain_biking')).toContain('Veraltete Sportart')
    expect(sportLabel('trail_running')).toContain('Veraltete Sportart')
    expect(sportLabel('gravel_cycling')).toContain('Veraltete Sportart')
    expect(sportLabel('indoor_cycling')).toContain('Veraltete Sportart')
    expect(sportLabel('treadmill_running')).toContain('Veraltete Sportart')
    expect(sportLabel('lap_swimming')).toContain('Veraltete Sportart')
    expect(sportLabel('open_water_swimming')).toContain('Veraltete Sportart')
  })

  it('keeps labels for representative frontend/backend taxonomy parity points', () => {
    expect(sportLabel('swimming', 'lap_swimming')).toBe('Bahnschwimmen')
    expect(sportLabel('alpine_skiing', 'backcountry')).toBe('Freeride')
    expect(sportLabel('training', 'flexibility_training')).toBe('Stretching')
    expect(sportLabel('fitness_equipment', 'indoor_rowing')).toBe('Ruder-Ergometer')
    expect(sportLabel('water_tubing')).toBe('Wassertubing')
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

  it('does not expose generic as a cycling subtype option', () => {
    const cycling = SPORT_DEFS.find((s) => s.sportType === 'cycling')
    expect(cycling?.subTypes.map((s) => s.subType)).not.toContain('generic')
  })
})
