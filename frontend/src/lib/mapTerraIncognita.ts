import maplibregl from 'maplibre-gl'

// ── Terra Incognita: Canvas Fog Overlay (Ansatz B) ──────────────────────────
// Statt MapLibre line-blur (zoom-abhängig, Spike-Artifacts an Vertices):
// Gradient-Fog-of-War: 5 gestapelte Donut-Fill-Layer mit additiver Alpha-Compositing.
// PostGIS liefert 5 konzentrische Ring-Polygone (innermost → outermost).
// Jedes wird als "Welt minus Ring" Donut gerendert. Da die Donuts überlappen,
// addieren sich ihre Opacities automatisch per WebGL-Compositing → weicher Gradient.
//
// Performance: vollständig GPU, kein Canvas, kein Re-Render bei Pan/Zoom.

const TERRA_RING_COUNT = 5
const TERRA_INCOGNITA_LAYER_IDS = Array.from(
  { length: TERRA_RING_COUNT },
  (_, i) => `terra-fog-ring-${i}`,
)

// Per-Layer-Opacity für additive Alpha-Compositing.
// Berechnet so, dass die Zonen-Gesamtopacity einem weichen Gradient entspricht:
//   Zone ring[0]→ring[1] (innermost):  ~0.10  (leichter Dunst)
//   Zone ring[1]→ring[2]:              ~0.25
//   Zone ring[2]→ring[3]:              ~0.50  (konfigurierter Radius ≈ 50%)
//   Zone ring[3]→ring[4]:              ~0.72
//   Zone außerhalb ring[4] (all 5):    ~0.88  (voller Nebel)
const RING_LAYER_OPACITIES = [0.10, 0.17, 0.33, 0.44, 0.57]

const FOG_COLOR = '#d0daea'

// Halo-Layer-Konfiguration: [ringIdx, lineWidth, lineBlur, targetOpacity]
// Drei äußere Ring-Grenzen erhalten verschwommene weiße Linien für den wolkigen Rand-Effekt.
const HALO_CONFIG: [number, number, number, number][] = [
  [2, 16, 10, 0.30], // konfigurierter Radius (innerste Halo)
  [3, 28, 18, 0.22], // mittlere Grenze
  [4, 44, 28, 0.16], // äußerste Grenze (diffusester Übergang)
]

// Modul-globaler State
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
  // Halo-Layer + Sources aufräumen
  for (const [ringIdx] of HALO_CONFIG) {
    const haloLayerId = `terra-fog-halo-${ringIdx}`
    const haloSrcId = `terra-fog-halo-src-${ringIdx}`
    if (map.getLayer(haloLayerId)) map.removeLayer(haloLayerId)
    if (map.getSource(haloSrcId)) map.removeSource(haloSrcId)
  }
}

/**
 * Baut ein Donut-GeoJSON: Weltrechteck mit der Ring-Geometrie als Loch.
 * PostGIS liefert Außenringe in CCW (GeoJSON-Standard) → reversieren für CW-Holes.
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

  // Wenn keine Löcher → leeres Feature (kein Nebel für diesen Ring)
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
 * Fügt 5 gestapelte Fill-Layer + 3 Halo-Line-Layer für Fog-of-War hinzu.
 *
 * Fill-Layer (Nebel-Körper):
 *   Jeder Layer ist ein Donut (Welt minus Ring[i]). Die Donuts überlappen —
 *   additive Alpha-Compositing in WebGL erzeugt den Gradient.
 *   rings[0] = innerster (-0.3R, kleinstes Loch → meiste Abdeckung)
 *   rings[4] = äußerster (+0.6R, größtes Loch → wenigste Abdeckung)
 *
 * Halo-Layer (weiche Ränder):
 *   Dicke weiße Linien mit line-blur auf den 3 äußeren Ring-Grenzen.
 *   GPU-Shader-Effekt — keine Canvas-Performance-Probleme.
 *   Erzeugen den charakteristischen wolkigen Rand-Effekt.
 */
export function addTerraIncognitaLayers(map: maplibregl.Map, rings: GeoJSON.Geometry[]) {
  removeTerraIncognitaLayers(map)

  // Fill-Layer von außen (ring[4]) nach innen (ring[0]) hinzufügen.
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

  // Halo-Layer: dicke verschwommene Linien auf den äußeren Ring-Grenzen.
  // Werden über den Fill-Layern gezeichnet → weiche, wolkige Außenkanten.
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

  // Einblend-Animation (easeInOutQuad, 1.8s) — Fill + Halo gleichzeitig
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
