import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/client'
import SegmentEditModal from './SegmentEditModal'
import type { Segment } from '@/api/segments'

const mocks = vi.hoisted(() => ({
  fetchSegment: vi.fn(),
  updateSegment: vi.fn(),
  publishSegment: vi.fn(),
  reverseSegment: vi.fn(),
  getMe: vi.fn(),
  openModal: vi.fn(),
  addToast: vi.fn(),
}))

vi.mock('@/api/segments', async () => {
  const actual = await vi.importActual<typeof import('@/api/segments')>('@/api/segments')
  return {
    ...actual,
    fetchSegment: mocks.fetchSegment,
    updateSegment: mocks.updateSegment,
    publishSegment: mocks.publishSegment,
    reverseSegment: mocks.reverseSegment,
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

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({
    data: {
      features: {
        segment_publish: { configured: true, available: true },
      },
    },
  }),
}))

vi.mock('./segment-editor/MapPreview', () => ({
  MapPreview: () => <div data-testid="map-preview" />,
}))

vi.mock('./target-objects/VisibilityAccessField', () => ({
  default: ({ onChange }: { onChange: (next: { visibility: string; access_scope_id?: string | null }) => void }) => (
    <button type="button" onClick={() => onChange({ visibility: 'public', access_scope_id: null })}>
      Öffentlich wählen
    </button>
  ),
}))

function segmentFixture(overrides: Partial<Segment> = {}): Segment {
  return {
    id: 'seg-1',
    name: 'Privates Segment',
    sport_group: 'bike',
    segment_type: 'standard',
    segment_type_meta: {},
    effort_count: 0,
    athlete_count: 0,
    ridden_count: 0,
    is_favorite: false,
    visibility: 'private',
    created_by: 'user-1',
    created_at: '2026-05-22T00:00:00Z',
    bidirectional: true,
    ...overrides,
  }
}

function renderModal(segment: Segment = segmentFixture()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <SegmentEditModal segment={segment} onClose={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('SegmentEditModal', () => {
  beforeEach(() => {
    mocks.fetchSegment.mockReset()
    mocks.updateSegment.mockReset()
    mocks.publishSegment.mockReset()
    mocks.reverseSegment.mockReset()
    mocks.getMe.mockReset()
    mocks.openModal.mockReset()
    mocks.addToast.mockReset()
    mocks.fetchSegment.mockImplementation((id: string) => Promise.resolve(segmentFixture({ id })))
    mocks.getMe.mockResolvedValue({ user_id: 'user-1', is_admin: false })
  })

  it('retries publish with source activity privacy confirmation after a publish conflict', async () => {
    mocks.publishSegment
      .mockRejectedValueOnce(new ApiError(409, JSON.stringify({
        title: 'Privacy Confirmation Required',
        detail: 'source activity intersects an active privacy zone; confirm publishing the current segment/course geometry',
      }), {
        title: 'Privacy Confirmation Required',
        detail: 'source activity intersects an active privacy zone; confirm publishing the current segment/course geometry',
      }))
      .mockResolvedValueOnce({ status: 'queued', run_id: 'run-1' })

    renderModal()

    fireEvent.click(await screen.findByRole('button', { name: 'Öffentlich wählen' }))
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    const publishModal = await waitFor(() => mocks.openModal.mock.calls.at(-1)?.[0])
    publishModal.buttons.find((button: { label: string }) => button.label === 'Veröffentlichen')?.onClick()

    await waitFor(() => {
      expect(mocks.publishSegment).toHaveBeenCalledWith('seg-1')
    })
    const confirmationModal = await waitFor(() => {
      const latest = mocks.openModal.mock.calls.at(-1)?.[0]
      expect(latest.title).toBe('Privacy-Zone bestätigen?')
      return latest
    })

    confirmationModal.buttons.find((button: { label: string }) => button.label === 'Trotzdem veröffentlichen')?.onClick()

    await waitFor(() => {
      expect(mocks.publishSegment).toHaveBeenLastCalledWith('seg-1', { sourceActivityPublishConfirmed: true })
    })
  })

  it('marks checkpoint controls immutable and saves only supported checkpoint metadata', async () => {
    mocks.updateSegment.mockResolvedValueOnce(segmentFixture({ course_kind: 'checkpoint_sequence' }))
    renderModal(segmentFixture({ course_kind: 'checkpoint_sequence', controls: [] }))

    expect(await screen.findByText('Checkpoint-Geometrie ist hier nicht bearbeitbar.')).toBeInTheDocument()
    expect(screen.queryByText('Segment-Typ')).not.toBeInTheDocument()
    expect(screen.queryByText('Bidirektionales Matching')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => {
      expect(mocks.updateSegment).toHaveBeenCalledWith('seg-1', expect.not.objectContaining({
        segment_type: expect.anything(),
        segment_type_meta: expect.anything(),
        bidirectional: expect.anything(),
        climb_type: expect.anything(),
      }))
    })
  })
})
