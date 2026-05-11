import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import LiftSection from './LiftSection'
import type { Activity, LiftCandidate } from '@/api/types'

// Mock apiFetch — tests verify which endpoints get called
vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(),
}))

import { apiFetch } from '@/lib/auth'
const mockApiFetch = apiFetch as ReturnType<typeof vi.fn>

function makeActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    user_id: 'user-1',
    title: 'Test Activity',
    sport_type: 'cycling',
    started_at: '2024-01-01T08:00:00Z',
    elapsed_time_s: 7200,
    has_gps: true,
    is_virtual: false,
    tags: [],
    lift_count: 2,
    lift_distance_m: 3000,
    lift_ascent_m: 400,
    lift_descent_m: 380,
    lift_duration_s: 600,
    distance_m: 20000,
    elevation_gain_m: 800,
    elevation_loss_m: 750,
    exclude_lift_subtraction: false,
    ...overrides,
  } as Activity
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

describe('LiftSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: return empty candidates list
    mockApiFetch.mockResolvedValue({
      json: () => Promise.resolve([]),
    })
  })

  it('shows bidirectional elevation for candidate with descent_m > 0', async () => {
    const candidates = makeCandidates([{ ascent_m: 200, descent_m: 180, confirmed: false }])
    mockApiFetch.mockImplementation((url: string) => {
      if (url.includes('/lifts/candidates')) {
        return Promise.resolve({ json: () => Promise.resolve(candidates) })
      }
      return Promise.resolve({ json: () => Promise.resolve({}) })
    })

    render(
      <LiftSection
        activity={makeActivity()}
        onToggleExclude={async () => {}}
      />,
      { wrapper },
    )

    // Wait for candidates to load
    const item = await screen.findByText(/\+200 \/ −180 m/)
    expect(item).toBeTruthy()
  })

  it('shows only heuristic candidates in sub-list, not OSM', async () => {
    const candidates = makeCandidates([
      { id: 'h-confirmed', source: 'heuristic', confirmed: true, ascent_m: 150, descent_m: 0 },
      { id: 'h-open', source: 'heuristic', confirmed: false, ascent_m: 120, descent_m: 0 },
      { id: 'osm-1', source: 'osm', confirmed: true, ascent_m: 200, descent_m: 0 },
    ])
    mockApiFetch.mockImplementation((url: string) => {
      if (url.includes('/lifts/candidates')) {
        return Promise.resolve({ json: () => Promise.resolve(candidates) })
      }
      return Promise.resolve({ json: () => Promise.resolve({}) })
    })

    render(
      <LiftSection
        activity={makeActivity({ lift_count: 2 })}
        onToggleExclude={async () => {}}
      />,
      { wrapper },
    )

    // Both heuristic candidates should appear (confirmed + unconfirmed)
    const bestätigt = await screen.findAllByText(/bestätigt/)
    expect(bestätigt.length).toBeGreaterThanOrEqual(1)

    const offen = await screen.findAllByText(/offen/)
    expect(offen.length).toBeGreaterThanOrEqual(1)

    // OSM candidate should NOT appear in the sub-list (no third item row for it)
    // The OSM candidate has ascent_m=200 while h-confirmed has 150 and h-open has 120
    // We check that only two candidate rows exist in the heuristic sub-list
    const rückgängigButtons = screen.getAllByText(/Rückgängig/)
    expect(rückgängigButtons).toHaveLength(1) // only one confirmed heuristic

    const bestätigenButtons = screen.getAllByText(/✓ Bestätigen/)
    expect(bestätigenButtons).toHaveLength(1) // only one unconfirmed heuristic
  })

  it('clicking Rückgängig calls PATCH .../unconfirm endpoint', async () => {
    const candidates = makeCandidates([
      { id: 'h-confirmed', source: 'heuristic', confirmed: true, ascent_m: 300, descent_m: 0 },
    ])
    mockApiFetch.mockImplementation((url: string) => {
      if (url.includes('/lifts/candidates')) {
        return Promise.resolve({ json: () => Promise.resolve(candidates) })
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) })
    })

    render(
      <LiftSection
        activity={makeActivity({ lift_count: 1 })}
        onToggleExclude={async () => {}}
      />,
      { wrapper },
    )

    const btn = await screen.findByText(/Rückgängig/)
    fireEvent.click(btn)

    // Wait for the mutation to fire — apiFetch is async
    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const unconfirmCall = calls.find(
        (args: unknown[]) => {
          const url = args[0] as string | undefined
          const opts = args[1] as RequestInit | undefined
          return !!url && url.includes('/lift-candidates/h-confirmed/unconfirm') && opts?.method === 'PATCH'
        },
      )
      expect(unconfirmCall).toBeTruthy()
    })
  })

  it('does not invalidate activity or candidates when confirm returns non-OK', async () => {
    const candidates = makeCandidates([
      { id: 'h-open', source: 'heuristic', confirmed: false, ascent_m: 120, descent_m: 0 },
    ])
    mockApiFetch.mockImplementation((url: string) => {
      if (url.includes('/lifts/candidates')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(candidates) })
      }
      return Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({ message: 'boom' }) })
    })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')

    render(
      <QueryClientProvider client={qc}>
        <LiftSection
          activity={makeActivity({ lift_count: 1 })}
          onToggleExclude={async () => {}}
        />
      </QueryClientProvider>,
    )

    fireEvent.click(await screen.findByText(/✓ Bestätigen/))

    await waitFor(() => {
      const confirmCall = mockApiFetch.mock.calls.find(
        (args: unknown[]) => {
          const url = args[0] as string | undefined
          const opts = args[1] as RequestInit | undefined
          return !!url && url.includes('/lift-candidates/h-open/confirm') && opts?.method === 'POST'
        },
      )
      expect(confirmCall).toBeTruthy()
    })
    await waitFor(() => {
      expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['activity', 'act-1'] })
      expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['lift-candidates', 'act-1'] })
    })
  })

  it('shows singular Liftfahrt for liftCount === 1', async () => {
    mockApiFetch.mockResolvedValue({ json: () => Promise.resolve([]) })

    render(
      <LiftSection
        activity={makeActivity({ lift_count: 1 })}
        onToggleExclude={async () => {}}
      />,
      { wrapper },
    )

    const headline = await screen.findByText(/1 Liftfahrt/)
    expect(headline).toBeTruthy()
    // Must NOT say "Liftfahrten"
    expect(headline.textContent).not.toMatch(/1 Liftfahrten/)
  })

  it('shows plural Liftfahrten for liftCount === 2', async () => {
    mockApiFetch.mockResolvedValue({ json: () => Promise.resolve([]) })

    render(
      <LiftSection
        activity={makeActivity({ lift_count: 2 })}
        onToggleExclude={async () => {}}
      />,
      { wrapper },
    )

    const headline = await screen.findByText(/2 Liftfahrten/)
    expect(headline).toBeTruthy()
  })

  it('shows review banner when lifts_acknowledged_at is null and unreviewed heuristic candidates exist', async () => {
    const candidates = makeCandidates([
      { id: 'h-0', source: 'heuristic', confirmed: false, rejected_by_user: false },
      { id: 'h-1', source: 'heuristic', confirmed: false, rejected_by_user: false },
    ])
    mockApiFetch.mockImplementation((url: string) => {
      if (url.includes('/lifts/candidates')) {
        return Promise.resolve({ json: () => Promise.resolve(candidates) })
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) })
    })

    render(
      <LiftSection
        activity={makeActivity({ lift_count: 2, lifts_acknowledged_at: null })}
        onToggleExclude={async () => {}}
      />,
      { wrapper },
    )

    const banner = await screen.findByText(/2 Liftfahrten unbestätigt/)
    expect(banner).toBeTruthy()
  })

  it('hides review banner when lifts_acknowledged_at is set', async () => {
    const candidates = makeCandidates([
      { id: 'h-0', source: 'heuristic', confirmed: false, rejected_by_user: false },
    ])
    mockApiFetch.mockImplementation((url: string) => {
      if (url.includes('/lifts/candidates')) {
        return Promise.resolve({ json: () => Promise.resolve(candidates) })
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) })
    })

    render(
      <LiftSection
        activity={makeActivity({ lift_count: 1, lifts_acknowledged_at: '2024-01-01T10:00:00Z' })}
        onToggleExclude={async () => {}}
      />,
      { wrapper },
    )

    // Wait for candidates to settle
    await screen.findByText(/1 Liftfahrt/)
    expect(screen.queryByText(/unbestätigt/)).toBeNull()
  })

  it('shows singular "1 Liftfahrt unbestätigt" when unreviewedCount === 1', async () => {
    const candidates = makeCandidates([
      { id: 'h-0', source: 'heuristic', confirmed: false, rejected_by_user: false },
    ])
    mockApiFetch.mockImplementation((url: string) => {
      if (url.includes('/lifts/candidates')) {
        return Promise.resolve({ json: () => Promise.resolve(candidates) })
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) })
    })

    render(
      <LiftSection
        activity={makeActivity({ lift_count: 1, lifts_acknowledged_at: null })}
        onToggleExclude={async () => {}}
      />,
      { wrapper },
    )

    const banner = await screen.findByText(/1 Liftfahrt unbestätigt/)
    expect(banner).toBeTruthy()
    expect(banner.textContent).not.toMatch(/1 Liftfahrten/)
  })
})
