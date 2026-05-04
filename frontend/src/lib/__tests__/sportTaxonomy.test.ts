import { describe, expect, it } from 'vitest'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'

import { GENERATED_SPORT_TAXONOMY_SOURCE_SHA256 } from '../generatedSportTaxonomy'
import { SPORT_DEFS, canonicalSportOptions, categoryForSport, modifierDefaultsForTuple, normalizeSportTuple, routeSportOptions, selectableParentSportOptions, selectableSportTupleOptions, sportColor, sportLabel, sportsInCategory } from '../sportTaxonomy'

describe('sport taxonomy aliases', () => {
  it('keeps the generated frontend taxonomy source hash in sync with sports.yaml', () => {
    const yaml = readFileSync('../config/sport-taxonomy/sports.yaml')
    const expected = createHash('sha256').update(yaml).digest('hex')

    expect(GENERATED_SPORT_TAXONOMY_SOURCE_SHA256).toBe(expected)
  })

  it('keeps generated frontend taxonomy content in sync with sports.yaml', () => {
    execFileSync('../scripts/generate-sport-taxonomy.sh', ['--check'], { stdio: 'pipe' })
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
    expect(normalizeSportTuple('roadcycling')).toEqual({
      sportType: 'cycling',
      subType: 'road',
      modifiers: {},
    })
    expect(normalizeSportTuple('wheelchair_push_walk')).toEqual({
      sportType: 'wheelchair_push',
      subType: 'walk_speed',
      modifiers: {},
    })
    expect(normalizeSportTuple('wheelchair_push_run')).toEqual({
      sportType: 'wheelchair_push',
      subType: 'run_speed',
      modifiers: {},
    })
    expect(normalizeSportTuple('wheelchair_push_walk', 'indoor_wheelchair_walk')).toEqual({
      sportType: 'wheelchair_push',
      subType: 'walk_speed',
      modifiers: { isIndoor: true },
    })
    expect(normalizeSportTuple('wheelchair_push_run', 'indoor_wheelchair_run')).toEqual({
      sportType: 'wheelchair_push',
      subType: 'run_speed',
      modifiers: { isIndoor: true },
    })
    expect(normalizeSportTuple('strength')).toEqual({
      sportType: 'strength_training',
      subType: '',
      modifiers: { isIndoor: true },
    })
  })

  it('keeps broad canonical sport classes distinct from legacy aliases', () => {
    expect(normalizeSportTuple('skiing')).toEqual({ sportType: 'skiing', subType: '', modifiers: {} })
    expect(normalizeSportTuple('canoeing')).toEqual({ sportType: 'canoeing', subType: '', modifiers: {} })
  })

  it('applies generated indoor modifier defaults to canonical tuples', () => {
    expect(modifierDefaultsForTuple('diving', 'apnea_pool').isIndoor).toBe(true)
    expect(normalizeSportTuple('diving', 'apnea_pool').modifiers.isIndoor).toBe(true)
    expect(normalizeSportTuple('rock_climbing', 'bouldering').modifiers.isIndoor).toBeUndefined()
    expect(normalizeSportTuple('swimming', 'lap_swimming').modifiers.isIndoor).toBeUndefined()
    expect(modifierDefaultsForTuple('fitness_equipment', 'treadmill').isIndoor).toBe(true)
    expect(normalizeSportTuple('fitness_equipment', 'indoor_rowing').modifiers.isIndoor).toBe(true)

    for (const [sportType, subType] of [
      ['running', 'indoor_running'],
      ['running', 'treadmill'],
      ['cycling', 'indoor_cycling'],
      ['cycling', 'spin'],
      ['rowing', 'indoor_rowing'],
      ['alpine_skiing', 'indoor_skiing'],
      ['walking', 'indoor_walking'],
      ['rock_climbing', 'indoor_climbing'],
    ]) {
      expect(normalizeSportTuple(sportType, subType).modifiers.isIndoor).toBe(true)
    }
  })

  it('returns a copy of generated modifier defaults', () => {
    const first = modifierDefaultsForTuple('running', 'treadmill')
    first.isIndoor = false

    expect(modifierDefaultsForTuple('running', 'treadmill').isIndoor).toBe(true)
  })

  it('uses canonical subtype labels', () => {
    expect(sportLabel('cycling', 'mountain')).toBe('MTB')
    expect(sportLabel('running', 'indoor_running')).toBe('Indoor')
    expect(sportLabel('running', 'indoor_running', { mode: 'withParent' })).toBe('Laufen (Indoor)')
    expect(sportLabel('paddling')).toBe('Paddeln (allgemein)')
    expect(sportLabel('skiing')).toBe('Skifahren')
    expect(sportLabel('training')).toBe('Training')
    expect(sportLabel('fitness_equipment')).toBe('Fitnessgerät')
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
    expect(sportTypes).not.toContain('wheelchair_push_walk')
    expect(sportTypes).not.toContain('wheelchair_push_run')
    expect(sportTypes).not.toContain('strength')
  })

  it('keeps frontend option order stable while generated source order is backend-owned', () => {
    expect(canonicalSportOptions().slice(0, 12).map((opt) => opt.value)).toEqual([
      'running',
      'walking',
      'hiking',
      'mountaineering',
      'wheelchair_push',
      'cycling',
      'inline_skating',
      'skateboarding',
      'alpine_skiing',
      'backcountry_skiing',
      'cross_country_skiing',
      'snowboarding',
    ])
    expect(sportsInCategory('fitness').map((s) => s.sportType)).toEqual([
      'training',
      'fitness_equipment',
      'strength_training',
      'hiit',
      'yoga',
      'pilates',
      'floor_climbing',
      'dance',
      'jump_rope',
      'meditation',
      'boxing',
      'mixed_martial_arts',
    ])
    expect(sportsInCategory('winter').map((s) => s.sportType)).toContain('snowshoeing')
  })


  it('marks legacy input values as invalid at runtime display boundaries', () => {
    expect(sportLabel('mountain_biking')).toContain('Veraltete Sportart')
    expect(sportLabel('trail_running')).toContain('Veraltete Sportart')
    expect(sportLabel('gravel_cycling')).toContain('Veraltete Sportart')
    expect(sportLabel('indoor_cycling')).toContain('Veraltete Sportart')
    expect(sportLabel('treadmill_running')).toContain('Veraltete Sportart')
    expect(sportLabel('lap_swimming')).toContain('Veraltete Sportart')
    expect(sportLabel('open_water_swimming')).toContain('Veraltete Sportart')
    expect(sportLabel('wheelchair_push_walk')).toContain('Veraltete Sportart')
    expect(sportLabel('wheelchair_push_run')).toContain('Veraltete Sportart')
    expect(sportLabel('strength')).toContain('Veraltete Sportart')
  })

  it('keeps labels for representative frontend/backend taxonomy parity points', () => {
    expect(sportLabel('swimming', 'lap_swimming')).toBe('Bahnschwimmen')
    expect(sportLabel('alpine_skiing', 'backcountry')).toBe('Freeride')
    expect(sportLabel('training', 'flexibility_training')).toBe('Stretching')
    expect(sportLabel('fitness_equipment', 'indoor_rowing')).toBe('Ruder-Ergometer')
    expect(sportLabel('water_tubing')).toBe('Wassertubing')
    expect(sportLabel('wheelchair_push', 'walk_speed')).toBe('Gehtempo')
    expect(sportLabel('wheelchair_push', 'run_speed')).toBe('Lauftempo')
  })

  it('exposes lifecycle metadata for fallback taxonomy values', () => {
    const bySport = new Map(SPORT_DEFS.map((s) => [s.sportType, s]))

    expect(bySport.get('skiing')?.lifecycle).toBe('broad_fallback')
    expect(bySport.get('training')?.lifecycle).toBe('broad_fallback')
    expect(bySport.get('fitness_equipment')?.lifecycle).toBe('broad_fallback')
    expect(bySport.get('paddling')?.lifecycle).toBe('broad_fallback')
    expect(bySport.get('canoeing')?.lifecycle).toBe('canonical')
  })

  it('exposes generated sport trait metadata', () => {
    expect(findSport('surfing')?.traits.equipmentFamilies).toEqual(['board'])
    expect(findSport('stand_up_paddleboarding')?.traits.equipmentFamilies).toEqual(['board', 'paddle'])
    expect(findSport('swimming')?.traits.environments).toEqual(['water'])
    expect(findSport('swimming')?.traits.movementFamilies).toEqual(['swim'])
    expect(findSport('tennis')?.traits.equipmentFamilies).toEqual(['racket', 'ball'])
    expect(findSport('golf')?.traits.equipmentFamilies).toEqual(['ball', 'stick'])
    expect(findSport('golf')?.traits.equipmentFamilies).not.toContain('racket')
    expect(findSubtype('racket', 'badminton')?.traits.equipmentFamilies).toEqual(['racket'])
    expect(findSubtype('racket', 'badminton')?.traits.equipmentFamilies).not.toContain('ball')
    expect(findSubtype('fitness_equipment', 'pilates')?.traits.movementFamilies).toEqual(['mind_body'])
    expect(findSubtype('fitness_equipment', 'elliptical')?.traits.activityForm).toBe('single')
    expect(findSubtype('running', 'treadmill')?.traits.environments).toEqual(['indoor'])
    expect(findSubtype('rock_climbing', 'indoor_climbing')?.traits.environments).toEqual(['indoor'])
    expect(findSubtype('rock_climbing', 'bouldering')?.traits.environments).toEqual(['land', 'indoor'])
  })

  it('keeps system taxonomy values selectable while exposing system traits', () => {
    const parentValues = selectableParentSportOptions().map((opt) => opt.value)

    for (const sportType of ['generic', 'unknown']) {
      expect(findSport(sportType)?.traits.activityForm).toBe('system')
      expect(findSport(sportType)?.traits.environments).toEqual(['generic'])
      expect(parentValues).toContain(sportType)
    }
  })

  it('keeps broad fallback parents out of selectable creation options while preserving selectable subtypes', () => {
    const parentValues = selectableParentSportOptions().map((opt) => opt.value)
    const tupleValues = selectableSportTupleOptions().map((opt) => `${opt.value}/${opt.subType ?? ''}`)

    expect(parentValues).not.toEqual(expect.arrayContaining(['skiing', 'training', 'fitness_equipment', 'paddling']))
    expect(tupleValues).toContain('fitness_equipment/stair_climbing')
    expect(tupleValues).toContain('training/flexibility_training')
    expect(tupleValues).not.toContain('training/strength_training')
    expect(tupleValues).not.toContain('training/pilates')
  })

  it('keeps normal broad parent defaults selectable as tuple options', () => {
    const tupleValues = selectableSportTupleOptions().map((opt) => `${opt.value}/${opt.subType ?? ''}`)

    expect(tupleValues).toContain('running/')
    expect(tupleValues).toContain('cycling/')
    expect(tupleValues).toContain('swimming/')
  })

  it('keeps broad fallback parent tuples out of selectable tuple options', () => {
    const tupleValues = selectableSportTupleOptions().map((opt) => `${opt.value}/${opt.subType ?? ''}`)

    expect(tupleValues).not.toContain('paddling/')
    expect(tupleValues).not.toContain('skiing/')
    expect(tupleValues).not.toContain('training/')
    expect(tupleValues).not.toContain('fitness_equipment/')
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
    expect(values).toContain('wheelchair_push/')
    expect(values).toContain('wheelchair_push/walk_speed')
    expect(values).toContain('wheelchair_push/run_speed')
    expect(values).not.toContain('mountain_biking/')
    expect(values).not.toContain('trail_running/')
    expect(values).not.toContain('wheelchair_push_walk/')
    expect(values).not.toContain('wheelchair_push_run/')
    expect(values).not.toContain('skiing/')
    expect(values).not.toContain('paddling/')
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
      'wheelchair_push/',
      'wheelchair_push/walk_speed',
      'wheelchair_push/run_speed',
      'cycling/',
      'cycling/mountain',
      'cycling/gravel_cycling',
      'cycling/road',
      'inline_skating/',
      'skateboarding/',
      'alpine_skiing/',
      'backcountry_skiing/',
      'cross_country_skiing/',
      'snowboarding/',
      'ice_skating/',
      'snowmobiling/',
      'sledding/',
      'swimming/',
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

function findSport(sportType: string) {
  return SPORT_DEFS.find((s) => s.sportType === sportType)
}

function findSubtype(sportType: string, subType: string) {
  return findSport(sportType)?.subTypes.find((s) => s.subType === subType)
}
