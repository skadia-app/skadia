import { useRef, useEffect } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent, VisualMapComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { HealthTrainingLoad } from '@/api/health'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, VisualMapComponent, CanvasRenderer])

interface Props {
  data: HealthTrainingLoad[]
}

function acwrStatusColor(status: string | null): string {
  switch (status) {
    case 'BALANCED': return '#22c55e'
    case 'HIGH': return '#f59e0b'
    case 'OVERREACHING': return '#ef4444'
    case 'LOW': return '#6b7280'
    default: return '#6b7280'
  }
}

function acwrStatusLabel(status: string | null): string {
  switch (status) {
    case 'BALANCED': return 'Ausgewogen'
    case 'HIGH': return 'Hoch'
    case 'OVERREACHING': return 'Übertraining'
    case 'LOW': return 'Niedrig'
    default: return status ?? ''
  }
}

export default function TrainingLoadChart({ data }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current || data.length === 0) return
    const isDark = document.documentElement.classList.contains('dark')
    const textColor = isDark ? '#9ca3af' : '#6b7280'
    const gridLineColor = isDark ? '#262626' : '#f5f5f5'

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })

    const sorted = [...data].sort((a, b) => a.calendar_date.localeCompare(b.calendar_date))
    const dates = sorted.map(d => d.calendar_date)
    const atl = sorted.map(d => d.atl_acute)
    const ctl = sorted.map(d => d.ctl_chronic)

    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      grid: { left: 50, right: 16, top: 40, bottom: 40 },
      legend: {
        data: ['ATL (kurzfristig)', 'CTL (langfristig)'],
        top: 8,
        textStyle: { fontSize: 10, color: textColor },
        itemHeight: 8,
        itemWidth: 16,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        formatter: (params: unknown) => {
          const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string; dataIndex: number }>
          if (!Array.isArray(items) || items.length === 0) return ''
          const idx = items[0].dataIndex
          const entry = sorted[idx]
          let html = `<div style="font-size:12px;margin-bottom:4px"><strong>${items[0].axisValueLabel}</strong>`
          if (entry?.acwr_status) {
            const color = acwrStatusColor(entry.acwr_status)
            html += ` <span style="color:${color};font-size:10px">${acwrStatusLabel(entry.acwr_status)}</span>`
          }
          html += `</div>`
          for (const p of items) {
            if (p.value != null) {
              html += `<div style="display:flex;align-items:center;gap:6px"><span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>${p.seriesName}: <strong>${p.value.toFixed(1)}</strong></div>`
            }
          }
          if (entry?.acwr_pct != null) {
            html += `<div style="margin-top:4px;font-size:11px;color:${textColor}">ACWR: ${entry.acwr_pct.toFixed(1)}%</div>`
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
        axisLabel: { fontSize: 10, color: textColor },
        splitLine: { lineStyle: { color: gridLineColor, type: 'dashed' } },
      },
      series: [
        {
          name: 'ATL (kurzfristig)',
          type: 'line',
          data: atl,
          showSymbol: false,
          lineStyle: { width: 2, color: '#f97316' },
          itemStyle: { color: '#f97316' },
          areaStyle: { color: 'rgba(249,115,22,0.08)' },
        },
        {
          name: 'CTL (langfristig)',
          type: 'line',
          data: ctl,
          showSymbol: false,
          lineStyle: { width: 2.5, color: '#3b82f6' },
          itemStyle: { color: '#3b82f6' },
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
        Keine Trainingsbelastungs-Daten vorhanden
      </div>
    )
  }

  return <div ref={containerRef} className="h-[220px] w-full" />
}
