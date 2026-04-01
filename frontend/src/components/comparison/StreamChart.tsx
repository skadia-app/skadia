// StreamChart — slim ECharts wrapper for one metric, N sources.
// Handles brush zoom, hover sync, resize, and zoom-reset button.

import { useState, useCallback } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  BrushComponent,
  ToolboxComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useECharts } from '@/hooks/useECharts'
import { useBrushZoom } from '@/hooks/useBrushZoom'
import { useHoverSync, type SourceGpsData } from '@/hooks/useHoverSync'
import { fmtDuration } from '@/lib/format'
import { getSourceLineStyle, getSourceChartLineWidth } from '@/lib/comparisonColors'

echarts.use([
  LineChart, GridComponent, TooltipComponent, LegendComponent,
  DataZoomComponent, BrushComponent, ToolboxComponent, CanvasRenderer,
])

// ── Types ─────────────────────────────────────────────────────────────────────

export interface StreamSeries {
  label: string
  color: string
  data: [number, number][]
}

interface StreamChartProps {
  series: StreamSeries[]
  unit: string
  /** GPS data per source for hover map markers */
  sourcesGps?: SourceGpsData[]
  height?: number
  /** Tooltip value formatter for the y-axis (e.g. pace formatting) */
  formatTooltipValue?: (value: number) => string
}

// ── Tooltip value formatting ──────────────────────────────────────────────────

/** Round value based on unit type: whole numbers for HR/power/cadence/ms/mm, 1 decimal otherwise. */
function formatValueWithUnit(value: number, unit: string): string {
  const wholeUnits = ['bpm', 'W', 'rpm', 'ms', 'mm']
  const rounded = wholeUnits.includes(unit) ? Math.round(value) : parseFloat(value.toFixed(1))
  return `${rounded} ${unit}`
}

// ── Chart option builder ──────────────────────────────────────────────────────

function buildOption(
  series: StreamSeries[],
  unit: string,
  formatTooltipValue?: (v: number) => string,
): EChartsCoreOption {
  const xMax = series.reduce((m, s) => {
    const last = s.data[s.data.length - 1]
    return last ? Math.max(m, last[0]) : m
  }, 1)

  return {
    backgroundColor: 'transparent',
    animation: false,
    grid: { left: 52, right: 12, top: 28, bottom: 8, containLabel: false },
    brush: {
      toolbox: [],
      brushType: 'lineX',
      brushStyle: {
        borderWidth: 1,
        color: 'rgba(249,115,22,0.12)',
        borderColor: '#f97316',
      },
      throttleType: 'debounce',
      throttleDelay: 100,
    },
    toolbox: { feature: { brush: { show: false } } },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(17,24,39,0.92)',
      borderColor: '#374151',
      textStyle: { color: '#f3f4f6', fontSize: 11 },
      formatter: (params: unknown) => {
        const items = params as Array<{ seriesName: string; value: [number, number]; color: string }>
        if (!items?.length) return ''
        const xLabel = fmtDuration(items[0].value[0])
        const lines = items.map(it => {
          const formatted = formatTooltipValue
            ? formatTooltipValue(it.value[1])
            : formatValueWithUnit(it.value[1], unit)
          return `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${it.color};margin-right:4px;"></span>${it.seriesName}: <b>${formatted}</b>`
        })
        return `${xLabel}<br/>${lines.join('<br/>')}`
      },
    },
    legend: {
      top: 4,
      right: 8,
      textStyle: { fontSize: 10, color: '#9ca3af' },
      itemHeight: 6,
      itemWidth: 12,
      selectedMode: false,
    },
    xAxis: {
      type: 'value',
      show: true,
      min: 0,
      max: xMax,
      axisLabel: {
        fontSize: 9,
        color: '#6b7280',
        formatter: (v: number) => fmtDuration(v),
      },
      splitLine: { show: false },
      axisLine: { show: false },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      name: unit,
      nameTextStyle: { fontSize: 9, color: '#6b7280', padding: [0, 0, 0, -12] },
      axisLabel: { fontSize: 9, color: '#6b7280' },
      splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      scale: true,
    },
    dataZoom: [
      {
        type: 'inside',
        xAxisIndex: 0,
        filterMode: 'none',
        zoomOnMouseWheel: true,
        moveOnMouseMove: true,
        moveOnMouseWheel: false,
      },
    ],
    series: series.map((s, idx) => ({
      name: s.label,
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: s.data,
      z: idx === 0 ? 10 : 2, // primary/first source on top layer
      lineStyle: {
        color: s.color,
        width: getSourceChartLineWidth(idx),
        type: getSourceLineStyle(idx),
      },
      itemStyle: { color: s.color },
      emphasis: { focus: 'none' },
    })),
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function StreamChart({
  series,
  unit,
  sourcesGps = [],
  height = 130,
  formatTooltipValue,
}: StreamChartProps) {
  const [isZoomed, setIsZoomed] = useState(false)

  const { bindBrushZoom, resetZoom } = useBrushZoom(setIsZoomed)
  const { attachHover, detachHover } = useHoverSync(sourcesGps)

  const containerRef = useECharts(
    () => buildOption(series, unit, formatTooltipValue),
    [series, unit, formatTooltipValue],
    (chart) => {
      const xMax = series.reduce((m, s) => {
        const last = s.data[s.data.length - 1]
        return last ? Math.max(m, last[0]) : m
      }, 1)
      bindBrushZoom(chart, xMax)

      const container = chart.getDom()
      attachHover(container, chart)
      return () => detachHover(container)
    },
  )

  const handleReset = useCallback(() => {
    // Access chart instance via the DOM element for reset
    const container = containerRef.current
    if (!container) return
    const chart = echarts.getInstanceByDom(container)
    if (!chart) return
    resetZoom(chart)
  }, [resetZoom, containerRef])

  return (
    <div className="relative">
      <div ref={containerRef} style={{ height }} className="w-full [&_canvas]:!cursor-crosshair" />
      {isZoomed && (
        <button
          onClick={handleReset}
          className="absolute top-1 right-1 px-1.5 py-0.5 rounded text-[10px] bg-neutral-800/80 text-neutral-200 hover:bg-neutral-700 cursor-pointer"
        >
          Zoom zurücksetzen
        </button>
      )}
    </div>
  )
}
