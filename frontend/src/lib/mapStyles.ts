// frontend/src/lib/mapStyles.ts
//
// During Phase 1 of the map-layer refactor, this file exports BOTH the new
// buildLayerStyle() API AND a Deprecation-Shim that mimics the old BASE_STYLES
// constant. The shim is removed in the final commit of the phase (Task 44).
//
// New consumers MUST use useMapLayers() — never import from this file directly.
import type { MapLayer } from '@/api/mapLayers'

export interface Capabilities {
  offline_map?: boolean
  [k: string]: boolean | undefined
}

/** Convert backend opacity (0–100, integer) to MapLibre paint opacity (0.0–1.0). */
export function opacityPercentToFloat(n: number): number {
  return Math.max(0, Math.min(100, n)) / 100
}

/** Expand `{switch:a,b,c}` to a `tiles` array, returning N URLs. */
export function expandSwitch(template: string): string[] {
  const m = template.match(/\{switch:([a-z0-9](?:,[a-z0-9])*)\}/)
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

// ---------------------------------------------------------------------------
// Deprecation-Shim (removed in Task 44)
// ---------------------------------------------------------------------------

/** @deprecated use useMapLayers(). */
export type BaseStyle = string

/** @deprecated populated lazily on first access; replaced by useMapLayers in Phase 1 final commit. */
export let BASE_STYLES: Record<string, MapLayer> = {}

/** @deprecated use buildLayerStyle(). */
export function buildRasterStyle(key: string, caps: Capabilities): unknown {
  const layer = BASE_STYLES[key]
  if (!layer) throw new Error(`unknown style key: ${key}`)
  return buildLayerStyle(layer, caps)
}

/**
 * @deprecated callers should subscribe via useMapLayers(); shim setter for hook.
 * Called once per fetch inside useMapLayers queryFn (not in useEffect) to keep
 * a single update point during the mixed-code Phase 1 transition.
 */
export function _setBaseStylesShim(layers: MapLayer[]) {
  BASE_STYLES = Object.fromEntries(
    layers.filter((l) => l.role === 'base').map((l) => [l.key, l]),
  )
}

/** @deprecated. */
export const RASTER_STYLES = {
  get all(): MapLayer[] {
    return Object.values(BASE_STYLES)
  },
}
