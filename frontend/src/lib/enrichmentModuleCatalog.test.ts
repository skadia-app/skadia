import { describe, expect, it } from 'vitest'
import {
  ANALYSIS_MODULES,
  MODULE_API_FIELD,
  reprocessScopeForModule,
  scopeRequiresML,
} from './enrichmentModuleCatalog'

describe('enrichment module catalog', () => {
  it('requires every automatic analysis setting to declare a reprocess policy', () => {
    for (const module of ANALYSIS_MODULES) {
      expect(module.reprocess).toBeDefined()
    }
  })

  it('maps automatic analyses to the scopes used by status tiles and checkboxes', () => {
    expect(reprocessScopeForModule('ascents')).toEqual(['ascents'])
    expect(reprocessScopeForModule('stops')).toEqual(['stops'])
    expect(reprocessScopeForModule('intervals')).toEqual(['intervals'])
    expect(reprocessScopeForModule('settlements')).toEqual(['settlements'])
    expect(reprocessScopeForModule('borders')).toEqual(['borders'])
    expect(reprocessScopeForModule('lap_detection')).toEqual(['segments'])
    expect(reprocessScopeForModule('auto_naming')).toEqual(['auto_title'])
  })

  it('keeps route embeddings explicitly excluded from manual reprocess', () => {
    expect(reprocessScopeForModule('embeddings')).toBeNull()
  })

  // ── lift sub-toggles ──────────────────────────────────────────────────────

  it('includes lifts_osm and lifts_heuristic modules with parentName lifts', () => {
    const osmModule = ANALYSIS_MODULES.find(m => m.name === 'lifts_osm')
    const heuristicModule = ANALYSIS_MODULES.find(m => m.name === 'lifts_heuristic')
    expect(osmModule).toBeDefined()
    expect(heuristicModule).toBeDefined()
    expect(osmModule?.parentName).toBe('lifts')
    expect(heuristicModule?.parentName).toBe('lifts')
  })

  it('sub-toggles have inherited reprocess policy', () => {
    const osmModule = ANALYSIS_MODULES.find(m => m.name === 'lifts_osm')
    const heuristicModule = ANALYSIS_MODULES.find(m => m.name === 'lifts_heuristic')
    expect(osmModule?.reprocess.type).toBe('inherited')
    expect(heuristicModule?.reprocess.type).toBe('inherited')
  })

  it('reprocessScopeForModule returns null for sub-toggles', () => {
    expect(reprocessScopeForModule('lifts_osm')).toBeNull()
    expect(reprocessScopeForModule('lifts_heuristic')).toBeNull()
  })

  it('reprocessScopeForModule for lifts parent still returns [lifts] (not duplicated)', () => {
    expect(reprocessScopeForModule('lifts')).toEqual(['lifts'])
  })

  it('scopeRequiresML is unaffected by sub-toggle entries', () => {
    expect(scopeRequiresML('lifts')).toBe(false)
    expect(scopeRequiresML('lifts_osm')).toBe(false)
    expect(scopeRequiresML('lifts_heuristic')).toBe(false)
  })

  it('MODULE_API_FIELD maps lifts_osm to enable_lift_detection_osm', () => {
    expect(MODULE_API_FIELD['lifts_osm']).toBe('enable_lift_detection_osm')
  })

  it('MODULE_API_FIELD maps lifts_heuristic to enable_lift_detection_heuristic', () => {
    expect(MODULE_API_FIELD['lifts_heuristic']).toBe('enable_lift_detection_heuristic')
  })

  // ── regression: existing modules untouched ────────────────────────────────

  it('existing standalone modules still return their scopes (regression)', () => {
    expect(reprocessScopeForModule('summits')).toEqual(['summits'])
    expect(reprocessScopeForModule('forests')).toEqual(['forests'])
    expect(reprocessScopeForModule('water')).toEqual(['water'])
    expect(reprocessScopeForModule('surfaces')).toEqual(['surfaces'])
    expect(reprocessScopeForModule('lifts')).toEqual(['lifts'])
  })

  it('top-level modules have no parentName', () => {
    const topLevel = ANALYSIS_MODULES.filter(m => !m.parentName)
    expect(topLevel.length).toBeGreaterThan(0)
    // The existing 13 modules before sub-toggles were all top-level
    for (const m of topLevel) {
      expect(m.parentName).toBeUndefined()
    }
  })
})
