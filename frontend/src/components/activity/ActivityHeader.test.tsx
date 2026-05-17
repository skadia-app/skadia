import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { Activity } from '@/api/types'
import { ToastProvider } from '@/lib/toast'
import ActivityHeader from './ActivityHeader'
import { reprocessActivity } from '@/api/activities'

const mockReprocessActivity = reprocessActivity as ReturnType<typeof vi.fn>

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({
    data: {
      features: {
        routing: { configured: true, available: true },
        offline_map: { configured: true, available: false, reason: 'missing_artifacts' },
      },
    },
  }),
}))

vi.mock('@/api/routes', () => ({
  getWaypointsFromActivity: vi.fn(),
}))

vi.mock('@/api/activities', () => ({
  updateActivity: vi.fn(),
  reprocessActivity: vi.fn(),
  ungroupActivity: vi.fn(),
}))

vi.mock('@/api/fusion', () => ({
  revertFusion: vi.fn(),
}))

vi.mock('@/lib/fusionCacheInvalidation', () => ({
  invalidateAfterFusion: vi.fn(),
}))

vi.mock('@/lib/useActiveJobs', () => ({
  useActiveJobs: () => ({ isJobActiveForEntity: () => false }),
}))

vi.mock('@/hooks/useClickOutside', () => ({
  useClickOutside: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiDownload: vi.fn(),
}))

vi.mock('@/components/SportIcon', () => ({ SportIcon: () => <span /> }))
vi.mock('@/components/SportConflictBadge', () => ({ default: () => null }))
vi.mock('@/components/SportConflictModal', () => ({ default: () => null }))
vi.mock('@/components/activity/ActivityQuickEditModal', () => ({ default: () => null }))
vi.mock('@/components/activity/ReprocessScopeModal', () => ({ default: () => null }))

function activity(): Activity {
  return {
    id: 'act-1',
    user_id: 'user-1',
    title: 'Route source',
    sport_type: 'cycling',
    started_at: '2026-05-12T08:00:00Z',
    elapsed_time_s: 3600,
    has_gps: true,
    is_virtual: false,
    tags: [],
  } as unknown as Activity
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <MemoryRouter>
      <ToastProvider>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </ToastProvider>
    </MemoryRouter>
  )
}

describe('ActivityHeader capability gates', () => {
  it('shows route planner action when routing is available without offline maps', async () => {
    render(<ActivityHeader activity={activity()} childCount={0} />, { wrapper })

    expect(await screen.findByRole('button', { name: /Im Routenplaner öffnen/i })).toBeInTheDocument()
  })

  it('maps Alles to core-safe scopes when full-only capabilities are unavailable', async () => {
    mockReprocessActivity.mockResolvedValue({ job_id: 'job-1' })

    render(<ActivityHeader activity={activity()} childCount={0} />, { wrapper })

    fireEvent.click(screen.getByTitle('Aktivität neu verarbeiten'))
    fireEvent.click(await screen.findByText('Alles'))

    await waitFor(() => {
      expect(mockReprocessActivity).toHaveBeenCalledWith(
        'act-1',
        expect.arrayContaining(['streams', 'metrics', 'auto_title', 'ascents', 'intervals', 'lifts']),
      )
    })
    const [, scope] = mockReprocessActivity.mock.calls[0]
    expect(scope).not.toContain('summits')
    expect(scope).not.toContain('stops')
    expect(scope).not.toContain('surfaces')
  })
})
