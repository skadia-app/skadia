import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import DashboardPage from './DashboardPage'

vi.mock('@/api/activities', () => ({
  fetchActivities: vi.fn(() => Promise.resolve({ activities: [], next_cursor: null, total: 0 })),
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
})
