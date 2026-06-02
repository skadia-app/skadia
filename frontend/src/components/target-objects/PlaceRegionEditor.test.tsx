import { fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import PlaceRegionEditor from './PlaceRegionEditor'
import '@/i18n/i18n'

vi.mock('./VisibilityAccessField', () => ({
  default: ({ visibility }: { visibility: string }) => (
    <div data-testid="visibility-field">Sichtbarkeit: {visibility}</div>
  ),
}))

describe('PlaceRegionEditor geometry validation', () => {
  it('rejects non-point geometry for places before submit', () => {
    const onSubmit = vi.fn()
    render(<PlaceRegionEditor mode="place" onSubmit={onSubmit} />)

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Treffpunkt' } })
    fireEvent.change(screen.getByLabelText('Geometrie'), {
      target: {
        value: '{ "type": "Polygon", "coordinates": [[[10,47],[11,47],[11,48],[10,48],[10,47]]] }',
      },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Ort speichern' }))

    expect(screen.getByText('Orte müssen als GeoJSON Point gespeichert werden.')).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('converts polygon input to multipolygon for regions intentionally', () => {
    const onSubmit = vi.fn()
    render(<PlaceRegionEditor mode="region" onSubmit={onSubmit} />)

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Bereich' } })
    fireEvent.change(screen.getByLabelText('Geometrie'), {
      target: {
        value: '{ "type": "Polygon", "coordinates": [[[10,47],[11,47],[11,48],[10,48],[10,47]]] }',
      },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Bereich speichern' }))

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'region',
      geometry: {
        type: 'MultiPolygon',
        coordinates: [[[[10, 47], [11, 47], [11, 48], [10, 48], [10, 47]]]],
      },
    }))
  })

  it('rejects point geometry for regions before submit', () => {
    const onSubmit = vi.fn()
    render(<PlaceRegionEditor mode="region" onSubmit={onSubmit} />)

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Bereich' } })
    fireEvent.change(screen.getByLabelText('Geometrie'), {
      target: { value: '{ "type": "Point", "coordinates": [10,47] }' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Bereich speichern' }))

    expect(screen.getByText('Bereiche müssen als GeoJSON Polygon oder MultiPolygon gespeichert werden.')).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })
})
