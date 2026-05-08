import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import LiftReviewModal from './LiftReviewModal'
import type { Activity, LiftCandidate } from '@/api/types'

// ResizeObserver and matchMedia are not available in jsdom.
global.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
}
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// Mock maplibre-gl so the inspector map doesn't crash in jsdom.
const maplibreMock = vi.hoisted(() => {
  class LngLatBoundsMock {
    extend() { return this }
    isEmpty() { return true }
  }
  class MapMock {
    addControl = vi.fn()
    on = vi.fn()
    remove = vi.fn()
    resize = vi.fn()
    isStyleLoaded = vi.fn(() => true)
    addSource = vi.fn()
    addLayer = vi.fn()
    getSource = vi.fn(() => null)
    getLayer = vi.fn(() => null)
    fitBounds = vi.fn()
    getCanvas = vi.fn(() => ({ style: { cursor: '' } }))
    setPaintProperty = vi.fn()
  }
  return { MapMock, LngLatBoundsMock, maps: [] as MapMock[] }
})

vi.mock('maplibre-gl', () => ({
  default: {
    Map: maplibreMock.MapMock,
    LngLatBounds: maplibreMock.LngLatBoundsMock,
    NavigationControl: class {},
  },
  Map: maplibreMock.MapMock,
  LngLatBounds: maplibreMock.LngLatBoundsMock,
  NavigationControl: class {},
}))

vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(),
}))

vi.mock('@/api/activities', () => ({
  fetchActivityStreams: vi.fn(),
}))

vi.mock('@/hooks/useMapLayers', () => ({
  useMapLayers: vi.fn(() => ({ base: [{ key: 'default', type: 'raster' }] })),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: vi.fn(() => ({ data: {} })),
}))

vi.mock('@/lib/mapStyles', () => ({
  buildLayerStyle: vi.fn(() => ({ version: 8, sources: {}, layers: [] })),
}))

import { apiFetch } from '@/lib/auth'
import { fetchActivityStreams } from '@/api/activities'
const mockApiFetch = apiFetch as ReturnType<typeof vi.fn>
const mockFetchStreams = fetchActivityStreams as ReturnType<typeof vi.fn>

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
    started_at: `2024-01-01T09:0${i}:00Z`,
    ended_at: `2024-01-01T09:0${i + 1}:00Z`,
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
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => twoHeuristicCandidates })
    mockFetchStreams.mockResolvedValue([])
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
    expect(screen.getByText(/Alle bestätigen/)).toBeTruthy()
    expect(screen.getByText(/Alle ablehnen/)).toBeTruthy()
    expect(screen.getByText(/Prüfen/)).toBeTruthy()
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

    fireEvent.click(screen.getByText(/Alle bestätigen/))

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

    fireEvent.click(screen.getByText(/Alle ablehnen/))

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

  it('clicking inspect switches to expanded mode showing inspector header', () => {
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

    expect(screen.getByText(/Lifte prüfen/)).toBeTruthy()
    // Compact-mode bulk buttons disappear at top level (they move to footer in expanded)
    expect(screen.queryByText(/Alle bestätigen/)).toBeTruthy() // present in footer
    // The map container is rendered
    expect(screen.getByTestId('lift-inspector-map')).toBeTruthy()
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
    // In expanded mode, "Zurück zur Übersicht" is a footer link
    expect(screen.getByText(/Zurück zur Übersicht/)).toBeTruthy()

    fireEvent.click(screen.getByText(/Zurück zur Übersicht/))

    expect(screen.getByText(/Lifte erkannt/)).toBeTruthy()
    expect(screen.queryByTestId('lift-inspector-map')).toBeNull()
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

  it('expanded mode: per-item Confirm button calls confirm endpoint', async () => {
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

    // Wait for inspector candidates to load (mocked apiFetch returns the candidates)
    await waitFor(() => {
      const confirmBtns = screen.getAllByLabelText(/Bestätigen/)
      expect(confirmBtns.length).toBeGreaterThan(0)
    })

    const confirmBtns = screen.getAllByLabelText(/Bestätigen/)
    fireEvent.click(confirmBtns[0])

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const confirmCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/lift-candidates/') && url.includes('/confirm') && opts?.method === 'POST'
      })
      expect(confirmCall).toBeTruthy()
    })
  })

  it('expanded mode: per-item Reject button calls DELETE endpoint', async () => {
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

    await waitFor(() => {
      const rejectBtns = screen.getAllByLabelText(/Ablehnen/)
      expect(rejectBtns.length).toBeGreaterThan(0)
    })

    const rejectBtns = screen.getAllByLabelText(/Ablehnen/)
    fireEvent.click(rejectBtns[0])

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const rejectCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/lift-candidates/') && opts?.method === 'DELETE'
      })
      expect(rejectCall).toBeTruthy()
    })
  })

  it('expanded mode: unconfirm button calls PATCH /unconfirm', async () => {
    const confirmedCandidates = makeCandidates([
      { id: 'c-confirmed', confirmed: true, rejected_by_user: false, source: 'heuristic' },
    ])

    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={confirmedCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Prüfen/))

    await waitFor(() => {
      expect(screen.getByLabelText(/Rückgängig/)).toBeTruthy()
    })

    fireEvent.click(screen.getByLabelText(/Rückgängig/))

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const unconfirmCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/unconfirm') && opts?.method === 'PATCH'
      })
      expect(unconfirmCall).toBeTruthy()
    })
  })

  it('expanded mode: restore button calls POST /restore for rejected candidate', async () => {
    const rejectedCandidates = makeCandidates([
      { id: 'c-rejected', rejected_by_user: true, confirmed: false, source: 'heuristic' },
    ])

    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={rejectedCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Prüfen/))

    await waitFor(() => {
      expect(screen.getByLabelText(/Wiederherstellen/)).toBeTruthy()
    })

    fireEvent.click(screen.getByLabelText(/Wiederherstellen/))

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const restoreCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/lift-candidates/') && url.includes('/restore') && opts?.method === 'POST'
      })
      expect(restoreCall).toBeTruthy()
    })
  })

  it('recalculate confirm dialog triggers POST /recalculate-all', async () => {
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

    fireEvent.click(screen.getByText(/Prüfen/))

    // Click "Neu erkennen" to open dialog
    await waitFor(() => expect(screen.getByText(/Neu erkennen/)).toBeTruthy())
    fireEvent.click(screen.getByText(/Neu erkennen/))

    // Confirm dialog appears
    await waitFor(() => expect(screen.getByText(/Alle erkannten Liftfahrten/)).toBeTruthy())

    // Confirm recalculate
    fireEvent.click(screen.getByText(/Ja, neu erkennen/))

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const recalcCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/recalculate-all') && opts?.method === 'POST'
      })
      expect(recalcCall).toBeTruthy()
    })

    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('expanded mode: map container is rendered', () => {
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

    expect(screen.getByTestId('lift-inspector-map')).toBeTruthy()
  })
})

describe('LiftInspectorMap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => [] })
    mockFetchStreams.mockResolvedValue([])
  })

  it('renders the map container', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const candidates = makeCandidates([{ id: 'c-0' }])
    const { container } = render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <LiftInspectorMap
          candidates={candidates}
          streams={[]}
          highlightedCandidateId={null}
          onCandidateClick={() => {}}
        />
      </QueryClientProvider>,
    )
    expect(container.querySelector('[data-testid="lift-inspector-map"]')).toBeTruthy()
  })
})
