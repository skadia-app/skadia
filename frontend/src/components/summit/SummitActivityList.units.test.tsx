import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import type { SummitActivity, SummitVisit } from '@/api/summits'
import SummitActivityList from './SummitActivityList'

const summitActivitiesMock = vi.hoisted(() => ({
  fetchSummitActivities: vi.fn<() => Promise<SummitActivity[]>>(),
}))

vi.mock('@/api/summits', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/summits')>()),
  fetchSummitActivities: summitActivitiesMock.fetchSummitActivities,
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

function renderList() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const summit: SummitVisit = {
    osm_id: 1,
    name: 'Synthetic Summit',
    elevation_m: 1500,
    visit_count: 1,
    first_visit: '2026-05-01T00:00:00Z',
    last_visit: '2026-05-01T00:00:00Z',
    lat: 47.1,
    lng: 11.3,
    country_codes: ['XX'],
  }

  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <SummitActivityList summit={summit} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('SummitActivityList unit preferences', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
    summitActivitiesMock.fetchSummitActivities.mockResolvedValue([
      {
        activity_id: 'activity-1',
        title: 'Summit day',
        started_at: '2026-05-01T12:00:00Z',
        sport_type: 'running',
        distance_m: 1609.344,
        elevation_gain_m: 100,
        timezone: 'UTC',
      },
    ])
  })

  it('formats distance and elevation with selected units', async () => {
    renderList()

    expect(await screen.findByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('↑328 ft')).toBeInTheDocument()
    expect(screen.queryByText('↑100 m')).not.toBeInTheDocument()
  })
})
