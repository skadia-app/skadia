import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchHealthSpO2 } from '@/api/analytics'
import { toLocalISO } from '@/lib/dateUtils'
import { useECharts } from '@/hooks/useECharts'

echarts.use([LineChart, GridComponent, TooltipComponent, DataZoomComponent, CanvasRenderer])

type TimePreset = '7d' | '30d' | '90d' | '1y' | 'all'

const presetLabels: Record<TimePreset, string> = {
  '7d': '7T',
  '30d': '30T',
  '90d': '90T',
  '1y': '1J',
  'all': 'Alle',
}

function getDateRange(preset: TimePreset): { from: string; to: string } {
  const to = new Date()
  const from = new Date()
  switch (preset) {
    case '7d': from.setDate(from.getDate() - 7); break
    case '30d': from.setDate(from.getDate() - 30); break
    case '90d': from.setDate(from.getDate() - 90); break
    case '1y': from.setFullYear(from.getFullYear() - 1); break
    case 'all': return { from: '0001-01-01', to: toLocalISO(to) }
  }
  return { from: toLocalISO(from), to: toLocalISO(to) }
}

export default function SpO2Chart() {
  const [preset, setPreset] = useState<TimePreset>('30d')

  const { from, to } = useMemo(() => getDateRange(preset), [preset])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['health', 'spo2', from, to],
    queryFn: () => fetchHealthSpO2(from, to),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const containerRef = useECharts(
    () => {
      if (!data || data.length === 0) return { series: [] }

      const byDate = new Map<string, { sum: number; count: number }>()
      for (const entry of data) {
        if (entry.spo2_pct == null) continue
        const date = entry.measured_at_utc.split('T')[0]
        const existing = byDate.get(date)
        if (existing) { existing.sum += entry.spo2_pct; existing.count++ }
        else byDate.set(date, { sum: entry.spo2_pct, count: 1 })
      }

      const dates = Array.from(byDate.keys()).sort()
      const values = dates.map(d => {
        const e = byDate.get(d)!
        return Math.round(e.sum / e.count * 10) / 10
      })

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
            const val = items[0].value
            let status = 'Normal'
            if (val < 90) status = 'Niedrig'
            else if (val < 95) status = 'Leicht reduziert'
            return `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>
              <div><strong style="color:#f9fafb">${val}%</strong> <span style="color:#9ca3af">(${status})</span></div>`
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
          min: 85,
          max: 100,
          axisLabel: { fontSize: 10, color: '#6b7280', formatter: (v: number) => `${v}%` },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        },
        dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
        series: [
          {
            type: 'line',
            data: values,
            showSymbol: false,
            smooth: 0.3,
            lineStyle: { width: 2, color: '#3b82f6' },
            itemStyle: { color: '#3b82f6' },
            areaStyle: {
              color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                { offset: 0, color: 'rgba(59,130,246,0.25)' },
                { offset: 1, color: 'rgba(59,130,246,0.02)' },
              ]),
            },
          },
        ],
      }
    },
    [data],
  )

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          SpO2-Verlauf
        </h2>
        <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {(Object.keys(presetLabels) as TimePreset[]).map(p => (
            <button
              key={p}
              onClick={() => setPreset(p)}
              className={
                p === preset
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
              }
            >
              {presetLabels[p]}
            </button>
          ))}
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            Lade SpO2-Daten...
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            Fehler beim Laden der SpO2-Daten
          </div>
        ) : !data || data.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-[280px] gap-3 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span className="text-sm">Keine SpO2-Daten vorhanden</span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[280px] w-full" />
        )}
      </div>
    </div>
  )
}
