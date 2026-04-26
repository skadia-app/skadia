import type { MapLayer } from '@/api/mapLayers'

export interface StackDiffOp {
  op: 'add' | 'remove' | 'replace'
  layer: MapLayer
}

/**
 * Compute the minimal set of MapLibre source/layer mutations needed to bring
 * an existing stack from `prev` to `next`.
 *
 * Identity is the layer key. Any visible-property change (URL, attribution,
 * zoom range, default opacity, enabled, sort_order) produces a 'replace' so
 * MapLibre re-fetches tiles or re-applies paint. Pure metadata fields
 * (id, created_at, is_system, etc.) don't trigger a replace.
 */
const VISIBLE_FIELDS = [
  'url_template',
  'style_url',
  'attribution',
  'min_zoom',
  'max_zoom',
  'default_opacity',
  'enabled',
  'sort_order',
] as const

type VisibleField = (typeof VISIBLE_FIELDS)[number]

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
    const changed = VISIBLE_FIELDS.some(
      (f: VisibleField) => old[f] !== layer[f],
    )
    if (changed) ops.push({ op: 'replace', layer })
  }

  for (const [key, layer] of prevByKey) {
    if (!nextByKey.has(key)) ops.push({ op: 'remove', layer })
  }

  return ops
}
