import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Activity, StreamPoint } from '@/api/types'
import ActivityComparisonPage from './ActivityComparisonPage'

const mocks = vi.hoisted(() => ({
  fetchActivity: vi.fn(),
  fetchActivityStreams: vi.fn(),
  echartsSetOption: vi.fn(),
}))

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'en',
  units: 'imperial',
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/api/activities', () => ({
  fetchActivity: mocks.fetchActivity,
  fetchActivityStreams: mocks.fetchActivityStreams,
}))

vi.mock('echarts/core', () => ({
  use: vi.fn(),
  init: vi.fn(() => ({
    setOption: mocks.echartsSetOption,
    resize: vi.fn(),
    dispose: vi.fn(),
  })),
}))

vi.mock('echarts/charts', () => ({ LineChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  LegendComponent: {},
  DataZoomComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('maplibre-gl', () => ({
  default: {
    Map: vi.fn(() => ({
      addControl: vi.fn(),
      on: vi.fn(),
      remove: vi.fn(),
    })),
    NavigationControl: vi.fn(),
    LngLatBounds: vi.fn(() => ({
      extend: vi.fn(),
    })),
  },
}))

function activity(id: string, overrides: Partial<Activity> = {}): Activity {
  return {
    id,
    user_id: 'user-1',
    title: null,
    sport_type: 'cycling',
    sub_type: null,
    is_electric: false,
    is_virtual: false,
    is_commute: false,
    exclude_from_heatmap: false,
    exclude_from_stats: false,
    started_at: '2026-05-31T08:00:00Z',
    created_at: '2026-05-31T08:00:00Z',
    updated_at: '2026-05-31T08:00:00Z',
    elapsed_time_s: 3600,
    distance_m: 1609.344,
    elevation_gain_m: 304.8,
    elevation_loss_m: 0,
    avg_speed_ms: 4.4704,
    avg_heart_rate: 150,
    max_heart_rate: 170,
    avg_power_w: 210,
    normalized_power_w: 220,
    avg_cadence: 84,
    tss: 42,
    is_indoor: false,
    tags: [],
    visibility: 'private',
    enrichment_status: 'done',
    needs_classification: false,
    has_gps: true,
    ...overrides,
  }
}

function streams(): StreamPoint[] {
  return [
    { time: '2026-05-31T08:00:00Z', distance_m: 0, speed_ms: 4.4704, altitude_m: 300 },
    { time: '2026-05-31T08:10:00Z', distance_m: 1609.344, speed_ms: 4.4704, altitude_m: 304.8 },
  ]
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/compare?ids=a,b']}>
        <Routes>
          <Route path="/compare" element={<ActivityComparisonPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ActivityComparisonPage units', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    mocks.fetchActivity.mockImplementation((id: string) => Promise.resolve(activity(id)))
    mocks.fetchActivityStreams.mockResolvedValue(streams())
  })

  it('formats metric rows and chart axes with the active unit preference', async () => {
    renderPage()

    expect(await screen.findAllByText('1.0 mi')).toHaveLength(2)
    expect(screen.getAllByText('1,000 ft')).toHaveLength(2)
    expect(screen.getAllByText('10.0 mph')).toHaveLength(2)
    expect(screen.queryByText('1.6 km')).not.toBeInTheDocument()
    expect(screen.queryByText('305 m')).not.toBeInTheDocument()
    expect(screen.queryByText('16.1 km/h')).not.toBeInTheDocument()

    await waitFor(() => {
      expect(mocks.echartsSetOption).toHaveBeenCalled()
    })

    const option = mocks.echartsSetOption.mock.calls.at(-1)?.[0]
    expect(option.xAxis.name).toBe('mi')
    expect(option.yAxis.name).toBe('mph')
    expect(option.series[0].data.at(-1)[0]).toBeCloseTo(1)
    expect(option.series[0].data.at(-1)[1]).toBeCloseTo(10)

    fireEvent.click(screen.getByRole('button', { name: /Höhe|Elevation/ }))

    await waitFor(() => {
      const altitudeOption = mocks.echartsSetOption.mock.calls.at(-1)?.[0]
      expect(altitudeOption.yAxis.name).toBe('ft')
      expect(altitudeOption.series[0].data.at(-1)[1]).toBeCloseTo(1000)
    })
  })
})
