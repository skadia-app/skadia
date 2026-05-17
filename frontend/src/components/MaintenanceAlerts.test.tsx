import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { describe, it, expect, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import MaintenanceAlerts from './MaintenanceAlerts'
import { fetchMaintenanceAlerts } from '@/api/gear'

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
      is_overdue: false,
      current_value: 450000,
      threshold_value: 500000,
    },
    {
      interval_id: 'i2',
      gear_id: 'g1',
      gear_name: 'Propain Tyee',
      component_id: 'c2',
      component_name: 'Hinterreifen',
      interval_name: 'Prüfen',
      interval_type: 'moving_time_s',
      is_overdue: true,
      current_value: 43200,
      threshold_value: 36000,
    },
  ])),
}))

function renderAlerts() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <MaintenanceAlerts />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('MaintenanceAlerts', () => {
  it('groups alerts by gear, links each row to the affected component, and formats metric units', async () => {
    renderAlerts()

    const group = await screen.findByRole('heading', { name: 'Propain Tyee' })
    const swapLink = screen.getByRole('link', { name: /Vorderreifen.*Tauschen/s })
    const checkLink = screen.getByRole('link', { name: /Hinterreifen.*Prüfen/s })

    expect(fetchMaintenanceAlerts).toHaveBeenCalled()
    expect(group).toBeInTheDocument()
    expect(swapLink).toHaveAttribute('href', '/gear/g1?tab=components&component=c1')
    expect(checkLink).toHaveAttribute('href', '/gear/g1?tab=components&component=c2')
    expect(screen.getByText('Tauschen: 450 km / 500 km')).toBeInTheDocument()
    expect(screen.getByText('Prüfen: 12 h / 10 h')).toBeInTheDocument()
  })
})
