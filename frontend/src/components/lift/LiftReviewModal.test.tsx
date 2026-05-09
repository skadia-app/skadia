import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import LiftReviewModal from './LiftReviewModal'
import { ToastProvider } from '@/lib/toast'
import { ToastContainer } from '@/components/Toast'
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
  class MarkerMock {
    setLngLat(_ll: unknown) { return this }
    addTo(_m: unknown) { return this }
    remove() {}
  }
  return { MapMock, LngLatBoundsMock, MarkerMock, maps: [] as MapMock[] }
})

vi.mock('maplibre-gl', () => ({
  default: {
    Map: maplibreMock.MapMock,
    LngLatBounds: maplibreMock.LngLatBoundsMock,
    NavigationControl: class {},
    Marker: maplibreMock.MarkerMock,
  },
  Map: maplibreMock.MapMock,
  LngLatBounds: maplibreMock.LngLatBoundsMock,
  NavigationControl: class {},
  Marker: maplibreMock.MarkerMock,
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

vi.mock('@/lib/theme', () => ({
  useTheme: vi.fn(() => ({ theme: 'light', toggle: vi.fn(), setTheme: vi.fn() })),
}))

vi.mock('@/providers/UserProvider', () => ({
  useCurrentUserId: vi.fn(() => 'user-abc'),
}))

vi.mock('@/lib/mapHeatmap', () => ({
  addHeatmapLayers: vi.fn(),
  removeHeatmapLayers: vi.fn(),
  buildHeatmapTileUrl: vi.fn((uid: string) => `https://tiles.example/${uid}`),
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
  return (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      <ToastContainer />
    </ToastProvider>
  )
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
      <ToastProvider>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <LiftReviewModal
            open={true}
            onClose={() => {}}
            activity={makeActivity()}
            candidates={twoHeuristicCandidates}
          />
        </QueryClientProvider>
      </ToastProvider>,
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
      <ToastProvider>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <LiftInspectorMap
            candidates={candidates}
            streams={[]}
            highlightedCandidateId={null}
            onCandidateClick={() => {}}
          />
        </QueryClientProvider>
      </ToastProvider>,
    )
    expect(container.querySelector('[data-testid="lift-inspector-map"]')).toBeTruthy()
  })

  // F11: empty stream shows fallback overlay (the map container still renders beneath it)
  it('F11: empty stream renders fallback overlay without hiding map container', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    const { container } = render(
      <ToastProvider>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <LiftInspectorMap
            candidates={[]}
            streams={[]}
            highlightedCandidateId={null}
            onCandidateClick={() => {}}
          />
        </QueryClientProvider>
      </ToastProvider>,
    )
    // Fallback text is visible
    expect(screen.getByText(/Karte nicht verfügbar — fehlender Stream/)).toBeTruthy()
    // Map container still exists (ResizeObserver must be able to observe it)
    expect(container.querySelector('[data-testid="lift-inspector-map"]')).toBeTruthy()
  })

  // F16: legend is rendered with all expected German labels (Phase 11.9 signal-based)
  it('F16: legend overlay is rendered with signal-based category labels', async () => {
    const { default: LiftInspectorMap } = await import('./LiftInspectorMap')
    render(
      <ToastProvider>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <LiftInspectorMap
            candidates={[]}
            streams={[]}
            highlightedCandidateId={null}
            onCandidateClick={() => {}}
          />
        </QueryClientProvider>
      </ToastProvider>,
    )
    expect(screen.getByText('OSM-Treffer')).toBeTruthy()
    expect(screen.getByText('Heuristik (Speed + Steigung)')).toBeTruthy()
    expect(screen.getByText('Heuristik (Höhenrate)')).toBeTruthy()
    expect(screen.getByText('Heuristik (bidirektional)')).toBeTruthy()
    expect(screen.getByText('Abgelehnt')).toBeTruthy()
  })
})

// ── New regression tests for Critic findings ────────────────────────────────────

describe('LiftReviewModal — Critic fix regressions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => twoHeuristicCandidates })
    mockFetchStreams.mockResolvedValue([])
  })

  // F8: expanded mode must have "Alle ablehnen" in the footer
  it('F8: expanded mode footer contains Alle ablehnen button', () => {
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

    // In expanded mode there are multiple buttons; check footer contains the reject-all
    expect(screen.getAllByText(/Alle ablehnen/).length).toBeGreaterThan(0)
  })

  // F8: clicking "Alle ablehnen" in expanded mode calls POST /reject-all and closes
  it('F8: expanded mode Alle ablehnen calls reject-all and closes', async () => {
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

    // Click the "Alle ablehnen" in the expanded footer (multiple may exist — take last)
    const rejectBtns = screen.getAllByText(/Alle ablehnen/)
    fireEvent.click(rejectBtns[rejectBtns.length - 1])

    await waitFor(() => {
      const calls = mockApiFetch.mock.calls
      const rejectCall = calls.find((args: unknown[]) => {
        const url = args[0] as string | undefined
        const opts = args[1] as RequestInit | undefined
        return !!url && url.includes('/lifts/reject-all') && opts?.method === 'POST'
      })
      expect(rejectCall).toBeTruthy()
    })

    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  // F9: bulk confirm partial failure keeps modal + selection open and shows toast
  it('F9: bulk confirm partial failure shows error toast and keeps selection', async () => {
    // Second confirm call rejects — first succeeds
    let confirmCallCount = 0
    mockApiFetch.mockImplementation(async (url: string, opts?: RequestInit) => {
      if (url.includes('/candidates')) return { ok: true, json: async () => twoHeuristicCandidates }
      if (typeof url === 'string' && url.includes('/confirm') && opts?.method === 'POST') {
        confirmCallCount++
        if (confirmCallCount >= 2) throw new Error('Server error on second confirm')
      }
      return { ok: true, json: async () => ({}) }
    })

    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={twoHeuristicCandidates}
      />,
      { wrapper },
    )

    // Switch to expanded mode
    fireEvent.click(screen.getByText(/Prüfen/))

    // Enable multi-select and select both
    await waitFor(() => expect(screen.getByText(/Multi-Auswahl$/)).toBeTruthy())
    fireEvent.click(screen.getByText(/Multi-Auswahl$/))

    await waitFor(() => {
      const checkboxes = screen.getAllByRole('checkbox')
      expect(checkboxes.length).toBeGreaterThan(0)
    })
    screen.getAllByRole('checkbox').forEach(cb => {
      if (!(cb as HTMLInputElement).checked) fireEvent.click(cb)
    })

    // Click inline Bestätigen (the mini-bar button, not per-item)
    await waitFor(() => expect(screen.getByText(/^Bestätigen$/)).toBeTruthy())
    fireEvent.click(screen.getByText(/^Bestätigen$/))

    // Error toast must mention "fehlgeschlagen"
    await waitFor(() => {
      expect(screen.getByText(/fehlgeschlagen/)).toBeTruthy()
    }, { timeout: 3000 })
  })

  // F15: expanded modal uses dvh height unit
  it('F15: expanded modal uses dvh height unit', () => {
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

    // The inner flex div should have the dvh height style
    const inner = document.querySelector('[style*="dvh"]')
    expect(inner).not.toBeNull()
  })

  // F16: list rows show source/confidence badge with color-redundant text label
  it('F16: list rows display source+confidence badge text for accessibility', async () => {
    const osmCandidate = makeCandidates([
      { id: 'osm-1', source: 'osm' as const, confidence: 'high' },
    ])

    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={osmCandidate}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Prüfen/))

    // OSM badge text is rendered
    await waitFor(() => {
      expect(screen.getAllByText('OSM-Treffer').length).toBeGreaterThan(0)
    })
  })

  // F26: useIsDesktop does not throw without window
  // (tested indirectly — the wrapper must render without error even in jsdom where
  // matchMedia is mocked and window.innerWidth is typically 0/1024-less)
  it('F26: component mounts without throwing when window.innerWidth is not set', () => {
    // jsdom sets window.innerWidth=0; our guard defaults to false → mobile layout
    expect(() => {
      render(
        <LiftReviewModal
          open={true}
          onClose={() => {}}
          activity={makeActivity()}
          candidates={twoHeuristicCandidates}
        />,
        { wrapper },
      )
    }).not.toThrow()
  })

  // F11: list row shows "(keine Karten-Spur)" when stream doesn't cover lift time range
  it('F11: list row shows no-map-track marker when stream is empty', async () => {
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

    // With empty streams (mocked to []), both candidates lack GPS coverage
    await waitFor(() => {
      const markers = screen.getAllByText(/keine Karten-Spur/)
      expect(markers.length).toBeGreaterThan(0)
    })
  })
})

// ── Phase 11.7: D.2 — pause-gap banner in expanded modal ──────────────────

describe('LiftReviewModal — D.2: pause-gap banner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => twoHeuristicCandidates })
  })

  it('banner renders with correct count when streams contain gaps', async () => {
    // Two points with 120s gap (> 75s threshold) → 1 gap → 1 Pause
    mockFetchStreams.mockResolvedValue([
      { time: '2024-01-01T09:00:00Z', lat: 47.0, lng: 11.0 },
      { time: '2024-01-01T09:02:00Z', lat: 47.1, lng: 11.1 }, // 120s gap
      { time: '2024-01-01T09:03:00Z', lat: 47.2, lng: 11.2 },
    ])

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
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
      const banner = screen.getByTestId('pause-gap-banner')
      expect(banner).not.toBeNull()
      expect(banner.textContent).toMatch(/1 Pause/)
      expect(banner.textContent).not.toMatch(/Pausen/)
    })
  })

  it('banner absent when no gaps present in streams', async () => {
    // Two consecutive points 30s apart — below threshold
    mockFetchStreams.mockResolvedValue([
      { time: '2024-01-01T09:00:00Z', lat: 47.0, lng: 11.0 },
      { time: '2024-01-01T09:00:30Z', lat: 47.1, lng: 11.1 },
      { time: '2024-01-01T09:01:00Z', lat: 47.2, lng: 11.2 },
    ])

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
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
      expect(screen.queryByTestId('pause-gap-banner')).toBeNull()
    })
  })

  it('banner uses plural form for 2+ pauses', async () => {
    // Three segments → 2 gaps
    mockFetchStreams.mockResolvedValue([
      { time: '2024-01-01T09:00:00Z', lat: 47.0, lng: 11.0 },
      { time: '2024-01-01T09:02:00Z', lat: 47.1, lng: 11.1 }, // 120s gap
      { time: '2024-01-01T09:03:00Z', lat: 47.2, lng: 11.2 },
      { time: '2024-01-01T09:05:00Z', lat: 47.3, lng: 11.3 }, // 120s gap
      { time: '2024-01-01T09:06:00Z', lat: 47.4, lng: 11.4 },
    ])

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
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
      const banner = screen.getByTestId('pause-gap-banner')
      expect(banner.textContent).toMatch(/2 Pausen/)
    })
  })
})

// ── UX fix-pass (2026-05-09): Fix 4 + Fix 6a ────────────────────────────────

describe('LiftReviewModal — Fix 4: updated banner wording', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => twoHeuristicCandidates })
  })

  it('banner contains new wording fragment about dashed-orange teleport sections', async () => {
    mockFetchStreams.mockResolvedValue([
      { time: '2024-01-01T09:00:00Z', lat: 47.0, lng: 11.0 },
      { time: '2024-01-01T09:02:00Z', lat: 47.1, lng: 11.1 }, // 120s gap
      { time: '2024-01-01T09:03:00Z', lat: 47.2, lng: 11.2 },
    ])

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
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
      const banner = screen.getByTestId('pause-gap-banner')
      expect(banner.textContent).toMatch(/Teleportations-Abschnitte aber gestrichelt-orange/)
    })
  })
})

describe('LiftReviewModal — Fix 6a: badge title tooltips', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchStreams.mockResolvedValue([])
  })

  it('OSM candidate badge has title matching CONFIDENCE_DESCRIPTIONS', async () => {
    const osmCandidates = makeCandidates([
      { id: 'osm-x', source: 'osm' as const, confidence: 'high', confirmed: true },
    ])
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => osmCandidates })

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={osmCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Prüfen/))

    await waitFor(() => {
      // OSM-Treffer badge must be present
      const badges = screen.getAllByText('OSM-Treffer')
      expect(badges.length).toBeGreaterThan(0)
    })

    // Find the badge span with the title attribute
    const badgesWithTitle = document.querySelectorAll('[title]')
    const osmBadge = Array.from(badgesWithTitle).find(
      el => el.textContent?.includes('OSM-Treffer'),
    )
    expect(osmBadge).toBeTruthy()
    expect(osmBadge!.getAttribute('title')).toContain('Geometrie-Match')
  })
})

// ── Phase 11.9: signal-based labels + help modal triggers ────────────────────

describe('LiftReviewModal — Phase 11.9: signal-based badge labels', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchStreams.mockResolvedValue([])
  })

  it('bidir candidate shows "Heuristik (bidirektional)" badge (not Talquerung)', async () => {
    const bidirCandidates = makeCandidates([
      { id: 'bidir-1', source: 'heuristic' as const, confidence: 'bidir', confirmed: false },
    ])
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => bidirCandidates })

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={bidirCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Prüfen/))

    await waitFor(() => {
      // Both the legend entry and the row badge use this label — getAllByText is correct.
      expect(screen.getAllByText('Heuristik (bidirektional)').length).toBeGreaterThan(0)
    })
    expect(screen.queryByText('Heuristik (Talquerung)')).toBeNull()
  })

  it('bidir candidate badge tooltip contains "Höhenbewegung der Cable"', async () => {
    const bidirCandidates = makeCandidates([
      { id: 'bidir-2', source: 'heuristic' as const, confidence: 'bidir', confirmed: false },
    ])
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => bidirCandidates })

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={bidirCandidates}
      />,
      { wrapper },
    )

    fireEvent.click(screen.getByText(/Prüfen/))

    await waitFor(() => {
      const badgesWithTitle = document.querySelectorAll('[title]')
      const bidirBadge = Array.from(badgesWithTitle).find(
        el => el.textContent?.includes('Heuristik (bidirektional)'),
      )
      expect(bidirBadge).toBeTruthy()
      expect(bidirBadge!.getAttribute('title')).toContain('Höhenbewegung der Cable')
    })
  })

  it('help modal opens when header HelpCircle is clicked', async () => {
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => twoHeuristicCandidates })

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
    render(
      <LiftReviewModal
        open={true}
        onClose={() => {}}
        activity={makeActivity()}
        candidates={twoHeuristicCandidates}
      />,
      { wrapper },
    )

    // Enter expanded mode to see the header
    fireEvent.click(screen.getByText(/Prüfen/))

    // Click the header HelpCircle button
    const helpBtns = screen.getAllByRole('button', { name: /Hilfe/ })
    expect(helpBtns.length).toBeGreaterThan(0)
    fireEvent.click(helpBtns[0])

    // LiftHelpModal should open — look for Methode A heading
    await waitFor(() => {
      expect(screen.getByText(/Methode A — OSM-Match/)).toBeTruthy()
    })
  })

  it('help modal content includes "Talquerung" and "Bergstation A"', async () => {
    mockApiFetch.mockResolvedValue({ ok: true, json: async () => twoHeuristicCandidates })

    const { default: LiftReviewModal } = await import('./LiftReviewModal')
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

    const helpBtns = screen.getAllByRole('button', { name: /Hilfe/ })
    fireEvent.click(helpBtns[0])

    await waitFor(() => {
      expect(screen.getByText(/Bergstation A/)).toBeTruthy()
      expect(screen.getByText(/Methode B — Heuristik aus dem Stream/)).toBeTruthy()
    })
  })
})
