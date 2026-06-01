import '@testing-library/jest-dom/vitest'
import { render } from '@testing-library/react'
import { useRef } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { EChartsOption } from 'echarts'
import type { HealthWeightEntry } from '@/api/health'
import { i18n } from '@/i18n/i18n'
import WeightChart from './WeightChart'

vi.mock('echarts/core', () => ({
  default: undefined,
  use: vi.fn(),
}))
vi.mock('echarts/charts', () => ({ LineChart: {} }))
vi.mock('echarts/components', () => ({ GridComponent: {}, TooltipComponent: {} }))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

const captured = vi.hoisted(() => ({ option: null as EChartsOption | null }))
const userPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' as 'metric' | 'imperial' }))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: (factory: (theme: { textColor: string; gridLineColor: string }) => EChartsOption) => {
    captured.option = factory({ textColor: '#111827', gridLineColor: '#e5e7eb' })
    return useRef<HTMLDivElement>(null)
  },
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

function entry(weightG: number): HealthWeightEntry {
  return {
    user_id: 'user-1',
    measured_at_utc: '2026-05-31T00:00:00Z',
    weight_g: weightG,
    bmi: 22.5,
    body_fat_pct: null,
    muscle_mass_g: null,
    bone_mass_g: null,
    source_type: 'manual',
    created_at: '2026-05-31T00:00:00Z',
  }
}

describe('Health dashboard WeightChart', () => {
  it('formats axis and tooltip weight values with the active unit preference', async () => {
    await i18n.changeLanguage('en')

    render(<WeightChart data={[entry(70_000)]} />)

    const option = captured.option as EChartsOption
    const yAxis = option.yAxis as { axisLabel: { formatter: (value: number) => string } }
    expect(yAxis.axisLabel.formatter(70)).toBe('154.3 lb')

    const tooltip = option.tooltip as { formatter: (params: unknown) => string }
    const html = tooltip.formatter([{ value: 70, axisValueLabel: '2026-05-31', color: '#22c55e', dataIndex: 0 }])
    expect(html).toContain('154.3 lb')
    expect(html).not.toContain('70 kg')
  })
})
