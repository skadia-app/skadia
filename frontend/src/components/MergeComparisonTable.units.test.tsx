import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Activity } from '@/api/types'
import { i18n } from '@/i18n/i18n'
import { MergeComparisonTable } from './MergeComparisonTable'

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'en',
  units: 'imperial',
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

function activity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'activity-1',
    user_id: 'user-1',
    title: 'Base ride',
    sport_type: 'cycling',
    sub_type: null,
    is_electric: false,
    is_virtual: false,
    is_commute: false,
    exclude_from_heatmap: false,
    exclude_from_stats: false,
    started_at: '2026-05-31T08:00:00Z',
    created_at: '2026-05-31T08:00:00Z',
    updated_at: '2026-05-31T08:00:00Z',
    elapsed_time_s: 3600,
    distance_m: 1609.344,
    elevation_gain_m: 304.8,
    elevation_loss_m: 0,
    avg_speed_ms: 4.4704,
    avg_temperature_c: 20,
    avg_heart_rate: 150,
    avg_power_w: 210,
    avg_cadence: 84,
    is_indoor: false,
    tags: [],
    visibility: 'private',
    enrichment_status: 'done',
    needs_classification: false,
    has_gps: true,
    ...overrides,
  }
}

describe('MergeComparisonTable units', () => {
  beforeEach(async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    await i18n.changeLanguage('en')
  })

  it('formats merge metric values with the active unit preference', () => {
    render(
      <MergeComparisonTable
        target={activity({
          id: 'target',
          distance_m: null,
          elevation_gain_m: null,
          avg_speed_ms: null,
          avg_temperature_c: null,
        })}
        source={activity({ id: 'source' })}
        selectedFields={[]}
        onToggleField={vi.fn()}
      />,
    )

    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('1,000 ft')).toBeInTheDocument()
    expect(screen.getByText('10.0 mph')).toBeInTheDocument()
    expect(screen.getByText('68 °F')).toBeInTheDocument()
    expect(screen.queryByText('1609.344 m')).not.toBeInTheDocument()
    expect(screen.queryByText('304.8 m')).not.toBeInTheDocument()
    expect(screen.queryByText('4.4704 m/s')).not.toBeInTheDocument()
    expect(screen.queryByText('20 °C')).not.toBeInTheDocument()
  })
})
