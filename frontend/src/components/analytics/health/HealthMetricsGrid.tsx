import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchHealthMetrics } from '@/api/analytics'
import type { HealthMetric } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { useECharts } from '@/hooks/useECharts'

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

interface MiniChartProps {
  points: { date: string; value: number }[]
  color: string
}

function MiniChart({ points, color }: MiniChartProps) {
  const containerRef = useECharts(
    () => ({
      backgroundColor: 'transparent',
      grid: { left: 0, right: 0, top: 2, bottom: 2 },
      xAxis: { type: 'category', show: false, data: points.map(p => p.date) },
      yAxis: { type: 'value', show: false, scale: true },
      tooltip: { show: false },
      series: [
        {
          type: 'line',
          data: points.map(p => p.value),
          showSymbol: false,
          lineStyle: { width: 1.5, color },
          itemStyle: { color },
          areaStyle: { color: `${color}22` },
          smooth: 0.3,
        },
      ],
    }),
    [points, color],
  )

  if (points.length === 0) return null

  return <div ref={containerRef} className="h-[80px] w-full" />
}

function trendIndicator(trend: number): { symbol: string; color: string } {
  if (trend > 0.05) return { symbol: '↑', color: 'text-emerald-500' }
  if (trend < -0.05) return { symbol: '↓', color: 'text-rose-500' }
  return { symbol: '→', color: 'text-neutral-400' }
}

function metricColor(name: string): string {
  const colors: Record<string, string> = {
    resting_hr: '#ef4444',
    spo2: '#3b82f6',
    stress: '#f97316',
    body_battery: '#22c55e',
    steps: '#8b5cf6',
    hydration: '#06b6d4',
  }
  for (const [key, color] of Object.entries(colors)) {
    if (name.toLowerCase().includes(key)) return color
  }
  return '#6366f1'
}

interface MetricCardProps {
  metric: HealthMetric
}

function MetricCard({ metric }: MetricCardProps) {
  const [expanded, setExpanded] = useState(false)
  const trend = trendIndicator(metric.trend)
  const color = metricColor(metric.name)

  return (
    <button
      onClick={() => setExpanded(e => !e)}
      className="w-full text-left rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 hover:border-neutral-300 dark:hover:border-neutral-700 transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-neutral-400"
    >
      <div className="flex items-start justify-between mb-1">
        <div className="text-xs text-neutral-500 dark:text-neutral-400 font-medium truncate pr-2">
          {metric.name}
        </div>
        <span className={`text-xs font-bold flex-shrink-0 ${trend.color}`}>
          {trend.symbol}
        </span>
      </div>
      <div className="flex items-baseline gap-1 mb-2">
        <span className="text-xl font-bold tabular-nums text-neutral-900 dark:text-neutral-100">
          {typeof metric.current === 'number' ? metric.current.toFixed(metric.current < 10 ? 1 : 0) : metric.current}
        </span>
        <span className="text-xs text-neutral-400">{metric.unit}</span>
      </div>

      {/* Mini chart — always visible */}
      {metric.points.length > 0 && (
        <div className="h-[80px]">
          <MiniChart points={metric.points} color={color} />
        </div>
      )}

      {/* Expanded: larger chart area */}
      {expanded && metric.points.length > 0 && (
        <div className="mt-2 h-[120px] border-t border-neutral-100 dark:border-neutral-800 pt-2">
          <MiniChart points={metric.points} color={color} />
        </div>
      )}
    </button>
  )
}

export default function HealthMetricsGrid() {
  const { toQueryString } = useAnalyticsFilters()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'health-metrics', toQueryString()],
    queryFn: () => fetchHealthMetrics(toQueryString()),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 animate-pulse">
            <div className="h-3 w-2/3 bg-neutral-200 dark:bg-neutral-700 rounded mb-2" />
            <div className="h-6 w-1/2 bg-neutral-200 dark:bg-neutral-700 rounded mb-2" />
            <div className="h-[80px] bg-neutral-100 dark:bg-neutral-800 rounded" />
          </div>
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="text-sm text-neutral-400 text-center py-4">
        Fehler beim Laden der Gesundheitsmetriken
      </div>
    )
  }

  if (!data || data.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-8 gap-2 text-neutral-400">
        <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
            d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
        </svg>
        <span className="text-sm">Keine Gesundheitsmetriken verfügbar</span>
      </div>
    )
  }

  return (
    <div>
      <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100 mb-3">
        Gesundheitsmetriken
      </h2>
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
        {data.map(metric => (
          <MetricCard key={metric.name} metric={metric} />
        ))}
      </div>
    </div>
  )
}
