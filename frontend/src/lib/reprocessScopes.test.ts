import { describe, expect, it } from 'vitest'
import {
  DEFAULT_SCOPES,
  GEO_SCOPE_KEYS,
  GEO_SCOPE_LABELS,
  REPROCESS_SCOPES,
  scopeLabel,
  toBackendScope,
} from './reprocessScopes'

describe('reprocess scope catalog', () => {
  it('exposes stream-based analysis modules as selectable reprocess scopes', () => {
    const labels = REPROCESS_SCOPES.map(([, label]) => label)

    expect(labels).toContain('Anstiege')
    expect(labels).toContain('Stopps & Einkehr')
    expect(labels).toContain('Intervalltraining')
  })

  it('keeps stops out of geo scopes and splits settlements from borders', () => {
    expect(GEO_SCOPE_KEYS).not.toContain('stops')
    expect(GEO_SCOPE_KEYS).toContain('settlements')
    expect(GEO_SCOPE_KEYS).toContain('borders')
    expect(GEO_SCOPE_LABELS.settlements).toBe('Ortschaften')
    expect(GEO_SCOPE_LABELS.borders).toBe('Grenzüberquerungen')
  })

  it('does not show route embeddings until the backend has an active implementation', () => {
    const allKeys = [
      ...REPROCESS_SCOPES.map(([key]) => key),
      ...GEO_SCOPE_KEYS,
    ]

    expect(allKeys).not.toContain('embeddings')
  })

  it('serializes selected analysis scopes directly for the backend', () => {
    const selected = new Set(['ascents', 'stops', 'intervals', 'settlements'])

    expect(toBackendScope(selected)).toEqual(['ascents', 'stops', 'intervals', 'settlements'])
    expect(scopeLabel('ascents')).toBe('Anstiege')
    expect(scopeLabel('settlements')).toBe('Ortschaften')
  })

  it('exposes auto title recompute as a default metadata scope', () => {
    const keys = REPROCESS_SCOPES.map(([key]) => key)

    expect(keys).toContain('auto_title')
    expect(DEFAULT_SCOPES).toContain('auto_title')
    expect(scopeLabel('auto_title')).toBe('Automatische Benennung')
    expect(toBackendScope(new Set(['auto_title']))).toEqual(['auto_title'])
  })
})
