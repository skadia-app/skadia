import { describe, expect, it } from 'vitest'
import type { CheckpointControl } from '@/api/segments'
import {
  buildCheckpointControlPreviewData,
  createCheckpointControlAtCoordinate,
} from './SegmentEditorModal'

describe('SegmentEditorModal checkpoint geometry helpers', () => {
  it('creates polygon insert controls as editable area polygons instead of points', () => {
    const control = createCheckpointControlAtCoordinate([8, 49], {
      sequenceIndex: 0,
      role: 'start',
      shape: 'polygon',
      radiusM: 40,
    })

    expect(control.primitive).toBe('area')
    expect(control.geometry_shape).toBe('polygon')
    expect(control.hit_radius_m).toBeUndefined()
    expect(control.area_radius_m).toBeUndefined()
    expect(control.geometry).toMatchObject({ type: 'Polygon' })
    expect((control.geometry as GeoJSON.Polygon).coordinates[0]).toHaveLength(5)
  })

  it('keeps polygon and multipolygon areas visible in checkpoint map preview data', () => {
    const controls: CheckpointControl[] = [
      {
        sequence_index: 0,
        role: 'start',
        primitive: 'area',
        geometry_shape: 'polygon',
        geometry: {
          type: 'Polygon',
          coordinates: [[
            [8, 49],
            [8.01, 49],
            [8.01, 49.01],
            [8, 49.01],
            [8, 49],
          ]],
        },
        required: true,
      },
      {
        sequence_index: 1,
        role: 'finish',
        primitive: 'area',
        geometry_shape: 'multipolygon',
        geometry: {
          type: 'MultiPolygon',
          coordinates: [
            [[
              [9, 50],
              [9.01, 50],
              [9.01, 50.01],
              [9, 50.01],
              [9, 50],
            ]],
          ],
        },
        required: true,
      },
    ]

    const data = buildCheckpointControlPreviewData(controls)
    const areaGeometries = data.features
      .filter(feature => feature.properties?.kind === 'area')
      .map(feature => feature.geometry.type)

    expect(areaGeometries).toEqual(['Polygon', 'MultiPolygon'])
  })
})
