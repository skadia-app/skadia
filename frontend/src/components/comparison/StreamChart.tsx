// StreamChart — slim ECharts wrapper for one metric, N sources.
// Handles brush zoom, hover sync, resize, and zoom-reset button.

import { useRef, useEffect, useState, useCallback } from 'react'
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
import { useBrushZoom } from '@/hooks/useBrushZoom'
import { useHoverSync, type SourceGpsData } from '@/hooks/useHoverSync'

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
      formatter: formatTooltipValue
        ? (params: unknown) => {
            const items = params as Array<{ seriesName: string; value: [number, number]; color: string }>
            if (!items?.length) return ''
            const xLabel = formatElapsed(items[0].value[0])
            const lines = items.map(it =>
              `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${it.color};margin-right:4px;"></span>${it.seriesName}: <b>${formatTooltipValue(it.value[1])}</b>`
            )
            return `${xLabel}<br/>${lines.join('<br/>')}`
          }
        : undefined,
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
        formatter: (v: number) => formatElapsed(v),
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
      lineStyle: {
        color: s.color,
        width: idx === 0 ? 2.5 : idx === 1 ? 1.5 : 1.5,
        type: idx === 0 ? 'solid' : idx === 1 ? 'dashed' : 'dotted',
      },
      itemStyle: { color: s.color },
      emphasis: { focus: 'none' },
    })),
  }
}

function formatElapsed(s: number): string {
  const m = Math.floor(s / 60)
  const h = Math.floor(m / 60)
  if (h > 0) return `${h}:${String(m % 60).padStart(2, '0')}h`
  return `${m}:${String(Math.floor(s % 60)).padStart(2, '0')}`
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function StreamChart({
  series,
  unit,
  sourcesGps = [],
  height = 130,
  formatTooltipValue,
}: StreamChartProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)
  const [isZoomed, setIsZoomed] = useState(false)

  const { bindBrushZoom, resetZoom } = useBrushZoom(setIsZoomed)
  const { attachHover, detachHover } = useHoverSync(sourcesGps)

  // Init chart
  useEffect(() => {
    if (!containerRef.current) return
    if (chartRef.current) chartRef.current.dispose()
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)
    const container = containerRef.current
    attachHover(container, chart)

    return () => {
      detachHover(container)
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Update option when series/unit changes
  useEffect(() => {
    const chart = chartRef.current
    if (!chart || series.length === 0) return
    const option = buildOption(series, unit, formatTooltipValue)
    chart.setOption(option, { notMerge: true })

    const xMax = series.reduce((m, s) => {
      const last = s.data[s.data.length - 1]
      return last ? Math.max(m, last[0]) : m
    }, 1)
    bindBrushZoom(chart, xMax)
  }, [series, unit, formatTooltipValue, bindBrushZoom])

  const handleReset = useCallback(() => {
    const chart = chartRef.current
    if (!chart) return
    resetZoom(chart)
  }, [resetZoom])

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
