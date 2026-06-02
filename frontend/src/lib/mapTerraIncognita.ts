import maplibregl from 'maplibre-gl'

// ── Terra Incognita: Canvas Fog Overlay (Approach B) ─────────────────────────
// Instead of MapLibre line-blur (zoom-dependent, spike artifacts at vertices):
// gradient fog of war: 5 stacked donut fill layers with additive alpha compositing.
// PostGIS returns 5 concentric ring polygons (innermost → outermost).
// Each is rendered as a "world minus ring" donut. Because the donuts overlap,
// their opacities add up automatically via WebGL compositing → soft gradient.
//
// Performance: fully GPU-backed, no canvas, no re-render on pan/zoom.

const TERRA_RING_COUNT = 5
const TERRA_INCOGNITA_LAYER_IDS = Array.from(
  { length: TERRA_RING_COUNT },
  (_, i) => `terra-fog-ring-${i}`,
)

// Per-layer opacity for additive alpha compositing.
// Computed so each zone's total opacity matches a soft gradient:
//   Zone ring[0]→ring[1] (innermost):  ~0.10  (light haze)
//   Zone ring[1]→ring[2]:              ~0.25
//   Zone ring[2]→ring[3]:              ~0.50  (configured radius ≈ 50%)
//   Zone ring[3]→ring[4]:              ~0.72
//   Zone outside ring[4] (all 5):      ~0.88  (full fog)
const RING_LAYER_OPACITIES = [0.10, 0.17, 0.33, 0.44, 0.57]

const FOG_COLOR = '#d0daea'

// Halo layer configuration: [ringIdx, lineWidth, lineBlur, targetOpacity]
// Three outer ring boundaries get blurred white lines for the cloudy edge effect.
const HALO_CONFIG: [number, number, number, number][] = [
  [2, 16, 10, 0.30], // configured radius (innermost halo)
  [3, 28, 18, 0.22], // middle boundary
  [4, 44, 28, 0.16], // outermost boundary (most diffuse transition)
]

// Module-global state
let terraAnimRafId: number | null = null

export function removeTerraIncognitaLayers(map: maplibregl.Map) {
  if (terraAnimRafId !== null) {
    cancelAnimationFrame(terraAnimRafId)
    terraAnimRafId = null
  }
  for (const id of TERRA_INCOGNITA_LAYER_IDS) {
    if (map.getLayer(id)) map.removeLayer(id)
  }
  for (let i = 0; i < TERRA_RING_COUNT; i++) {
    const srcId = `terra-fog-src-${i}`
    if (map.getSource(srcId)) map.removeSource(srcId)
  }
  // Clean up halo layers + sources
  for (const [ringIdx] of HALO_CONFIG) {
    const haloLayerId = `terra-fog-halo-${ringIdx}`
    const haloSrcId = `terra-fog-halo-src-${ringIdx}`
    if (map.getLayer(haloLayerId)) map.removeLayer(haloLayerId)
    if (map.getSource(haloSrcId)) map.removeSource(haloSrcId)
  }
}

/**
 * Builds a donut GeoJSON: world rectangle with the ring geometry as a hole.
 * PostGIS returns outer rings in CCW order (GeoJSON standard) → reverse for CW holes.
 */
export function buildDonutFeature(ringGeometry: GeoJSON.Geometry): GeoJSON.FeatureCollection {
  const worldRing: GeoJSON.Position[] = [
    [-180, -85.051], [180, -85.051], [180, 85.051], [-180, 85.051], [-180, -85.051],
  ]
  const reverseRing = (r: GeoJSON.Position[]): GeoJSON.Position[] => [...r].reverse()
  const holes: GeoJSON.Position[][] = []

  if (ringGeometry.type === 'Polygon' && ringGeometry.coordinates.length > 0
      && ringGeometry.coordinates[0].length > 0) {
    holes.push(reverseRing(ringGeometry.coordinates[0]))
  } else if (ringGeometry.type === 'MultiPolygon') {
    for (const poly of ringGeometry.coordinates) {
      if (poly[0] && poly[0].length > 0) holes.push(reverseRing(poly[0]))
    }
  }

  // No holes → empty feature (no fog for this ring)
  if (holes.length === 0) {
    return { type: 'FeatureCollection', features: [] }
  }

  return {
    type: 'FeatureCollection',
    features: [{
      type: 'Feature',
      geometry: { type: 'Polygon', coordinates: [worldRing, ...holes] },
      properties: {},
    }],
  }
}

/**
 * Adds 5 stacked fill layers + 3 halo line layers for fog of war.
 *
 * Fill layers (fog body):
 *   Each layer is a donut (world minus ring[i]). The donuts overlap —
 *   additive alpha compositing in WebGL creates the gradient.
 *   rings[0] = innermost (-0.3R, smallest hole → most coverage)
 *   rings[4] = outermost (+0.6R, largest hole → least coverage)
 *
 * Halo layers (soft edges):
 *   Thick white lines with line-blur on the 3 outer ring boundaries.
 *   GPU shader effect — no canvas performance issues.
 *   Creates the characteristic cloudy edge effect.
 */
export function addTerraIncognitaLayers(map: maplibregl.Map, rings: GeoJSON.Geometry[]) {
  removeTerraIncognitaLayers(map)

  // Add fill layers from outside (ring[4]) to inside (ring[0]).
  for (let i = TERRA_RING_COUNT - 1; i >= 0; i--) {
    const ring = rings[i]
    if (!ring) continue
    const srcId = `terra-fog-src-${i}`
    const layerId = `terra-fog-ring-${i}`
    const donut = buildDonutFeature(ring)

    map.addSource(srcId, { type: 'geojson', data: donut })
    map.addLayer({
      id: layerId,
      type: 'fill',
      source: srcId,
      paint: {
        'fill-color': FOG_COLOR,
        'fill-opacity': 0,
        'fill-antialias': true,
      },
    })
  }

  // Halo layers: thick blurred lines on the outer ring boundaries.
  // Drawn above the fill layers → soft, cloudy outer edges.
  for (const [ringIdx, lineWidth, lineBlur] of HALO_CONFIG) {
    const ring = rings[ringIdx]
    if (!ring) continue
    const haloSrcId = `terra-fog-halo-src-${ringIdx}`
    const haloLayerId = `terra-fog-halo-${ringIdx}`
    map.addSource(haloSrcId, {
      type: 'geojson',
      data: { type: 'FeatureCollection', features: [{ type: 'Feature', geometry: ring, properties: {} }] },
    })
    map.addLayer({
      id: haloLayerId,
      type: 'line',
      source: haloSrcId,
      paint: {
        'line-color': '#ffffff',
        'line-width': lineWidth,
        'line-blur': lineBlur,
        'line-opacity': 0,
      },
    })
  }

  // Fade-in animation (easeInOutQuad, 1.8s) — fill + halo together
  const DURATION = 1800
  const ease = (t: number) => t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2
  const startTs = performance.now()
  function animate(ts: number) {
    const p = Math.min((ts - startTs) / DURATION, 1)
    const e = ease(p)
    for (let i = 0; i < TERRA_RING_COUNT; i++) {
      const layerId = `terra-fog-ring-${i}`
      if (map.getLayer(layerId)) {
        const targetOpacity = RING_LAYER_OPACITIES[TERRA_RING_COUNT - 1 - i]
        map.setPaintProperty(layerId, 'fill-opacity', e * targetOpacity)
      }
    }
    for (const [ringIdx, , , targetOpacity] of HALO_CONFIG) {
      const haloLayerId = `terra-fog-halo-${ringIdx}`
      if (map.getLayer(haloLayerId)) {
        map.setPaintProperty(haloLayerId, 'line-opacity', e * targetOpacity)
      }
    }
    if (p < 1) terraAnimRafId = requestAnimationFrame(animate)
    else terraAnimRafId = null
  }
  terraAnimRafId = requestAnimationFrame(animate)
}
