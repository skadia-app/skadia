import { useRef, useEffect } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { HealthSleep } from '@/api/health'

echarts.use([BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

interface Props {
  data: HealthSleep[]
}

function fmtHours(seconds: number | null): number {
  if (seconds == null) return 0
  return Math.round((seconds / 3600) * 100) / 100
}

function fmtHHmm(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}:${m.toString().padStart(2, '0')}`
}

export default function SleepChart({ data }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current || data.length === 0) return
    const isDark = document.documentElement.classList.contains('dark')
    const textColor = isDark ? '#9ca3af' : '#6b7280'
    const gridLineColor = isDark ? '#262626' : '#f5f5f5'

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })

    const dates = data.map(d => d.calendar_date)

    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      grid: { left: 50, right: 16, top: 40, bottom: 50 },
      legend: {
        data: ['Tiefschlaf', 'Leichtschlaf', 'REM', 'Wach'],
        top: 8,
        textStyle: { fontSize: 10, color: textColor },
        itemHeight: 8,
        itemWidth: 12,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const items = params as Array<{ seriesName: string; value: number; axisValueLabel: string; color: string }>
          if (!Array.isArray(items) || items.length === 0) return ''
          const idx = data.findIndex(d => d.calendar_date === items[0].axisValueLabel)
          const entry = idx >= 0 ? data[idx] : null
          const totalSec = (entry?.deep_seconds ?? 0) + (entry?.light_seconds ?? 0) + (entry?.rem_seconds ?? 0)
          let html = `<div style="font-size:12px;margin-bottom:4px"><strong>${items[0].axisValueLabel}</strong>`
          if (totalSec > 0) html += ` · ${fmtHHmm(totalSec)}`
          if (entry?.score_overall != null) html += ` · Score: ${entry.score_overall}`
          html += `</div>`
          for (const p of items) {
            if (p.value > 0) {
              html += `<div style="display:flex;align-items:center;gap:6px"><span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:${p.color}"></span>${p.seriesName}: <strong>${fmtHHmm(Math.round(p.value * 3600))}</strong></div>`
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
          formatter: (v: number) => `${v}h`,
        },
        splitLine: { lineStyle: { color: gridLineColor, type: 'dashed' } },
      },
      series: [
        {
          name: 'Tiefschlaf',
          type: 'bar',
          stack: 'sleep',
          data: data.map(d => fmtHours(d.deep_seconds)),
          itemStyle: { color: '#1e3a5f' },
          barMaxWidth: 20,
        },
        {
          name: 'Leichtschlaf',
          type: 'bar',
          stack: 'sleep',
          data: data.map(d => fmtHours(d.light_seconds)),
          itemStyle: { color: '#60a5fa' },
          barMaxWidth: 20,
        },
        {
          name: 'REM',
          type: 'bar',
          stack: 'sleep',
          data: data.map(d => fmtHours(d.rem_seconds)),
          itemStyle: { color: '#a78bfa' },
          barMaxWidth: 20,
        },
        {
          name: 'Wach',
          type: 'bar',
          stack: 'sleep',
          data: data.map(d => fmtHours(d.awake_seconds)),
          itemStyle: { color: '#6b7280' },
          barMaxWidth: 20,
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
  }, [data])

  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center h-[220px] text-sm text-neutral-400">
        Keine Schlaf-Daten vorhanden
      </div>
    )
  }

  return <div ref={containerRef} className="h-[220px] w-full" />
}
