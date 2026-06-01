import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Activity } from '@/api/types'
import type { PlannedRoute } from '@/api/routes'
import { ToastProvider } from '@/lib/toast'
import ActivityHeader from './ActivityHeader'
import { reprocessActivity } from '@/api/activities'
import { getRouteDraftFromActivity } from '@/api/routes'
import { i18n } from '@/i18n/i18n'

const mockReprocessActivity = reprocessActivity as ReturnType<typeof vi.fn>
const mockGetRouteDraftFromActivity = getRouteDraftFromActivity as ReturnType<typeof vi.fn>
const navigateMock = vi.hoisted(() => vi.fn())
const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'de',
  units: 'metric',
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => navigateMock,
  }
})

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({
    data: {
      features: {
        routing: { configured: true, available: true },
        offline_map: { configured: true, available: false, reason: 'missing_artifacts' },
      },
    },
  }),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/api/routes', () => ({
  getRouteDraftFromActivity: vi.fn(),
}))

vi.mock('@/api/activities', () => ({
  updateActivity: vi.fn(),
  reprocessActivity: vi.fn(),
  ungroupActivity: vi.fn(),
}))

vi.mock('@/api/fusion', () => ({
  revertFusion: vi.fn(),
}))

vi.mock('@/lib/fusionCacheInvalidation', () => ({
  invalidateAfterFusion: vi.fn(),
}))

vi.mock('@/lib/useActiveJobs', () => ({
  useActiveJobs: () => ({ isJobActiveForEntity: () => false }),
}))

vi.mock('@/hooks/useClickOutside', () => ({
  useClickOutside: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiDownload: vi.fn(),
}))

vi.mock('@/components/SportIcon', () => ({ SportIcon: () => <span /> }))
vi.mock('@/components/SportConflictBadge', () => ({ default: () => null }))
vi.mock('@/components/SportConflictModal', () => ({ default: () => null }))
vi.mock('@/components/activity/ActivityQuickEditModal', () => ({ default: () => null }))
vi.mock('@/components/activity/ReprocessScopeModal', () => ({ default: () => null }))

function activity(): Activity {
  return {
    id: 'act-1',
    user_id: 'user-1',
    title: 'Route source',
    sport_type: 'cycling',
    started_at: '2026-05-12T08:00:00Z',
    elapsed_time_s: 3600,
    has_gps: true,
    is_virtual: false,
    tags: [],
  } as unknown as Activity
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <MemoryRouter>
      <ToastProvider>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </ToastProvider>
    </MemoryRouter>
  )
}

describe('ActivityHeader capability gates', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
    navigateMock.mockClear()
    mockReprocessActivity.mockReset()
    mockGetRouteDraftFromActivity.mockReset()
  })

  it('localizes visible header actions in English', async () => {
    await i18n.changeLanguage('en')

    render(<ActivityHeader activity={activity()} childCount={0} />, { wrapper })

    expect(await screen.findByRole('button', { name: /Open in route planner/i })).toBeInTheDocument()
    expect(screen.getByLabelText('Exclude from Heatmap')).toBeInTheDocument()
    expect(screen.getByLabelText('Exclude from statistics')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Smooth speed/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Metadata' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Edit track/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Export/i })).toBeInTheDocument()
    expect(screen.getByTitle('Recompute activity')).toBeInTheDocument()
    expect(screen.queryByText('Im Routenplaner öffnen')).not.toBeInTheDocument()
    expect(screen.queryByText('Von Statistik ausschließen')).not.toBeInTheDocument()
  })

  it('shows route planner action when routing is available without offline maps', async () => {
    render(<ActivityHeader activity={activity()} childCount={0} />, { wrapper })

    expect(await screen.findByRole('button', { name: /Im Routenplaner öffnen/i })).toBeInTheDocument()
  })

  it('opens activity in route planner as a full route draft', async () => {
    const routeDraft = {
      id: '',
      name: 'Route: Route source',
      sport_type: 'cycling',
      profile: 'trekking',
      waypoints: [
        { id: 'start', latlng: [49, 12], type: 'start' },
        { id: 'end', latlng: [49, 12], type: 'end' },
      ],
      segments: [{
        from_waypoint: 'start',
        to_waypoint: 'end',
        mode: 'routed',
        geometry: { type: 'LineString', coordinates: [[12, 49], [12.01, 49.01], [12, 49]] },
        control_points: [],
        smooth: false,
        distance_m: 1234,
        elevation_gain: 56,
        elevation_loss: 34,
      }],
      time_override: false,
      auto_tag: false,
      photos: [],
      tags: [],
      created_at: '2026-05-17T00:00:00Z',
      updated_at: '2026-05-17T00:00:00Z',
    } satisfies PlannedRoute
    mockGetRouteDraftFromActivity.mockResolvedValue(routeDraft)

    render(<ActivityHeader activity={activity()} childCount={0} />, { wrapper })

    fireEvent.click(await screen.findByRole('button', { name: /Im Routenplaner öffnen/i }))

    await waitFor(() => {
      expect(mockGetRouteDraftFromActivity).toHaveBeenCalledWith('act-1')
    })
    expect(navigateMock).toHaveBeenCalledWith('/map', { state: { plannerRouteDraft: routeDraft } })
  })

  it('maps the all option to core-safe scopes when full-only capabilities are unavailable', async () => {
    mockReprocessActivity.mockResolvedValue({ job_id: 'job-1' })

    render(<ActivityHeader activity={activity()} childCount={0} />, { wrapper })

    fireEvent.click(screen.getByTitle('Aktivität neu verarbeiten'))
    fireEvent.click(await screen.findByText('Alle'))

    await waitFor(() => {
      expect(mockReprocessActivity).toHaveBeenCalledWith(
        'act-1',
        expect.arrayContaining(['streams', 'metrics', 'auto_title', 'ascents', 'intervals', 'lifts']),
      )
    })
    const [, scope] = mockReprocessActivity.mock.calls[0]
    expect(scope).not.toContain('summits')
    expect(scope).not.toContain('stops')
    expect(scope).not.toContain('surfaces')
  })

  it('formats GPS accuracy with user units while keeping VO2 units fixed', async () => {
    await i18n.changeLanguage('en')
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'

    render(
      <ActivityHeader
        activity={{ ...activity(), gps_accuracy_m: 10, vo2max_ml_kg_min: 48.2 } as Activity}
        childCount={0}
      />,
      { wrapper },
    )

    expect(screen.getByText('GPS ±33 ft')).toBeInTheDocument()
    expect(screen.queryByText('GPS ±10m')).not.toBeInTheDocument()
    expect(screen.getByText('ml/kg/min')).toBeInTheDocument()
  })
})
