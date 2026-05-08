import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import LiftReviewModal from './LiftReviewModal'
import type { Activity, LiftCandidate } from '@/api/types'

vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(),
}))

import { apiFetch } from '@/lib/auth'
const mockApiFetch = apiFetch as ReturnType<typeof vi.fn>

function makeActivity(overrides: Partial<Activity> = {}): Pick<Activity, 'id' | 'lifts_acknowledged_at'> {
  return {
    id: 'act-1',
    lifts_acknowledged_at: null,
    ...overrides,
  }
}

function makeCandidates(overrides: Partial<LiftCandidate>[] = []): LiftCandidate[] {
  return overrides.map((o, i) => ({
    id: `cand-${i}`,
    activity_id: 'act-1',
    started_at: '2024-01-01T09:00:00Z',
    ended_at: '2024-01-01T09:05:00Z',
    duration_s: 300,
    ascent_m: 200,
    descent_m: 0,
    distance_m: 1500,
    confidence: 'high',
    confirmed: false,
    rejected_by_user: false,
    source: 'heuristic' as const,
    segment_effort_id: null,
    ...o,
  }))
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

const twoHeuristicCandidates = makeCandidates([
  { id: 'c-0', ascent_m: 300, descent_m: 280 },
  { id: 'c-1', ascent_m: 100, descent_m: 0 },
])

describe('LiftReviewModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiFetch.mockResolvedValue({ ok: true })
  })

  it('shows compact mode by default with confirm/reject/inspect buttons', () => {
    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={twoHeuristicCandidates}
      />,
      { wrapper },
    )

    expect(screen.getByText(/Lifte erkannt/)).toBeTruthy()
    expect(screen.getByText(/Ja, alle bestätigen/)).toBeTruthy()
    expect(screen.getByText(/Nein, alle ablehnen/)).toBeTruthy()
    expect(screen.getByText(/Prüfen/)).toBeTruthy()
    // Expanded content must not be visible
    expect(screen.queryByText(/Inspector-Ansicht/)).toBeNull()
  })

  it('clicking confirm-all calls POST /confirm-all and closes modal', async () => {
    const onClose = vi.fn()
    render(
      <LiftReviewModal
        open={true}
        onClose={onClose}
        activity={makeActivity()}
        candidates={twoHeuristicCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Ja, alle bestätigen/))

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const confirmCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/lifts/confirm-all') && opts?.method === 'POST'
      })
      expect(confirmCall).toBeTruthy()
    })

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
  })

  it('clicking reject-all calls POST /reject-all and closes modal', async () => {
    const onClose = vi.fn()
    render(
      <LiftReviewModal
        open={true}
        onClose={onClose}
        activity={makeActivity()}
        candidates={twoHeuristicCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Nein, alle ablehnen/))

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const rejectCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/lifts/reject-all') && opts?.method === 'POST'
      })
      expect(rejectCall).toBeTruthy()
    })

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
  })

  it('clicking inspect switches to expanded mode (Pass-B stub visible)', () => {
    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={twoHeuristicCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Prüfen/))

    expect(screen.getByText(/Inspector-Ansicht/)).toBeTruthy()
    expect(screen.queryByText(/Ja, alle bestätigen/)).toBeNull()
  })

  it('clicking back from expanded returns to compact', () => {
    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={twoHeuristicCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Prüfen/))
    expect(screen.getByText(/Zurück zur Übersicht/)).toBeTruthy()

    fireEvent.click(screen.getByText(/Zurück zur Übersicht/))

    expect(screen.getByText(/Ja, alle bestätigen/)).toBeTruthy()
    expect(screen.queryByText(/Inspector-Ansicht/)).toBeNull()
  })

  it('closing via X calls POST /acknowledge', async () => {
    const onClose = vi.fn()
    render(
      <LiftReviewModal
        open={true}
        onClose={onClose}
        activity={makeActivity()}
        candidates={twoHeuristicCandidates}
      />,
      { wrapper },
    )

    const closeBtn = screen.getByRole('button', { name: /Schließen/ })
    fireEvent.click(closeBtn)

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const ackCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/lifts/acknowledge') && opts?.method === 'POST'
      })
      expect(ackCall).toBeTruthy()
    })

    expect(onClose).toHaveBeenCalled()
  })

  it('singular Liftfahrt for n===1, plural for n>1', () => {
    const { rerender } = render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={makeCandidates([{ id: 'single' }])}
      />,
      { wrapper },
    )

    expect(screen.getByText(/1 Liftfahrt wurde erkannt/)).toBeTruthy()
    expect(screen.queryByText(/1 Liftfahrten/)).toBeNull()

    rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <LiftReviewModal
          open={true}
          onClose={() => {}}
          activity={makeActivity()}
          candidates={twoHeuristicCandidates}
        />
      </QueryClientProvider>,
    )

    expect(screen.getByText(/2 Liftfahrten wurden erkannt/)).toBeTruthy()
  })
})
