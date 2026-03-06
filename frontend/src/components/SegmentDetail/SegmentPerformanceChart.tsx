import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import * as echarts from 'echarts'

interface SegmentPerformanceChartProps {
  segmentId: number
}

function formatTime(s: number): string {
  const m = Math.floor(s / 60)
  const sec = Math.round(s % 60)
  return `${m}:${String(sec).padStart(2, '0')}`
}

export function SegmentPerformanceChart({ segmentId }: SegmentPerformanceChartProps) {
  const navigate = useNavigate()
  const chartRef = useRef<HTMLDivElement>(null)
  const chartInstance = useRef<echarts.ECharts | null>(null)

  const { data } = useQuery({
    queryKey: ['segment-performances', segmentId],
    queryFn: () =>
      fetch(`/api/v1/segments/${segmentId}/performances`).then(r => r.json()),
  })

  const performances: any[] = data?.performances ?? []

  const sorted = useMemo(
    () =>
      [...performances].sort(
        (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime()
      ),
    [performances]
  )

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

  useEffect(() => {
    if (!chartRef.current || sorted.length === 0) return

    if (!chartInstance.current) {
      chartInstance.current = echarts.init(chartRef.current, null, { renderer: 'canvas' })
    }

    const option: echarts.EChartsOption = {
      tooltip: {
        trigger: 'item',
        formatter: (params: any) => {
          if (params.seriesIndex !== 0) return ''
          const perf = sorted[params.dataIndex]
          return `${new Date(perf.started_at).toLocaleDateString('de-DE')}<br/><b>${formatTime(perf.elapsed_time_s)}</b><br/>${perf.title}`
        },
      },
      grid: { left: 60, right: 20, top: 20, bottom: 50 },
      xAxis: {
        type: 'category',
        data: sorted.map((p: any) => new Date(p.started_at).toLocaleDateString('de-DE')),
        axisLabel: { rotate: 30, fontSize: 10 },
      },
      yAxis: {
        type: 'value',
        name: 'Zeit',
        axisLabel: { formatter: (v: number) => formatTime(v) },
      },
      series: [
        {
          type: 'scatter',
          data: sorted.map((p: any) => p.elapsed_time_s),
          symbolSize: 10,
          itemStyle: {
            color: (params: any) =>
              sorted[params.dataIndex].elapsed_time_s === bestTime ? '#FFD700' : '#888',
          },
        },
        {
          type: 'line',
          data: sorted.map(() => Math.round(avgTime)),
          lineStyle: { type: 'dashed', color: '#aaa', width: 1 },
          symbol: 'none',
          tooltip: { show: false },
        },
      ],
    }

    chartInstance.current.setOption(option)

    chartInstance.current.off('click')
    chartInstance.current.on('click', (params: any) => {
      if (params.seriesIndex === 0) {
        const perf = sorted[params.dataIndex]
        navigate(`/activities/${perf.activity_id}`)
      }
    })

    const handleResize = () => chartInstance.current?.resize()
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [sorted, bestTime, avgTime, navigate])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      chartInstance.current?.dispose()
      chartInstance.current = null
    }
  }, [])

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
            <b className="text-yellow-600 dark:text-yellow-400">{formatTime(bestTime)}</b>
          </span>
          <span>&#216; {formatTime(Math.round(avgTime))}</span>
        </div>
      </div>
      <div ref={chartRef} style={{ height: '320px', width: '100%' }} />
    </div>
  )
}
