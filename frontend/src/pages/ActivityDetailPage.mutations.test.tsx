import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Activity } from '@/api/types'
import type { Capabilities } from '@/api/routes'
import { ToastProvider } from '@/lib/toast'

const activityDetailMocks = vi.hoisted(() => ({
  toggleResolved: vi.fn(),
  toggleRejected: vi.fn(),
  capabilities: {
    features: {
      geo_poi_enrichment: { configured: true, available: true },
      surface_enrichment: { configured: true, available: true },
      osm_lift_snap: { configured: true, available: true },
    },
  } as Capabilities,
}))

vi.mock('react-router-dom', () => ({
  useParams: () => ({ id: 'act-1' }),
  Link: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(),
}))

vi.mock('@/api/activities', () => ({
  fetchActivity: vi.fn(),
  fetchActivityStreams: vi.fn(),
  fetchActivitySensors: vi.fn(),
  fetchActivityLaps: vi.fn(),
  fetchActivitySplits: vi.fn(),
  fetchActivityLengths: vi.fn(),
  fetchActivityClimbPro: vi.fn(),
  getZoneDistribution: vi.fn(),
  fetchActivitySets: vi.fn(),
  fetchActivityJumps: vi.fn(),
  fetchActivityEvents: vi.fn(),
  fetchDiveInfo: vi.fn(),
}))

vi.mock('@/api/workouts', () => ({ fetchWorkout: vi.fn() }))
vi.mock('@/api/recordingGroups', () => ({ fetchRecordingGroup: vi.fn() }))
vi.mock('@/api/settings', () => ({ getWeatherSettings: vi.fn() }))
vi.mock('@/api/tags', () => ({ fetchTags: vi.fn() }))
vi.mock('@/api/activityPhotos', () => ({ uploadActivityPhoto: vi.fn() }))
vi.mock('@/api/enrichment', () => ({ fetchGeoTags: vi.fn(), fetchStops: vi.fn() }))
vi.mock('@/api/bestEfforts', () => ({ fetchActivityBestEfforts: vi.fn() }))
vi.mock('@/api/trackSummary', () => ({ useActivityTrackSummary: vi.fn() }))
vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({
    data: activityDetailMocks.capabilities,
  }),
}))

vi.mock('@/hooks/useStickyHighlight', () => ({
  useStickyHighlight: () => ({
    sticky: null,
    effective: null,
    setHover: vi.fn(),
    handleClick: vi.fn(),
  }),
}))

vi.mock('@/components/LiftSection', async () => {
  const React = await vi.importActual<typeof import('react')>('react')
  return {
    default: ({ onToggleExclude }: { onToggleExclude: (v: boolean) => Promise<void> }) =>
      React.createElement(
        'button',
        {
          type: 'button',
          onClick: () => {
            void onToggleExclude(true).then(
              activityDetailMocks.toggleResolved,
              activityDetailMocks.toggleRejected,
            )
          },
        },
        'toggle lift exclusion',
      ),
  }
})

vi.mock('@/components/lift/LiftReviewModal', () => ({ default: () => null }))
vi.mock('@/components/ActivityLocationTagSection', () => ({ default: () => null }))
vi.mock('@/components/ActivityCharts', () => ({ default: () => null }))
vi.mock('@/components/MapFullscreen', () => ({ default: () => null }))
vi.mock('@/components/WeatherBadge', () => ({ default: () => null }))
vi.mock('@/components/GeoTagsList', () => ({ default: () => null }))
vi.mock('@/components/GearSelector', () => ({ default: () => null }))
vi.mock('@/components/ActivityPhotoSection', () => ({ default: () => null }))
vi.mock('@/components/LapTable', () => ({ default: () => null }))
vi.mock('@/components/SplitTable', () => ({ default: () => null }))
vi.mock('@/components/SwimLengthTable', () => ({ default: () => null }))
vi.mock('@/components/ClimbProChart', () => ({ default: () => null }))
vi.mock('@/components/ClimbsSection', () => ({ default: () => null }))
vi.mock('@/components/IntervalSection', () => ({ default: () => null }))
vi.mock('@/components/StopsSection', () => ({ default: () => null }))
vi.mock('@/components/SurfacesSection', () => ({ default: () => null }))
vi.mock('@/components/EnrichmentStatusSection', () => ({ default: () => null }))
vi.mock('@/components/ActivitySegments', () => ({ default: () => null }))
vi.mock('@/components/DetectedLapsTable', () => ({ default: () => null }))
vi.mock('@/components/analytics/RouteSimilarityCard', () => ({ default: () => null }))
vi.mock('@/components/RunningFormCard', () => ({ RunningFormCard: () => null }))
vi.mock('@/components/activity/MultisportSection', () => ({ default: () => null }))
vi.mock('@/components/activity/MultisportMap', () => ({ MultisportMap: () => null }))
vi.mock('@/components/activity/MultisportEnrichments', () => ({ default: () => null }))
vi.mock('@/components/activity/PiPMap', () => ({ PiPMap: () => null }))
vi.mock('@/components/activity/ChartTabs', () => ({ ChartTabs: ({ children }: { children: (tab: string) => ReactNode }) => children('overview') }))
vi.mock('@/components/activity/StatsGrid', () => ({ StatsGrid: () => null }))
vi.mock('@/components/activity/RunningDynamicsStats', () => ({ RunningDynamicsStats: () => null }))
vi.mock('@/components/activity/DiveStats', () => ({ DiveStats: () => null }))
vi.mock('@/components/activity/DiveGasTable', () => ({ DiveGasTable: () => null }))
vi.mock('@/components/activity/DataCorrections', () => ({ DataCorrectionsSection: () => null }))
vi.mock('@/components/activity/VisibilitySettings', () => ({ VisibilitySettings: () => null }))
vi.mock('@/components/activity/SunInfo', () => ({ SunInfo: () => null }))
vi.mock('@/components/activity/LocationLine', () => ({ LocationLine: () => null }))
vi.mock('@/components/activity/StrengthSetsTable', () => ({ StrengthSetsTable: () => null }))
vi.mock('@/components/activity/WorkoutOverview', () => ({ WorkoutOverview: () => null }))
vi.mock('@/components/activity/BallSportStats', () => ({ BallSportStats: () => null }))
vi.mock('@/components/activity/JumpSection', () => ({ JumpSection: () => null }))
vi.mock('@/components/activity/RadarSummary', () => ({ RadarSummary: () => null }))
vi.mock('@/components/activity/RadarDensityChart', () => ({ RadarDensityChart: () => null }))
vi.mock('@/components/activity/SensorList', () => ({ SensorList: () => null }))
vi.mock('@/components/activity/ActivityHeader', () => ({ default: () => null }))
vi.mock('@/components/activity/TrackMap', () => ({ default: () => null }))
vi.mock('@/components/activity/TagsBadges', () => ({ default: () => null }))
vi.mock('@/components/activity/ZoneDistributionChart', () => ({ default: () => null }))
vi.mock('@/components/activity/HealthContextCards', () => ({ HealthContextCards: () => null }))
vi.mock('@/components/track-summary/StatsRow', () => ({ default: () => null }))
vi.mock('@/components/track-summary/TrackSummary', async () => {
  const React = await vi.importActual<typeof import('react')>('react')
  return {
    default: ({ summary, children }: { summary?: unknown; children?: ReactNode }) =>
      React.createElement('div', null, summary === undefined ? 'track summary loading' : children),
    buildStatsRowProps: vi.fn(() => ({})),
  }
})

import { fetchActivity, fetchActivityStreams, fetchActivitySensors, fetchActivityLaps, fetchActivitySplits, fetchActivityLengths, fetchActivityClimbPro, getZoneDistribution, fetchActivitySets, fetchActivityJumps, fetchActivityEvents, fetchDiveInfo } from '@/api/activities'
import { fetchWorkout } from '@/api/workouts'
import { fetchRecordingGroup } from '@/api/recordingGroups'
import { getWeatherSettings } from '@/api/settings'
import { fetchTags } from '@/api/tags'
import { fetchGeoTags, fetchStops } from '@/api/enrichment'
import { fetchActivityBestEfforts } from '@/api/bestEfforts'
import { useActivityTrackSummary } from '@/api/trackSummary'
import { apiFetch } from '@/lib/auth'
import ActivityDetailPage from './ActivityDetailPage'

const mockFetchActivity = fetchActivity as ReturnType<typeof vi.fn>
const mockFetchActivityStreams = fetchActivityStreams as ReturnType<typeof vi.fn>
const mockFetchActivitySensors = fetchActivitySensors as ReturnType<typeof vi.fn>
const mockFetchActivityLaps = fetchActivityLaps as ReturnType<typeof vi.fn>
const mockFetchActivitySplits = fetchActivitySplits as ReturnType<typeof vi.fn>
const mockFetchActivityLengths = fetchActivityLengths as ReturnType<typeof vi.fn>
const mockFetchActivityClimbPro = fetchActivityClimbPro as ReturnType<typeof vi.fn>
const mockGetZoneDistribution = getZoneDistribution as ReturnType<typeof vi.fn>
const mockFetchActivitySets = fetchActivitySets as ReturnType<typeof vi.fn>
const mockFetchActivityJumps = fetchActivityJumps as ReturnType<typeof vi.fn>
const mockFetchActivityEvents = fetchActivityEvents as ReturnType<typeof vi.fn>
const mockFetchDiveInfo = fetchDiveInfo as ReturnType<typeof vi.fn>
const mockFetchWorkout = fetchWorkout as ReturnType<typeof vi.fn>
const mockFetchRecordingGroup = fetchRecordingGroup as ReturnType<typeof vi.fn>
const mockGetWeatherSettings = getWeatherSettings as ReturnType<typeof vi.fn>
const mockFetchTags = fetchTags as ReturnType<typeof vi.fn>
const mockFetchGeoTags = fetchGeoTags as ReturnType<typeof vi.fn>
const mockFetchStops = fetchStops as ReturnType<typeof vi.fn>
const mockFetchActivityBestEfforts = fetchActivityBestEfforts as ReturnType<typeof vi.fn>
const mockUseActivityTrackSummary = useActivityTrackSummary as ReturnType<typeof vi.fn>
const mockApiFetch = apiFetch as ReturnType<typeof vi.fn>

function makeActivity(): Activity {
  return {
    id: 'act-1',
    user_id: 'user-1',
    title: 'Lift Test',
    sport_type: 'cycling',
    sub_type: null,
    started_at: '2026-05-12T08:00:00Z',
    elapsed_time_s: 3600,
    distance_m: null,
    elevation_gain_m: null,
    elevation_loss_m: null,
    has_gps: false,
    is_electric: false,
    is_commute: false,
    is_indoor: false,
    is_virtual: false,
    exclude_from_heatmap: false,
    exclude_from_stats: false,
    tags: [],
    visibility: 'private',
    enrichment_status: 'pending',
    needs_classification: false,
    created_at: '2026-05-12T08:00:00Z',
    updated_at: '2026-05-12T08:00:00Z',
    lift_count: 1,
    lift_distance_m: 1000,
    lift_ascent_m: 0,
    lift_descent_m: 200,
    lift_duration_s: 300,
    exclude_lift_subtraction: false,
  }
}

function wrapper(queryClient: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <ToastProvider>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </ToastProvider>
  )
}

describe('ActivityDetailPage mutation errors', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    activityDetailMocks.capabilities = {
      features: {
        geo_poi_enrichment: { configured: true, available: true },
        surface_enrichment: { configured: true, available: true },
        osm_lift_snap: { configured: true, available: true },
      },
    }
    global.IntersectionObserver = class {
      observe() {}
      disconnect() {}
      unobserve() {}
    } as unknown as typeof IntersectionObserver

    mockFetchActivity.mockResolvedValue(makeActivity())
    mockFetchActivityStreams.mockResolvedValue([])
    mockFetchActivitySensors.mockResolvedValue([])
    mockFetchActivityLaps.mockResolvedValue([])
    mockFetchActivitySplits.mockResolvedValue({ splits: [], summaries: [] })
    mockFetchActivityLengths.mockResolvedValue([])
    mockFetchActivityClimbPro.mockResolvedValue([])
    mockGetZoneDistribution.mockResolvedValue({ hr_zones: [], power_zones: [] })
    mockFetchActivitySets.mockResolvedValue([])
    mockFetchActivityJumps.mockResolvedValue([])
    mockFetchActivityEvents.mockResolvedValue([])
    mockFetchDiveInfo.mockResolvedValue(null)
    mockFetchWorkout.mockResolvedValue(null)
    mockFetchRecordingGroup.mockResolvedValue(null)
    mockGetWeatherSettings.mockResolvedValue({ display_fields: [] })
    mockFetchTags.mockResolvedValue([])
    mockFetchGeoTags.mockResolvedValue([])
    mockFetchStops.mockResolvedValue([])
    mockFetchActivityBestEfforts.mockResolvedValue({ best_efforts: [], near_misses: [] })
    mockUseActivityTrackSummary.mockReturnValue({ data: null, isError: false, isLoading: false })
    mockApiFetch.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url === '/api/v1/activities/act-1' && init?.method === 'PATCH') {
        return new Response('patch failed', {
          status: 500,
          headers: { 'Content-Type': 'text/plain' },
        })
      }
      if (url.includes('/lifts/candidates')) {
        return new Response('[]', {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response('{}', {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })
  })

  it('does not invalidate the activity after exclude-lift PATCH fails', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    render(<ActivityDetailPage />, { wrapper: wrapper(queryClient) })

    fireEvent.click(await screen.findByRole('button', { name: 'toggle lift exclusion' }))

    await waitFor(() => {
      expect(activityDetailMocks.toggleRejected).toHaveBeenCalled()
    })
    expect(activityDetailMocks.toggleResolved).not.toHaveBeenCalled()
    expect(invalidateSpy.mock.calls).not.toContainEqual([{ queryKey: ['activity', 'act-1'] }])
  })

  it('does not leave TrackSummary in loading state when geodata capabilities are unavailable', async () => {
    activityDetailMocks.capabilities = {
      features: {
        geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
        surface_enrichment: { configured: false, available: false, reason: 'core_profile' },
        osm_lift_snap: { configured: false, available: false, reason: 'core_profile' },
      },
    }
    mockUseActivityTrackSummary.mockReturnValue({ data: undefined, isError: false, isLoading: false })
    mockFetchActivity.mockResolvedValue({ ...makeActivity(), has_gps: true, lift_count: 0 })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })

    render(<ActivityDetailPage />, { wrapper: wrapper(queryClient) })

    await waitFor(() => expect(mockUseActivityTrackSummary).toHaveBeenCalled())
    expect(screen.queryByText('track summary loading')).toBeNull()
  })
})
