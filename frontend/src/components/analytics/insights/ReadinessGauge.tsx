import { useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { GaugeChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchInsightsReadiness } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'

echarts.use([GaugeChart, TooltipComponent, CanvasRenderer])

function statusColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'gut':
    case 'good':
    case 'hoch':
    case 'high':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'moderat':
    case 'moderate':
    case 'mittel':
    case 'medium':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default:
      return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
  }
}

function levelLabel(level: string): string {
  const labels: Record<string, string> = {
    high: 'Hoch', good: 'Gut', optimal: 'Optimal',
    moderate: 'Moderat', medium: 'Mittel',
    low: 'Niedrig', poor: 'Schlecht', critical: 'Kritisch',
  }
  return labels[level.toLowerCase()] ?? level
}

export default function ReadinessGauge() {
  const { toQueryString } = useAnalyticsFilters()
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  const queryParams = toQueryString()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'insights', 'readiness', queryParams],
    queryFn: async () => {
      try {
        return await fetchInsightsReadiness(queryParams)
      } catch {
        return null
      }
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  useEffect(() => {
    if (!containerRef.current) return
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart
    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)
    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    if (!chartRef.current || !data) return
    const option: EChartsCoreOption = {
      backgroundColor: 'transparent',
      series: [
        {
          type: 'gauge',
          startAngle: 200,
          endAngle: -20,
          min: 0,
          max: 100,
          splitNumber: 4,
          axisLine: {
            lineStyle: {
              width: 20,
              color: [
                [0.4, '#ef4444'],
                [0.7, '#f59e0b'],
                [1, '#10b981'],
              ],
            },
          },
          pointer: {
            itemStyle: { color: 'auto' },
            length: '55%',
            width: 4,
          },
          axisTick: { show: false },
          splitLine: { show: false },
          axisLabel: {
            fontSize: 10,
            color: '#9ca3af',
            distance: -30,
          },
          detail: {
            valueAnimation: true,
            fontSize: 28,
            fontWeight: 'bold',
            color: 'auto',
            formatter: '{value}',
            offsetCenter: [0, '30%'],
          },
          title: {
            fontSize: 12,
            color: '#6b7280',
            offsetCenter: [0, '52%'],
          },
          data: [{ value: data.score, name: levelLabel(data.level) }],
        },
      ],
    }
    chartRef.current.setOption(option, { notMerge: true })
  }, [data])

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 animate-pulse">
        <div className="h-4 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
        <div className="w-full h-[200px] bg-neutral-200 dark:bg-neutral-700 rounded" />
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200 mb-3">Trainingsbereitschaft</h3>
        <div className="flex flex-col items-center justify-center h-[200px] text-sm text-neutral-400 gap-2">
          <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
              d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span>Dein Readiness-Score wird nach der nächsten Aktivität berechnet.</span>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200">Trainingsbereitschaft</h3>

      {/* Gauge */}
      <div ref={containerRef} className="w-full h-[200px]" />

      {/* Factors */}
      {data.factors && data.factors.length > 0 && (
        <div className="space-y-2">
          {data.factors.map((factor, i) => (
            <div key={i} className="flex items-center justify-between gap-2 text-xs">
              <span className="text-neutral-600 dark:text-neutral-300 truncate">{factor.name}</span>
              <div className="flex items-center gap-2">
                <span className="tabular-nums text-neutral-700 dark:text-neutral-200 font-medium">
                  {typeof factor.value === 'number' ? factor.value.toFixed(0) : factor.value}
                </span>
                <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${statusColor(factor.status)}`}>
                  {factor.status}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* LLM Explanation */}
      {data.explanation && (
        <blockquote className="border-l-2 border-accent pl-3 italic text-xs text-neutral-500 dark:text-neutral-400">
          <svg className="inline w-3 h-3 mr-1 opacity-50" fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M18 10c0 3.866-3.582 7-8 7a8.841 8.841 0 01-4.083-.98L2 17l1.338-3.123C2.493 12.767 2 11.434 2 10c0-3.866 3.582-7 8-7s8 3.134 8 7zM7 9H5v2h2V9zm8 0h-2v2h2V9zM9 9h2v2H9V9z" clipRule="evenodd" />
          </svg>
          {data.explanation}
        </blockquote>
      )}
    </div>
  )
}
