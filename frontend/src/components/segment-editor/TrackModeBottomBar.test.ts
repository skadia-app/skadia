import { describe, expect, it } from 'vitest'
import { fmtDist, fmtElevation } from '@/lib/format'
import { formatTrackModeDistanceKm, formatTrackModeElevationM } from './TrackModeBottomBar'

describe('TrackModeBottomBar unit formatting', () => {
  it('formats chart distances through the central distance formatter', () => {
    expect(formatTrackModeDistanceKm(1.234)).toBe(fmtDist(1234))
  })

  it('formats chart elevations through the central elevation formatter', () => {
    expect(formatTrackModeElevationM(235.4)).toBe(fmtElevation(235.4))
    expect(formatTrackModeElevationM(null)).toBe(fmtElevation(null))
  })
})
