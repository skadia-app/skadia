import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PowerDurationChart } from './PowerDurationChart'
import type { BestEffortRanked } from '@/api/bestEfforts'

const state = vi.hoisted(() => ({
  option: undefined as any,
}))

vi.mock('react-i18next', () => ({
  initReactI18next: {
    type: '3rdParty',
    init: vi.fn(),
  },
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: (getOption: (theme: { textColor: string; gridLineColor: string }) => any) => {
    state.option = getOption({ textColor: '#111827', gridLineColor: '#e5e7eb' })
    return { current: null }
  },
}))

const record: BestEffortRanked = {
  id: 'record-1',
  activity_id: 'activity-1',
  category: 'power_20min',
  sport_group: 'cycling',
  achieved_at: '2026-05-01T12:00:00Z',
  rank: 1,
  value_numeric: 1234.6,
}

describe('PowerDurationChart formatting', () => {
  beforeEach(() => {
    state.option = undefined
  })

  it('keeps Power chart values locale-aware and uses W as the fixed axis unit', () => {
    render(
      <PowerDurationChart
        records={[record]}
        series={[{ name: 'Bike', color: '#f97316', sportGroups: ['cycling'] }]}
        language="fr"
      />,
    )

    const expectedWatts = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 0 }).format(1235)

    expect(state.option.yAxis.name).toBe('W')
    expect(state.option.tooltip.valueFormatter(1234.6)).toBe(`${expectedWatts} W`)
  })
})
