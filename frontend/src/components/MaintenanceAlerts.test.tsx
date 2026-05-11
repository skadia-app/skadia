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
  it('links alerts to the affected component and formats metric units', async () => {
    renderAlerts()

    const link = await screen.findByRole('link', { name: /Propain Tyee/ })

    expect(fetchMaintenanceAlerts).toHaveBeenCalled()
    expect(link).toHaveAttribute('href', '/gear/g1?tab=components&component=c1')
    expect(screen.getByText('Tauschen: 450 km / 500 km')).toBeInTheDocument()
  })
})
