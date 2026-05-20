import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { RoutePlannerPanelProps } from '../RoutePlannerPanel'
import RoutePlannerPanel from '../RoutePlannerPanel'
import { ModalProvider } from '@/lib/modal'
import { ToastProvider } from '@/lib/toast'
import * as routesApi from '@/api/routes'

vi.mock('@/api/routes', () => ({
  getProfileDefaults: vi.fn().mockResolvedValue({ profile: 'hiking' }),
  createRouteSnapshot: vi.fn().mockResolvedValue({
    snapshot_id: 'snap-1',
    snapshot_hash: 'hash-1',
    geometry_hash: 'geom-1',
    source_route_version: 7,
  }),
  publishRouteCourse: vi.fn().mockResolvedValue({
    publication_id: 'pub-1',
    course_segment_id: 'seg-1',
    course_segment_version_id: 'ver-1',
  }),
}))

vi.mock('@/api/routingProfiles', () => ({
  fetchAllProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
  createProfile: vi.fn().mockResolvedValue({}),
  deleteProfile: vi.fn().mockResolvedValue({}),
  importProfiles: vi.fn().mockResolvedValue({ imported: 0 }),
  exportProfileUrl: vi.fn(() => '/api/v1/routing/profiles/profile-1/export'),
}))

vi.mock('@/api/client', () => ({
  apiDownload: vi.fn(),
}))

function renderPanel(overrides: Partial<RoutePlannerPanelProps> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  const props: RoutePlannerPanelProps = {
    waypoints: [],
    segments: [],
    profileConfig: { profile: 'hiking' },
    showHeatmap: false,
    routingEngine: 'badrouter',
    brouterProfile: 'trekking',
    badRouterAvailable: false,
    routeSuggestionAvailable: false,
    onRoutingEngineChange: vi.fn(),
    onBrouterProfileChange: vi.fn(),
    totalDistanceM: 0,
    totalElevationGain: 0,
    totalElevationLoss: 0,
    onProfileChange: vi.fn(),
    onRemoveWaypoint: vi.fn(),
    onReorderWaypoints: vi.fn(),
    onRecalculateAll: vi.fn(),
    onToggleSegmentMode: vi.fn(),
    onToggleHeatmap: vi.fn(),
    onClearRoute: vi.fn(),
    onSave: vi.fn(),
    onSuggest: vi.fn(),
    onClose: vi.fn(),
    ...overrides,
  }

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ToastProvider>
        <ModalProvider>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ModalProvider>
      </ToastProvider>
    )
  }

  return render(<RoutePlannerPanel {...props} />, { wrapper: Wrapper })
}

describe('RoutePlannerPanel capability gates', () => {
  it('hides BadRouter-only controls and falls back to BRouter surface in Core', async () => {
    renderPanel()

    expect(screen.queryByRole('button', { name: 'BadRouter' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'BRouter' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Route vorschlagen/ })).not.toBeInTheDocument()
    expect(screen.queryByTitle('Profilverwaltung')).not.toBeInTheDocument()

    await waitFor(() => {
      expect(routesApi.getProfileDefaults).not.toHaveBeenCalled()
    })
  })

  it('publishes a saved route with explicit operation ids', async () => {
    const randomUUID = vi.spyOn(crypto, 'randomUUID').mockReturnValue('00000000-0000-4000-8000-000000000001')
    renderPanel({
      waypoints: [{ id: 'a', latlng: [0, 0], type: 'start' }, { id: 'b', latlng: [1, 1], type: 'end' }],
      segments: [{
        from_waypoint: 'a',
        to_waypoint: 'b',
        mode: 'routed',
        control_points: [],
        smooth: false,
        distance_m: 1000,
        elevation_gain: 10,
        elevation_loss: 0,
        geometry: { type: 'LineString', coordinates: [[0, 0], [1, 1]] },
      }],
      totalDistanceM: 1000,
      editingRouteId: 'route-1',
      editingRouteVersion: 7,
    })

    fireEvent.click(screen.getByRole('button', { name: 'Als Kurs veröffentlichen' }))

    await waitFor(() => {
      expect(routesApi.createRouteSnapshot).toHaveBeenCalledWith('route-1', '00000000-0000-4000-8000-000000000001:snapshot', 7)
    })
    expect(routesApi.publishRouteCourse).toHaveBeenCalledWith('route-1', {
      route_snapshot_id: 'snap-1',
      source_route_version: 7,
      publish_policy: 'new_course',
      equivalence_decision: 'new_group',
      visibility: 'private',
      compatibility_preset: 'standard',
      operation_id: '00000000-0000-4000-8000-000000000001:publish',
    })

    randomUUID.mockRestore()
  })
})
