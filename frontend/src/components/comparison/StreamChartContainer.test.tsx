import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StreamPoint } from '@/api/types'
import { SourcesProvider } from './SourcesContext'
import StreamChartContainer from './StreamChartContainer'

const chartProps = vi.hoisted(() => ({
  latest: null as null | { unit: string; series: Array<{ data: [number, number][] }> },
}))

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: vi.fn() },
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/lib/useUnitFormatters', () => ({
  useUnitFormatters: () => ({
    options: { locale: 'en-US', units: 'imperial' },
  }),
}))

vi.mock('./StreamChart', () => ({
  default: (props: { unit: string; series: Array<{ data: [number, number][] }> }) => {
    chartProps.latest = props
    return <div data-testid="stream-chart" data-unit={props.unit} />
  },
}))

function streams(): StreamPoint[] {
  return [
    {
      time: '2026-05-31T08:00:00Z',
      speed_ms: 4.4704,
      altitude_m: 100,
      heart_rate: 150,
      power_w: 220,
      cadence: 88,
      temperature_c: 20,
      ground_contact_time_ms: 250,
      vertical_oscillation_mm: 10,
      stance_time_percent: 48,
      step_length_mm: 1000,
      left_right_balance: 50,
    },
  ]
}

async function selectMetric(labelKey: string) {
  await userEvent.click(screen.getByRole('button', { name: `comparisonCharts.metrics.${labelKey}` }))
}

function renderContainer(sportType = 'cycling') {
  render(
    <SourcesProvider
      sources={[
        {
          id: 'source-a',
          label: 'Source A',
          color: '#3b82f6',
          activity: { sport_type: sportType } as never,
          streams: streams(),
        },
      ]}
    >
      <StreamChartContainer />
    </SourcesProvider>,
  )
}

describe('StreamChartContainer unit preferences', () => {
  beforeEach(() => {
    chartProps.latest = null
  })

  it('converts user-facing chart units while leaving technical sensor units unchanged', async () => {
    renderContainer()

    await selectMetric('speed')
    expect(chartProps.latest?.unit).toBe('mph')
    expect(chartProps.latest?.series[0].data[0][1]).toBeCloseTo(10)

    await selectMetric('altitude')
    expect(chartProps.latest?.unit).toBe('ft')
    expect(chartProps.latest?.series[0].data[0][1]).toBeCloseTo(328.084)

    await selectMetric('temperature')
    expect(chartProps.latest?.unit).toBe('°F')
    expect(chartProps.latest?.series[0].data[0][1]).toBe(68)

    await selectMetric('verticalMotion')
    expect(chartProps.latest?.unit).toBe('in')
    expect(chartProps.latest?.series[0].data[0][1]).toBeCloseTo(0.394)

    await selectMetric('stepLength')
    expect(chartProps.latest?.unit).toBe('in')
    expect(chartProps.latest?.series[0].data[0][1]).toBeCloseTo(39.37)

    await selectMetric('heartRate')
    expect(chartProps.latest?.unit).toBe('bpm')

    await selectMetric('power')
    expect(chartProps.latest?.unit).toBe('W')

    await selectMetric('cadence')
    expect(chartProps.latest?.unit).toBe('rpm')

    await selectMetric('groundContact')
    expect(chartProps.latest?.unit).toBe('ms')

    await selectMetric('stanceTime')
    expect(chartProps.latest?.unit).toBe('%')
  })

  it('uses imperial pace units for running speed charts', async () => {
    renderContainer('running')

    await selectMetric('speed')
    expect(chartProps.latest?.unit).toBe('min/mi')
    expect(chartProps.latest?.series[0].data[0][1]).toBeCloseTo(6)
  })
})
