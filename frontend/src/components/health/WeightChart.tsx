import { useMemo } from 'react'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { useTranslation } from 'react-i18next'
import { useECharts } from '@/hooks/useECharts'
import type { HealthWeightEntry } from '@/api/health'
import { formatWeight } from '@/lib/units'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

interface Props {
  data: HealthWeightEntry[]
}

export default function WeightChart({ data }: Props) {
  const { t } = useTranslation()
  const { options: unitOptions } = useUnitFormatters()
  const sorted = useMemo(() =>
    [...data].sort((a, b) => a.measured_at_utc.localeCompare(b.measured_at_utc)),
    [data]
  )

  const containerRef = useECharts(
    (theme) => {
      const dates = sorted.map(d => d.measured_at_utc.split('T')[0])
      const weights = sorted.map(d => d.weight_g / 1000)

      return {
        backgroundColor: 'transparent',
        grid: { left: 55, right: 16, top: 20, bottom: 40 },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'cross' },
          formatter: (params: unknown) => {
            const items = params as Array<{ value: number; axisValueLabel: string; color: string; dataIndex: number }>
            if (!Array.isArray(items) || items.length === 0) return ''
            const p = items[0]
            const entry = sorted[p.dataIndex]
            let html = `<div style="font-size:12px;margin-bottom:4px"><strong>${p.axisValueLabel}</strong></div>`
            html += `<div><span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color};margin-right:6px"></span>${t('healthCharts.weight')}: <strong>${formatWeight(p.value, unitOptions)}</strong></div>`
            if (entry?.bmi != null) html += `<div style="margin-left:14px">BMI: ${entry.bmi.toFixed(1)}</div>`
            if (entry?.body_fat_pct != null) html += `<div style="margin-left:14px">${t('healthCharts.bodyFat')}: ${entry.body_fat_pct.toFixed(1)}%</div>`
            return html
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
            formatter: (v: number) => formatWeight(v, unitOptions),
          },
          splitLine: { lineStyle: { color: theme.gridLineColor, type: 'dashed' } },
          scale: true,
        },
        series: [
          {
            type: 'line',
            data: weights,
            showSymbol: sorted.length < 30,
            symbolSize: 5,
            lineStyle: { width: 2, color: '#22c55e' },
            itemStyle: { color: '#22c55e' },
            areaStyle: { color: 'rgba(34,197,94,0.08)' },
            smooth: 0.2,
          },
        ],
      }
    },
    [sorted, t, unitOptions],
  )

  if (sorted.length === 0) {
    return (
      <div className="flex items-center justify-center h-[220px] text-sm text-neutral-400">
        {t('healthCharts.noWeightData')}
      </div>
    )
  }

  return <div ref={containerRef} className="h-[220px] w-full" />
}
