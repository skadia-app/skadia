import { describe, expect, it } from 'vitest'
import { mapLayerStackDiff } from './mapLayerStackDiff'
import type { MapLayer } from '@/api/mapLayers'

const L = (key: string, urlTemplate: string): MapLayer => ({
  id: 1,
  key,
  role: 'overlay',
  name: key,
  source_kind: 'raster_tiles',
  url_template: urlTemplate,
  default_opacity: 100,
  is_system: false,
  enabled: true,
  sort_order: 0,
  proxy_through_backend: false,
  created_at: '',
  updated_at: '',
})

describe('mapLayerStackDiff', () => {
  it('detects add', () => {
    const ops = mapLayerStackDiff([], [L('a', 'https://t.example.org/{z}/{x}/{y}')])
    expect(ops).toEqual([{ op: 'add', layer: expect.objectContaining({ key: 'a' }) }])
  })

  it('detects remove', () => {
    const ops = mapLayerStackDiff([L('a', 'https://t.example.org/{z}/{x}/{y}')], [])
    expect(ops).toHaveLength(1)
    expect(ops[0].op).toBe('remove')
  })

  it('detects URL change as replace', () => {
    const a1 = L('a', 'https://t1.example.org/{z}/{x}/{y}')
    const a2 = L('a', 'https://t2.example.org/{z}/{x}/{y}')
    const ops = mapLayerStackDiff([a1], [a2])
    expect(ops).toHaveLength(1)
    expect(ops[0].op).toBe('replace')
  })

  it('treats identical layers as no-op', () => {
    const a = L('a', 'https://t.example.org/{z}/{x}/{y}')
    expect(mapLayerStackDiff([a], [a])).toHaveLength(0)
  })

  it('detects pure opacity change as paint-update (no tile refetch)', () => {
    const a1 = { ...L('a', 'https://t.example.org/{z}/{x}/{y}'), default_opacity: 100 }
    const a2 = { ...a1, default_opacity: 60 }
    const ops = mapLayerStackDiff([a1], [a2])
    expect(ops).toHaveLength(1)
    expect(ops[0].op).toBe('paint-update')
  })

  it('treats opacity change combined with URL change as replace (recreate wins)', () => {
    const a1 = { ...L('a', 'https://t1.example.org/{z}/{x}/{y}'), default_opacity: 100 }
    const a2 = { ...a1, url_template: 'https://t2.example.org/{z}/{x}/{y}', default_opacity: 60 }
    const ops = mapLayerStackDiff([a1], [a2])
    expect(ops).toHaveLength(1)
    expect(ops[0].op).toBe('replace')
  })

  it('detects min_zoom / max_zoom / sort_order / enabled changes as replace', () => {
    const a1 = L('a', 'https://t.example.org/{z}/{x}/{y}')
    expect(mapLayerStackDiff([a1], [{ ...a1, min_zoom: 5 }])[0].op).toBe('replace')
    expect(mapLayerStackDiff([a1], [{ ...a1, max_zoom: 18 }])[0].op).toBe('replace')
    expect(mapLayerStackDiff([a1], [{ ...a1, sort_order: 99 }])[0].op).toBe('replace')
    expect(mapLayerStackDiff([a1], [{ ...a1, enabled: false }])[0].op).toBe('replace')
  })

  it('pure metadata change (created_at) produces no op', () => {
    const a1 = L('a', 'https://t.example.org/{z}/{x}/{y}')
    const a2 = { ...a1, created_at: '2099-01-01T00:00:00Z' }
    expect(mapLayerStackDiff([a1], [a2])).toHaveLength(0)
  })

  it('handles multiple concurrent changes', () => {
    const a = L('a', 'https://a.example.org/{z}/{x}/{y}')
    const b = L('b', 'https://b.example.org/{z}/{x}/{y}')
    const b2 = { ...b, url_template: 'https://b2.example.org/{z}/{x}/{y}' }
    const c = L('c', 'https://c.example.org/{z}/{x}/{y}')
    const ops = mapLayerStackDiff([a, b], [b2, c])
    const opsByKey = Object.fromEntries(ops.map((o) => [o.layer.key, o.op]))
    expect(opsByKey['a']).toBe('remove')
    expect(opsByKey['b']).toBe('replace')
    expect(opsByKey['c']).toBe('add')
  })
})
