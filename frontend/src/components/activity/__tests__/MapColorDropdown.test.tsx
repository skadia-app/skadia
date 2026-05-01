import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { StreamPoint } from '@/api/types'
import MapColorDropdown from '../MapColorDropdown'

const streamsWithSpeed: StreamPoint[] = Array.from({ length: 10 }, (_, i) => ({
  time: `2026-01-01T00:0${i}:00Z`,
  lat: 47 + i * 0.001,
  lng: 11 + i * 0.001,
  speed_ms: 2 + i * 0.1,
}))

function renderDropdown(sportType: string, subType?: string) {
  return render(
    <MapColorDropdown
      streams={streamsWithSpeed}
      sportType={sportType}
      subType={subType}
      metric="sport"
      onChange={vi.fn()}
      modeByMetric={{}}
      setMode={vi.fn()}
      zonesAvailable={() => false}
      effectiveMode="linear"
    />,
  )
}

describe('MapColorDropdown', () => {
  it('uses tuple-aware pace labels for wheelchair run-speed tracks', () => {
    renderDropdown('wheelchair_push', 'run_speed')

    fireEvent.click(screen.getByRole('button', { name: 'Einfärbung wählen' }))

    expect(screen.getByRole('menuitemradio', { name: 'Tempo' })).toBeTruthy()
    expect(screen.queryByRole('menuitemradio', { name: 'Geschwindigkeit' })).toBeNull()
  })

  it('keeps speed labels for cycling tracks', () => {
    renderDropdown('cycling')

    fireEvent.click(screen.getByRole('button', { name: 'Einfärbung wählen' }))

    expect(screen.getByRole('menuitemradio', { name: 'Geschwindigkeit' })).toBeTruthy()
    expect(screen.queryByRole('menuitemradio', { name: 'Tempo' })).toBeNull()
  })
})
