import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import type { ScaleDefinition } from '@/lib/mapColorUtils'
import MapColorLegend from '../MapColorLegend'

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'en',
  units: 'imperial',
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

describe('MapColorLegend unit preferences', () => {
  beforeEach(async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    await i18n.changeLanguage('en')
  })

  it('formats linear speed, pace, altitude, and temperature scales with imperial display units', () => {
    render(<>
      <MapColorLegend scale={linearScale('speed', 'km/h', 18, 36)} />
      <MapColorLegend scale={linearScale('pace', 'min/km', 3, 6)} />
      <MapColorLegend scale={linearScale('altitude', 'm', 100, 200)} />
      <MapColorLegend scale={linearScale('temperature', '°C', 0, 20)} />
    </>)

    expect(screen.getByText('11.2 mph')).toBeInTheDocument()
    expect(screen.getByText('22.4 mph')).toBeInTheDocument()
    expect(screen.getByText('4:50 min/mi')).toBeInTheDocument()
    expect(screen.getByText('9:39 min/mi')).toBeInTheDocument()
    expect(screen.getByText('328 ft')).toBeInTheDocument()
    expect(screen.getByText('656 ft')).toBeInTheDocument()
    expect(screen.getByText('32 °F')).toBeInTheDocument()
    expect(screen.getByText('68 °F')).toBeInTheDocument()
  })

  it('formats speed zones with imperial display units while keeping HR and power units fixed', () => {
    render(<>
      <MapColorLegend scale={zonesScale('speed', 'km/h', [18, 36, 9999])} />
      <MapColorLegend scale={zonesScale('hr', 'bpm', [140, 160, 9999])} />
      <MapColorLegend scale={zonesScale('power', 'W', [200, 300, 9999])} />
    </>)

    expect(screen.getByText(/≤ 11\.2 mph/)).toHaveTextContent('Z1: ≤ 11.2 mph')
    expect(screen.getByText(/≤ 22\.4 mph/)).toHaveTextContent('Z2: ≤ 22.4 mph')
    expect(screen.getByText(/> 22\.4 mph/)).toHaveTextContent('Z3: > 22.4 mph')
    expect(screen.getByText(/≤ 140 bpm/)).toBeInTheDocument()
    expect(screen.getByText(/> 160 bpm/)).toBeInTheDocument()
    expect(screen.getByText(/≤ 200 W/)).toBeInTheDocument()
    expect(screen.getByText(/> 300 W/)).toBeInTheDocument()
  })
})

function linearScale(
  metric: Extract<ScaleDefinition['metric'], 'speed' | 'pace' | 'altitude' | 'temperature'>,
  unit: string,
  min: number,
  max: number,
): Extract<ScaleDefinition, { type: 'linear' }> {
  return {
    type: 'linear',
    metric,
    unit,
    clamped: true,
    stops: [
      { value: min, color: '#000000' },
      { value: max, color: '#ffffff' },
    ],
  }
}

function zonesScale(
  metric: Extract<ScaleDefinition['metric'], 'speed' | 'hr' | 'power'>,
  unit: string,
  zoneHighs: number[],
): Extract<ScaleDefinition, { type: 'zones' }> {
  return {
    type: 'zones',
    metric,
    unit,
    zoneHighs,
    zoneColors: zoneHighs.map((_, i) => `#00000${i}`),
  }
}
