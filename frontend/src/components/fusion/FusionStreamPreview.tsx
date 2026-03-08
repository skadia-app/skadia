import { useRef, useEffect } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

export interface StreamData {
  activityId: string
  label: string
  color: string
  hrData: Array<[number, number]>    // [elapsed_s, hr]
  powerData: Array<[number, number]>  // [elapsed_s, power]
}

interface FusionStreamPreviewProps {
  streams: StreamData[]
}

function EChartsContainer({
  option,
  height = 140,
}: {
  option: EChartsCoreOption
  height?: number
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    if (!containerRef.current) return
    if (chartRef.current) chartRef.current.dispose()
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!chartRef.current) return
    chartRef.current.setOption(option, { notMerge: true })
  }, [option])

  return <div ref={containerRef} style={{ height }} className="w-full" />
}

export default function FusionStreamPreview({ streams }: FusionStreamPreviewProps) {
  const hasHR = streams.some(s => s.hrData.length > 0)
  const hasPower = streams.some(s => s.powerData.length > 0)

  if (!hasHR && !hasPower) {
    return (
      <div className="flex items-center justify-center h-32 text-neutral-400 text-xs rounded-xl border border-neutral-200 dark:border-neutral-800">
        Keine HR/Leistungsdaten zum Vorschauen
      </div>
    )
  }

  const baseOption: Partial<EChartsCoreOption> = {
    backgroundColor: 'transparent',
    grid: { left: 48, right: 12, top: 28, bottom: 24, containLabel: false },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(17,24,39,0.92)',
      borderColor: '#374151',
      textStyle: { color: '#f3f4f6', fontSize: 11 },
    },
    legend: {
      top: 4,
      right: 8,
      textStyle: { fontSize: 10, color: '#9ca3af' },
      itemHeight: 6,
      itemWidth: 12,
    },
    xAxis: {
      type: 'value',
      show: false,
    },
  }

  const hrOption: EChartsCoreOption = hasHR
    ? {
        ...baseOption,
        yAxis: {
          type: 'value',
          name: 'bpm',
          nameTextStyle: { fontSize: 9, color: '#6b7280', padding: [0, 0, 0, -12] },
          axisLabel: { fontSize: 9, color: '#6b7280' },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
          scale: true,
        },
        series: streams
          .filter(s => s.hrData.length > 0)
          .map(s => ({
            name: s.label,
            type: 'line',
            smooth: true,
            symbol: 'none',
            data: s.hrData,
            lineStyle: { color: s.color, width: 1.5 },
            itemStyle: { color: s.color },
            sampling: 'lttb',
          })),
      }
    : {}

  const powerOption: EChartsCoreOption = hasPower
    ? {
        ...baseOption,
        yAxis: {
          type: 'value',
          name: 'W',
          nameTextStyle: { fontSize: 9, color: '#6b7280', padding: [0, 0, 0, -12] },
          axisLabel: { fontSize: 9, color: '#6b7280' },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
          scale: true,
        },
        series: streams
          .filter(s => s.powerData.length > 0)
          .map(s => ({
            name: s.label,
            type: 'line',
            smooth: true,
            symbol: 'none',
            data: s.powerData,
            lineStyle: { color: s.color, width: 1.5 },
            itemStyle: { color: s.color },
            sampling: 'lttb',
          })),
      }
    : {}

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-3">
      <p className="text-xs font-semibold text-neutral-500">Stream-Vorschau</p>
      {hasHR && (
        <div>
          <p className="text-xs text-neutral-400 mb-1">Herzfrequenz</p>
          <EChartsContainer option={hrOption} height={130} />
        </div>
      )}
      {hasPower && (
        <div>
          <p className="text-xs text-neutral-400 mb-1">Leistung</p>
          <EChartsContainer option={powerOption} height={130} />
        </div>
      )}
    </div>
  )
}
