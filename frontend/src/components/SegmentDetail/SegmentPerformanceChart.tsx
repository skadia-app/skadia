import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { ScatterChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { apiClient } from '@/api/client'

echarts.use([ScatterChart, LineChart, GridComponent, TooltipComponent, CanvasRenderer])
import { fmtDuration } from '@/lib/format'
import { useECharts } from '@/hooks/useECharts'

interface SegmentPerformanceChartProps {
  segmentId: string
  dateFrom?: string
  dateTo?: string
}


export function SegmentPerformanceChart({ segmentId, dateFrom, dateTo }: SegmentPerformanceChartProps) {
  const navigate = useNavigate()

  const { data } = useQuery({
    queryKey: ['segment-performances', segmentId],
    queryFn: () =>
      apiClient<{ performances: any[] }>(`/segments/${segmentId}/performances`),
  })

  const performances: any[] = data?.performances ?? []

  const sorted = useMemo(() => {
    const fromMs = dateFrom ? new Date(dateFrom).setHours(0, 0, 0, 0) : null
    const toMs = dateTo ? new Date(dateTo).setHours(23, 59, 59, 999) : null
    return [...performances]
      .filter(p => {
        const t = new Date(p.started_at).getTime()
        if (fromMs !== null && t < fromMs) return false
        if (toMs !== null && t > toMs) return false
        return true
      })
      .sort((a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime())
  }, [performances, dateFrom, dateTo])

  const trend = useMemo(() => {
    if (sorted.length < 3) return null
    const recent = sorted.slice(-Math.min(5, Math.ceil(sorted.length / 2)))
    const older = sorted.slice(0, sorted.length - recent.length)
    if (older.length === 0) return null
    const avgRecent = recent.reduce((s, p) => s + p.elapsed_time_s, 0) / recent.length
    const avgOlder = older.reduce((s, p) => s + p.elapsed_time_s, 0) / older.length
    return avgRecent < avgOlder ? 'improving' : 'declining'
  }, [sorted])

  const bestTime =
    sorted.length > 0 ? Math.min(...sorted.map((p: any) => p.elapsed_time_s)) : 0
  const avgTime =
    sorted.length > 0
      ? sorted.reduce((s, p) => s + p.elapsed_time_s, 0) / sorted.length
      : 0

  const containerRef = useECharts(
    () => {
      if (sorted.length === 0) return {}

      const timeData = sorted.map((p: any) => {
        const isBest = p.elapsed_time_s === bestTime
        return {
          value: [new Date(p.started_at).getTime(), p.elapsed_time_s],
          symbolSize: isBest ? 12 : 10,
          itemStyle: { color: isBest ? '#FFD700' : '#888', borderWidth: 0 },
          emphasis: {
            symbolSize: isBest ? 15 : 13,
            itemStyle: {
              color: isBest ? '#FFD700' : '#ef4444',
              borderWidth: isBest ? 1.5 : 0,
              borderColor: isBest ? '#d97706' : 'transparent',
            },
          },
        }
      })
      const minTime = timeData.length > 0 ? Math.min(...timeData.map(d => (d.value as number[])[0])) : 0
      const maxTime = timeData.length > 0 ? Math.max(...timeData.map(d => (d.value as number[])[0])) : 0

      return {
        tooltip: {
          trigger: 'item',
          formatter: (params: any) => {
            if (params.seriesIndex !== 0) return ''
            const [ts, time] = params.data.value ?? params.data
            const perf = sorted.find((p: any) => new Date(p.started_at).getTime() === ts)
            const dateStr = new Date(ts).toLocaleDateString('de-DE')
            return `${dateStr}<br/><b>${fmtDuration(time)}</b>${perf ? '<br/>' + perf.title : ''}`
          },
        },
        grid: { left: 60, right: 20, top: 20, bottom: 50 },
        xAxis: {
          type: 'time',
          min: minTime,
          max: maxTime,
          axisLabel: {
            formatter: (v: number) => new Date(v).toLocaleDateString('de-DE', { month: 'short', year: '2-digit' }),
            fontSize: 10,
            rotate: 30,
          },
        },
        yAxis: {
          type: 'value',
          name: 'Zeit',
          axisLabel: { formatter: (v: number) => fmtDuration(v) },
        },
        series: [
          {
            type: 'scatter',
            data: timeData,
            symbolSize: 10,
            emphasis: { scale: false },
          },
          {
            type: 'line',
            data: [[minTime, Math.round(avgTime)], [maxTime, Math.round(avgTime)]],
            lineStyle: {
              type: 'dashed',
              color: getComputedStyle(document.documentElement)
                .getPropertyValue('--color-accent-DEFAULT')
                .trim() || '#f97316',
              width: 1.5,
            },
            symbol: 'none',
            tooltip: { show: false },
          },
        ],
      } as EChartsCoreOption
    },
    [sorted, bestTime, avgTime],
    (chart) => {
      const handler = (params: any) => {
        if (params.seriesIndex === 0) {
          const [ts] = params.data.value ?? params.data
          const perf = sorted.find((p: any) => new Date(p.started_at).getTime() === ts)
          if (perf) navigate(`/activities/${perf.activity_id}`)
        }
      }
      chart.on('click', handler)
      return () => chart.off('click', handler)
    },
  )

  if (performances.length === 0) return null

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-lg p-4">
      <div className="flex justify-between items-center mb-4">
        <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
          Leistungsverlauf
        </h3>
        <div className="flex items-center gap-4 text-sm text-neutral-500 dark:text-neutral-400">
          {trend === 'improving' && (
            <span className="text-green-600 dark:text-green-400">&#8599; Verbesserung</span>
          )}
          {trend === 'declining' && (
            <span className="text-orange-500">&#8600; Verschlechterung</span>
          )}
          <span>
            Beste:{' '}
            <b className="text-yellow-600 dark:text-yellow-400">{fmtDuration(bestTime)}</b>
          </span>
          <span>&#216; {fmtDuration(Math.round(avgTime))}</span>
        </div>
      </div>
      <div ref={containerRef} style={{ height: '320px', width: '100%' }} />
    </div>
  )
}
