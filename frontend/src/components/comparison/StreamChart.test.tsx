import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import StreamChart from './StreamChart'

const state = vi.hoisted(() => ({
  option: undefined as any,
}))

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: vi.fn() },
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('echarts/core', () => ({
  use: vi.fn(),
  getInstanceByDom: vi.fn(),
}))

vi.mock('echarts/charts', () => ({ LineChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  LegendComponent: {},
  DataZoomComponent: {},
  BrushComponent: {},
  ToolboxComponent: {},
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))

vi.mock('@/lib/useUnitFormatters', () => ({
  useUnitFormatters: () => ({
    options: { locale: 'de-DE', units: 'imperial' },
  }),
}))

vi.mock('@/hooks/useECharts', () => ({
  useECharts: (getOption: () => any) => {
    state.option = getOption()
    return { current: null }
  },
}))

vi.mock('@/hooks/useBrushZoom', () => ({
  useBrushZoom: () => ({
    bindBrushZoom: vi.fn(),
    resetZoom: vi.fn(),
  }),
}))

vi.mock('@/hooks/useHoverSync', () => ({
  useHoverSync: () => ({
    attachHover: vi.fn(),
    detachHover: vi.fn(),
  }),
}))

function renderChart(unit: string, value: number) {
  render(
    <StreamChart
      unit={unit}
      series={[{ label: 'Source A', color: '#3b82f6', data: [[0, value]] }]}
    />,
  )
}

function tooltipFor(value: number) {
  return state.option.tooltip.formatter([
    { seriesName: 'Source A', value: [0, value], color: '#3b82f6', axisValue: 0 },
  ])
}

describe('StreamChart unit formatting', () => {
  it('formats converted elevation and temperature tooltip values with whole display units', () => {
    renderChart('ft', 328.084)

    expect(tooltipFor(328.084)).toContain('<b>328 ft</b>')
    expect(tooltipFor(328.084)).not.toContain('328.1 ft')

    renderChart('°F', 68)

    expect(tooltipFor(68)).toContain('<b>68 °F</b>')
    expect(tooltipFor(68)).not.toContain('68.0 °F')
  })
})
