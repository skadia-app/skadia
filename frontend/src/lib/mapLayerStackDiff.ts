import type { MapLayer } from '@/api/mapLayers'

export interface StackDiffOp {
  op: 'add' | 'remove' | 'replace' | 'paint-update'
  layer: MapLayer
}

/**
 * Compute the minimal set of MapLibre source/layer mutations needed to bring
 * an existing stack from `prev` to `next`.
 *
 * Identity is the layer key. URL/zoom/enabled/sort_order changes force a
 * 'replace' (full source+layer recreate). A pure default_opacity change emits
 * 'paint-update' instead so the caller can call setPaintProperty without
 * re-fetching tiles — this matters for the user-overlay opacity slider where
 * each step would otherwise tear down and re-add the raster source.
 * Pure metadata fields (id, created_at, is_system, etc.) don't trigger any op.
 */
const RECREATE_FIELDS = [
  'url_template',
  'style_url',
  'attribution',
  'min_zoom',
  'max_zoom',
  'enabled',
  'sort_order',
] as const

type RecreateField = (typeof RECREATE_FIELDS)[number]

export function mapLayerStackDiff(prev: MapLayer[], next: MapLayer[]): StackDiffOp[] {
  const ops: StackDiffOp[] = []
  const prevByKey = new Map(prev.map((l) => [l.key, l]))
  const nextByKey = new Map(next.map((l) => [l.key, l]))

  for (const [key, layer] of nextByKey) {
    const old = prevByKey.get(key)
    if (!old) {
      ops.push({ op: 'add', layer })
      continue
    }
    const recreate = RECREATE_FIELDS.some(
      (f: RecreateField) => old[f] !== layer[f],
    )
    if (recreate) {
      ops.push({ op: 'replace', layer })
      continue
    }
    if (old.default_opacity !== layer.default_opacity) {
      ops.push({ op: 'paint-update', layer })
    }
  }

  for (const [key, layer] of prevByKey) {
    if (!nextByKey.has(key)) ops.push({ op: 'remove', layer })
  }

  return ops
}
