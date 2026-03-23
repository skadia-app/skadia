import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchWeightAnalysis } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { toLocalISO } from '@/lib/dateUtils'
import { useECharts } from '@/hooks/useECharts'

echarts.use([
  LineChart,
  BarChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  CanvasRenderer,
])

export default function WeightChart() {
  const { toQueryString } = useAnalyticsFilters()

  const queryParams = useMemo(() => {
    const base = toQueryString()
    const params = new URLSearchParams(base)
    const to = new Date()
    const from = new Date()
    from.setDate(from.getDate() - 90)
    params.set('from', toLocalISO(from))
    params.set('to', toLocalISO(to))
    return params.toString()
  }, [toQueryString])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'weight-analysis', queryParams],
    queryFn: () => fetchWeightAnalysis(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const containerRef = useECharts(
    () => {
      if (!data) return { series: [] }

      const points = data.points ?? []
      if (points.length === 0) return { series: [] }

      const dates = points.map(p => p.date)
      const weights = points.map(p => p.weight_kg)
      const hasFatData = points.some(p => p.body_fat_pct != null)
      const fatData = points.map(p => p.body_fat_pct)

      const yAxes = hasFatData
        ? [
            {
              type: 'value',
              axisLabel: { fontSize: 10, color: '#6b7280', formatter: (v: number) => `${v} kg` },
              splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
              scale: true,
            },
            {
              type: 'value',
              axisLabel: { fontSize: 10, color: '#6b7280', formatter: (v: number) => `${v}%` },
              splitLine: { show: false },
              scale: true,
            },
          ]
        : {
            type: 'value',
            axisLabel: { fontSize: 10, color: '#6b7280', formatter: (v: number) => `${v} kg` },
            splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
            scale: true,
          }

      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const series: any[] = [
        {
          name: 'Gewicht',
          type: 'line',
          yAxisIndex: 0,
          data: weights,
          showSymbol: points.length < 30,
          symbolSize: 5,
          lineStyle: { width: 2.5, color: '#22c55e' },
          itemStyle: { color: '#22c55e' },
          areaStyle: { color: 'rgba(34,197,94,0.06)' },
          smooth: 0.2,
          z: 2,
        },
      ]

      if (hasFatData) {
        series.push({
          name: 'Körperfett',
          type: 'bar',
          yAxisIndex: 1,
          data: fatData,
          barMaxWidth: 12,
          itemStyle: { color: 'rgba(251,146,60,0.5)' },
          z: 1,
        })
      }

      return {
        backgroundColor: 'transparent',
        grid: { left: 55, right: hasFatData ? 50 : 20, top: hasFatData ? 44 : 24, bottom: 40 },
        legend: hasFatData
          ? {
              data: ['Gewicht', 'Körperfett'],
              top: 8,
              textStyle: { fontSize: 10, color: '#9ca3af' },
              itemHeight: 8,
              itemWidth: 14,
            }
          : undefined,
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'cross' },
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string }>
            if (!Array.isArray(items) || items.length === 0) return ''
            let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
            for (const p of items) {
              if (p.value == null) continue
              const unit = p.seriesName === 'Körperfett' ? '%' : 'kg'
              html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
                <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
                <span style="color:#d1d5db">${p.seriesName}:</span>
                <strong style="color:#f9fafb">${p.value.toFixed(1)} ${unit}</strong>
              </div>`
            }
            return html
          },
        },
        xAxis: {
          type: 'category',
          data: dates,
          axisLabel: { fontSize: 10, color: '#6b7280', rotate: dates.length > 14 ? 30 : 0 },
          splitLine: { show: false },
          axisLine: { lineStyle: { color: '#374151' } },
        },
        yAxis: yAxes,
        series,
      }
    },
    [data],
  )

  const changeColor = data && data.change_30d !== 0
    ? data.change_30d < 0 ? 'text-emerald-500' : 'text-rose-500'
    : 'text-neutral-400'

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          Gewichtsverlauf
        </h2>
        {data && (
          <div className="flex items-center gap-2">
            <span className="text-lg font-bold tabular-nums text-emerald-500">
              {data.current_kg.toFixed(1)} kg
            </span>
            {data.change_30d !== 0 && (
              <span className={`text-sm font-medium tabular-nums ${changeColor}`}>
                {data.change_30d > 0 ? '+' : ''}{data.change_30d.toFixed(1)} kg (30T)
              </span>
            )}
          </div>
        )}
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[260px] text-sm text-neutral-400">
            Lade Gewichtsdaten…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[260px] text-sm text-neutral-400">
            Fehler beim Laden der Gewichtsdaten
          </div>
        ) : !data || !data.points?.length ? (
          <div className="flex flex-col items-center justify-center h-[260px] gap-2 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M3 6l3 1m0 0l-3 9a5.002 5.002 0 006.001 0M6 7l3 9M6 7l6-2m6 2l3-1m-3 1l-3 9a5.002 5.002 0 006.001 0M18 7l3 9m-3-9l-6-2m0-2v2m0 16V5m0 16H9m3 0h3" />
            </svg>
            <span className="text-sm">Keine Gewichtsdaten vorhanden</span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[260px] w-full" />
        )}
      </div>
    </div>
  )
}
