import { useRef, useEffect } from 'react'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, MarkLineComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { HealthDailySummary } from '@/api/health'

echarts.use([BarChart, GridComponent, TooltipComponent, MarkLineComponent, CanvasRenderer])

interface Props {
  data: HealthDailySummary[]
}

export default function StepsChart({ data }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current || data.length === 0) return
    const isDark = document.documentElement.classList.contains('dark')
    const textColor = isDark ? '#9ca3af' : '#6b7280'
    const gridLineColor = isDark ? '#262626' : '#f5f5f5'

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })

    const dates = data.map(d => d.calendar_date)
    const steps = data.map(d => d.total_steps ?? 0)
    const goal = data.find(d => d.step_goal != null)?.step_goal ?? null

    const barColors = data.map(d => {
      const s = d.total_steps ?? 0
      const g = d.step_goal ?? 10000
      return s >= g ? '#22c55e' : '#fb923c'
    })

    const seriesData = steps.map((v, i) => ({
      value: v,
      itemStyle: { color: barColors[i] },
    }))

    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      grid: { left: 50, right: 16, top: 20, bottom: 40 },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const items = params as Array<{ axisValueLabel: string; value: number }>
          if (!Array.isArray(items) || items.length === 0) return ''
          const item = items[0]
          return `<div style="font-size:12px"><strong>${item.axisValueLabel}</strong><br/>${item.value.toLocaleString('de-DE')} Schritte</div>`
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
          formatter: (v: number) => v >= 1000 ? `${(v / 1000).toFixed(0)}k` : String(v),
        },
        splitLine: { lineStyle: { color: gridLineColor, type: 'dashed' } },
      },
      series: [
        {
          type: 'bar',
          data: seriesData,
          barMaxWidth: 20,
          ...(goal != null
            ? {
                markLine: {
                  silent: true,
                  symbol: 'none',
                  lineStyle: { color: '#3b82f6', type: 'dashed', width: 1.5 },
                  label: {
                    formatter: `Ziel: ${goal.toLocaleString('de-DE')}`,
                    position: 'end',
                    fontSize: 10,
                    color: '#3b82f6',
                  },
                  data: [{ yAxis: goal }],
                },
              }
            : {}),
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
        Keine Schrittzähler-Daten vorhanden
      </div>
    )
  }

  return <div ref={containerRef} className="h-[220px] w-full" />
}
