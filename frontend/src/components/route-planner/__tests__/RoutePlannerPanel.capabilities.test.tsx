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
  forkRoute: vi.fn().mockResolvedValue({ route_id: 'route-fork', relationship_id: 'rel-fork' }),
  createRouteRelationship: vi.fn().mockResolvedValue({ relationship_id: 'rel-route' }),
  createRouteCourseRelationship: vi.fn().mockResolvedValue({ relationship_id: 'rel-course' }),
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
  createRoutePublicationPrivacyReview: vi.fn().mockResolvedValue({
    route_snapshot_id: 'snap-1',
    audience_class: 'shared',
    review_hash: 'review-1',
  }),
}))

vi.mock('@/api/routingProfiles', () => ({
  fetchAllProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
  createProfile: vi.fn().mockResolvedValue({}),
  deleteProfile: vi.fn().mockResolvedValue({}),
  importProfiles: vi.fn().mockResolvedValue({ imported: 0 }),
  exportProfileUrl: vi.fn(() => '/api/v1/routing/profiles/profile-1/export'),
}))

vi.mock('@/api/accessScopes', () => ({
  listObjectAccessScopes: vi.fn().mockResolvedValue({
    scopes: [
      {
        id: 'scope-1',
        owner_kind: 'user',
        scope_kind: 'manual',
        label: 'Freunde',
        created_at: '2026-05-22T00:00:00Z',
      },
    ],
  }),
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
      waypoints: [
        { id: 'a', latlng: [0, 0], type: 'start' },
        { id: 'b', latlng: [1, 1], type: 'end' },
      ],
      segments: [
        {
          from_waypoint: 'a',
          to_waypoint: 'b',
          mode: 'routed',
          control_points: [],
          smooth: false,
          distance_m: 1000,
          elevation_gain: 10,
          elevation_loss: 0,
          geometry: {
            type: 'LineString',
            coordinates: [
              [0, 0],
              [1, 1],
            ],
          },
        },
      ],
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

  it('publishes shared existing-course route publications with target context', async () => {
    const randomUUID = vi.spyOn(crypto, 'randomUUID').mockReturnValue('00000000-0000-4000-8000-000000000002')
    renderPanel({
      waypoints: [
        { id: 'a', latlng: [0, 0], type: 'start' },
        { id: 'b', latlng: [1, 1], type: 'end' },
      ],
      segments: [
        {
          from_waypoint: 'a',
          to_waypoint: 'b',
          mode: 'routed',
          control_points: [],
          smooth: false,
          distance_m: 1000,
          elevation_gain: 10,
          elevation_loss: 0,
          geometry: {
            type: 'LineString',
            coordinates: [
              [0, 0],
              [1, 1],
            ],
          },
        },
      ],
      totalDistanceM: 1000,
      editingRouteId: 'route-1',
      editingRouteVersion: 7,
    })

    fireEvent.change(screen.getByLabelText('Sichtbarkeit der Kursveröffentlichung'), { target: { value: 'shared' } })
    await screen.findByRole('option', { name: 'Freunde' })
    fireEvent.change(screen.getByLabelText('Freigabe-Scope'), {
      target: { value: 'scope-1' },
    })
    fireEvent.change(screen.getByLabelText('Veröffentlichungsart'), {
      target: { value: 'new_version_same_course' },
    })
    fireEvent.change(screen.getByLabelText('Ziel-Veröffentlichung'), {
      target: { value: 'pub-existing' },
    })
    fireEvent.change(screen.getByLabelText('Ziel-Kurs'), {
      target: { value: 'course-1' },
    })
    fireEvent.change(screen.getByLabelText('Ziel-Version'), {
      target: { value: 'version-1' },
    })
    fireEvent.click(screen.getByRole('checkbox', { name: /Privacy-Review bestätigt/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Als Kurs veröffentlichen' }))

    await waitFor(() => {
      expect(routesApi.createRoutePublicationPrivacyReview).toHaveBeenCalledWith('route-1', {
        route_snapshot_id: 'snap-1',
        source_route_version: 7,
        audience_class: 'shared',
        owner_confirmed: true,
      })
      expect(routesApi.publishRouteCourse).toHaveBeenCalledWith(
        'route-1',
        expect.objectContaining({
          publish_policy: 'new_version_same_course',
          equivalence_decision: 'version_specific',
          visibility: 'shared',
          access_scope_id: 'scope-1',
          target_publication_id: 'pub-existing',
          target_course_segment_id: 'course-1',
          target_course_segment_version_id: 'version-1',
        }),
      )
    })

    randomUUID.mockRestore()
  })

  it('keeps existing-course publication disabled until target publication and course are set', async () => {
    renderPanel({
      waypoints: [
        { id: 'a', latlng: [0, 0], type: 'start' },
        { id: 'b', latlng: [1, 1], type: 'end' },
      ],
      segments: [
        {
          from_waypoint: 'a',
          to_waypoint: 'b',
          mode: 'routed',
          control_points: [],
          smooth: false,
          distance_m: 1000,
          elevation_gain: 10,
          elevation_loss: 0,
          geometry: {
            type: 'LineString',
            coordinates: [
              [0, 0],
              [1, 1],
            ],
          },
        },
      ],
      totalDistanceM: 1000,
      editingRouteId: 'route-1',
      editingRouteVersion: 7,
    })

    const publishButton = screen.getByRole('button', {
      name: 'Als Kurs veröffentlichen',
    })
    fireEvent.change(screen.getByLabelText('Veröffentlichungsart'), {
      target: { value: 'new_version_same_course' },
    })
    expect(publishButton).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Ziel-Kurs'), {
      target: { value: 'course-1' },
    })
    expect(publishButton).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Ziel-Veröffentlichung'), {
      target: { value: 'pub-existing' },
    })
    expect(publishButton).toBeEnabled()
  })

  it('creates fork and relationship operations from the route panel', async () => {
    const randomUUID = vi.spyOn(crypto, 'randomUUID').mockReturnValue('00000000-0000-4000-8000-000000000003')
    renderPanel({
      editingRouteId: 'route-1',
      editingRouteVersion: 7,
      editingRoute: {
        id: 'route-1',
        name: 'Ausgangsroute',
        sport_type: 'cycling',
        profile: 'trekking',
        waypoints: [],
        segments: [],
        time_override: false,
        auto_tag: false,
        photos: [],
        tags: [],
        created_at: '2026-05-22T00:00:00Z',
        updated_at: '2026-05-22T00:00:00Z',
      },
    })

    fireEvent.change(screen.getByLabelText('Fork-Name'), {
      target: { value: 'Variante Nord' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Fork erstellen' }))

    await waitFor(() => {
      expect(routesApi.forkRoute).toHaveBeenCalledWith('route-1', {
        name: 'Variante Nord',
        source_route_version: 7,
        operation_id: '00000000-0000-4000-8000-000000000003:fork',
      })
    })
    expect(await screen.findByRole('link', { name: 'Fork öffnen' })).toHaveAttribute(
      'href',
      '/map?planner=1&edit_route_id=route-fork',
    )

    fireEvent.change(screen.getByLabelText('Zielroute'), {
      target: { value: 'route-2' },
    })
    expect(screen.queryByRole('option', { name: 'Fork' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Beziehung speichern' }))

    await waitFor(() => {
      expect(routesApi.createRouteRelationship).toHaveBeenCalledWith('route-1', {
        target_route_id: 'route-2',
        relation_type: 'variant_of',
        operation_id: '00000000-0000-4000-8000-000000000003:route-relationship',
      })
    })

    fireEvent.change(screen.getByLabelText('Quell-Kursveröffentlichung'), {
      target: { value: 'pub-source' },
    })
    fireEvent.change(screen.getByLabelText('Ziel-Kursveröffentlichung'), {
      target: { value: 'pub-target' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Kursbeziehung speichern' }))

    await waitFor(() => {
      expect(routesApi.createRouteCourseRelationship).toHaveBeenCalledWith('pub-source', {
        target_publication_id: 'pub-target',
        relation_type: 'variant_of',
        operation_id: '00000000-0000-4000-8000-000000000003:course-relationship',
      })
    })

    randomUUID.mockRestore()
  })

  it('resets route-publication privacy review when audience changes', async () => {
    renderPanel({
      waypoints: [
        { id: 'a', latlng: [0, 0], type: 'start' },
        { id: 'b', latlng: [1, 1], type: 'end' },
      ],
      segments: [
        {
          from_waypoint: 'a',
          to_waypoint: 'b',
          mode: 'routed',
          control_points: [],
          smooth: false,
          distance_m: 1000,
          elevation_gain: 10,
          elevation_loss: 0,
          geometry: {
            type: 'LineString',
            coordinates: [
              [0, 0],
              [1, 1],
            ],
          },
        },
      ],
      totalDistanceM: 1000,
      editingRouteId: 'route-1',
      editingRouteVersion: 7,
    })

    fireEvent.change(screen.getByLabelText('Sichtbarkeit der Kursveröffentlichung'), { target: { value: 'shared' } })
    await screen.findByRole('option', { name: 'Freunde' })
    fireEvent.change(screen.getByLabelText('Freigabe-Scope'), {
      target: { value: 'scope-1' },
    })
    const review = screen.getByRole('checkbox', {
      name: /Privacy-Review bestätigt/,
    })
    fireEvent.click(review)
    expect(review).toBeChecked()

    fireEvent.change(screen.getByLabelText('Sichtbarkeit der Kursveröffentlichung'), { target: { value: 'public' } })

    await waitFor(() => {
      expect(screen.getByRole('checkbox', { name: /Privacy-Review bestätigt/ })).not.toBeChecked()
    })
  })
})
