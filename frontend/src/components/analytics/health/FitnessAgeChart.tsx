import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchHealthFitnessAge } from '@/api/analytics'
import { toLocalISO } from '@/lib/dateUtils'
import { useECharts } from '@/hooks/useECharts'

echarts.use([LineChart, GridComponent, TooltipComponent, DataZoomComponent, CanvasRenderer])

export default function FitnessAgeChart() {
  const { from, to } = useMemo(() => {
    const to = new Date()
    return {
      from: '0001-01-01',
      to: toLocalISO(to),
    }
  }, [])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['health', 'fitness-age', from, to],
    queryFn: () => fetchHealthFitnessAge(from, to),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const validEntries = useMemo(() => {
    if (!data) return []
    return data.filter(e => e.fitness_age != null)
  }, [data])

  const latestAge = validEntries.length > 0 ? validEntries[validEntries.length - 1].fitness_age : null

  const containerRef = useECharts(
    () => {
      if (validEntries.length === 0) return { series: [] }

      const dates = validEntries.map(e => e.calendar_date)
      const values = validEntries.map(e => e.fitness_age!)

      return {
        backgroundColor: 'transparent',
        grid: { left: 52, right: 20, top: 20, bottom: 40 },
        tooltip: {
          trigger: 'axis',
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{ value: number; axisValueLabel: string }>
            if (!Array.isArray(items) || items.length === 0) return ''
            return `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>
              <div><strong style="color:#f9fafb">${items[0].value}</strong> <span style="color:#9ca3af">Jahre</span></div>`
          },
        },
        xAxis: {
          type: 'category',
          data: dates,
          axisLabel: { fontSize: 10, color: '#6b7280' },
          splitLine: { show: false },
          axisLine: { lineStyle: { color: '#374151' } },
        },
        yAxis: {
          type: 'value',
          axisLabel: { fontSize: 10, color: '#6b7280', formatter: (v: number) => `${v}J` },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        },
        dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
        series: [
          {
            type: 'line',
            data: values,
            showSymbol: true,
            symbolSize: 6,
            smooth: 0.3,
            lineStyle: { width: 2, color: '#22c55e' },
            itemStyle: { color: '#22c55e' },
            areaStyle: {
              color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: 'rgba(34,197,94,0.25)' },
                { offset: 1, color: 'rgba(34,197,94,0.02)' },
              ]),
            },
          },
        ],
      }
    },
    [validEntries],
  )

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-3">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
            Fitness-Alter
          </h2>
          {latestAge != null && (
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
              Aktuell: {latestAge} Jahre
            </span>
          )}
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[240px] text-sm text-neutral-400">
            Lade Fitness-Alter-Daten...
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[240px] text-sm text-neutral-400">
            Fehler beim Laden der Fitness-Alter-Daten
          </div>
        ) : validEntries.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-[240px] gap-3 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
            </svg>
            <span className="text-sm">Keine Fitness-Alter-Daten vorhanden</span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[240px] w-full" />
        )}
      </div>
    </div>
  )
}
