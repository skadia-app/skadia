import type { TrackSummaryGeoTagItem, TrackSummaryWaterItem } from '@/api/trackSummary'
import type { StreamPoint } from '@/api/types'

export function resolveTrackSummaryPoiCoords(
  item: TrackSummaryGeoTagItem | TrackSummaryWaterItem,
  gpsStreams: Array<StreamPoint & { lat: number; lng: number }>,
): { lat: number; lng: number } | null {
  if (item.lat != null && item.lng != null) return { lat: item.lat, lng: item.lng }

  if (item.track_pos != null && gpsStreams.length > 0) {
    const idx = Math.max(0, Math.min(gpsStreams.length - 1, Math.round(item.track_pos * (gpsStreams.length - 1))))
    const point = gpsStreams[idx]
    if (point) return { lat: point.lat, lng: point.lng }
  }

  return null
}
