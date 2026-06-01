import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/client'
import SegmentDetailPage from './SegmentDetailPage'

const mocks = vi.hoisted(() => ({
  fetchSegment: vi.fn(),
  publishSegment: vi.fn(),
  getMe: vi.fn(),
  openModal: vi.fn(),
  addToast: vi.fn(),
}))
const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'de',
  units: 'metric',
}))

vi.mock('@/api/segments', async () => {
  const actual = await vi.importActual<typeof import('@/api/segments')>('@/api/segments')
  return {
    ...actual,
    fetchSegment: mocks.fetchSegment,
    publishSegment: mocks.publishSegment,
    recomputeSegment: vi.fn(),
    deleteSegment: vi.fn(),
    exportSegment: vi.fn(),
  }
})

vi.mock('@/api/auth', () => ({
  getMe: mocks.getMe,
}))

vi.mock('@/lib/modal', () => ({
  useModal: () => ({ openModal: mocks.openModal }),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: mocks.addToast }),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/lib/useActiveJobs', () => ({
  useActiveJobs: () => ({ isJobActiveForEntity: () => false }),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({
    data: {
      features: {
        segment_publish: { configured: true, available: true },
        segment_recompute: { configured: false, available: false },
      },
    },
  }),
}))

vi.mock('@/components/SegmentDetail/SegmentMinimap', () => ({
  SegmentMinimap: () => <div data-testid="segment-minimap" />,
}))
vi.mock('@/components/SegmentDetail/SegmentActivityList', () => ({
  SegmentActivityList: () => <div data-testid="segment-activity-list" />,
}))
vi.mock('@/components/SegmentDetail/SegmentPerformanceChart', () => ({
  SegmentPerformanceChart: () => <div data-testid="segment-performance-chart" />,
}))
vi.mock('@/components/SegmentDetail/FavoriteButton', () => ({
  FavoriteButton: () => <button type="button">Favorit</button>,
}))
vi.mock('@/components/target-overlays/TargetOverlayTabs', () => ({
  default: () => <div data-testid="target-overlay-tabs" />,
}))
vi.mock('@/components/SegmentEditModal', () => ({
  default: () => <div data-testid="segment-edit-modal" />,
}))
vi.mock('@/components/DifficultyBadge', () => ({
  default: () => <span data-testid="difficulty-badge" />,
}))
vi.mock('@/components/DateRangePicker', () => ({
  default: () => <div data-testid="date-range-picker" />,
}))

function segmentFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: 'seg-1',
    name: 'Testsegment',
    sport_group: 'bike',
    segment_type: 'standard',
    segment_type_meta: {},
    effort_count: 0,
    athlete_count: 0,
    ridden_count: 0,
    is_favorite: false,
    visibility: 'private' as const,
    created_by: 'user-1',
    created_at: '2026-05-22T00:00:00Z',
    bidirectional: true,
    ...overrides,
  }
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <MemoryRouter initialEntries={['/segments/seg-1']}>
      <QueryClientProvider client={queryClient}>
        <Routes>
          <Route path="/segments/:id" element={<SegmentDetailPage />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('SegmentDetailPage publish privacy gate', () => {
  beforeEach(() => {
    mocks.fetchSegment.mockReset()
    mocks.publishSegment.mockReset()
    mocks.getMe.mockReset()
    mocks.openModal.mockReset()
    mocks.addToast.mockReset()
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
    mocks.fetchSegment.mockResolvedValue(segmentFixture())
    mocks.getMe.mockResolvedValue({ user_id: 'user-1', is_admin: false })
  })

  it('confirms a privacy-zone publish conflict and retries with source_activity_publish_confirmed', async () => {
    mocks.publishSegment
      .mockRejectedValueOnce(new ApiError(409, JSON.stringify({
        title: 'Privacy Confirmation Required',
        detail: 'source activity intersects an active privacy zone; confirm publishing the current segment/course geometry',
      }), {
        title: 'Privacy Confirmation Required',
        detail: 'source activity intersects an active privacy zone; confirm publishing the current segment/course geometry',
      }))
      .mockResolvedValueOnce({ status: 'queued', run_id: 'run-1' })

    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: 'Veröffentlichen' }))
    const firstModal = await waitFor(() => mocks.openModal.mock.calls.at(-1)?.[0])
    firstModal.buttons.find((button: { label: string }) => button.label === 'Veröffentlichen')?.onClick()

    await waitFor(() => {
      expect(mocks.publishSegment).toHaveBeenCalledWith('seg-1')
    })
    const confirmationModal = await waitFor(() => {
      const latest = mocks.openModal.mock.calls.at(-1)?.[0]
      expect(latest.title).toBe('Privacy-Zone bestätigen?')
      return latest
    })
    expect(confirmationModal.message).toContain('aktive Privacy-Zone')

    confirmationModal.buttons.find((button: { label: string }) => button.label === 'Trotzdem veröffentlichen')?.onClick()

    await waitFor(() => {
      expect(mocks.publishSegment).toHaveBeenLastCalledWith('seg-1', { sourceActivityPublishConfirmed: true })
    })
  })

  it('exposes supported edit and delete actions for checkpoint courses', async () => {
    mocks.fetchSegment.mockResolvedValue(segmentFixture({
      course_kind: 'checkpoint_sequence',
      control_summary: { control_count: 2, required_control_count: 2, primitive_kinds: ['point'] },
    }))

    renderPage()

    expect(await screen.findByText('Checkpoint-Kurs')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Bearbeiten' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Löschen' })).toBeInTheDocument()
  })

  it('uses generic labels for unknown segment taxonomy and climb values', async () => {
    mocks.fetchSegment.mockResolvedValue(segmentFixture({
      sport_group: 'mystery_group',
      segment_type: 'mystery_type',
      climb_type: 'mystery_climb',
    }))

    renderPage()

    expect(await screen.findByText('Testsegment')).toBeInTheDocument()
    expect(screen.getByText('Unbekannte Sportgruppe')).toBeInTheDocument()
    expect(screen.getByText('Unbekannter Segmenttyp')).toBeInTheDocument()
    expect(screen.getByText('Unbekanntes Höhenprofil')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('mystery_group')
    expect(document.body).not.toHaveTextContent('mystery_type')
    expect(document.body).not.toHaveTextContent('mystery_climb')
  })

  it('formats segment distance and elevation with the selected user units', async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    mocks.fetchSegment.mockResolvedValue(segmentFixture({
      distance_m: 1609.344,
      elevation_gain_m: 100,
    }))

    renderPage()

    expect(await screen.findByText('Testsegment')).toBeInTheDocument()
    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('+328 ft')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('1.61 km')
    expect(document.body).not.toHaveTextContent('+100 m')
  })
})
