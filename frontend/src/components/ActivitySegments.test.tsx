import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import '@testing-library/jest-dom'
import { act } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ActivitySegments from './ActivitySegments'
import { fetchActivitySegments } from '@/api/segments'

vi.mock('@/api/segments', () => ({
  fetchActivitySegments: vi.fn(),
}))

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('@/components/SegmentEditorModal', () => ({
  default: ({ onSuccess }: { onSuccess: (segmentId?: string) => void }) => (
    <button type="button" onClick={() => onSuccess('segment-1')}>
      Mock segment save
    </button>
  ),
}))

import '@/i18n/i18n'

describe('ActivitySegments', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not refetch activity segment efforts immediately after creating a segment', async () => {
    vi.mocked(fetchActivitySegments).mockResolvedValue([])
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    render(
      <QueryClientProvider client={queryClient}>
        <ActivitySegments
          activityId="activity-1"
          streams={[
            { time: '2026-05-20T12:00:00Z', lat: 48.1, lng: 11.1 },
            { time: '2026-05-20T12:00:10Z', lat: 48.2, lng: 11.2 },
          ]}
        />
      </QueryClientProvider>,
    )

    await screen.findByRole('button', { name: /Segment erstellen/i })

    await userEvent.click(screen.getByRole('button', { name: /Segment erstellen/i }))
    await userEvent.click(screen.getByRole('button', { name: 'Mock segment save' }))

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['segments'] })
    })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ['activity-segments', 'activity-1'] })
  })

  it('silently polls activity segment efforts after creating a segment', async () => {
    vi.mocked(fetchActivitySegments).mockResolvedValue([])
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const refetchSpy = vi.spyOn(queryClient, 'refetchQueries').mockResolvedValue()

    render(
      <QueryClientProvider client={queryClient}>
        <ActivitySegments
          activityId="activity-1"
          streams={[
            { time: '2026-05-20T12:00:00Z', lat: 48.1, lng: 11.1 },
            { time: '2026-05-20T12:00:10Z', lat: 48.2, lng: 11.2 },
          ]}
        />
      </QueryClientProvider>,
    )

    await screen.findByRole('button', { name: /Segment erstellen/i })

    fireEvent.click(screen.getByRole('button', { name: /Segment erstellen/i }))
    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: 'Mock segment save' }))

    expect(refetchSpy).not.toHaveBeenCalledWith({
      queryKey: ['activity-segments', 'activity-1'],
      type: 'active',
    })

    await act(async () => {
      vi.advanceTimersByTime(1_000)
    })

    expect(refetchSpy).toHaveBeenCalledWith({
      queryKey: ['activity-segments', 'activity-1'],
      type: 'active',
    })
  })

  it('labels checkpoint course hits distinctly', async () => {
    vi.mocked(fetchActivitySegments).mockResolvedValue([
      {
        id: 'effort-1',
        segment_id: 'segment-1',
        activity_id: 'activity-1',
        started_at: '2026-05-20T12:00:00Z',
        elapsed_time_s: 120,
        is_reverse: false,
        sport_type: 'ride',
        rank_overall: 1,
        is_personal_best: true,
        is_segment_record: true,
        segment_name: 'Freie Linie',
        course_kind: 'checkpoint_sequence',
        control_summary: { control_count: 3, required_control_count: 3, primitive_kinds: ['point'] },
      },
    ])
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    render(
      <QueryClientProvider client={queryClient}>
        <ActivitySegments activityId="activity-1" />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('Freie Linie')).toBeInTheDocument()
    expect(screen.getByText('Checkpoint-Kurs absolviert')).toBeInTheDocument()
    expect(screen.getByText('Absolviert')).toBeInTheDocument()
  })
})
