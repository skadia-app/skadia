import { describe, it, expect } from 'vitest'
import { SURFACE_GROUPS, SURFACE_COLORS, aggregateSurfaces } from './surfaceConstants'

describe('SURFACE_GROUPS — new mappings', () => {
  it('splits Schotter into fein/grob', () => {
    expect(SURFACE_GROUPS.compacted).toBe('Schotter (fein)')
    expect(SURFACE_GROUPS.gravel).toBe('Schotter (grob)')
    expect(SURFACE_GROUPS.fine_gravel).toBe('Schotter (grob)')
    expect(SURFACE_GROUPS.pebblestone).toBe('Schotter (grob)')
  })
  it('maps new OSM keys', () => {
    expect(SURFACE_GROUPS.chipseal).toBe('Asphalt')
    expect(SURFACE_GROUPS.brick).toBe('Pflaster')
    expect(SURFACE_GROUPS.stepping_stones).toBe('Naturweg')
    expect(SURFACE_GROUPS.stone).toBe('Fels')
    expect(SURFACE_GROUPS.decoturf).toBe('Sportbelag')
    expect(SURFACE_GROUPS.carpet).toBe('Sportbelag')
    expect(SURFACE_GROUPS.acrylic).toBe('Sportbelag')
  })
})

describe('SURFACE_COLORS — new entries', () => {
  for (const key of ['Schotter (fein)', 'Schotter (grob)', 'Naturweg (fest)', 'Naturweg (weich)', 'Wechselnd']) {
    it(`has a color for '${key}'`, () => {
      expect(SURFACE_COLORS[key]).toMatch(/^#[0-9a-fA-F]{6}$/)
    })
  }
})

describe('aggregateSurfaces — back-compat', () => {
  it('aggregates a mixed surface list', () => {
    const result = aggregateSurfaces([
      { surface_type: 'asphalt', length_m: 1000 },
      { surface_type: 'asphalt', length_m: 1000 },
      { surface_type: 'gravel', length_m: 500 },
      { surface_type: 'fine_gravel', length_m: 500 },
    ])
    const asphalt = result.find(s => s.surface_type === 'Asphalt')
    const schotter = result.find(s => s.surface_type === 'Schotter (grob)')
    expect(asphalt?.length_m).toBe(2000)
    expect(schotter?.length_m).toBe(1000)
  })
})
