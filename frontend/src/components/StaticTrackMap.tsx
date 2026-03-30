// Leichtgewichtige statische Karte: OSM-Kacheln als Hintergrund + Track als SVG-Overlay.
// Kein MapLibre nötig – rein deklarativ, kein Interaktion, kein State.

import { useMemo } from 'react'
import { cn } from '@/lib/utils'

const TILE_PX = 256

/** Längengrad → World-Pixel X bei Zoom z */
function lngToWorldX(lng: number, z: number): number {
  return ((lng + 180) / 360) * TILE_PX * Math.pow(2, z)
}

/** Breitengrad → World-Pixel Y bei Zoom z (Web-Mercator) */
function latToWorldY(lat: number, z: number): number {
  const r = lat * Math.PI / 180
  return (1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2 * TILE_PX * Math.pow(2, z)
}

interface Props {
  geojsonStr: string
  color: string
  className?: string
}

interface TileData {
  z: number
  tx: number
  ty: number
  /** Position in SVG/World-Pixel-Space */
  x: number
  y: number
}

interface MapData {
  viewBox: string
  tiles: TileData[]
  trackPoints: string
  /** Koordinaten des Attribution-Texts im SVG-Raum */
  attrX: number
  attrY: number
  VW: number
  VH: number
}

/**
 * Rendert einen GeoJSON-LineString als Track-Overlay auf OSM-Kacheln.
 *
 * Funktioniert komplett ohne MapLibre – die Kacheln werden als SVG <image>
 * geladen. Die Track-Koordinaten werden in denselben World-Pixel-Raum
 * projiziert, sodass Karte und Track exakt übereinander liegen.
 *
 * Attribution: © OpenStreetMap contributors (OSM-Nutzungsbedingungen).
 */
export default function StaticTrackMap({ geojsonStr, color, className }: Props) {
  const data = useMemo<MapData | null>(() => {
    let coords: [number, number][]
    try {
      const geo = JSON.parse(geojsonStr) as { type: string; coordinates: [number, number][] }
      if (geo.type !== 'LineString' || !geo.coordinates?.length) return null
      coords = geo.coordinates
    } catch {
      return null
    }
    if (coords.length < 2) return null

    let minLng = Infinity, maxLng = -Infinity, minLat = Infinity, maxLat = -Infinity
    for (const [lng, lat] of coords) {
      if (lng < minLng) minLng = lng
      if (lng > maxLng) maxLng = lng
      if (lat < minLat) minLat = lat
      if (lat > maxLat) maxLat = lat
    }

    // ViewBox-Dimensionen (5:3 – passt zum Karten-Vorschau-Seitenverhältnis)
    const VW = 500, VH = 300

    // Besten Zoom suchen: Track soll ~70% der Viewport-Fläche füllen, ohne Scaling
    let zoom = 5
    for (let z = 17; z >= 1; z--) {
      const xSpan = lngToWorldX(maxLng, z) - lngToWorldX(minLng, z)
      const ySpan = latToWorldY(minLat, z) - latToWorldY(maxLat, z) // minLat = unten = größeres Y
      if (xSpan <= VW * 0.72 && ySpan <= VH * 0.72) {
        zoom = z
        break
      }
    }

    // Track-Zentrum in World-Pixels
    const trackCenterX = (lngToWorldX(minLng, zoom) + lngToWorldX(maxLng, zoom)) / 2
    const trackCenterY = (latToWorldY(minLat, zoom) + latToWorldY(maxLat, zoom)) / 2

    // ViewBox: zentriert auf Track-Mittelpunkt
    const vbLeft = trackCenterX - VW / 2
    const vbTop  = trackCenterY - VH / 2
    const viewBox = `${vbLeft.toFixed(0)} ${vbTop.toFixed(0)} ${VW} ${VH}`

    // Kacheln bestimmen, die den ViewBox-Bereich abdecken
    const maxTile = Math.pow(2, zoom) - 1
    const leftTile   = Math.max(0, Math.floor(vbLeft / TILE_PX))
    const rightTile  = Math.min(maxTile, Math.floor((vbLeft + VW) / TILE_PX))
    const topTile    = Math.max(0, Math.floor(vbTop / TILE_PX))
    const bottomTile = Math.min(maxTile, Math.floor((vbTop + VH) / TILE_PX))

    const tiles: TileData[] = []
    for (let ty = topTile; ty <= bottomTile; ty++) {
      for (let tx = leftTile; tx <= rightTile; tx++) {
        tiles.push({ z: zoom, tx, ty, x: tx * TILE_PX, y: ty * TILE_PX })
      }
    }

    // Track-Punkte in World-Pixel-Raum projizieren (passt direkt zum ViewBox)
    const trackPoints = coords.map(([lng, lat]) =>
      `${lngToWorldX(lng, zoom).toFixed(1)},${latToWorldY(lat, zoom).toFixed(1)}`
    ).join(' ')

    return {
      viewBox,
      tiles,
      trackPoints,
      attrX: vbLeft + VW - 3,
      attrY: vbTop  + VH - 4,
      VW,
      VH,
    }
  }, [geojsonStr])

  if (!data) return null
  const { viewBox, tiles, trackPoints, attrX, attrY, VW, VH } = data

  return (
    <div className={cn('overflow-hidden', className)}>
      <svg
        viewBox={viewBox}
        className="w-full h-full block"
        aria-hidden="true"
      >
        {/* OSM-Kacheln als Hintergrund */}
        {tiles.map(t => (
          <image
            key={`${t.tx}-${t.ty}`}
            href={`https://tile.openstreetmap.org/${t.z}/${t.tx}/${t.ty}.png`}
            x={t.x}
            y={t.y}
            width={TILE_PX}
            height={TILE_PX}
            preserveAspectRatio="none"
          />
        ))}

        {/* Halbtransparente Abdunkelung für besseren Track-Kontrast */}
        <rect
          x={attrX - VW + 3}
          y={attrY - VH + 4}
          width={VW}
          height={VH}
          fill="rgba(0,0,0,0.18)"
        />

        {/* Track-Glow-Effekt: dicker, halbtransparenter Schatten-Stroke */}
        <polyline
          points={trackPoints}
          fill="none"
          stroke={color}
          strokeWidth="8"
          strokeLinecap="round"
          strokeLinejoin="round"
          opacity="0.35"
        />

        {/* Track-Hauptlinie */}
        <polyline
          points={trackPoints}
          fill="none"
          stroke={color}
          strokeWidth="3.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          opacity="0.95"
        />

        {/* OSM-Attribution (Pflicht laut Nutzungsbedingungen) */}
        <text
          x={attrX}
          y={attrY}
          textAnchor="end"
          fontSize="9"
          fill="rgba(255,255,255,0.85)"
          stroke="rgba(0,0,0,0.6)"
          strokeWidth="2.5"
          paintOrder="stroke"
          style={{ fontFamily: 'sans-serif' }}
        >
          © OpenStreetMap
        </text>
      </svg>
    </div>
  )
}
