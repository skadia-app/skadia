import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { ToastProvider } from '@/lib/toast'
import type { Activity } from '@/api/types'

const { fetchActivitiesMock, reprocessAllActivitiesMock } = vi.hoisted(() => ({
  fetchActivitiesMock: vi.fn(),
  reprocessAllActivitiesMock: vi.fn(),
}))

vi.mock('@/api/activities', () => ({
  fetchActivities: fetchActivitiesMock,
  reprocessAllActivities: reprocessAllActivitiesMock,
  isReprocessAccepted: (value: unknown) => Boolean(value && typeof value === 'object' && 'job_id' in value),
}))

import WorkshopReprocessPage from '../WorkshopReprocessPage'

function makeActivity(id: string, title: string): Activity {
  return {
    id,
    user_id: 'user-1',
    title,
    title_auto: null,
    sport_type: 'cycling',
    sub_type: 'mountain',
    started_at: '2025-09-03T10:00:00Z',
    timezone: 'Europe/Berlin',
  } as Activity
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <ToastProvider>
          <WorkshopReprocessPage />
        </ToastProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('WorkshopReprocessPage activity selection', () => {
  beforeEach(() => {
    fetchActivitiesMock.mockReset()
    reprocessAllActivitiesMock.mockReset()
  })

  it('sends the search text to the activities API instead of filtering only the first loaded page', async () => {
    fetchActivitiesMock.mockResolvedValue({ activities: [makeActivity('a1', 'Beispielwald Nord')], next_cursor: null, total: 1 })

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Auswahl' }))
    fireEvent.change(screen.getByPlaceholderText('Titel oder Sportart suchen...'), {
      target: { value: 'derchinger forst' },
    })

    await waitFor(() => {
      expect(fetchActivitiesMock).toHaveBeenCalledWith(
        undefined,
        100,
        undefined,
        undefined,
        { search: 'derchinger forst' },
      )
    })
  })

  it('loads the next cursor page without clearing selected ids', async () => {
    fetchActivitiesMock
      .mockResolvedValueOnce({ activities: [makeActivity('a1', 'Beispielwald Nord')], next_cursor: 'cursor-1', total: 2 })
      .mockResolvedValueOnce({ activities: [makeActivity('a2', 'Beispielwald Nord')], next_cursor: null, total: 2 })

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Auswahl' }))

    await screen.findByText('Beispielwald Nord')
    fireEvent.click(screen.getByText('Beispielwald Nord'))
    fireEvent.click(screen.getByRole('button', { name: 'Weitere laden' }))

    await waitFor(() => {
      expect(fetchActivitiesMock).toHaveBeenCalledWith('cursor-1', 100, undefined, undefined, undefined)
    })
    expect(screen.getByText('1 ausgewählt')).toBeInTheDocument()
  })

  it('offers auto title recompute and does not claim tags always remain unchanged', () => {
    fetchActivitiesMock.mockResolvedValue({ activities: [], next_cursor: null, total: 0 })

    renderPage()

    expect(screen.getByLabelText('Automatische Benennung')).toBeInTheDocument()
    expect(screen.queryByText(/Tags\) bleiben erhalten/)).not.toBeInTheDocument()
    expect(screen.getByText(/Tags bleiben nur unverändert/)).toBeInTheDocument()
  })

  it('uses scope-aware tag preservation copy in the confirmation dialog', () => {
    fetchActivitiesMock.mockResolvedValue({ activities: [], next_cursor: null, total: 0 })

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Neu verarbeiten' }))

    expect(screen.queryByText(/Tags, Sichtbarkeit\) bleiben erhalten/)).not.toBeInTheDocument()
    expect(screen.getByText(/Automations- und Systemtags neu ausgewertet/)).toBeInTheDocument()
  })
})
