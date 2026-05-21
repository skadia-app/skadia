import { describe, expect, it } from 'vitest'
import { resolveTrackSummaryPoiCoords } from '../highlightedPois'
import type { TrackSummaryGeoTagItem } from '@/api/trackSummary'
import type { StreamPoint } from '@/api/types'

const gpsStreams: Array<StreamPoint & { lat: number; lng: number }> = [
  { time: '2026-01-01T00:00:00Z', lat: 1.0, lng: 1.0 },
  { time: '2026-01-01T00:01:00Z', lat: 1.5, lng: 1.5 },
  { time: '2026-01-01T00:02:00Z', lat: 2.0, lng: 2.0 },
]

describe('resolveTrackSummaryPoiCoords', () => {
  it('uses the stored GeoTag coordinates even when track_pos is present', () => {
    const item: TrackSummaryGeoTagItem = {
      id: 'synthetic-peak',
      category: 'summit',
      name: 'Synthetic Peak',
      osm_id: null,
      lat: 3.25,
      lng: 4.75,
      ele_m: 1000,
      dist_m: 66,
      track_pos: 0.5,
    }

    expect(resolveTrackSummaryPoiCoords(item, gpsStreams)).toEqual({
      lat: 3.25,
      lng: 4.75,
    })
  })

  it('falls back to track_pos only when a legacy summary item has no coordinates', () => {
    const item: TrackSummaryGeoTagItem = {
      id: 'summit-legacy',
      category: 'summit',
      name: 'Legacy Synthetic Peak',
      osm_id: null,
      lat: null,
      lng: null,
      ele_m: null,
      dist_m: null,
      track_pos: 0.5,
    }

    expect(resolveTrackSummaryPoiCoords(item, gpsStreams)).toEqual({
      lat: 1.5,
      lng: 1.5,
    })
  })
})
