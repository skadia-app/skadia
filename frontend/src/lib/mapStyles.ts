import type { StyleSpecification, RasterSourceSpecification } from 'maplibre-gl'

export type BaseStyle = 'osm' | 'topo' | 'cyclosm' | 'satellite' | 'dark' | 'local'

export type RasterStyleDef = {
  label: string
  type: 'raster'
  tiles: string[]
  attribution: string
  maxzoom?: number
}
export type VectorStyleDef = { label: string; type: 'vector'; styleUrl: string }
export type StyleDef = RasterStyleDef | VectorStyleDef

export const BASE_STYLES: Record<BaseStyle, StyleDef> = {
  osm: {
    label: 'OSM',
    type: 'raster',
    tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
    maxzoom: 19,
  },
  topo: {
    label: 'Topo',
    type: 'raster',
    tiles: ['https://tile.opentopomap.org/{z}/{x}/{y}.png'],
    attribution: '&copy; OpenTopoMap',
    maxzoom: 17,
  },
  cyclosm: {
    label: 'CyclOSM',
    type: 'raster',
    tiles: [
      'https://a.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png',
      'https://b.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png',
      'https://c.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png',
    ],
    attribution: '&copy; CyclOSM &copy; OpenStreetMap',
    maxzoom: 19,
  },
  satellite: {
    label: 'Satellit',
    type: 'raster',
    tiles: ['https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}'],
    attribution: '&copy; Esri',
    maxzoom: 19,
  },
  dark: {
    label: 'Dunkel',
    type: 'raster',
    tiles: [
      'https://a.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}@2x.png',
      'https://b.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}@2x.png',
      'https://c.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}@2x.png',
    ],
    attribution: '&copy; CARTO &copy; OpenStreetMap',
    maxzoom: 19,
  },
  local: {
    label: 'Lokal',
    type: 'vector',
    styleUrl: '/tiles/style.json',
  },
}

export function buildRasterStyle(style: BaseStyle): StyleSpecification | string {
  const def = BASE_STYLES[style]
  if (def.type === 'vector') {
    // Vector style: return URL so MapLibre fetches the complete style JSON
    return def.styleUrl
  }
  return {
    version: 8,
    sources: {
      basemap: {
        type: 'raster',
        tiles: def.tiles,
        tileSize: 256,
        attribution: def.attribution,
        maxzoom: def.maxzoom ?? 19,
      } as RasterSourceSpecification,
    },
    layers: [{ id: 'basemap-tiles', type: 'raster', source: 'basemap' }],
  }
}

export const RASTER_STYLES = Object.entries(BASE_STYLES).filter(
  ([, def]) => def.type === 'raster'
) as [BaseStyle, RasterStyleDef][]
