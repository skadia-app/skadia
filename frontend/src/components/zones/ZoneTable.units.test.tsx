import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ZoneTable from './ZoneTable'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      const labels: Record<string, string> = {
        'activityTables.zoneTable.name': 'Name',
        'activityTables.zoneTable.low': 'Low',
        'activityTables.zoneTable.high': 'High',
        'activityTables.zoneTable.max': 'max',
        'activityTables.zoneTable.add': '+ Add zone',
        'activityTables.zoneTable.removeTitle': 'Remove zone',
      }
      if (key === 'activityTables.zoneTable.defaultName') return `Zone ${params?.number}`
      if (key === 'activityTables.zoneTable.nameAria') return `Name for zone ${params?.number}`
      if (key === 'activityTables.zoneTable.upperBoundAria') return `Upper bound for zone ${params?.number}`
      if (key === 'activityTables.zoneTable.removeAria') return `Remove zone ${params?.number}`
      return labels[key] ?? key
    },
  }),
}))

describe('ZoneTable unit preferences', () => {
  it('formats editable speed zone bounds as mph while preserving stored m/s values', () => {
    const onChange = vi.fn()

    render(
      <ZoneTable
        rows={[
          { name: 'Z1', high: 5 },
          { name: 'Z2', high: 10 },
          { name: 'Z3', high: 9999 },
        ]}
        onChange={onChange}
        unit="mph"
        convertMpsToKmh
      />,
    )

    expect(screen.getByText('High (mph)')).toBeInTheDocument()
    expect(screen.getByLabelText('Upper bound for zone 1')).toHaveValue(11.2)
    expect(screen.getByText('11.3')).toBeInTheDocument()
    expect(screen.getByLabelText('Upper bound for zone 2')).toHaveValue(22.4)
    expect(screen.queryByDisplayValue('18')).not.toBeInTheDocument()
    expect(screen.queryByDisplayValue('36')).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Upper bound for zone 2'), { target: { value: '30' } })

    expect(onChange).toHaveBeenCalledWith([
      { name: 'Z1', high: 5 },
      { name: 'Z2', high: expect.closeTo(13.411, 3) },
      { name: 'Z3', high: 9999 },
    ])
  })

  it('keeps heart-rate and power units fixed instead of converting them', () => {
    render(
      <>
        <ZoneTable rows={[{ name: 'Z1', high: 140 }, { name: 'Z2', high: 9999 }]} onChange={vi.fn()} unit="bpm" />
        <ZoneTable rows={[{ name: 'Z1', high: 250 }, { name: 'Z2', high: 9999 }]} onChange={vi.fn()} unit="W" />
      </>,
    )

    expect(screen.getByText('High (bpm)')).toBeInTheDocument()
    expect(screen.getByText('High (W)')).toBeInTheDocument()
    const firstZoneBounds = screen.getAllByLabelText('Upper bound for zone 1')
    expect(firstZoneBounds[0]).toHaveValue(140)
    expect(firstZoneBounds[1]).toHaveValue(250)
  })
})
