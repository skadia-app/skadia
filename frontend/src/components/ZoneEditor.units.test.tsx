import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ZoneEditor from './ZoneEditor'

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'en',
  units: 'imperial',
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      const labels: Record<string, string> = {
        'zoneEditor.circle': 'Circle',
        'zoneEditor.polygon': 'Polygon',
        'zoneEditor.circleHint': 'Click the map to set the center.',
        'zoneEditor.radius': 'Radius',
      }
      if (key === 'zoneEditor.pinCoordinates') return `Pin: ${params?.lat}, ${params?.lng}`
      return labels[key] ?? key
    },
  }),
}))

vi.mock('maplibre-gl', () => ({
  default: {
    Map: vi.fn(function () {
      return {
        addControl: vi.fn(),
        addLayer: vi.fn(),
        addSource: vi.fn(),
        fitBounds: vi.fn(),
        getCanvas: () => document.createElement('canvas'),
        getLayer: vi.fn(),
        getSource: vi.fn(),
        isStyleLoaded: () => false,
        on: vi.fn(),
        once: vi.fn(),
        project: vi.fn(),
        remove: vi.fn(),
        setLayoutProperty: vi.fn(),
      }
    }),
    Marker: vi.fn(function () {
      return {
        addTo: vi.fn().mockReturnThis(),
        getLngLat: () => ({ lng: 10, lat: 50 }),
        on: vi.fn().mockReturnThis(),
        remove: vi.fn(),
        setLngLat: vi.fn().mockReturnThis(),
      }
    }),
    NavigationControl: vi.fn(function () {}),
    LngLatBounds: vi.fn(function () {
      return {
        extend: vi.fn(),
      }
    }),
  },
}))

describe('ZoneEditor unit preferences', () => {
  it('formats circle radius controls with the selected user unit', () => {
    mockUserPrefs.units = 'imperial'

    render(
      <ZoneEditor
        value={{ mode: 'circle', center: [10, 50], radiusM: 1609.344 }}
        onChange={vi.fn()}
        minRadiusM={100}
        maxRadiusM={1609.344}
      />,
    )

    expect(screen.getByText('Radius')).toBeInTheDocument()
    expect(screen.getByRole('spinbutton')).toHaveValue(5280)
    expect(screen.getByText('ft')).toBeInTheDocument()
    expect(screen.getByText('(1.0 mi)')).toBeInTheDocument()
    expect(screen.getByText('328 ft')).toBeInTheDocument()
    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.queryByText('m')).not.toBeInTheDocument()
  })
})
