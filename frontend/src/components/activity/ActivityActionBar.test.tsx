import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { Activity } from '@/api/types'
import { reprocessActivity } from '@/api/activities'
import ActivityActionBar from './ActivityActionBar'

const mockReprocessActivity = reprocessActivity as ReturnType<typeof vi.fn>

vi.mock('@/api/activities', () => ({
  reprocessActivity: vi.fn().mockResolvedValue({ job_id: 'job-1' }),
  ungroupActivity: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/api/fusion', () => ({
  revertFusion: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/fusionCacheInvalidation', () => ({
  invalidateAfterFusion: vi.fn(),
}))

vi.mock('@/lib/useActiveJobs', () => ({
  useActiveJobs: () => ({ isJobActiveForEntity: () => false }),
}))

vi.mock('@/api/client', () => ({
  apiDownload: vi.fn(),
}))

vi.mock('@/api/routes', () => ({
  getRouteDraftFromActivity: vi.fn(),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: { features: { routing: { configured: true, available: true } } } }),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

vi.mock('@/components/activity/ActivityQuickEditModal', () => ({ default: () => null }))
vi.mock('@/components/activity/ReprocessScopeModal', () => ({ default: ({ onConfirm }: { onConfirm: (scope: string[]) => void }) => (
  <button onClick={() => onConfirm(['streams'])}>Scope modal</button>
) }))
vi.mock('@/components/SportConflictModal', () => ({ default: () => <div>Sport conflict modal</div> }))

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
    title: 'Action test',
    sport_type: 'cycling',
    sub_type: null,
    started_at: '2026-05-18T08:00:00Z',
    elapsed_time_s: 3600,
    has_gps: true,
    is_virtual: false,
    tags: [],
  } as unknown as Activity
}

describe('ActivityActionBar', () => {
  it('keeps smoothing and restore actions in the overflow menu', async () => {
    render(<ActivityActionBar activity={activity()} />, { wrapper })

    fireEvent.click(screen.getByRole('button', { name: 'Weitere Aktionen' }))
    fireEvent.click(screen.getByRole('button', { name: 'Geschwindigkeit glätten' }))
    fireEvent.click(screen.getByRole('button', { name: 'Anwenden' }))

    await waitFor(() => {
      expect(mockReprocessActivity).toHaveBeenCalledWith('activity-1', ['speed_smooth'], 2)
    })

    fireEvent.click(screen.getByRole('button', { name: 'Weitere Aktionen' }))
    fireEvent.click(screen.getByRole('button', { name: 'Originaldaten wiederherstellen' }))

    await waitFor(() => {
      expect(mockReprocessActivity).toHaveBeenCalledWith('activity-1', ['streams'])
    })
  })

  it('keeps scoped reprocess and sport conflict actions reachable', () => {
    render(<ActivityActionBar activity={{ ...activity(), sport_conflict_pending: true } as Activity} />, { wrapper })

    expect(screen.getByRole('button', { name: /Sportkonflikt/ })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Weitere Aktionen' }))
    fireEvent.click(screen.getByRole('button', { name: 'Bereiche neu berechnen...' }))

    expect(screen.getByText('Scope modal')).toBeInTheDocument()
  })

  it('keeps fusion revert and multisport ungroup actions in the overflow menu', () => {
    const fusionActivity = {
      ...activity(),
      fusion_memberships: [{ field_key: 'distance_m', source_id: 'source-1' }],
    } as Activity

    const { rerender } = render(<ActivityActionBar activity={fusionActivity} />, { wrapper })

    fireEvent.click(screen.getByRole('button', { name: 'Weitere Aktionen' }))
    fireEvent.click(screen.getByRole('button', { name: 'Fusion rückgängig' }))
    expect(screen.getByText('Fusion rückgängig machen?')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Abbrechen' }))

    rerender(<ActivityActionBar activity={{ ...activity(), sport_type: 'multisport', has_gps: false } as Activity} />)
    fireEvent.click(screen.getByRole('button', { name: 'Weitere Aktionen' }))
    fireEvent.click(screen.getByRole('button', { name: 'Gruppe auflösen' }))
    expect(screen.getByText('Gruppe wirklich auflösen?')).toBeInTheDocument()
  })
})
