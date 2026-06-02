import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import '@testing-library/jest-dom'
import '@/i18n/i18n'
import { i18n } from '@/i18n/i18n'

const mockUserPrefs = vi.hoisted(() => ({ locale: 'de', units: 'metric' }))
const mockDiveInfo = vi.hoisted(() => ({ value: null as DiveInfo | null }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/api/activities', () => ({
  fetchDiveInfo: vi.fn(() => Promise.resolve(mockDiveInfo.value)),
}))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: () => vi.fn(),
}))

import { DiveDecoPanel } from '../DiveDecoPanel'
import { DiveStats } from '../DiveStats'
import { GlucoseChartPanel } from '../GlucoseChartPanel'
import { JumpSection } from '../JumpSection'
import { RadarDensityChart } from '../RadarDensityChart'
import { RadarSummary } from '../RadarSummary'
import { WorkoutOverview } from '../WorkoutOverview'
import type { Activity, ActivityEvent, ActivityJump, DiveInfo, StreamPoint, Workout } from '@/api/types'

function renderWithQueryClient(children: ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>)
}

describe('special activity panels i18n and units', () => {
  beforeEach(async () => {
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
    mockDiveInfo.value = null
    await i18n.changeLanguage('de')
  })

  it('localizes the radar summary sentence', async () => {
    await i18n.changeLanguage('en')
    mockUserPrefs.locale = 'en'

    const events = [
      { radar_threat_count: 1, radar_threat_level_max: 1 },
      { radar_threat_count: 2, radar_threat_level_max: 3 },
    ] as ActivityEvent[]

    render(<RadarSummary events={events} />)

    expect(screen.getByText(/Radar: 3 vehicles detected/)).toBeInTheDocument()
    expect(screen.getByText(/\(max\. threat level: 3\)/)).toBeInTheDocument()
  })

  it('localizes the radar density panel controls', () => {
    const streams = [
      { time: '2026-01-01T00:00:00Z', distance_m: 0, radar_vehicle_count: 1, radar_nearest_range_m: 8 },
      { time: '2026-01-01T00:01:00Z', distance_m: 100, radar_vehicle_count: 2, radar_nearest_range_m: 20 },
    ] as StreamPoint[]

    render(<RadarDensityChart streams={streams} />)

    expect(screen.getByText('Radar - Fahrzeugdichte')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Distanz' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Zeit' })).toBeInTheDocument()
  })

  it('localizes glucose panel text', () => {
    const streams = [
      { time: '2026-01-01T00:00:00Z', glucose_mg_dl: 120 },
      { time: '2026-01-01T00:01:00Z', glucose_mg_dl: 180 },
    ] as StreamPoint[]

    render(<GlucoseChartPanel streams={streams} />)

    expect(screen.getByText('Blutzucker')).toBeInTheDocument()
    expect(screen.getByText('Erhöht (160-200)')).toBeInTheDocument()
    expect(screen.getByText(/Bewegungsbezogene Schwellen/)).toBeInTheDocument()
  })

  it('formats dive depths with user units and localizes dive labels', async () => {
    await i18n.changeLanguage('en')
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'

    const activity = {
      max_depth_m: 10,
      avg_depth_m: 5,
      start_cns_pct: 1.2,
      end_cns_pct: 3.4,
    } as Activity
    const diveInfo = {
      po2_stats: {
        min_po2: 0.8,
        min_po2_depth_m: 10,
        max_po2: 1.3,
        max_po2_depth_m: 20,
      },
    } as DiveInfo

    render(<DiveStats activity={activity} diveInfo={diveInfo} />)

    expect(screen.getByText('Max depth')).toBeInTheDocument()
    expect(screen.getByText('33 ft')).toBeInTheDocument()
    expect(screen.getByText('CNS end')).toBeInTheDocument()
    expect(screen.getByText('1.30 bar @ 66 ft')).toBeInTheDocument()
  })

  it('formats dive deco density and setpoint depths with selected user units', async () => {
    await i18n.changeLanguage('en')
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    mockDiveInfo.value = {
      gases: [],
      settings: {
        activity_id: 'activity-1',
        gf_low: 30,
        gf_high: 85,
        water_type: 'fresh',
        water_density: 1.025,
        deco_model: 'ZH-L16C',
        po2_warn: 1.4,
        po2_critical: 1.6,
        po2_deco: 1.6,
        safety_stop_enabled: true,
        safety_stop_time_s: 180,
        repeat_dive_interval_s: 0,
        ccr_low_setpoint: 0.7,
        ccr_low_setpoint_depth_m: 10,
        ccr_high_setpoint: 1.3,
        ccr_high_setpoint_depth_m: 20,
      },
    }

    renderWithQueryClient(<DiveDecoPanel activityId="activity-1" streams={[]} />)

    expect(await screen.findByText('Equipment & settings')).toBeInTheDocument()
    expect(screen.getByText('64.0 lb/ft³')).toBeInTheDocument()
    expect(screen.getByText(/Low: 0\.70 bar @ 33 ft/)).toBeInTheDocument()
    expect(screen.getByText(/High: 1\.30 bar @ 66 ft/)).toBeInTheDocument()
    expect(screen.queryByText('1.025 kg/l')).not.toBeInTheDocument()
  })

  it('formats jump height and speed with user units', async () => {
    await i18n.changeLanguage('en')
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'

    const jumps = [{
      timestamp: 'jump-1',
      height_m: 2,
      hang_time_s: 0.8,
      speed_ms: 10,
      score: 91,
    }] as ActivityJump[]

    render(<JumpSection jumpCount={1} jumps={jumps} />)
    fireEvent.click(screen.getByRole('button', { name: /Jumps: 1/ }))

    expect(screen.getByText('Hang time')).toBeInTheDocument()
    expect(screen.getByText('6.6 ft')).toBeInTheDocument()
    expect(screen.getByText('22.4 mph')).toBeInTheDocument()
  })

  it('formats workout distance steps with user units', async () => {
    await i18n.changeLanguage('en')
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'

    const workout = {
      id: 'workout-1',
      name: 'Tempo',
      steps: [{
        message_index: 0,
        intensity: 0,
        duration_type: 1,
        duration_value: 1609.344,
      }],
    } as Workout

    render(<WorkoutOverview workout={workout} />)

    expect(screen.getByText('Workout: Tempo')).toBeInTheDocument()
    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
  })
})
