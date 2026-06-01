import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { Activity } from '@/api/types'
import type { BestEffortRanked } from '@/api/bestEfforts'
import ActivityContextCard from './ActivityContextCard'

vi.mock('@/components/activity/TagsBadges', () => ({
  default: ({ tags }: { tags: string[] }) => (
    <div aria-label="Tags">
      {tags.map(tag => <span key={tag}>{tag}</span>)}
    </div>
  ),
}))

vi.mock('@/api/gear', () => ({
  fetchGear: vi.fn(async () => []),
}))

vi.mock('@/api/activities', async () => {
  const actual = await vi.importActual<typeof import('@/api/activities')>('@/api/activities')
  return {
    ...actual,
    fetchActivity: vi.fn(),
    addActivityGearAssignment: vi.fn(),
    removeActivityGearAssignment: vi.fn(),
  }
})

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'de', units: 'metric' }),
}))

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <MemoryRouter>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </MemoryRouter>
  )
}

function activity(): Activity {
  return {
    id: 'activity-1',
    user_id: 'user-1',
    title: 'Monte Tamaro',
    sport_type: 'cycling',
    sub_type: 'mountain_biking',
    started_at: '2025-10-16T09:01:00Z',
    ended_at: '2025-10-16T17:52:00Z',
    timezone: 'Europe/Zurich',
    elapsed_time_s: 31860,
    has_gps: true,
    is_virtual: false,
    tags: ['Rundtour', 'bewölkt'],
    recording_group_id: 'group-1',
    recording_device_name: 'Garmin Instinct 2X',
    gear_assignments: [
      {
        id: 'assignment-bike',
        gear_id: 'gear-bike',
        gear_name: 'Propain Tyee 2020',
        gear_type: 'bike.mtb.enduro',
        role_key: 'primary',
        role_label: 'Hauptausrüstung',
      },
      {
        id: 'assignment-helmet',
        gear_id: 'gear-helmet',
        gear_name: 'Trailhelm',
        gear_type: 'helmet',
        role_key: 'worn',
        role_label: 'Getragen',
      },
    ],
    sunrise_at: '2025-10-16T05:43:00Z',
    sunset_at: '2025-10-16T16:38:00Z',
    start_city: 'Monte Ceneri',
    start_state: 'Ticino',
    start_country: 'Schweiz',
    start_country_code: 'CH',
    location_country_code: 'CH',
    visited_country_codes: ['CH', 'IT'],
    garmin_activity_id: '123',
    strava_activity_id: '456',
    komoot_tour_id: '789',
    exclude_from_heatmap: false,
    exclude_from_stats: false,
    weather_readings: [],
  } as unknown as Activity
}

const bestEffort: BestEffortRanked = {
  id: 'effort-1',
  activity_id: 'activity-1',
  category: 'longest_duration',
  sport_group: 'bike',
  achieved_at: '2025-10-16T09:01:00Z',
  rank: 1,
  percentile: 1,
}

describe('ActivityContextCard', () => {
  it('keeps the compact context order and visible activity metadata', () => {
    const { container } = render(
      <ActivityContextCard
        activity={activity()}
        bestEfforts={[bestEffort]}
        nearMisses={[]}
      />,
      { wrapper },
    )

    expect(screen.getByText('Autogruppierung')).toBeInTheDocument()
    expect(screen.getByText('Monte Tamaro')).toBeInTheDocument()
    expect(screen.getByText('🇨🇭 🇮🇹')).toBeInTheDocument()
    expect(screen.getByText('Gerät')).toBeInTheDocument()
    expect(screen.getByText('Garmin Instinct 2X')).toBeInTheDocument()
    expect(screen.getByText('Aufzeichnung')).toBeInTheDocument()
    expect(screen.getByText('11:01 - 19:52')).toBeInTheDocument()
    expect(screen.getByText('Monte Ceneri · Ticino, Schweiz')).toBeInTheDocument()
    expect(screen.getByText('07:43')).toBeInTheDocument()
    expect(screen.getByText('18:38')).toBeInTheDocument()

    const headings = Array.from(container.querySelectorAll('.uppercase')).map(el => el.textContent)
    expect(headings).toEqual(['Gerät', 'Aufzeichnung', 'Wetter', 'Tags', 'Ausrüstung', 'Plattformen', 'Bestleistungen'])

    expect(within(screen.getByLabelText('Tags')).getByText('Rundtour')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Propain Tyee 2020/ })).toHaveAttribute('href', '/gear/gear-bike')
    expect(screen.getByRole('link', { name: /Trailhelm/ })).toHaveAttribute('href', '/gear/gear-helmet')
    expect(screen.getByRole('button', { name: 'Ausrüstung hinzufügen' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Propain Tyee 2020 entfernen' })).toHaveClass('opacity-0')
    expect(screen.queryByText('Sichtbarkeit')).not.toBeInTheDocument()
    expect(screen.queryByRole('switch', { name: 'Heatmap aktiv' })).not.toBeInTheDocument()
    expect(screen.queryByRole('switch', { name: 'Statistik aktiv' })).not.toBeInTheDocument()
    expect(screen.getByText('Garmin Connect')).toBeInTheDocument()
    expect(screen.getByText('Strava')).toBeInTheDocument()
    expect(screen.getByText('Komoot')).toBeInTheDocument()
    expect(screen.getByText('Längste Dauer')).toBeInTheDocument()
    expect(screen.getByText('Top 1%')).toBeInTheDocument()
  })

  it('keeps location visible when only region and country are known', () => {
    render(
      <ActivityContextCard
        activity={{
          ...activity(),
          start_city: null,
          end_city: null,
          start_state: 'Bayern',
          start_country: 'Deutschland',
        }}
        bestEfforts={[]}
        nearMisses={[]}
      />,
      { wrapper },
    )

    expect(screen.getByText('Bayern, Deutschland')).toBeInTheDocument()
  })
})
