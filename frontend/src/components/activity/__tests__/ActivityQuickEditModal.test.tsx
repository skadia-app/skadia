import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import ActivityQuickEditModal from '../ActivityQuickEditModal'
import type { Activity } from '@/api/types'

vi.mock('@/api/activities', () => ({
  fetchActivity: vi.fn(() => new Promise(() => {})),
  fetchActivityStreams: vi.fn(async () => []),
  updateActivity: vi.fn(),
}))

vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => []),
  createTag: vi.fn(),
  assignActivitiesToTag: vi.fn(),
  unassignActivitiesFromTag: vi.fn(),
}))

vi.mock('../ActivityQuickEditMiniMap', () => ({
  default: () => <div data-testid="quick-edit-minimap" />,
  trackCoordsFromStreams: () => [],
}))

vi.mock('../SportTuplePicker', () => ({
  default: () => <button type="button">Sport Picker</button>,
}))

function activity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    user_id: 'user-1',
    title: 'Partial row',
    description: null,
    sport_type: 'running',
    sub_type: null,
    is_electric: false,
    is_virtual: false,
    is_commute: false,
    exclude_from_heatmap: false,
    exclude_from_stats: false,
    started_at: '2026-05-01T10:00:00Z',
    elapsed_time_s: 3600,
    distance_m: 10000,
    elevation_gain_m: 100,
    elevation_loss_m: 100,
    is_indoor: false,
    tags: [],
    visibility: 'private',
    enrichment_status: 'complete',
    needs_classification: false,
    has_gps: true,
    created_at: '2026-05-01T10:00:00Z',
    updated_at: '2026-05-01T10:00:00Z',
    ...overrides,
  }
}

function renderModal(activityValue: Activity) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ActivityQuickEditModal
          activity={activityValue}
          open
          onClose={vi.fn()}
          onSaved={vi.fn()}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ActivityQuickEditModal', () => {
  it('does not render editable fields until the full activity is loaded', () => {
    renderModal(activity())

    expect(screen.getByText('Aktivität wird geladen...')).toBeInTheDocument()
    expect(screen.queryByText('Titel')).not.toBeInTheDocument()
    expect(screen.queryByText('Tags')).not.toBeInTheDocument()
  })
})
