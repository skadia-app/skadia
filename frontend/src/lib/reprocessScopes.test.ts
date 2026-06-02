import { describe, expect, it } from 'vitest'
import {
  DEFAULT_SCOPES,
  GEO_SCOPE_KEYS,
  GEO_SCOPE_LABELS,
  REPROCESS_SCOPES,
  availableReprocessScopeKeys,
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

  it('does not expose unknown backend scope keys as UI labels', () => {
    expect(scopeLabel('private_backend_scope')).toBe('Unbekannter Umfang')
  })

  it('exposes auto title recompute as a default metadata scope', () => {
    const keys = REPROCESS_SCOPES.map(([key]) => key)

    expect(keys).toContain('auto_title')
    expect(DEFAULT_SCOPES).toContain('auto_title')
    expect(scopeLabel('auto_title')).toBe('Automatische Benennung')
    expect(toBackendScope(new Set(['auto_title']))).toEqual(['auto_title'])
  })

  it('maps all scopes to the core-safe subset when full-only capabilities are unavailable', () => {
    const keys = availableReprocessScopeKeys({
      features: {
        geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
        surface_enrichment: { configured: false, available: false, reason: 'core_profile' },
        osm_lift_snap: { configured: false, available: false, reason: 'core_profile' },
      },
    })

    expect(keys).toContain('streams')
    expect(keys).toContain('auto_title')
    expect(keys).toContain('ascents')
    expect(keys).toContain('intervals')
    expect(keys).toContain('lifts')
    expect(keys).not.toContain('summits')
    expect(keys).not.toContain('stops')
    expect(keys).not.toContain('surfaces')
  })
})
