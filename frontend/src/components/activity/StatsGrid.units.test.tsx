import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Activity } from '@/api/types'
import { i18n } from '@/i18n/i18n'
import { StatsGrid } from './StatsGrid'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

function activity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'activity-1',
    user_id: 'user-1',
    title: 'Morning ride',
    sport_type: 'cycling',
    sub_type: null,
    is_electric: false,
    is_virtual: false,
    is_commute: false,
    is_indoor: false,
    exclude_from_heatmap: false,
    exclude_from_stats: false,
    started_at: '2026-05-01T07:00:00Z',
    elapsed_time_s: 3600,
    distance_m: 1609.344,
    elevation_gain_m: 100,
    elevation_loss_m: 50,
    avg_speed_ms: 10,
    max_speed_ms: 12,
    avg_temperature_c: 20,
    tags: [],
    visibility: 'private',
    enrichment_status: 'complete',
    needs_classification: false,
    has_gps: true,
    created_at: '2026-05-01T08:00:00Z',
    updated_at: '2026-05-01T08:00:00Z',
    ...overrides,
  }
}

describe('StatsGrid unit preferences', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('formats distance, elevation, and temperature with the selected user units', () => {
    render(<StatsGrid activity={activity()} />)

    expect(screen.getByText('Distance')).toBeInTheDocument()
    expect(screen.getByText('Ascent')).toBeInTheDocument()
    expect(screen.getByText('Descent')).toBeInTheDocument()
    expect(screen.getByText('Avg temperature')).toBeInTheDocument()
    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('+328 ft')).toBeInTheDocument()
    expect(screen.getByText('−164 ft')).toBeInTheDocument()
    expect(screen.getByText('22.4 mph')).toBeInTheDocument()
    expect(screen.getByText('26.8 mph')).toBeInTheDocument()
    expect(screen.getByText('68 °F')).toBeInTheDocument()
  })

  it('formats foot pace and secondary speed with the selected user units', () => {
    render(<StatsGrid activity={activity({ sport_type: 'running', avg_speed_ms: 1609.344 / 600 })} />)

    expect(screen.getByText('Avg pace')).toBeInTheDocument()
    expect(screen.getByText('10:00 min/mi')).toBeInTheDocument()
    expect(screen.getByText('6.0 mph')).toBeInTheDocument()
  })

  it('formats swimming length metrics with the selected user units', () => {
    render(<StatsGrid activity={activity({
      sport_type: 'swimming',
      pool_length_m: 25,
      avg_stroke_distance_m: 1.25,
    })} />)

    expect(screen.getByText('Pool length')).toBeInTheDocument()
    expect(screen.getByText('Avg distance/stroke')).toBeInTheDocument()
    expect(screen.getByText('82 ft')).toBeInTheDocument()
    expect(screen.getByText('4.1 ft')).toBeInTheDocument()
    expect(screen.queryByText('25 m')).not.toBeInTheDocument()
    expect(screen.queryByText('1.25 m')).not.toBeInTheDocument()
  })
})
