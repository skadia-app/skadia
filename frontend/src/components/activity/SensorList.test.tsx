import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it } from 'vitest'
import type { Sensor } from '@/api/types'
import { i18n } from '@/i18n/i18n'
import { SensorList } from './SensorList'

function sensor(overrides: Partial<Sensor> = {}): Sensor {
  return {
    id: 'sensor-1',
    device_index: 1,
    source_type: 'bluetooth',
    primary_type: 'heart_rate',
    primary_type_label: 'Herzfrequenz',
    all_types: ['heart_rate'],
    all_type_labels: ['Herzfrequenz'],
    manufacturer: 'Acme',
    product_name: 'Pulse Strap',
    ...overrides,
  }
}

describe('SensorList', () => {
  beforeEach(() => i18n.changeLanguage('de'))

  it('uses the shared connection label for Bluetooth sensor badges', () => {
    render(
      <MemoryRouter>
        <SensorList sensors={[sensor()]} />
      </MemoryRouter>,
    )

    expect(screen.getByText('Bluetooth LE')).toBeInTheDocument()
    expect(screen.queryByText('BT')).not.toBeInTheDocument()
  })

  it('keeps functional serial and ANT+ identifiers unchanged', () => {
    render(
      <MemoryRouter>
        <SensorList
          sensors={[
            sensor({
              source_type: 'antplus',
              serial_number: 12345,
              ant_device_number: 678,
            }),
          ]}
        />
      </MemoryRouter>,
    )

    expect(screen.getByText('ANT+')).toBeInTheDocument()
    expect(screen.getByText('S/N:')).toBeInTheDocument()
    expect(screen.getByText('12345')).toBeInTheDocument()
    expect(screen.getByText('678')).toBeInTheDocument()
  })

  it('localizes known battery status values without changing protocol labels', async () => {
    await i18n.changeLanguage('en')

    render(
      <MemoryRouter>
        <SensorList
          sensors={[
            sensor({
              source_type: 'antplus',
              battery_status: 'low',
              ant_device_number: 678,
            }),
          ]}
        />
      </MemoryRouter>,
    )

    expect(screen.getByText('ANT+')).toBeInTheDocument()
    expect(screen.getByText('Battery: Low')).toBeInTheDocument()
    expect(screen.queryByText('Battery: low')).not.toBeInTheDocument()
  })
})
