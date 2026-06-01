import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '@/i18n/I18nProvider'
import { i18n } from '@/i18n/i18n'
import type { Lap } from '@/api/types'
import LapTable from './LapTable'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

function renderTable(laps: Lap[], sportType: string, speedMode?: 'speed' | 'pace') {
  render(
    <I18nProvider>
      <LapTable laps={laps} sportType={sportType} speedMode={speedMode} />
    </I18nProvider>,
  )
}

describe('LapTable unit preferences', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('formats lap distance, pace, and step length with selected units', () => {
    renderTable([
      {
        id: 'lap-1',
        lap_index: 0,
        start_time: '2026-05-01T08:00:00Z',
        timer_time_s: 600,
        distance_m: 1609.344,
        avg_speed_ms: 4,
        avg_step_length_mm: 1000,
      },
    ], 'running')

    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('6:42 min/mi')).toBeInTheDocument()
    expect(screen.getByText('3.3 ft')).toBeInTheDocument()
  })

  it('formats swim stroke distance with selected units', () => {
    renderTable([
      {
        id: 'lap-1',
        lap_index: 0,
        start_time: '2026-05-01T08:00:00Z',
        timer_time_s: 120,
        distance_m: 100,
        avg_speed_ms: 1,
        num_lengths: 4,
        swim_stroke: 'freestyle',
        avg_stroke_distance_m: 1.25,
      },
    ], 'swimming')

    expect(screen.getByText('4.1 ft')).toBeInTheDocument()
  })
})
