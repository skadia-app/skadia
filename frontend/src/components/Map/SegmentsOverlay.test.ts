import { describe, expect, it } from 'vitest'
import { buildSegmentsOverlayFetchParams, DEFAULT_SEGMENT_FILTERS } from './SegmentsOverlay'

describe('SegmentsOverlay fetch params', () => {
  it('pushes supported overlay filters into segment requests', () => {
    expect(buildSegmentsOverlayFetchParams({
      bbox: '10,47,11,48',
      filters: {
        ...DEFAULT_SEGMENT_FILTERS,
        terrainTypes: ['trail'],
        courseKinds: ['line', 'checkpoint_sequence'],
        riddenOnly: true,
        sportGroups: ['bike'],
      },
    })).toEqual({
      bbox: '10,47,11,48',
      completed_only: true,
      course_kinds: ['line', 'checkpoint_sequence'],
      include_control_map_features: true,
      include_geojson: true,
      limit: 500,
      segment_types: 'trail',
      sport_group: 'bike',
    })
  })

  it('leaves client-only favorite filtering out of the server request', () => {
    expect(buildSegmentsOverlayFetchParams({
      bbox: '',
      filters: {
        ...DEFAULT_SEGMENT_FILTERS,
        favoritesOnly: true,
      },
    })).toEqual({
      course_kind: 'line',
      include_geojson: true,
      limit: 500,
    })
  })
})
