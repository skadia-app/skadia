import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Activity } from '@/api/types'
import { fetchActivities } from '@/api/activities'
import { i18n } from '@/i18n/i18n'

import DashboardPage from './DashboardPage'

const userPrefs = vi.hoisted(() => ({ locale: 'de', units: 'metric' as 'metric' | 'imperial' }))

vi.mock('@/api/activities', () => ({
  fetchActivities: vi.fn(() => Promise.resolve({ activities: [], next_cursor: null, total: 0 })),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

vi.mock('@/api/gear', () => ({
  fetchMaintenanceAlerts: vi.fn(() => Promise.resolve([
    {
      interval_id: 'i1',
      gear_id: 'g1',
      gear_name: 'Propain Tyee',
      component_id: 'c1',
      component_name: 'Vorderreifen',
      interval_name: 'Tauschen',
      interval_type: 'distance_m',
      is_overdue: true,
      current_value: 3262000,
      threshold_value: 500,
    },
    {
      interval_id: 'i2',
      gear_id: 'g1',
      gear_name: 'Propain Tyee',
      component_id: 'c2',
      component_name: 'Hinterreifen',
      interval_name: 'Prüfen',
      interval_type: 'moving_time_s',
      is_overdue: false,
      current_value: 1494000,
      threshold_value: 1800000,
    },
  ])),
}))

vi.mock('@/api/analytics', () => ({
  fetchHRVAnalysis: vi.fn(() => Promise.resolve(null)),
}))

vi.mock('@/components/analytics/ReadinessCard', () => ({
  ReadinessCardCompact: () => <div data-testid="readiness-card">Bereitschaft</div>,
}))

function renderDashboard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('DashboardPage', () => {
  beforeEach(() => {
    userPrefs.locale = 'de'
    userPrefs.units = 'metric'
    vi.mocked(fetchActivities).mockResolvedValue({ activities: [], next_cursor: null, total: 0 })
  })

  afterEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('renders grouped maintenance alerts at the top with component links', async () => {
    renderDashboard()

    const alertHeading = await screen.findByRole('heading', { name: 'Wartungshinweise' })
    const readinessCard = screen.getByTestId('readiness-card')
    const gearHeading = screen.getByRole('heading', { name: 'Propain Tyee' })
    const swapLink = screen.getByRole('link', { name: /Vorderreifen.*Tauschen/s })
    const checkLink = screen.getByRole('link', { name: /Hinterreifen.*Prüfen/s })

    expect(alertHeading.compareDocumentPosition(readinessCard) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(gearHeading).toBeInTheDocument()
    expect(swapLink).toHaveAttribute('href', '/gear/g1?tab=components&component=c1')
    expect(checkLink).toHaveAttribute('href', '/gear/g1?tab=components&component=c2')
  })

  it('formats this-week distance and elevation with user unit preferences', async () => {
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'
    vi.mocked(fetchActivities).mockImplementation((_cursor, _limit, _search, _signal, filters) => {
      if (filters?.dateFrom) {
        return Promise.resolve({
          activities: [
            {
              id: 'a1',
              distance_m: 1609.344,
              elevation_gain_m: 304.8,
              moving_time_s: 3600,
            } as Activity,
          ],
          next_cursor: null,
          total: 1,
        })
      }
      return Promise.resolve({ activities: [], next_cursor: null, total: 0 })
    })

    renderDashboard()

    expect(await screen.findByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('1,000 ft')).toBeInTheDocument()
  })

  it('localizes fallback activity sport labels from the app language', async () => {
    await i18n.changeLanguage('en')
    vi.mocked(fetchActivities).mockImplementation((_cursor, _limit, _search, _signal, filters) => {
      if (filters?.dateFrom) {
        return Promise.resolve({ activities: [], next_cursor: null, total: 0 })
      }
      return Promise.resolve({
        activities: [
          {
            id: 'a1',
            sport_type: 'cycling',
            sub_type: null,
            started_at: '2026-05-31T08:00:00Z',
          } as Activity,
        ],
        next_cursor: null,
        total: 1,
      })
    })

    renderDashboard()

    expect(await screen.findByText('Cycling')).toBeInTheDocument()
    expect(screen.queryByText('Radfahren')).not.toBeInTheDocument()
  })
})
