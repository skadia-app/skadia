import * as echarts from 'echarts/core'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, MarkLineComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useTranslation } from 'react-i18next'
import { useECharts } from '@/hooks/useECharts'
import type { HealthDailySummary } from '@/api/health'

echarts.use([BarChart, GridComponent, TooltipComponent, MarkLineComponent, CanvasRenderer])

interface Props {
  data: HealthDailySummary[]
}

export default function StepsChart({ data }: Props) {
  const { t, i18n } = useTranslation()
  const containerRef = useECharts(
    (theme) => {
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

      return {
        backgroundColor: 'transparent',
        grid: { left: 50, right: 16, top: 20, bottom: 40 },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'shadow' },
          formatter: (params: unknown) => {
            const items = params as Array<{ axisValueLabel: string; value: number }>
            if (!Array.isArray(items) || items.length === 0) return ''
            const item = items[0]
            return `<div style="font-size:12px"><strong>${item.axisValueLabel}</strong><br/>${t('healthCharts.stepsCount', { count: item.value.toLocaleString(i18n.language) })}</div>`
          },
        },
        xAxis: {
          type: 'category',
          data: dates,
          axisLabel: { fontSize: 10, color: theme.textColor, rotate: 30 },
          splitLine: { show: false },
        },
        yAxis: {
          type: 'value',
          axisLabel: {
            fontSize: 10,
            color: theme.textColor,
            formatter: (v: number) => v >= 1000 ? `${(v / 1000).toFixed(0)}k` : String(v),
          },
          splitLine: { lineStyle: { color: theme.gridLineColor, type: 'dashed' } },
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
                      formatter: t('healthCharts.goal', { value: goal.toLocaleString(i18n.language) }),
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
    },
    [data, i18n.language, t],
  )

  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center h-[220px] text-sm text-neutral-400">
        {t('healthCharts.noStepsData')}
      </div>
    )
  }

  return <div ref={containerRef} className="h-[220px] w-full" />
}
