import { useState, useRef, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import type { EChartsCoreOption } from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  BrushComponent,
  ToolboxComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchPMC } from '@/api/stats'
import { toLocalISO } from '@/lib/dateUtils'
import type { PMCDataPoint } from '@/api/stats'
import { useECharts } from '@/hooks/useECharts'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, BrushComponent, ToolboxComponent, CanvasRenderer])

type TimeRange = '1m' | '3m' | '6m' | '1y' | 'ly' | 'all'

const rangeLabels: Record<TimeRange, string> = {
  '1m': '1M',
  '3m': '3M',
  '6m': '6M',
  '1y': '1J',
  'ly': 'VJ',
  'all': 'Alles',
}

const rangeTooltips: Record<TimeRange, string> = {
  '1m': 'Letzter Monat',
  '3m': 'Letzte 3 Monate',
  '6m': 'Letzte 6 Monate',
  '1y': 'Letztes Jahr (365 Tage)',
  'ly': `Vorjahr (${new Date().getFullYear() - 1})`,
  'all': 'Gesamter Zeitraum',
}

function getDateRange(range: TimeRange): { from: string; to: string } {
  if (range === 'ly') {
    const year = new Date().getFullYear() - 1
    return { from: `${year}-01-01`, to: `${year}-12-31` }
  }
  const to = new Date()
  const from = new Date()
  switch (range) {
    case '1m': from.setMonth(from.getMonth() - 1); break
    case '3m': from.setMonth(from.getMonth() - 3); break
    case '6m': from.setMonth(from.getMonth() - 6); break
    case '1y': from.setFullYear(from.getFullYear() - 1); break
    case 'all': return { from: '0001-01-01', to: toLocalISO(to) }
  }
  return {
    from: toLocalISO(from),
    to: toLocalISO(to),
  }
}

export default function PMCChart() {
  const [range, setRange] = useState<TimeRange | null>('3m')
  const today = toLocalISO(new Date())
  const threeMonthsAgo = (() => { const d = new Date(); d.setMonth(d.getMonth() - 3); return toLocalISO(d) })()
  const [dateFrom, setDateFrom] = useState(threeMonthsAgo)
  const [dateTo, setDateTo] = useState(today)
  const [isZoomed, setIsZoomed] = useState(false)
  const chartInstanceRef = useRef<echarts.ECharts | null>(null)

  // When a preset range is selected, update the date fields to match
  const handleRangeClick = (r: TimeRange) => {
    setRange(r)
    const { from, to } = getDateRange(r)
    setDateFrom(from)
    setDateTo(to)
  }

  // When a date field changes, deselect any active preset
  const handleDateFromChange = (value: string) => {
    setDateFrom(value)
    setRange(null)
  }
  const handleDateToChange = (value: string) => {
    setDateTo(value)
    setRange(null)
  }

  // Compute from/to: if a preset is active use it, otherwise use date fields
  const { from, to } = useMemo(() => {
    if (range !== null) {
      return getDateRange(range)
    }
    return { from: dateFrom, to: dateTo }
  }, [range, dateFrom, dateTo])

  const { data: pmcData, isError, isLoading } = useQuery({
    queryKey: ['pmc', from, to],
    queryFn: () => fetchPMC(from, to),
    retry: 1,
  })

  const data: PMCDataPoint[] = pmcData ?? []

  const containerRef = useECharts(
    () => {
      if (data.length === 0) return {}

      const dates = data.map(d => d.date)
      const ctl = data.map(d => d.ctl)
      const atl = data.map(d => d.atl)
      const tsb = data.map(d => d.tsb)

      return {
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
        brush: {
          toolbox: [],
          brushType: 'lineX',
          brushStyle: {
            borderWidth: 1,
            color: 'rgba(249,115,22,0.12)',
            borderColor: '#f97316',
          },
          throttleType: 'debounce',
          throttleDelay: 100,
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
      } as EChartsCoreOption
    },
    [data],
    (chart) => {
      chartInstanceRef.current = chart

      // Activate brush cursor so click-drag draws a range selection
      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: 'lineX', brushMode: 'single' },
      })

      // Brush areas type
      type BrushArea = { range?: [number, number]; coordRange?: [number, number] }
      // Re-entry guard: dataZoom dispatch triggers cascading brushEnd events
      let brushZoomLock = false

      const dates = data.map(d => d.date)

      chart.on('brushEnd', (params: unknown) => {
        if (brushZoomLock) return
        brushZoomLock = true

        const p = params as {
          areas?: BrushArea[]
          batch?: Array<{ areas?: BrushArea[] }>
        }
        const areas = p.areas ?? p.batch?.[0]?.areas
        if (!areas || areas.length === 0) { brushZoomLock = false; return }

        const area = areas[0]
        let startIdx: number
        let endIdx: number

        if (area.coordRange) {
          [startIdx, endIdx] = area.coordRange
        } else if (area.range) {
          const s = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
          const e = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
          if (s == null || e == null) { brushZoomLock = false; return }
          startIdx = Math.round(s as number)
          endIdx = Math.round(e as number)
        } else {
          brushZoomLock = false; return
        }

        if (endIdx <= startIdx) { brushZoomLock = false; return }
        startIdx = Math.max(0, startIdx)
        endIdx = Math.min(dates.length - 1, endIdx)

        chart.dispatchAction({
          type: 'dataZoom',
          dataZoomIndex: 0,
          startValue: startIdx,
          endValue: endIdx,
        })

        chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
        chart.dispatchAction({
          type: 'takeGlobalCursor',
          key: 'brush',
          brushOption: { brushType: false },
        })

        setIsZoomed(true)
        requestAnimationFrame(() => { brushZoomLock = false })
      })

      return () => { chartInstanceRef.current = null }
    },
  )

  // Reset zoom: restore full view, reactivate brush
  const handleResetZoom = () => {
    const chart = chartInstanceRef.current
    if (!chart) return

    chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
    // Reactivate brush so the user can draw a new selection
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })
    setIsZoomed(false)
  }

  const latest = data.length > 0 ? data[data.length - 1] : null

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Performance Management</h2>
        <div className="flex items-center gap-2">
          {isZoomed && (
            <button onClick={handleResetZoom}
              className="cursor-pointer text-sm px-3 py-1 rounded-md bg-accent-100 text-orange-700 hover:bg-orange-200
                         dark:bg-orange-900/30 dark:text-orange-300 dark:hover:bg-orange-900/50 transition-colors">
              Zoom zurücksetzen
            </button>
          )}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            {(Object.keys(rangeLabels) as TimeRange[]).map(r => (
              <button
                key={r}
                onClick={() => handleRangeClick(r)}
                title={rangeTooltips[r]}
                className={
                  r === range
                    ? 'cursor-pointer px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                    : 'cursor-pointer px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
                }
              >
                {rangeLabels[r]}
              </button>
            ))}
          </div>
          <input
            type="date"
            value={dateFrom}
            max={dateTo}
            onChange={e => handleDateFromChange(e.target.value)}
            className="border border-neutral-200 dark:border-neutral-700 rounded px-2 py-1 text-xs
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
          <span className="text-neutral-400 text-xs">bis</span>
          <input
            type="date"
            value={dateTo}
            min={dateFrom}
            max={today}
            onChange={e => handleDateToChange(e.target.value)}
            className="border border-neutral-200 dark:border-neutral-700 rounded px-2 py-1 text-xs
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
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
            <div className="text-xl font-semibold text-accent-DEFAULT tabular-nums">{latest.atl.toFixed(1)}</div>
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
        {isLoading ? (
          <div className="flex items-center justify-center h-[300px] text-sm text-neutral-400">
            Lade PMC-Daten…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[300px] text-sm text-neutral-400">
            Fehler beim Laden der PMC-Daten
          </div>
        ) : data.length === 0 ? (
          <div className="flex items-center justify-center h-[300px] text-sm text-neutral-400">
            Keine Daten für diesen Zeitraum
          </div>
        ) : (
          <div ref={containerRef} className="h-[300px] w-full" />
        )}
      </div>
    </div>
  )
}
