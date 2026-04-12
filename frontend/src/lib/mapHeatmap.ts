import type { Map as MaplibreMap } from 'maplibre-gl'

/** Tile base URL — routed through the Go API for authentication + per-user heatmap isolation. */
const MARTIN_BASE: string =
  (import.meta as unknown as { env: Record<string, string> }).env?.VITE_MARTIN_URL ??
  `${window.location.origin}/api/v1/tiles`

export const HEATMAP_SOURCE_ID = 'heatmap'

export const HEATMAP_LAYER_IDS = [
  'heatmap-glow-outer',
  'heatmap-glow-mid',
  'heatmap-base',
  'heatmap-bright',
  'heatmap-hot',
] as const

export const DEFAULT_HEATMAP_COLOR = '#F97316'

/** Liest die persistierte Heatmap-Farbe aus localStorage (gleicher Key wie MapPage). */
export function loadHeatmapColor(): string {
  try {
    const saved = localStorage.getItem('skadia-sport-colors')
    if (saved) {
      const parsed = JSON.parse(saved) as Record<string, unknown>
      if (typeof parsed.heatmapColor === 'string') return parsed.heatmapColor
    }
  } catch { /* ignorieren */ }
  return DEFAULT_HEATMAP_COLOR
}

/** Speichert die Heatmap-Farbe persistent (gleicher Key wie MapPage). */
export function saveHeatmapColor(hex: string): void {
  try {
    const saved = localStorage.getItem('skadia-sport-colors')
    const existing = saved ? (JSON.parse(saved) as Record<string, unknown>) : {}
    localStorage.setItem('skadia-sport-colors', JSON.stringify({ ...existing, heatmapColor: hex }))
  } catch { /* ignorieren */ }
}

/**
 * Erzeugt aus einer Hex-Farbe die 5 Glow-Stufen für den Heatmap-Modus.
 * Exakt dieselbe Logik wie in MapPage.tsx deriveHeatmapGlowColors.
 */
export function deriveHeatmapGlowColors(hex: string): [string, string, string, string, string] {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  const toHex = (n: number) => Math.round(Math.max(0, Math.min(255, n))).toString(16).padStart(2, '0')
  const rgb = (rr: number, gg: number, bb: number) => `#${toHex(rr)}${toHex(gg)}${toHex(bb)}`
  return [
    rgb(r * 0.15, g * 0.15, b * 0.15),
    rgb(r * 0.35, g * 0.35, b * 0.35),
    hex,
    rgb(r + (255 - r) * 0.35, g + (255 - g) * 0.25, b * 0.6),
    rgb(r + (255 - r) * 0.7, g + (255 - g) * 0.7, b + (255 - b) * 0.5),
  ]
}

/** Baut die Martin-Tile-URL für heatmap_lines für einen User. */
export function buildHeatmapTileUrl(userID: string): string {
  const params = new URLSearchParams({ user_id: userID, _v: '125' })
  return `${MARTIN_BASE}/heatmap_lines/{z}/{x}/{y}?${params.toString()}`
}

/**
 * Fügt die 5 Heatmap-Glow-Layer zur Karte hinzu.
 * @param beforeLayerId  Layer-ID vor der eingefügt wird — Heatmap liegt dann darunter.
 */
export function addHeatmapLayers(
  map: MaplibreMap,
  tileUrl: string,
  colorHex: string,
  beforeLayerId?: string,
  opacityScale = 1.0,
): void {
  if (map.getSource(HEATMAP_SOURCE_ID)) return // schon vorhanden

  // Only use beforeLayerId if the layer actually exists (avoids MapLibre error
  // when heatmap effect runs before track layers are added after style change)
  const safeBeforeId = beforeLayerId && map.getLayer(beforeLayerId) ? beforeLayerId : undefined

  map.addSource(HEATMAP_SOURCE_ID, {
    type: 'vector',
    tiles: [tileUrl],
    minzoom: 0,
    maxzoom: 14,
  })

  const [cOuter, cMid, cBase, cBright, cHot] = deriveHeatmapGlowColors(colorHex)

  const layers: Array<{ id: string; color: string; width: number; opacity: number; blur: number }> = [
    { id: 'heatmap-glow-outer', color: cOuter, width: 16, opacity: 0.10 * opacityScale, blur: 8 },
    { id: 'heatmap-glow-mid',   color: cMid,   width: 9,  opacity: 0.15 * opacityScale, blur: 4 },
    { id: 'heatmap-base',       color: cBase,  width: 4,  opacity: 0.28 * opacityScale, blur: 0 },
    { id: 'heatmap-bright',     color: cBright, width: 2, opacity: 0.20 * opacityScale, blur: 0 },
    { id: 'heatmap-hot',        color: cHot,   width: 1,  opacity: 0.12 * opacityScale, blur: 0 },
  ]

  for (const l of layers) {
    map.addLayer(
      {
        id: l.id,
        type: 'line',
        source: HEATMAP_SOURCE_ID,
        'source-layer': 'heatmap_lines',
        paint: {
          'line-color': l.color,
          'line-width': l.width,
          'line-opacity': l.opacity,
          'line-blur': l.blur,
        },
      },
      safeBeforeId,
    )
  }
}

/** Entfernt alle Heatmap-Layer und die Heatmap-Source von der Karte. */
export function removeHeatmapLayers(map: MaplibreMap): void {
  for (const id of HEATMAP_LAYER_IDS) {
    if (map.getLayer(id)) map.removeLayer(id)
  }
  if (map.getSource(HEATMAP_SOURCE_ID)) map.removeSource(HEATMAP_SOURCE_ID)
}
