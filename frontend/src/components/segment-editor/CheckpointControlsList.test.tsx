import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CheckpointControl } from '@/api/segments'
import { CheckpointControlsList } from './CheckpointControlsList'

const zoneEditorRenders: unknown[] = []

vi.mock('@/components/ZoneEditor', () => ({
  default: ({ value }: { value: unknown }) => {
    zoneEditorRenders.push(value)
    return <div data-testid="zone-editor" />
  },
}))

describe('CheckpointControlsList', () => {
  beforeEach(() => {
    zoneEditorRenders.length = 0
  })

  it('shows multipolygon controls as read-only without collapsing their shape', () => {
    const onUpdate = vi.fn()
    const control: CheckpointControl = {
      sequence_index: 0,
      role: 'start',
      primitive: 'area',
      geometry_shape: 'multipolygon',
      geometry: {
        type: 'MultiPolygon',
        coordinates: [
          [[
            [8, 49],
            [8.01, 49],
            [8.01, 49.01],
            [8, 49.01],
            [8, 49],
          ]],
        ],
      },
      required: true,
    }

    render(
      <CheckpointControlsList
        controls={[control]}
        onRemove={vi.fn()}
        onMove={vi.fn()}
        onUpdate={onUpdate}
      />,
    )

    const shape = screen.getByLabelText('Checkpoint-Form')
    expect(shape).toHaveValue('multipolygon')
    expect(shape).toBeDisabled()
    expect(screen.getByText('Mehrfachfläche nur lesbar. Einzelne Flächen können hier nicht bearbeitet werden.')).toBeInTheDocument()
    expect(screen.queryByLabelText('Checkpoint-Radius in Metern')).not.toBeInTheDocument()
    expect(screen.queryByTestId('zone-editor')).not.toBeInTheDocument()

    fireEvent.change(shape, { target: { value: 'point' } })
    expect(onUpdate).not.toHaveBeenCalled()
  })

  it('converts polygon geometry back to a point when switching to point or circle', () => {
    const onUpdate = vi.fn()
    const control: CheckpointControl = {
      sequence_index: 0,
      role: 'start',
      primitive: 'area',
      geometry_shape: 'polygon',
      geometry: {
        type: 'Polygon',
        coordinates: [[
          [8, 49],
          [8.02, 49],
          [8.02, 49.02],
          [8, 49.02],
          [8, 49],
        ]],
      },
      required: true,
    }

    const { rerender } = render(
      <CheckpointControlsList
        controls={[control]}
        onRemove={vi.fn()}
        onMove={vi.fn()}
        onUpdate={onUpdate}
      />,
    )

    fireEvent.change(screen.getByLabelText('Checkpoint-Form'), { target: { value: 'point' } })
    expect(onUpdate).toHaveBeenLastCalledWith(0, expect.objectContaining({
      primitive: 'point',
      geometry_shape: 'point',
      geometry: { type: 'Point', coordinates: [8.008, 49.008] },
      hit_radius_m: 30,
      area_radius_m: undefined,
    }))

    onUpdate.mockClear()
    rerender(
      <CheckpointControlsList
        controls={[control]}
        onRemove={vi.fn()}
        onMove={vi.fn()}
        onUpdate={onUpdate}
      />,
    )
    fireEvent.change(screen.getByLabelText('Checkpoint-Form'), { target: { value: 'circle' } })
    expect(onUpdate).toHaveBeenLastCalledWith(0, expect.objectContaining({
      primitive: 'area',
      geometry_shape: 'circle',
      geometry: { type: 'Point', coordinates: [8.008, 49.008] },
      hit_radius_m: undefined,
      area_radius_m: 30,
    }))
  })

  it('remounts polygon editors when reordered controls bring different geometry into the same slot', () => {
    const controls: CheckpointControl[] = [
      {
        sequence_index: 0,
        role: 'start',
        primitive: 'area',
        geometry_shape: 'polygon',
        geometry: {
          type: 'Polygon',
          coordinates: [[[8, 49], [8.01, 49], [8.01, 49.01], [8, 49.01], [8, 49]]],
        },
        required: true,
      },
    ]
    const { rerender } = render(
      <CheckpointControlsList controls={controls} onRemove={vi.fn()} onMove={vi.fn()} onUpdate={vi.fn()} />,
    )

    rerender(
      <CheckpointControlsList
        controls={[{
          ...controls[0],
          geometry: {
            type: 'Polygon',
            coordinates: [[[9, 50], [9.01, 50], [9.01, 50.01], [9, 50.01], [9, 50]]],
          },
        }]}
        onRemove={vi.fn()}
        onMove={vi.fn()}
        onUpdate={vi.fn()}
      />,
    )

    expect(zoneEditorRenders).toHaveLength(2)
    expect(zoneEditorRenders[1]).toMatchObject({
      mode: 'polygon',
      polygon: { coordinates: [[[9, 50], [9.01, 50], [9.01, 50.01], [9, 50.01], [9, 50]]] },
    })
  })
})
