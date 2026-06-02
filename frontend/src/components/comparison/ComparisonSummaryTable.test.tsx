import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Activity } from '@/api/types'
import { SourcesProvider } from './SourcesContext'
import ComparisonSummaryTable from './ComparisonSummaryTable'

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'en',
  units: 'imperial',
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

function activity(overrides: Partial<Activity>): Activity {
  return {
    id: 'activity-1',
    user_id: 'user-1',
    title: null,
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
    avg_heart_rate: 150,
    max_heart_rate: 170,
    avg_power_w: 210,
    normalized_power_w: 220,
    avg_cadence: 84,
    tss: 42,
    is_indoor: false,
    tags: [],
    visibility: 'private',
    enrichment_status: 'done',
    needs_classification: false,
    has_gps: true,
    ...overrides,
  }
}

describe('ComparisonSummaryTable units', () => {
  beforeEach(() => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
  })

  it('formats distance, elevation, and speed with the active unit preference', () => {
    render(
      <SourcesProvider
        sources={[
          {
            id: 'source-a',
            label: 'Source A',
            color: '#3b82f6',
            activity: activity({ id: 'activity-a' }),
          },
          {
            id: 'source-b',
            label: 'Source B',
            color: '#ef4444',
            activity: activity({
              id: 'activity-b',
              distance_m: 3218.688,
              elevation_gain_m: 609.6,
              avg_speed_ms: 8.9408,
            }),
          },
        ]}
      >
        <ComparisonSummaryTable />
      </SourcesProvider>,
    )

    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('2,000 ft')).toBeInTheDocument()
    expect(screen.getByText('10.0 mph')).toBeInTheDocument()
    expect(screen.queryByText('1.6 km')).not.toBeInTheDocument()
    expect(screen.queryByText('305 m')).not.toBeInTheDocument()
    expect(screen.queryByText('16.1 km/h')).not.toBeInTheDocument()
  })
})
