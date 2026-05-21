import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import SegmentsPage, { shouldFetchNextSegmentPage } from '../SegmentsPage'

const mocks = vi.hoisted(() => ({
  fetchSegments: vi.fn(),
  capabilities: {
    features: {
      segment_recompute: { configured: true, available: true },
      segment_enrichment_suggest: { configured: true, available: true },
    },
  } as Record<string, unknown>,
}))

vi.mock('@/api/segments', async () => {
  const actual = await vi.importActual<typeof import('@/api/segments')>('@/api/segments')
  return {
    ...actual,
    fetchSegments: mocks.fetchSegments,
    fetchSegmentLeaderboard: vi.fn().mockResolvedValue([]),
    deleteSegment: vi.fn().mockResolvedValue(undefined),
    deleteSegments: vi.fn().mockResolvedValue({ deleted: 0 }),
    recomputeSegment: vi.fn().mockResolvedValue(undefined),
    exportSegments: vi.fn(),
  }
})

vi.mock('@/api/auth', () => ({
  getMe: vi.fn().mockResolvedValue({ id: 'user-1', is_admin: true }),
}))

vi.mock('@/lib/globalSportSettings', () => ({
  useGlobalSportSettings: () => ({ hideUnused: false }),
}))

vi.mock('@/lib/useActiveJobs', () => ({
  useActiveJobs: () => ({
    isJobActiveForEntity: () => false,
  }),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

vi.mock('@/lib/modal', () => ({
  useModal: () => ({ openModal: vi.fn() }),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location-search">{location.search}</div>
}

function renderSegmentsPage(initialEntry: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <QueryClientProvider client={queryClient}>
        <Routes>
          <Route path="/segments" element={<><SegmentsPage /><LocationProbe /></>} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('SegmentsPage filters', () => {
  beforeEach(() => {
    mocks.fetchSegments.mockReset()
    mocks.fetchSegments.mockResolvedValue({ segments: [], has_more: false })
    mocks.capabilities = {
      features: {
        segment_recompute: { configured: true, available: true },
        segment_enrichment_suggest: { configured: true, available: true },
      },
    }
  })

  it('preserves explicit empty segment type selection after unchecking the last type', async () => {
    renderSegmentsPage('/segments?segment_types=wassersport')

    const wassersport = await screen.findByRole('checkbox', { name: 'Wassersport' })
    expect(wassersport).toBeChecked()

    fireEvent.click(wassersport)

    await waitFor(() => {
      expect(screen.getByTestId('location-search')).toHaveTextContent('segment_types=')
    })

    for (const label of ['Standard', 'Trail', 'Runde', 'Lift', 'Wintersport', 'Wassersport']) {
      expect(screen.getByRole('checkbox', { name: label })).not.toBeChecked()
    }
    expect(screen.getByText('Keine Segmente gefunden.')).toBeInTheDocument()
  })

  it('uses a global all-segments toggle for completed-only filtering', async () => {
    renderSegmentsPage('/segments')

    const allToggle = await screen.findByRole('checkbox', { name: 'Alle anzeigen' })
    expect(allToggle).not.toBeChecked()

    await waitFor(() => {
      expect(mocks.fetchSegments).toHaveBeenCalledWith(expect.objectContaining({
        completed_only: true,
      }))
    })

    fireEvent.click(allToggle)

    await waitFor(() => {
      expect(screen.getByTestId('location-search')).toHaveTextContent('show_all_segments=true')
    })
    await waitFor(() => {
      expect(screen.getByRole('checkbox', { name: 'Alle anzeigen' })).toBeChecked()
    })
    expect(mocks.fetchSegments).toHaveBeenCalledWith(expect.not.objectContaining({
      completed_only: true,
    }))
  })

  it('offers to show all segments when completed-only filters out every result', async () => {
    renderSegmentsPage('/segments')

    expect(await screen.findByText('Keine abgeschlossenen Segmente gefunden.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Alle anzeigen' }))

    await waitFor(() => {
      expect(screen.getByTestId('location-search')).toHaveTextContent('show_all_segments=true')
    })
  })

  it('hides uncompleted lift rows while completed-only mode is active', async () => {
    mocks.fetchSegments.mockResolvedValue({
      segments: [
        {
          id: 'lift-1',
          name: 'Uncompleted lift',
          sport_group: 'generic',
          segment_type: 'lift',
          segment_type_meta: {},
          effort_count: 0,
          athlete_count: 0,
          ridden_count: 0,
          is_favorite: false,
          visibility: 'public',
          created_by: 'user-1',
          created_at: '2026-05-07T00:00:00Z',
          bidirectional: false,
        },
      ],
      has_more: false,
    })

    renderSegmentsPage('/segments?segment_types=lift')

    expect(await screen.findByText('Keine abgeschlossenen Segmente gefunden.')).toBeInTheDocument()
    expect(screen.queryByText('Uncompleted lift')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Alle anzeigen' }))

    expect(await screen.findByText('Uncompleted lift')).toBeInTheDocument()
  })

  it('migrates legacy show_lifts URLs to the global all-segments toggle', async () => {
    renderSegmentsPage('/segments?segment_types=lift&show_lifts=true')

    await waitFor(() => {
      expect(screen.getByTestId('location-search')).toHaveTextContent('segment_types=lift')
      expect(screen.getByTestId('location-search')).toHaveTextContent('show_all_segments=true')
      expect(screen.getByTestId('location-search')).not.toHaveTextContent('show_lifts')
    })
    expect(await screen.findByRole('checkbox', { name: 'Alle anzeigen' })).toBeChecked()
  })

  it('drops legacy show_lifts URLs without lift context without enabling all segments', async () => {
    renderSegmentsPage('/segments?show_lifts=true')

    await waitFor(() => {
      expect(screen.getByTestId('location-search')).not.toHaveTextContent('show_lifts')
    })
    expect(screen.getByTestId('location-search')).not.toHaveTextContent('show_all_segments=true')
    expect(await screen.findByRole('checkbox', { name: 'Alle anzeigen' })).not.toBeChecked()
    expect(mocks.fetchSegments).toHaveBeenCalledWith(expect.objectContaining({
      completed_only: true,
    }))
  })

  it('keeps core-safe segment actions but hides full-only recompute and enrichment filters', async () => {
    mocks.capabilities = {
      features: {
        segment_recompute: { configured: false, available: false, reason: 'core_profile' },
        segment_enrichment_suggest: { configured: false, available: false, reason: 'core_profile' },
      },
    }
    mocks.fetchSegments.mockResolvedValue({
      segments: [
        {
          id: 'seg-1',
          name: 'Core Segment',
          sport_group: 'foot',
          segment_type: 'standard',
          segment_type_meta: {},
          effort_count: 1,
          athlete_count: 1,
          ridden_count: 1,
          is_favorite: false,
          visibility: 'public',
          created_by: 'user-1',
          created_at: '2026-05-07T00:00:00Z',
          bidirectional: true,
        },
      ],
      has_more: false,
    })

    renderSegmentsPage('/segments')

    expect(await screen.findByText('Core Segment')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Segment zeichnen' })).toBeInTheDocument()
    expect(screen.queryByTitle('Matching neu berechnen')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /Erweiterte Filter/ }))

    expect(screen.queryByText('SAC-Skala')).not.toBeInTheDocument()
    expect(screen.queryByText('MTB-Skala')).not.toBeInTheDocument()
    expect(screen.queryByText('Oberfläche')).not.toBeInTheDocument()
    expect(screen.getByText('Sichtbarkeit')).toBeInTheDocument()
  })

  it('loads checkpoint courses through course kind filters', async () => {
    renderSegmentsPage('/segments?checkpoint_courses=true')

    const checkpointToggle = await screen.findByRole('checkbox', { name: 'Checkpoint-Kurse' })
    expect(checkpointToggle).toBeChecked()

    await waitFor(() => {
      expect(mocks.fetchSegments).toHaveBeenCalledWith(expect.objectContaining({
        course_kinds: ['line', 'checkpoint_sequence'],
      }))
    })
  })

  it('labels checkpoint courses without showing the legacy standard type', async () => {
    mocks.fetchSegments.mockResolvedValue({
      segments: [
        {
          id: 'checkpoint-1',
          name: 'Freie Linie',
          sport_group: 'bike',
          segment_type: 'standard',
          segment_type_meta: {},
          effort_count: 1,
          athlete_count: 1,
          ridden_count: 1,
          is_favorite: false,
          visibility: 'public',
          created_by: 'user-1',
          created_at: '2026-05-07T00:00:00Z',
          bidirectional: false,
          course_kind: 'checkpoint_sequence',
          control_summary: { control_count: 3, required_control_count: 3, primitive_kinds: ['point'] },
        },
      ],
      has_more: false,
    })

    renderSegmentsPage('/segments?checkpoint_courses=true')

    expect(await screen.findByText('Freie Linie')).toBeInTheDocument()
    expect(screen.getByText('Checkpoint-Kurs')).toBeInTheDocument()
    expect(screen.getByText('3 Checkpoints')).toBeInTheDocument()
  })
})

describe('shouldFetchNextSegmentPage', () => {
  it('only fetches when the sentinel is visible, another page exists, and no page fetch is running', () => {
    expect(shouldFetchNextSegmentPage({
      isIntersecting: true,
      hasNextPage: true,
      isFetchingNextPage: false,
    })).toBe(true)

    expect(shouldFetchNextSegmentPage({
      isIntersecting: false,
      hasNextPage: true,
      isFetchingNextPage: false,
    })).toBe(false)

    expect(shouldFetchNextSegmentPage({
      isIntersecting: true,
      hasNextPage: false,
      isFetchingNextPage: false,
    })).toBe(false)

    expect(shouldFetchNextSegmentPage({
      isIntersecting: true,
      hasNextPage: true,
      isFetchingNextPage: true,
    })).toBe(false)
  })
})
