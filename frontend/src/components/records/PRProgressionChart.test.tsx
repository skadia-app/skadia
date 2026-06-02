import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PRProgressionChart } from './PRProgressionChart'
import type { PRHistoryEntry } from '@/api/bestEfforts'

const state = vi.hoisted(() => ({
  option: undefined as any,
  queryData: [] as PRHistoryEntry[],
}))

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('@tanstack/react-query', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({
    data: state.queryData,
    isLoading: false,
    isError: false,
  }),
}))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: (getOption: (theme: { textColor: string; gridLineColor: string }) => any) => {
    state.option = getOption({ textColor: '#111827', gridLineColor: '#e5e7eb' })
    return { current: null }
  },
}))

describe('PRProgressionChart formatting', () => {
  beforeEach(() => {
    state.option = undefined
    state.queryData = [{
      activity_id: 'activity-1',
      achieved_at: '2026-05-01T12:00:00Z',
      value_numeric: 1234.6,
      rank: 1,
      activity_name: 'Test activity',
    }]
  })

  it('formats tooltip dates, tick dates, and power values with the active locale', () => {
    render(
      <PRProgressionChart
        category="power_5min"
        sportGroup="cycling"
        isExpanded
        language="fr"
      />,
    )

    const timestamp = new Date('2026-05-01T12:00:00Z').getTime()
    const entry = state.queryData[0]
    const tooltip = state.option.tooltip.formatter({ data: [timestamp, 1234.6, entry] })
    const tick = state.option.xAxis.axisLabel.formatter(timestamp)
    const expectedTooltipDate = new Date(timestamp).toLocaleDateString('fr-FR', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
    })
    const expectedTickDate = new Date(timestamp).toLocaleDateString('fr-FR', {
      month: 'short',
      year: '2-digit',
    })
    const expectedWatts = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 0 }).format(1235)

    expect(tooltip).toContain(expectedTooltipDate)
    expect(tooltip).toContain(`<b>${expectedWatts} W</b>`)
    expect(tick).toBe(expectedTickDate)
  })
})
