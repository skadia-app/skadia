// Rendert ein GeoJSON-LineString-Objekt als SVG-Polyline-Points-String.
// Kein MapLibre nötig — leichtgewichtig für Miniaturkarten in der Galerie.

interface GeoJSONLineString {
  type: 'LineString'
  coordinates: [number, number][]  // [lng, lat]
}

/**
 * Wandelt einen GeoJSON-LineString-String in einen SVG-Polyline-Points-String um.
 * Normalisiert Koordinaten auf [0, viewBoxSize].
 * Gibt null zurück wenn coordinates leer oder kein LineString.
 */
export function lineStringToSvgPoints(
  geojsonStr: string,
  viewBoxSize = 200,
  padding = 8,
): string | null {
  let geojson: GeoJSONLineString
  try {
    geojson = JSON.parse(geojsonStr) as GeoJSONLineString
  } catch {
    return null
  }
  if (geojson.type !== 'LineString' || !geojson.coordinates?.length) return null

  const coords = geojson.coordinates
  if (coords.length < 2) return null

  const lngs = coords.map(c => c[0])
  const lats = coords.map(c => c[1])

  const minLng = Math.min(...lngs)
  const maxLng = Math.max(...lngs)
  const minLat = Math.min(...lats)
  const maxLat = Math.max(...lats)

  const rangeLng = maxLng - minLng || 1e-9
  const rangeLat = maxLat - minLat || 1e-9

  const innerSize = viewBoxSize - padding * 2
  const scaleX = innerSize / rangeLng
  const scaleY = innerSize / rangeLat
  const scale = Math.min(scaleX, scaleY)

  const offsetX = (viewBoxSize - rangeLng * scale) / 2
  const offsetY = (viewBoxSize - rangeLat * scale) / 2

  return coords
    .map(([lng, lat]) => {
      const x = offsetX + (lng - minLng) * scale
      const y = viewBoxSize - offsetY - (lat - minLat) * scale
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
}
