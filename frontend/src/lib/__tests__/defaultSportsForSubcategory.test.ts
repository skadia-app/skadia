import { describe, it, expect } from 'vitest'
import { defaultSportsForSubcategory } from '../gearSchemaHelpers'

describe('defaultSportsForSubcategory', () => {
  it('TestHelmetSkiSubcategoryReturnsSnowhelmSports', () => {
    // alpine_skiing used over the broad-fallback 'skiing' type (selectable=false in taxonomy)
    const result = defaultSportsForSubcategory('helmet', 'ski')
    expect(result.map(p => p.sport_type)).toEqual(['alpine_skiing', 'snowboarding'])
  })

  it('TestHelmetRoadBikeSubcategoryReturnsCycling', () => {
    const result = defaultSportsForSubcategory('helmet', 'road_bike')
    expect(result.map(p => p.sport_type)).toEqual(['cycling'])
  })

  it('TestHelmetMtbSubcategoryReturnsCycling', () => {
    const result = defaultSportsForSubcategory('helmet', 'mtb')
    expect(result.map(p => p.sport_type)).toEqual(['cycling'])
  })

  it('TestHelmetFullFaceDhSubcategoryReturnsCycling', () => {
    const result = defaultSportsForSubcategory('helmet', 'full_face_dh')
    expect(result.map(p => p.sport_type)).toEqual(['cycling'])
  })

  it('TestHelmetClimbSubcategoryReturnsRockClimbing', () => {
    const result = defaultSportsForSubcategory('helmet', 'climb')
    expect(result.map(p => p.sport_type)).toEqual(['rock_climbing'])
  })

  it('TestHelmetEquestrianSubcategoryReturnsHorsebackRiding', () => {
    const result = defaultSportsForSubcategory('helmet', 'equestrian')
    expect(result.map(p => p.sport_type)).toEqual(['horseback_riding'])
  })

  it('TestGogglesSwimSubcategoryReturnsSwimming', () => {
    const result = defaultSportsForSubcategory('goggles', 'swim')
    expect(result.map(p => p.sport_type)).toEqual(['swimming'])
  })

  it('TestGogglesSkiSubcategoryReturnsSkiSports', () => {
    const result = defaultSportsForSubcategory('goggles', 'ski')
    expect(result.map(p => p.sport_type)).toEqual(['alpine_skiing', 'snowboarding'])
  })

  it('TestGogglesDhSubcategoryReturnsCycling', () => {
    const result = defaultSportsForSubcategory('goggles', 'dh')
    expect(result.map(p => p.sport_type)).toEqual(['cycling'])
  })

  it('TestGogglesMxSubcategoryReturnsMotorcycling', () => {
    const result = defaultSportsForSubcategory('goggles', 'mx')
    expect(result.map(p => p.sport_type)).toEqual(['motorcycling'])
  })

  it('TestGogglesSunSubcategoryReturnsEmpty', () => {
    const result = defaultSportsForSubcategory('goggles', 'sun')
    expect(result).toEqual([])
  })

  it('TestApparelJerseyCyclingSubcategoryReturnsCycling', () => {
    const result = defaultSportsForSubcategory('apparel.jersey', 'cycling')
    expect(result.map(p => p.sport_type)).toEqual(['cycling'])
  })

  it('TestApparelPantsRunningSubcategoryReturnsRunning', () => {
    const result = defaultSportsForSubcategory('apparel.pants', 'running')
    expect(result.map(p => p.sport_type)).toEqual(['running'])
  })

  it('TestApparelGlovesSkiSubcategoryReturnsAlpineSkiing', () => {
    const result = defaultSportsForSubcategory('apparel.gloves', 'ski')
    // ski apparel covers both alpine_skiing and snowboarding (mirrors HELMET_MAP/GOGGLES_MAP)
    expect(result.map(p => p.sport_type)).toEqual(['alpine_skiing', 'snowboarding'])
  })

  it('TestApparelJacketSkiSubcategoryIncludesSnowboarding', () => {
    // Re-Critic #3: APPAREL_MAP['ski'] must include snowboarding, not only alpine_skiing.
    // Helmet and goggles already include both — apparel must be consistent.
    const result = defaultSportsForSubcategory('apparel.jacket', 'ski')
    expect(result.map(p => p.sport_type)).toContain('snowboarding')
    expect(result.map(p => p.sport_type)).toContain('alpine_skiing')
  })

  it('TestApparelSocksClimbingSubcategoryReturnsRockClimbing', () => {
    const result = defaultSportsForSubcategory('apparel.socks', 'climbing')
    expect(result.map(p => p.sport_type)).toEqual(['rock_climbing'])
  })

  it('TestApparelJacketSwimmingSubcategoryReturnsSwimming', () => {
    const result = defaultSportsForSubcategory('apparel.jacket', 'swimming')
    expect(result.map(p => p.sport_type)).toEqual(['swimming'])
  })

  it('TestApparelShortsHikingSubcategoryReturnsHiking', () => {
    const result = defaultSportsForSubcategory('apparel.shorts', 'hiking')
    expect(result.map(p => p.sport_type)).toEqual(['hiking'])
  })

  it('TestApparelGeneralSubcategoryReturnsEmpty', () => {
    const result = defaultSportsForSubcategory('apparel.pants', 'general')
    expect(result).toEqual([])
  })

  it('TestUnknownSubcategoryReturnsEmpty', () => {
    const result = defaultSportsForSubcategory('helmet', 'unicycle')
    expect(result).toEqual([])
  })

  it('TestUnknownTypeKeyReturnsEmpty', () => {
    const result = defaultSportsForSubcategory('other', 'ski')
    expect(result).toEqual([])
  })

  it('TestUndefinedSubcategoryReturnsEmpty', () => {
    const result = defaultSportsForSubcategory('helmet', undefined)
    expect(result).toEqual([])
  })
})
