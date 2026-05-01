import { describe, expect, it } from 'vitest'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'

import { GENERATED_SPORT_TAXONOMY_SOURCE_SHA256 } from '../generatedSportTaxonomy'
import { SPORT_DEFS, categoryForSport, normalizeSportTuple, routeSportOptions, sportColor, sportLabel } from '../sportTaxonomy'

describe('sport taxonomy aliases', () => {
  it('keeps the generated frontend taxonomy source hash in sync with sports.yaml', () => {
    const yaml = readFileSync('../config/sport-taxonomy/sports.yaml')
    const expected = createHash('sha256').update(yaml).digest('hex')

    expect(GENERATED_SPORT_TAXONOMY_SOURCE_SHA256).toBe(expected)
  })

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

  it('keeps generated category colors as hex defaults', () => {
    expect(categoryForSport('running').color).toBe('#EF4444')
    expect(categoryForSport('cycling').color).toBe('#F97316')
    expect(sportColor('totally_unknown')).toBe('#6B7280')
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

  it('keeps current route sport option keys stable while projections are deferred', () => {
    const values = routeSportOptions().map((opt) => `${opt.sportType}/${opt.subType ?? ''}`)

    expect(values).toEqual([
      'running/',
      'running/trail',
      'walking/',
      'hiking/',
      'mountaineering/',
      'snowshoeing/',
      'wheelchair_push_walk/',
      'wheelchair_push_run/',
      'cycling/',
      'cycling/mountain',
      'cycling/gravel_cycling',
      'cycling/road',
      'inline_skating/',
      'alpine_skiing/',
      'backcountry_skiing/',
      'cross_country_skiing/',
      'snowboarding/',
      'ice_skating/',
      'snowmobiling/',
      'sledding/',
      'skiing/',
      'swimming/',
      'paddling/',
      'kayaking/',
      'stand_up_paddleboarding/',
      'rowing/',
      'rafting/',
      'canoeing/',
      'surfing/',
      'kitesurfing/',
      'windsurfing/',
      'wakeboarding/',
      'wakesurfing/',
      'water_skiing/',
      'water_tubing/',
      'snorkeling/',
      'boating/',
      'sailing/',
      'fishing/',
      'cricket/',
      'golf/',
      'disc_golf/',
      'rock_climbing/',
      'flying/',
      'hang_gliding/',
      'sky_diving/',
      'jumpmaster/',
      'horseback_riding/',
      'hunting/',
      'motorcycling/',
      'driving/',
      'multisport/',
      'transition/',
    ])
  })

  it('does not expose generic as a cycling subtype option', () => {
    const cycling = SPORT_DEFS.find((s) => s.sportType === 'cycling')
    expect(cycling?.subTypes.map((s) => s.subType)).not.toContain('generic')
  })
})
