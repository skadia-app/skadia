import type { StreamPoint } from '@/api/types'
import type { ZoomRange } from '@/components/ActivityCharts'

export interface TrackMapFitTarget {
  coordinates: [number, number][]
  padding: number
  maxZoom: number
}

export function buildTrackMapFitTarget(
  streams: StreamPoint[],
  fullTrackCoordinates: [number, number][],
  zoomRange: ZoomRange | null,
): TrackMapFitTarget | null {
  if (zoomRange) {
    const selectedCoordinates = streams
      .slice(zoomRange.startIdx, zoomRange.endIdx + 1)
      .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
      .map(s => [s.lng, s.lat] as [number, number])

    if (selectedCoordinates.length < 2) return null
    return { coordinates: selectedCoordinates, padding: 50, maxZoom: 18 }
  }

  if (fullTrackCoordinates.length < 2) return null
  return { coordinates: fullTrackCoordinates, padding: 40, maxZoom: 15 }
}
