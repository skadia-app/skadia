import { useRef, useEffect, useMemo } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { HealthHRVSnapshot } from '@/api/health'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

interface Props {
  data: HealthHRVSnapshot[]
}

function compute7DayAvg(values: (number | null)[], index: number): number | null {
  const window = values.slice(Math.max(0, index - 6), index + 1).filter(v => v != null) as number[]
  if (window.length === 0) return null
  return window.reduce((a, b) => a + b, 0) / window.length
}

export default function HRVChart({ data }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)

  const sorted = useMemo(() =>
    [...data].sort((a, b) => a.measured_at_utc.localeCompare(b.measured_at_utc)),
    [data]
  )

  useEffect(() => {
    if (!containerRef.current || sorted.length === 0) return
    const isDark = document.documentElement.classList.contains('dark')
    const textColor = isDark ? '#9ca3af' : '#6b7280'
    const gridLineColor = isDark ? '#262626' : '#f5f5f5'

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })

    const dates = sorted.map(d => d.measured_at_utc.split('T')[0])
    const rmssd = sorted.map(d => d.rmssd_ms)
    const avg7 = rmssd.map((_, i) => {
      const v = compute7DayAvg(rmssd, i)
      return v != null ? Math.round(v * 10) / 10 : null
    })

    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      grid: { left: 50, right: 16, top: 40, bottom: 40 },
      legend: {
        data: ['RMSSD', '7-Tage Ø'],
        top: 8,
        textStyle: { fontSize: 10, color: textColor },
        itemHeight: 8,
        itemWidth: 16,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        formatter: (params: unknown) => {
          const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string }>
          if (!Array.isArray(items) || items.length === 0) return ''
          let html = `<div style="font-size:12px;margin-bottom:4px"><strong>${items[0].axisValueLabel}</strong></div>`
          for (const p of items) {
            if (p.value != null) {
              html += `<div style="display:flex;align-items:center;gap:6px"><span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>${p.seriesName}: <strong>${p.value.toFixed(1)} ms</strong></div>`
            }
          }
          return html
        },
      },
      xAxis: {
        type: 'category',
        data: dates,
        axisLabel: { fontSize: 10, color: textColor, rotate: 30 },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value',
        axisLabel: {
          fontSize: 10,
          color: textColor,
          formatter: (v: number) => `${v}ms`,
        },
        splitLine: { lineStyle: { color: gridLineColor, type: 'dashed' } },
      },
      series: [
        {
          name: 'RMSSD',
          type: 'line',
          data: rmssd,
          showSymbol: sorted.length < 15,
          symbolSize: 4,
          lineStyle: { width: 2, color: '#6366f1' },
          itemStyle: { color: '#6366f1' },
          areaStyle: { color: 'rgba(99,102,241,0.08)' },
        },
        {
          name: '7-Tage Ø',
          type: 'line',
          data: avg7,
          showSymbol: false,
          lineStyle: { width: 2, color: '#f97316', type: 'dashed' },
          itemStyle: { color: '#f97316' },
        },
      ],
    }

    chart.setOption(option)

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current!)

    return () => {
      observer.disconnect()
      chart.dispose()
    }
  }, [sorted])

  if (sorted.length === 0) {
    return (
      <div className="flex items-center justify-center h-[220px] text-sm text-neutral-400">
        Keine HRV-Daten vorhanden
      </div>
    )
  }

  return <div ref={containerRef} className="h-[220px] w-full" />
}
