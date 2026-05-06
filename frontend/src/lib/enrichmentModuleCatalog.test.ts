import { describe, expect, it } from 'vitest'
import {
  ANALYSIS_MODULES,
  reprocessScopeForModule,
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
  })

  it('keeps route embeddings explicitly excluded from manual reprocess', () => {
    expect(reprocessScopeForModule('embeddings')).toBeNull()
  })
})
