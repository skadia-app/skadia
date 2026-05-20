import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/client'
import type { Activity } from '@/api/types'
import ActivityGearSection from './ActivityGearSection'

const addActivityGearAssignment = vi.fn()
const fetchActivity = vi.fn()
const removeActivityGearAssignment = vi.fn()
const fetchGear = vi.fn()

vi.mock('@/api/activities', () => ({
  addActivityGearAssignment: (...args: unknown[]) => addActivityGearAssignment(...args),
  fetchActivity: (...args: unknown[]) => fetchActivity(...args),
  removeActivityGearAssignment: (...args: unknown[]) => removeActivityGearAssignment(...args),
}))

vi.mock('@/api/gear', () => ({
  fetchGear: (...args: unknown[]) => fetchGear(...args),
}))

function wrapper(queryClient: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </MemoryRouter>
  )
}

function renderSection() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return {
    queryClient,
    user: userEvent.setup(),
    ...render(<ActivityGearSection activityId="activity-1" assignments={[]} />, { wrapper: wrapper(queryClient) }),
  }
}

describe('ActivityGearSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchGear.mockResolvedValue([
      { id: 'gear-1', name: 'Propain Tyee 2020', type: 'bike.mtb.enduro', is_retired: false },
    ])
    fetchActivity.mockResolvedValue({ id: 'activity-1', gear_assignments: [] } as unknown as Activity)
  })

  it('surfaces failed assignment mutations instead of silently resetting the selector', async () => {
    addActivityGearAssignment.mockRejectedValue(new ApiError(404, 'not found'))
    const { user } = renderSection()

    await user.click(screen.getByRole('button', { name: 'Ausrüstung hinzufügen' }))
    await user.selectOptions(await screen.findByRole('combobox', { name: 'Ausrüstung wählen' }), 'gear-1')

    expect(addActivityGearAssignment).toHaveBeenCalledWith('activity-1', 'gear-1')
    expect(await screen.findByText('Ausrüstung konnte nicht hinzugefügt werden.')).toBeInTheDocument()
    expect(screen.getByText('Der API-Endpunkt ist im laufenden Backend noch nicht verfügbar.')).toBeInTheDocument()
  })

  it('refreshes the activity after a successful assignment', async () => {
    addActivityGearAssignment.mockResolvedValue({
      id: 'assignment-1',
      gear_id: 'gear-1',
      gear_name: 'Propain Tyee 2020',
      gear_type: 'bike.mtb.enduro',
      role_key: 'primary',
      role_label: 'Hauptausrüstung',
    })
    fetchActivity.mockResolvedValue({
      id: 'activity-1',
      gear_assignments: [{
        id: 'assignment-1',
        gear_id: 'gear-1',
        gear_name: 'Propain Tyee 2020',
        gear_type: 'bike.mtb.enduro',
        role_key: 'primary',
        role_label: 'Hauptausrüstung',
      }],
    } as unknown as Activity)
    const { queryClient, user } = renderSection()

    await user.click(screen.getByRole('button', { name: 'Ausrüstung hinzufügen' }))
    await user.selectOptions(await screen.findByRole('combobox', { name: 'Ausrüstung wählen' }), 'gear-1')

    await waitFor(() => expect(fetchActivity).toHaveBeenCalledWith('activity-1'))
    expect(queryClient.getQueryData<Activity>(['activity', 'activity-1'])?.gear_assignments?.[0]?.gear_name).toBe('Propain Tyee 2020')
  })
})
