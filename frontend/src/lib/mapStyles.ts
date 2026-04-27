// frontend/src/lib/mapStyles.ts
//
// Exports the buildLayerStyle() API for converting a map_layers DB row into a
// MapLibre StyleSpecification. Consumers MUST use useMapLayers() — never
// import MapLayer objects from outside that hook.
import type { MapLayer } from '@/api/mapLayers'
// Single source-of-truth: Capabilities is defined in routes.ts with an open
// index signature so future backend capability keys compile without casting.
export type { Capabilities } from '@/api/routes'
import type { Capabilities } from '@/api/routes'

/** Convert backend opacity (0–100, integer) to MapLibre paint opacity (0.0–1.0). */
export function opacityPercentToFloat(n: number): number {
  return Math.max(0, Math.min(100, n)) / 100
}

/** Expand `{switch:a,b,c}` to a `tiles` array, returning N URLs. */
export function expandSwitch(template: string): string[] {
  const m = template.match(/\{switch:([a-z0-9]+(?:,[a-z0-9]+)*)\}/)
  if (!m) return [template]
  const tokens = m[1].split(',')
  return tokens.map((t) => template.replace(m[0], t))
}

/**
 * Build a MapLibre StyleSpec for a given map_layers row.
 * Returns either a Style URL string (for vector_style) or an inline StyleSpec
 * object (for raster_tiles / protomaps_offline).
 */
export function buildLayerStyle(layer: MapLayer, caps: Capabilities): unknown {
  if (layer.requires_capability && !caps[layer.requires_capability]) {
    throw new Error(`capability_missing:${layer.requires_capability}`)
  }
  switch (layer.source_kind) {
    case 'raster_tiles':
      return {
        version: 8,
        sources: {
          [layer.key]: {
            type: 'raster',
            tiles: expandSwitch(layer.url_template!),
            tileSize: 256,
            attribution: layer.attribution ?? '',
            ...(layer.min_zoom != null && { minzoom: layer.min_zoom }),
            ...(layer.max_zoom != null && { maxzoom: layer.max_zoom }),
          },
        },
        layers: [{ id: layer.key, type: 'raster', source: layer.key }],
      }
    case 'vector_style':
      // MapLibre accepts a URL string as a style; consumer must cast to StyleSpecification
      return layer.style_url!
    case 'protomaps_offline':
      // Built client-side via @protomaps/basemaps; consumer pulls the local PMTiles file.
      // Return a sentinel that MapPage recognizes and handles.
      return { __protomaps_offline: true, attribution: layer.attribution ?? '' }
  }
}

/** String key identifying a base layer (matches map_layers.key). */
export type BaseStyle = string
