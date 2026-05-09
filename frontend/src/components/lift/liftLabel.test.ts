import { describe, it, expect } from 'vitest'
import { liftDisplayLabel, AERIALWAY_LABELS } from './liftLabel'
import type { LiftCandidate } from '@/api/types'

function makeCandidate(overrides: Partial<LiftCandidate>): LiftCandidate {
  return {
    id: 'cand-x',
    activity_id: 'act-x',
    started_at: '2026-01-01T09:00:00Z',
    ended_at: '2026-01-01T09:10:00Z',
    duration_s: 600,
    ascent_m: 200,
    descent_m: 5,
    distance_m: 1000,
    confidence: 'high',
    confirmed: false,
    rejected_by_user: false,
    source: 'osm',
    segment_effort_id: 'se-x',
    osm_name: null,
    osm_aerialway: null,
    ...overrides,
  }
}

describe('liftDisplayLabel', () => {
  it('returns empty for heuristic candidates regardless of OSM fields', () => {
    const c = makeCandidate({ source: 'heuristic', osm_name: 'example chairlift', osm_aerialway: 'chair_lift' })
    expect(liftDisplayLabel(c)).toEqual({})
  })

  it('returns name + type for whitelisted aerialway with name', () => {
    const c = makeCandidate({ osm_name: 'example chairlift', osm_aerialway: 'chair_lift' })
    expect(liftDisplayLabel(c)).toEqual({ name: 'example chairlift', type: 'Sessellift' })
  })

  it('uses Seilbahn (not Pendelbahn) for cable_car', () => {
    const c = makeCandidate({ osm_name: null, osm_aerialway: 'cable_car' })
    expect(liftDisplayLabel(c)).toEqual({ type: 'Seilbahn' })
  })

  it('returns only type for whitelisted aerialway without name', () => {
    const c = makeCandidate({ osm_name: null, osm_aerialway: 'gondola' })
    expect(liftDisplayLabel(c)).toEqual({ type: 'Gondelbahn' })
  })

  it('returns empty for goods (freight, never a passenger lift)', () => {
    const c = makeCandidate({ osm_name: 'Materialseilbahn Kraftwerk', osm_aerialway: 'goods' })
    expect(liftDisplayLabel(c)).toEqual({})
  })

  it('returns empty for unknown aerialway value (defensive whitelist)', () => {
    const c = makeCandidate({ osm_name: 'Mystery Lift', osm_aerialway: 'pylon' })
    expect(liftDisplayLabel(c)).toEqual({})
  })

  it('returns empty when OSM fields are NULL (segment join missed)', () => {
    const c = makeCandidate({ osm_name: null, osm_aerialway: null })
    expect(liftDisplayLabel(c)).toEqual({})
  })

  it('strips and ignores whitespace-only names', () => {
    const c = makeCandidate({ osm_name: '   ', osm_aerialway: 'chair_lift' })
    expect(liftDisplayLabel(c)).toEqual({ type: 'Sessellift' })
  })

  it('whitelist covers exactly the 11 passenger-relevant aerialway values', () => {
    expect(Object.keys(AERIALWAY_LABELS).sort()).toEqual(
      [
        'cable_car', 'chair_lift', 'drag_lift', 'gondola',
        'j-bar', 'magic_carpet', 'mixed_lift', 'platter',
        'rope_tow', 't-bar', 'zip_line',
      ].sort()
    )
  })
})
