import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/lib/toast'
import type { FusionQueueEntry } from '@/api/importWizard'
import type { RecordingGroup, RecordingGroupMember } from '@/api/types'
import FusionPage from './FusionPage'
import FusionQueuePage from './FusionQueuePage'

const mocks = vi.hoisted(() => ({
  fetchRecordingGroups: vi.fn(),
  fetchRecordingGroup: vi.fn(),
  fetchRecordingGroupMembers: vi.fn(),
  setGroupPrimary: vi.fn(),
  getFusionQueue: vi.fn(),
  dismissFusionCandidate: vi.fn(),
  fetchActivity: vi.fn(),
  fetchActivityStreams: vi.fn(),
  executeFusionMerge: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

vi.mock('@/api/recordingGroups', () => ({
  fetchRecordingGroups: mocks.fetchRecordingGroups,
  fetchRecordingGroup: mocks.fetchRecordingGroup,
  fetchRecordingGroupMembers: mocks.fetchRecordingGroupMembers,
  setGroupPrimary: mocks.setGroupPrimary,
}))

vi.mock('@/api/importWizard', () => ({
  getFusionQueue: mocks.getFusionQueue,
  dismissFusionCandidate: mocks.dismissFusionCandidate,
}))

vi.mock('@/api/activities', () => ({
  fetchActivity: mocks.fetchActivity,
  fetchActivityStreams: mocks.fetchActivityStreams,
}))

vi.mock('@/api/fusion', () => ({
  executeFusionMerge: mocks.executeFusionMerge,
}))

vi.mock('@/components/DuplicateReviewPanel', () => ({
  default: () => <div data-testid="duplicate-review-panel" />,
}))

vi.mock('@/components/fusion/FusionGroupContextDialog', () => ({
  default: () => <div data-testid="fusion-group-context-dialog" />,
}))

vi.mock('@/components/comparison/MultiTrackMap', () => ({
  default: () => <div data-testid="multi-track-map" />,
}))

vi.mock('@/components/comparison/StreamChartContainer', () => ({
  default: () => <div data-testid="stream-chart-container" />,
}))

vi.mock('@/components/fusion/FusionFieldTable', () => ({
  FUSION_FIELDS: {
    GPS: 'gps',
    HR: 'heart_rate',
    POWER: 'power',
    ALTITUDE: 'altitude',
    CADENCE: 'cadence',
    TEMPERATURE: 'temperature',
  },
  default: () => <div data-testid="fusion-field-table" />,
}))

function wrapper(initialEntry = '/workshop/fusion') {
  return function TestWrapper({ children }: { children: ReactNode }) {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    return (
      <MemoryRouter initialEntries={[initialEntry]}>
        <ToastProvider>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ToastProvider>
      </MemoryRouter>
    )
  }
}

const queueEntry: FusionQueueEntry = {
  activity_id_a: 'activity-a',
  activity_id_b: 'activity-b',
  title_a: 'Morning Ride',
  title_b: 'Device Track',
  started_at_a: '2026-05-31T08:00:00Z',
  started_at_b: '2026-05-31T08:01:00Z',
  sport_type_a: 'cycling',
  sport_type_b: 'cycling',
  sub_type_a: 'road',
  sub_type_b: 'road',
  is_electric_a: false,
  is_electric_b: false,
  is_indoor_a: false,
  is_indoor_b: false,
  is_virtual_a: false,
  is_virtual_b: false,
  distance_m_a: 1609.344,
  distance_m_b: 804.672,
  overlap_percent: 98.4,
  field_richness_a: 3,
  field_richness_b: 2,
  time_diff_sec: 60,
  source_device_a: 'Device A',
  source_a: 'fit',
  original_file_format_a: 'fit',
  source_device_b: 'Device B',
  source_b: 'fit',
  original_file_format_b: 'fit',
  group_id_a: null,
  group_id_b: null,
}

const groupMembers: RecordingGroupMember[] = [
  {
    activity_id: 'activity-a',
    is_fused: false,
    fusion_source_count: 0,
    is_primary: true,
    title: 'Morning Ride',
    display_title: 'Morning Ride',
    title_auto: null,
    started_at: '2026-05-31T08:00:00Z',
    sport_type: 'cycling',
    sub_type: 'road',
    is_electric: false,
    is_indoor: false,
    is_virtual: false,
    distance_m: 1609.344,
    elapsed_time_s: 3600,
    field_richness: 3,
    source_device: 'Device A',
    source: 'fit',
    recording_device_name: null,
    original_file_format: 'fit',
    has_gps: true,
    elevation_gain_m: 100,
    elevation_loss_m: 100,
  },
  {
    activity_id: 'activity-b',
    is_fused: false,
    fusion_source_count: 0,
    is_primary: false,
    title: 'Device Track',
    display_title: 'Device Track',
    title_auto: null,
    started_at: '2026-05-31T08:01:00Z',
    sport_type: 'cycling',
    sub_type: 'road',
    is_electric: false,
    is_indoor: false,
    is_virtual: false,
    distance_m: 3218.688,
    elapsed_time_s: 3700,
    field_richness: 2,
    source_device: 'Device B',
    source: 'fit',
    recording_device_name: null,
    original_file_format: 'fit',
    has_gps: true,
    elevation_gain_m: 90,
    elevation_loss_m: 90,
  },
]

const recordingGroup: RecordingGroup = {
  id: 'group-1',
  user_id: 'user-1',
  primary_activity_id: 'activity-a',
  sport_type: 'cycling',
  sub_type: 'road',
  is_electric: false,
  is_indoor: false,
  is_virtual: false,
  created_at: '2026-05-31T08:00:00Z',
  confirmed: false,
  members: groupMembers,
}

describe('Fusion unit formatting', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.fetchRecordingGroups.mockResolvedValue([])
    mocks.fetchRecordingGroup.mockResolvedValue(recordingGroup)
    mocks.fetchRecordingGroupMembers.mockResolvedValue(groupMembers)
    mocks.getFusionQueue.mockResolvedValue({ items: [queueEntry], computing: false })
    mocks.fetchActivity.mockResolvedValue(null)
    mocks.fetchActivityStreams.mockResolvedValue([])
  })

  it('formats fusion queue candidate distances with the active unit preference', async () => {
    render(<FusionQueuePage />, { wrapper: wrapper() })

    expect(await screen.findByText((content) => content.includes('1.0 mi'))).toBeInTheDocument()
    expect(screen.getByText((content) => content.includes('2,640 ft'))).toBeInTheDocument()
    expect(screen.queryByText((content) => content.includes('1.6 km'))).not.toBeInTheDocument()
    expect(screen.queryByText((content) => content.includes('805 m'))).not.toBeInTheDocument()
  })

  it('formats fusion source dropdown distances with the active unit preference', async () => {
    render(<FusionPage />, { wrapper: wrapper('/fusion?group=group-1') })

    expect(await screen.findByText((content, element) => (
      element?.tagName === 'OPTION' && content.includes('1.0 mi')
    ))).toBeInTheDocument()
    expect(screen.queryByText((content, element) => (
      element?.tagName === 'OPTION' && content.includes('1.6 km')
    ))).not.toBeInTheDocument()
  })
})
