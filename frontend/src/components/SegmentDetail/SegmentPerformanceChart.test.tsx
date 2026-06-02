import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { SegmentPerformanceChart } from './SegmentPerformanceChart'

const state = vi.hoisted(() => ({
  option: undefined as any,
  performances: [] as any[],
}))

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('echarts/core', () => ({
  default: undefined,
  use: vi.fn(),
}))
vi.mock('echarts/charts', () => ({ LineChart: {}, ScatterChart: {} }))
vi.mock('echarts/components', () => ({ GridComponent: {}, TooltipComponent: {} }))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('@tanstack/react-query', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({
    data: { performances: state.performances },
  }),
}))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: (factory: () => any) => {
    state.option = factory()
    return { current: null }
  },
}))

describe('SegmentPerformanceChart formatting', () => {
  beforeEach(() => {
    state.option = undefined
    state.performances = [
      {
        activity_id: 'activity-1',
        started_at: '2026-05-31T12:00:00Z',
        elapsed_time_s: 180,
        sport_type: 'running',
        title: 'Morning run',
      },
      {
        activity_id: 'activity-2',
        started_at: '2026-06-01T12:00:00Z',
        elapsed_time_s: 170,
        sport_type: 'running',
        title: 'Lunch run',
      },
      {
        activity_id: 'activity-3',
        started_at: '2026-06-02T12:00:00Z',
        elapsed_time_s: 160,
        sport_type: 'running',
        title: 'Evening run',
      },
    ]
  })

  it('formats tooltip and axis dates with the active i18n locale', async () => {
    await i18n.changeLanguage('en')

    render(<SegmentPerformanceChart segmentId="segment-1" />)

    const timestamp = new Date('2026-05-31T12:00:00Z').getTime()
    const tooltip = state.option.tooltip.formatter({
      seriesIndex: 0,
      data: { value: [timestamp, 180] },
    })
    const tick = state.option.xAxis.axisLabel.formatter(timestamp)

    expect(tooltip).toContain('05/31/2026')
    expect(tick).toBe('May 26')
  })
})
