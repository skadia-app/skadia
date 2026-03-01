import { useState, useRef, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchPMC } from '@/api/stats'
import type { PMCDataPoint } from '@/api/stats'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, CanvasRenderer])

// Mock data for development when API is not ready
const mockPMC: PMCDataPoint[] = Array.from({ length: 90 }, (_, i) => ({
  date: new Date(Date.now() - (89 - i) * 86400000).toISOString().split('T')[0],
  ctl: 40 + Math.sin(i / 10) * 15,
  atl: 35 + Math.sin(i / 5) * 20,
  tsb: 5 + Math.cos(i / 8) * 10,
  tss: Math.max(0, 60 + Math.random() * 80 - 40),
}))

type TimeRange = '3m' | '6m' | '1y' | 'all'

const rangeLabels: Record<TimeRange, string> = {
  '3m': '3M',
  '6m': '6M',
  '1y': '1J',
  'all': 'Alles',
}

function getDateRange(range: TimeRange): { from: string; to: string } {
  const to = new Date()
  const from = new Date()
  switch (range) {
    case '3m': from.setMonth(from.getMonth() - 3); break
    case '6m': from.setMonth(from.getMonth() - 6); break
    case '1y': from.setFullYear(from.getFullYear() - 1); break
    case 'all': from.setFullYear(from.getFullYear() - 5); break
  }
  return {
    from: from.toISOString().split('T')[0],
    to: to.toISOString().split('T')[0],
  }
}

export default function PMCChart() {
  const [range, setRange] = useState<TimeRange>('3m')
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)

  const { from, to } = useMemo(() => getDateRange(range), [range])

  const { data: pmcData, isError } = useQuery({
    queryKey: ['pmc', from, to],
    queryFn: () => fetchPMC(from, to),
    retry: 1,
  })

  // Use mock data if API fails or returns empty
  const data = (pmcData && pmcData.length > 0) ? pmcData : (isError ? mockPMC : pmcData ?? [])

  useEffect(() => {
    if (!containerRef.current || data.length === 0) return

    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chartRef.current = chart

    const dates = data.map(d => d.date)
    const ctl = data.map(d => d.ctl)
    const atl = data.map(d => d.atl)
    const tsb = data.map(d => d.tsb)

    const option: echarts.EChartsOption = {
      grid: {
        left: 50,
        right: 16,
        top: 40,
        bottom: 60,
      },
      legend: {
        data: ['CTL (Fitness)', 'ATL (Ermüdung)', 'TSB (Form)'],
        top: 8,
        textStyle: { fontSize: 11, color: '#999' },
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        formatter: (params: unknown) => {
          const items = params as Array<{ seriesName: string; value: number; axisValueLabel: string; color: string }>
          if (!Array.isArray(items) || items.length === 0) return ''
          let html = `<div style="font-size:12px;margin-bottom:4px">${items[0].axisValueLabel}</div>`
          for (const p of items) {
            html += `<div style="display:flex;align-items:center;gap:6px"><span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>${p.seriesName}: <strong>${p.value.toFixed(1)}</strong></div>`
          }
          return html
        },
      },
      xAxis: {
        type: 'category',
        data: dates,
        axisLabel: { fontSize: 10, color: '#999', rotate: 0 },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value',
        axisLabel: { fontSize: 11, color: '#999' },
        splitLine: { lineStyle: { color: '#e5e5e5', type: 'dashed' } },
      },
      series: [
        {
          name: 'TSB (Form)',
          type: 'bar',
          data: tsb.map(v => ({
            value: v,
            itemStyle: { color: v >= 0 ? '#22c55e80' : '#ef444480' },
          })),
          barWidth: '60%',
          z: 0,
        },
        {
          name: 'CTL (Fitness)',
          type: 'line',
          data: ctl,
          showSymbol: false,
          lineStyle: { width: 2, color: '#3b82f6' },
          itemStyle: { color: '#3b82f6' },
          z: 2,
        },
        {
          name: 'ATL (Ermüdung)',
          type: 'line',
          data: atl,
          showSymbol: false,
          lineStyle: { width: 2, color: '#f97316' },
          itemStyle: { color: '#f97316' },
          z: 2,
        },
      ],
      dataZoom: [
        {
          type: 'inside',
          xAxisIndex: 0,
          filterMode: 'none',
        },
      ],
    }

    chart.setOption(option)

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [data])

  // Current values (last data point)
  const latest = data.length > 0 ? data[data.length - 1] : null

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Performance Management</h2>
        <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {(Object.keys(rangeLabels) as TimeRange[]).map(r => (
            <button
              key={r}
              onClick={() => setRange(r)}
              className={
                r === range
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
              }
            >
              {rangeLabels[r]}
            </button>
          ))}
        </div>
      </div>

      {/* Current values */}
      {latest && (
        <div className="grid grid-cols-3 gap-3">
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">CTL (Fitness)</div>
            <div className="text-xl font-semibold text-blue-500 tabular-nums">{latest.ctl.toFixed(1)}</div>
          </div>
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">ATL (Ermüdung)</div>
            <div className="text-xl font-semibold text-orange-500 tabular-nums">{latest.atl.toFixed(1)}</div>
          </div>
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">TSB (Form)</div>
            <div className={`text-xl font-semibold tabular-nums ${latest.tsb >= 0 ? 'text-emerald-500' : 'text-rose-500'}`}>
              {latest.tsb >= 0 ? '+' : ''}{latest.tsb.toFixed(1)}
            </div>
          </div>
        </div>
      )}

      {/* Chart */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        {data.length > 0 ? (
          <div ref={containerRef} className="h-[300px] w-full" />
        ) : (
          <div className="flex items-center justify-center h-[300px] text-sm text-neutral-400">
            Lade PMC-Daten...
          </div>
        )}
      </div>

      {isError && (
        <p className="text-xs text-neutral-400 text-center">
          API nicht erreichbar - Mockdaten werden angezeigt
        </p>
      )}
    </div>
  )
}
