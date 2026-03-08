import { useState, useRef, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchHRVCorrelation } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { toLocalISO } from '@/lib/dateUtils'

echarts.use([ScatterChart, GridComponent, TooltipComponent, CanvasRenderer])

type TimePreset = '30d' | '90d' | '1y' | 'all'

const presetLabels: Record<TimePreset, string> = {
  '30d': '30T',
  '90d': '90T',
  '1y': '1J',
  'all': 'Alle',
}

function getDateRange(preset: TimePreset): { from: string; to: string } {
  const to = new Date()
  const from = new Date()
  switch (preset) {
    case '30d': from.setDate(from.getDate() - 30); break
    case '90d': from.setDate(from.getDate() - 90); break
    case '1y': from.setFullYear(from.getFullYear() - 1); break
    case 'all': return { from: '0001-01-01', to: toLocalISO(to) }
  }
  return {
    from: toLocalISO(from),
    to: toLocalISO(to),
  }
}

function interpretLabel(interpretation: string): string {
  const labels: Record<string, string> = {
    'strong positive correlation': 'Starke positive Korrelation',
    'moderate positive correlation': 'Moderate positive Korrelation',
    'weak positive correlation': 'Schwache positive Korrelation',
    'strong negative correlation': 'Starke negative Korrelation',
    'moderate negative correlation': 'Moderate negative Korrelation',
    'weak negative correlation': 'Schwache negative Korrelation',
    'no meaningful correlation': 'Keine bedeutsame Korrelation',
  }
  return labels[interpretation] ?? interpretation
}

export default function HRVCorrelationChart() {
  const { toQueryString } = useAnalyticsFilters()
  const [preset, setPreset] = useState<TimePreset>('90d')
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  const { from, to } = useMemo(() => getDateRange(preset), [preset])

  const queryParams = useMemo(() => {
    const base = toQueryString()
    const params = new URLSearchParams(base)
    params.set('from', from)
    params.set('to', to)
    return params.toString()
  }, [toQueryString, from, to])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'hrv-correlation', queryParams],
    queryFn: () => fetchHRVCorrelation(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  useEffect(() => {
    if (!containerRef.current) return
    if (chartRef.current) chartRef.current.dispose()
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [data])

  useEffect(() => {
    if (!chartRef.current || !data || !data.points || data.points.length === 0) return

    const scatterData = data.points.map(p => [p.daily_tss, p.rmssd])

    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      grid: { left: 60, right: 20, top: 20, bottom: 50 },
      tooltip: {
        trigger: 'item',
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 12 },
        formatter: (params: unknown) => {
          const p = params as { value: [number, number]; dataIndex: number }
          if (!p.value) return ''
          const date = data.points[p.dataIndex]?.date ?? ''
          return `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${date}</div>
            <div>TSS: <strong style="color:#f9fafb">${p.value[0].toFixed(0)}</strong></div>
            <div>RMSSD: <strong style="color:#f9fafb">${p.value[1].toFixed(1)} ms</strong></div>`
        },
      },
      xAxis: {
        type: 'value',
        name: 'TSS (Vortag)',
        nameLocation: 'center',
        nameGap: 30,
        nameTextStyle: { fontSize: 11, color: '#6b7280' },
        axisLabel: { fontSize: 10, color: '#6b7280' },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        axisLine: { lineStyle: { color: '#374151' } },
      },
      yAxis: {
        type: 'value',
        name: 'RMSSD (ms)',
        nameLocation: 'center',
        nameGap: 42,
        nameTextStyle: { fontSize: 11, color: '#6b7280' },
        axisLabel: { fontSize: 10, color: '#6b7280' },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      },
      series: [
        {
          type: 'scatter',
          data: scatterData,
          symbolSize: 8,
          itemStyle: {
            color: '#6366f1',
            opacity: 0.7,
          },
          emphasis: {
            itemStyle: {
              opacity: 1,
              borderColor: '#6366f1',
              borderWidth: 2,
            },
          },
        },
      ],
    }

    chartRef.current.setOption(option, { notMerge: true })
  }, [data])

  const corrColor = data
    ? Math.abs(data.correlation) >= 0.4
      ? '#f59e0b'
      : Math.abs(data.correlation) >= 0.2
        ? '#6b7280'
        : '#9ca3af'
    : '#9ca3af'

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          HRV-Korrelation
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

      {/* Correlation info badges */}
      {data && data.points.length > 0 && (
        <div className="flex items-center gap-3 flex-wrap">
          <span
            className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium"
            style={{ backgroundColor: `${corrColor}20`, color: corrColor }}
          >
            Pearson r = {data.correlation.toFixed(2)}
          </span>
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
            {interpretLabel(data.interpretation)}
          </span>
          <span className="text-xs text-neutral-400">
            {data.points.length} Datenpunkte
          </span>
        </div>
      )}

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[300px] text-sm text-neutral-400">
            Lade HRV-Korrelationsdaten...
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[300px] text-sm text-neutral-400">
            Fehler beim Laden der HRV-Korrelationsdaten
          </div>
        ) : !data || !data.points || data.points.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-[300px] gap-3 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z" />
            </svg>
            <span className="text-sm">Keine HRV-Korrelationsdaten vorhanden</span>
            <span className="text-xs text-neutral-500">
              Es werden HRV- und TSS-Daten benötigt
            </span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[300px] w-full" />
        )}
      </div>
    </div>
  )
}
