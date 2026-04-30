import { describe, expect, it } from 'vitest'
import { buildRouteSavePayload } from '../RouteSaveDialog'
import type { RouteMetadataValues } from '../RouteMetadataForm'

describe('RouteSaveDialog save payload', () => {
  it('round-trips route sport modifiers from metadata', () => {
    const metadata: RouteMetadataValues = {
      name: 'E-MTB Runde',
      description: '',
      sport_type: 'cycling',
      sub_type: 'mountain',
      is_electric: true,
      is_indoor: false,
      is_virtual: false,
      is_commute: true,
      difficulty: '',
      tag_ids: [],
      auto_tag: true,
      profile_override: 'mtb',
    }

    const payload = buildRouteSavePayload({
      metadata,
      waypoints: [],
      segments: [{
        from_waypoint: 'a',
        to_waypoint: 'b',
        mode: 'routed',
        geometry: { type: 'LineString', coordinates: [[11, 47], [11.01, 47.01]] },
        control_points: [],
        smooth: false,
        distance_m: 100,
        elevation_gain: 10,
        elevation_loss: 5,
      }],
    })

    expect(payload).toMatchObject({
      sport_type: 'cycling',
      sub_type: 'mountain',
      is_electric: true,
      is_indoor: false,
      is_virtual: false,
      is_commute: true,
    })
  })
})
