import { describe, expect, it } from 'vitest'
import { buildLayerStyle, expandSwitch, opacityPercentToFloat } from './mapStyles'
import type { MapLayer } from '@/api/mapLayers'

const baseLayer = (overrides: Partial<MapLayer>): MapLayer => ({
  id: 1,
  key: 'osm',
  role: 'base',
  name: 'OSM',
  source_kind: 'raster_tiles',
  url_template: 'https://tile.example.org/{z}/{x}/{y}.png',
  default_opacity: 100,
  is_system: true,
  enabled: true,
  sort_order: 0,
  proxy_through_backend: false,
  created_at: '2026-04-26T00:00:00Z',
  updated_at: '2026-04-26T00:00:00Z',
  ...overrides,
})

describe('opacityPercentToFloat', () => {
  it('maps 0..100 to 0..1', () => {
    expect(opacityPercentToFloat(0)).toBe(0)
    expect(opacityPercentToFloat(50)).toBe(0.5)
    expect(opacityPercentToFloat(100)).toBe(1)
  })
  it('clamps out-of-range values', () => {
    expect(opacityPercentToFloat(-5)).toBe(0)
    expect(opacityPercentToFloat(150)).toBe(1)
  })
})

describe('expandSwitch', () => {
  it('returns single template when no switch', () => {
    expect(expandSwitch('https://t.example.org/{z}/{x}/{y}.png')).toEqual([
      'https://t.example.org/{z}/{x}/{y}.png',
    ])
  })
  it('expands a,b,c subdomains', () => {
    const out = expandSwitch('https://{switch:a,b,c}.tile.example.org/{z}/{x}/{y}.png')
    expect(out).toHaveLength(3)
    expect(out[0]).toBe('https://a.tile.example.org/{z}/{x}/{y}.png')
    expect(out[2]).toBe('https://c.tile.example.org/{z}/{x}/{y}.png')
  })
  it('expands multi-char tokens (matches backend reSwitch regex)', () => {
    // Backend reSwitch: [a-z0-9]+(?:,[a-z0-9]+)* — multi-char tokens must work
    const out = expandSwitch('https://tiles.example.org/{switch:q1,q2,q3}/{z}/{x}/{y}.png')
    expect(out).toHaveLength(3)
    expect(out[0]).toBe('https://tiles.example.org/q1/{z}/{x}/{y}.png')
    expect(out[1]).toBe('https://tiles.example.org/q2/{z}/{x}/{y}.png')
    expect(out[2]).toBe('https://tiles.example.org/q3/{z}/{x}/{y}.png')
  })
})

describe('buildLayerStyle', () => {
  it('builds raster_tiles style with attribution + minzoom', () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const style = buildLayerStyle(
      baseLayer({ min_zoom: 1, max_zoom: 18, attribution: '&copy; OSM' }),
      {},
    ) as any
    expect(style.sources.osm.tiles[0]).toContain('https://tile.example.org/')
    expect(style.sources.osm.attribution).toBe('&copy; OSM')
    expect(style.sources.osm.minzoom).toBe(1)
    expect(style.sources.osm.maxzoom).toBe(18)
  })

  it('omits minzoom/maxzoom when not set', () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const style = buildLayerStyle(baseLayer({}), {}) as any
    expect(style.sources.osm.minzoom).toBeUndefined()
    expect(style.sources.osm.maxzoom).toBeUndefined()
  })

  it('returns style URL for vector_style', () => {
    const style = buildLayerStyle(
      baseLayer({ source_kind: 'vector_style', url_template: undefined, style_url: '/style.json' }),
      { offline_map: true },
    )
    expect(style).toBe('/style.json')
  })

  it('returns protomaps sentinel for protomaps_offline', () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const style = buildLayerStyle(
      baseLayer({ source_kind: 'protomaps_offline', url_template: undefined }),
      {},
    ) as any
    expect(style.__protomaps_offline).toBe(true)
  })

  it('throws when requires_capability missing from caps', () => {
    expect(() =>
      buildLayerStyle(baseLayer({ requires_capability: 'offline_map' }), {}),
    ).toThrow(/capability_missing/)
  })

  it('passes when requires_capability is present and true in caps', () => {
    expect(() =>
      buildLayerStyle(baseLayer({ requires_capability: 'offline_map' }), { offline_map: true }),
    ).not.toThrow()
  })

  it('passes when requires_capability is available only in feature state', () => {
    expect(() =>
      buildLayerStyle(baseLayer({ requires_capability: 'offline_map' }), {
        features: {
          offline_map: { configured: true, available: true },
        },
      }),
    ).not.toThrow()
  })
})
