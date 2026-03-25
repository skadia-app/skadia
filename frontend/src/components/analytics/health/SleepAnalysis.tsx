import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchSleepAnalysis } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { toLocalISO } from '@/lib/dateUtils'
import { useECharts } from '@/hooks/useECharts'

echarts.use([
  BarChart,
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  CanvasRenderer,
])

type TimePreset = '7d' | '30d' | '90d'

const presetLabels: Record<TimePreset, string> = {
  '7d': '7T',
  '30d': '30T',
  '90d': '90T',
}

function getDateRange(preset: TimePreset): { from: string; to: string } {
  const to = new Date()
  const from = new Date()
  switch (preset) {
    case '7d': from.setDate(from.getDate() - 7); break
    case '30d': from.setDate(from.getDate() - 30); break
    case '90d': from.setDate(from.getDate() - 90); break
  }
  return {
    from: toLocalISO(from),
    to: toLocalISO(to),
  }
}

function trendArrow(trend: string): { arrow: string; color: string } {
  switch (trend) {
    case 'improving': return { arrow: '↑', color: 'text-emerald-500' }
    case 'declining': return { arrow: '↓', color: 'text-rose-500' }
    default: return { arrow: '→', color: 'text-neutral-400' }
  }
}

function fmtMinutes(min: number): string {
  const h = Math.floor(min / 60)
  const m = Math.round(min % 60)
  return `${h}h ${m.toString().padStart(2, '0')}m`
}

export default function SleepAnalysisChart() {
  const { toQueryString } = useAnalyticsFilters()
  const [preset, setPreset] = useState<TimePreset>('30d')

  const { from, to } = useMemo(() => getDateRange(preset), [preset])

  const queryParams = useMemo(() => {
    const base = toQueryString()
    const params = new URLSearchParams(base)
    params.set('from', from)
    params.set('to', to)
    return params.toString()
  }, [toQueryString, from, to])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'sleep-analysis', queryParams],
    queryFn: () => fetchSleepAnalysis(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const containerRef = useECharts(
    () => {
      if (!data) return { series: [] }

      const points = data.points ?? []
      if (points.length === 0) return { series: [] }

      const dates = points.map(p => p.date)

      const deepData = points.map(p => {
        if (p.duration_min == null || p.deep_sleep_pct == null) return null
        return Math.round((p.duration_min * p.deep_sleep_pct) / 100)
      })
      const remData = points.map(p => {
        if (p.duration_min == null || p.rem_sleep_pct == null) return null
        return Math.round((p.duration_min * p.rem_sleep_pct) / 100)
      })
      const otherData = points.map(p => {
        if (p.duration_min == null) return null
        const deep = p.deep_sleep_pct != null ? (p.duration_min * p.deep_sleep_pct) / 100 : 0
        const rem = p.rem_sleep_pct != null ? (p.duration_min * p.rem_sleep_pct) / 100 : 0
        const other = p.duration_min - deep - rem
        return Math.max(0, Math.round(other))
      })
      const scoreData = points.map(p => p.score_overall)

      return {
        backgroundColor: 'transparent',
        grid: { left: 52, right: 52, top: 44, bottom: 40 },
        legend: {
          data: ['Tiefschlaf', 'REM', 'Sonstig', 'Score'],
          top: 8,
          textStyle: { fontSize: 10, color: '#9ca3af' },
          itemHeight: 8,
          itemWidth: 12,
        },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'shadow' },
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{
              seriesName: string
              value: number | null
              axisValueLabel: string
              color: string
            }>
            if (!Array.isArray(items) || items.length === 0) return ''
            let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
            for (const p of items) {
              if (p.value == null) continue
              const displayVal = p.seriesName === 'Score'
                ? `${p.value}`
                : `${p.value} min`
              html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
                <span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:${p.color}"></span>
                <span style="color:#d1d5db">${p.seriesName}:</span>
                <strong style="color:#f9fafb">${displayVal}</strong>
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
        yAxis: [
          {
            type: 'value',
            name: 'Minuten',
            nameTextStyle: { fontSize: 10, color: '#6b7280' },
            axisLabel: {
              fontSize: 10,
              color: '#6b7280',
              formatter: (v: number) => `${Math.floor(v / 60)}h`,
            },
            splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
          },
          {
            type: 'value',
            name: 'Score',
            nameTextStyle: { fontSize: 10, color: '#6b7280' },
            min: 0,
            max: 100,
            axisLabel: { fontSize: 10, color: '#6b7280' },
            splitLine: { show: false },
          },
        ],
        series: [
          {
            name: 'Tiefschlaf',
            type: 'bar',
            stack: 'sleep',
            yAxisIndex: 0,
            data: deepData,
            itemStyle: { color: '#1e3a5f' },
            barMaxWidth: 20,
          },
          {
            name: 'REM',
            type: 'bar',
            stack: 'sleep',
            yAxisIndex: 0,
            data: remData,
            itemStyle: { color: '#3b82f6' },
            barMaxWidth: 20,
          },
          {
            name: 'Sonstig',
            type: 'bar',
            stack: 'sleep',
            yAxisIndex: 0,
            data: otherData,
            itemStyle: { color: '#93c5fd' },
            barMaxWidth: 20,
          },
          {
            name: 'Score',
            type: 'line',
            yAxisIndex: 1,
            data: scoreData,
            showSymbol: points.length < 20,
            symbolSize: 4,
            lineStyle: { width: 2, color: '#f97316' },
            itemStyle: { color: '#f97316' },
            z: 3,
          },
        ],
      }
    },
    [data],
  )

  const trend = data ? trendArrow(data.trend) : null

  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          Schlaf-Analyse
        </h2>
        <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {(Object.keys(presetLabels) as TimePreset[]).map(p => (
            <button
              key={p}
              onClick={() => setPreset(p)}
              className={
                p === preset
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'
              }
            >
              {presetLabels[p]}
            </button>
          ))}
        </div>
      </div>

      {/* Summary row */}
      {data && (
        <div className="flex items-center gap-3 flex-wrap text-xs">
          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300 font-medium">
            Avg Schlafdauer: {fmtMinutes(data.avg_duration_min)}
          </span>
          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full bg-accent-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300 font-medium">
            Avg Score: {data.avg_score.toFixed(0)}
          </span>
          {trend && (
            <span className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full bg-neutral-100 dark:bg-neutral-800 font-medium ${trend.color}`}>
              Trend {trend.arrow}
            </span>
          )}
        </div>
      )}

      {/* Chart */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[260px] text-sm text-neutral-400">
            Lade Schlaf-Daten…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[260px] text-sm text-neutral-400">
            Fehler beim Laden der Schlaf-Daten
          </div>
        ) : !data || !data.points?.length ? (
          <div className="flex flex-col items-center justify-center h-[260px] gap-2 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
            </svg>
            <span className="text-sm">Keine Schlafdaten für diesen Zeitraum</span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[260px] w-full" />
        )}
      </div>
    </div>
  )
}
