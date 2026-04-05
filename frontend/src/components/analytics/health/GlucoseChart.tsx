import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  MarkLineComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchActivityGlucose } from '@/api/analytics'
import { toLocalISO } from '@/lib/dateUtils'
import { useECharts } from '@/hooks/useECharts'

echarts.use([
  ScatterChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  MarkLineComponent,
  CanvasRenderer,
])

// ── Labels ────────────────────────────────────────────────────────────────────

const LABELS = {
  title: 'Blood Glucose (per Activity)',
  unit: 'mg/dL',
  loading: 'Loading glucose data...',
  error: 'Failed to load glucose data',
  noData: 'No activity glucose data available',
  zoneNormal: 'Normal',
  zoneElevated: 'Elevated',
  zoneHigh: 'High',
  presets: {
    '30d': '30D',
    '90d': '90D',
    '1y': '1Y',
    'all': 'All',
  } as Record<string, string>,
}

// ── Types ─────────────────────────────────────────────────────────────────────

type TimePreset = '30d' | '90d' | '1y' | 'all'

// ── Helpers ───────────────────────────────────────────────────────────────────

function getDateRange(preset: TimePreset): { from: string; to: string } {
  const to = new Date()
  const from = new Date()
  switch (preset) {
    case '30d': from.setDate(from.getDate() - 30); break
    case '90d': from.setDate(from.getDate() - 90); break
    case '1y': from.setFullYear(from.getFullYear() - 1); break
    case 'all': return { from: '0001-01-01', to: toLocalISO(to) }
  }
  return { from: toLocalISO(from), to: toLocalISO(to) }
}

function glucoseColor(val: number): string {
  if (val >= 125) return '#ef4444'
  if (val >= 100) return '#f59e0b'
  return '#22c55e'
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function GlucoseChart() {
  const [preset, setPreset] = useState<TimePreset>('90d')

  const { from, to } = useMemo(() => getDateRange(preset), [preset])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'activity-glucose', from, to],
    queryFn: () => fetchActivityGlucose(from, to),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const containerRef = useECharts(
    (theme) => {
      if (!data || data.length === 0) return { series: [] }

      const points = data.map(p => ({
        value: [p.started_at.split('T')[0], Math.round(p.avg_glucose * 10) / 10] as [string, number],
        itemStyle: { color: glucoseColor(p.avg_glucose) },
      }))

      return {
        backgroundColor: 'transparent',
        grid: { left: 52, right: 20, top: 20, bottom: 48 },
        tooltip: {
          trigger: 'item',
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (param: unknown) => {
            const p = param as { value: [string, number] }
            const val = p.value[1]
            const date = p.value[0]
            let zone = LABELS.zoneNormal
            let zoneColor = '#22c55e'
            if (val >= 125) { zone = LABELS.zoneHigh; zoneColor = '#ef4444' }
            else if (val >= 100) { zone = LABELS.zoneElevated; zoneColor = '#f59e0b' }
            return `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${date}</div>
              <div><strong style="color:#f9fafb">${val} mg/dL</strong> <span style="color:${zoneColor};font-size:10px">${zone}</span></div>`
          },
        },
        xAxis: {
          type: 'category',
          data: Array.from(new Set(points.map(p => p.value[0]))).sort(),
          axisLabel: { fontSize: 10, color: theme.textColor },
          splitLine: { show: false },
          axisLine: { lineStyle: { color: '#374151' } },
        },
        yAxis: {
          type: 'value',
          min: (minVal: number) => Math.max(0, Math.floor(minVal * 0.9)),
          axisLabel: {
            fontSize: 10,
            color: theme.textColor,
            formatter: (v: number) => `${v}`,
          },
          splitLine: { lineStyle: { color: theme.gridLineColor, type: 'dashed' } },
          name: LABELS.unit,
          nameTextStyle: { color: theme.textColor, fontSize: 10 },
        },
        dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
        series: [
          {
            type: 'scatter',
            data: points,
            symbolSize: 8,
            markLine: {
              silent: true,
              symbol: ['none', 'none'],
              data: [
                {
                  yAxis: 100,
                  lineStyle: { color: '#f59e0b', type: 'dashed', width: 1.5, opacity: 0.7 },
                  label: { show: true, position: 'insideEndTop', formatter: '100 mg/dL', fontSize: 9, color: '#f59e0b' },
                },
                {
                  yAxis: 125,
                  lineStyle: { color: '#ef4444', type: 'dashed', width: 1.5, opacity: 0.7 },
                  label: { show: true, position: 'insideEndTop', formatter: '125 mg/dL', fontSize: 9, color: '#ef4444' },
                },
              ],
            },
          },
        ],
      }
    },
    [data],
  )

  // Do not render if there is no data at all (user has no glucose-enabled activities)
  if (!isLoading && !isError && data && data.length === 0) return null

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {LABELS.title}
        </h2>
        <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {(Object.keys(LABELS.presets) as TimePreset[]).map(p => (
            <button
              key={p}
              onClick={() => setPreset(p)}
              className={
                p === preset
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'
              }
            >
              {LABELS.presets[p]}
            </button>
          ))}
        </div>
      </div>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            {LABELS.loading}
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[280px] text-sm text-neutral-400">
            {LABELS.error}
          </div>
        ) : (
          <div ref={containerRef} className="h-[280px] w-full" />
        )}
      </div>

      <div className="flex gap-4 text-xs text-neutral-500 dark:text-neutral-400">
        <span className="flex items-center gap-1">
          <span className="inline-block w-2.5 h-2.5 rounded-full bg-green-500" />
          {LABELS.zoneNormal} (&lt;100)
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block w-2.5 h-2.5 rounded-full bg-amber-500" />
          {LABELS.zoneElevated} (100–125)
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block w-2.5 h-2.5 rounded-full bg-red-500" />
          {LABELS.zoneHigh} (&ge;125)
        </span>
      </div>
    </div>
  )
}
