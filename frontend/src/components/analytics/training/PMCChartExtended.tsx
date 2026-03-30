import { useState, useMemo, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  MarkAreaComponent,
  MarkLineComponent,
  ToolboxComponent,
  BrushComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchAnalyticsPMC } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { toLocalISO } from '@/lib/dateUtils'
import { useChangepoints, changepointsToMarkLines } from '@/components/analytics/ChangepointMarkers'
import { useECharts } from '@/hooks/useECharts'

echarts.use([
  LineChart,
  BarChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  MarkAreaComponent,
  MarkLineComponent,
  ToolboxComponent,
  BrushComponent,
  CanvasRenderer,
])

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

function acwrStatusColor(status: string): string {
  switch (status) {
    case 'optimal':   return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'reduced':   return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'overreach': return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
    case 'danger':    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default:          return 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300'
  }
}

function acwrStatusLabel(status: string): string {
  switch (status) {
    case 'optimal':   return 'Optimal'
    case 'reduced':   return 'Reduziert'
    case 'overreach': return 'Erhöhtes Risiko'
    case 'danger':    return 'Spike'
    default:          return '—'
  }
}

export default function PMCChartExtended() {
  const { toQueryString } = useAnalyticsFilters()
  const [range, setRange] = useState<TimeRange>('3m')
  const changepoints = useChangepoints('ctl', 180)
  const [localFrom, setLocalFrom] = useState<string>('')
  const [localTo, setLocalTo] = useState<string>('')
  const [isCustomRange, setIsCustomRange] = useState(false)
  const [isZoomed, setIsZoomed] = useState(false)
  const chartInstanceRef = useRef<echarts.ECharts | null>(null)
  const today = toLocalISO(new Date())

  const { from, to } = useMemo(() => {
    if (isCustomRange && localFrom && localTo) {
      return { from: localFrom, to: localTo }
    }
    return getDateRange(range)
  }, [range, isCustomRange, localFrom, localTo])

  const handleRangeClick = (r: TimeRange) => {
    setRange(r)
    setIsCustomRange(false)
  }

  const handleDateFromChange = (val: string) => {
    setLocalFrom(val)
    setIsCustomRange(true)
  }

  const handleDateToChange = (val: string) => {
    setLocalTo(val)
    setIsCustomRange(true)
  }

  // Build params combining global filters + local date range
  const queryParams = useMemo(() => {
    const base = toQueryString()
    const params = new URLSearchParams(base)
    params.set('from', from)
    params.set('to', to)
    return params.toString()
  }, [toQueryString, from, to])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'pmc-extended', queryParams],
    queryFn: () => fetchAnalyticsPMC(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const handleResetZoom = () => {
    const chart = chartInstanceRef.current
    if (!chart) return
    chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
    chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 1, start: 0, end: 100 })
    chart.dispatchAction({
      type: 'takeGlobalCursor',
      key: 'brush',
      brushOption: { brushType: 'lineX', brushMode: 'single' },
    })
    setIsZoomed(false)
  }

  const containerRef = useECharts(
    () => {
      if (!data) return { series: [] }

      const days = data.days ?? []
      const projection = data.projection ?? []

      const dates = days.map(d => d.date)
      const ctlData = days.map(d => d.ctl)
      const atlData = days.map(d => d.atl)
      const tsbData = days.map(d => d.tsb)

      const projDates = projection.map(d => d.date)
      const projCTL = projection.map(d => d.ctl)
      const projATL = projection.map(d => d.atl)
      const projTSB = projection.map(d => d.tsb)

      const allDates = [...dates, ...projDates]

      const ctlFull = [...ctlData, ...Array(projDates.length).fill(null)]
      const atlFull = [...atlData, ...Array(projDates.length).fill(null)]
      const tsbFull = [...tsbData, ...Array(projDates.length).fill(null)]
      const ctlProjFull = [...Array(dates.length).fill(null), ...projCTL]
      const atlProjFull = [...Array(dates.length).fill(null), ...projATL]
      const tsbProjFull = [...Array(dates.length).fill(null), ...projTSB]

      return {
        backgroundColor: 'transparent',
        grid: { left: 52, right: 20, top: 40, bottom: 80 },
        brush: {
          toolbox: [],
          brushType: 'lineX',
          brushStyle: {
            borderWidth: 1,
            color: 'rgba(59,130,246,0.12)',
            borderColor: 'rgba(59,130,246,0.6)',
          },
          throttleType: 'debounce',
          throttleDelay: 100,
        },
        toolbox: {
          right: 8,
          top: 2,
          feature: {
            dataZoom: {
              yAxisIndex: 'none',
              title: { zoom: 'Bereich zoomen (ziehen)', back: 'Zoom zurücksetzen' },
              brushStyle: { borderWidth: 1, color: 'rgba(59,130,246,0.1)', borderColor: 'rgba(59,130,246,0.6)' },
            },
            restore: { title: 'Zurücksetzen' },
            brush: { show: false },
            saveAsImage: { title: 'Als PNG speichern', pixelRatio: 2 },
          },
          iconStyle: { borderColor: '#6b7280' },
          emphasis: { iconStyle: { borderColor: '#d1d5db' } },
        },
        legend: {
          data: ['CTL (Fitness)', 'ATL (Ermüdung)', 'TSB (Form)', 'CTL Prognose', 'ATL Prognose', 'TSB Prognose'],
          top: 6,
          right: 80,
          textStyle: { fontSize: 11, color: '#9ca3af' },
        },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'cross' },
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string }>
            if (!Array.isArray(items) || items.length === 0) return ''
            let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
            for (const p of items) {
              if (p.value == null) continue
              html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
                <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
                <span style="color:#d1d5db">${p.seriesName}:</span>
                <strong style="color:#f9fafb">${p.value.toFixed(1)}</strong>
              </div>`
            }
            return html
          },
        },
        xAxis: {
          type: 'category',
          data: allDates,
          axisLabel: { fontSize: 10, color: '#6b7280' },
          splitLine: { show: false },
          axisLine: { lineStyle: { color: '#374151' } },
        },
        yAxis: {
          type: 'value',
          axisLabel: { fontSize: 10, color: '#6b7280' },
          splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        },
        dataZoom: [
          { type: 'inside', xAxisIndex: 0, filterMode: 'none' },
          {
            type: 'slider',
            xAxisIndex: 0,
            bottom: 8,
            height: 24,
            borderColor: '#374151',
            fillerColor: 'rgba(59,130,246,0.1)',
            handleStyle: { color: '#3b82f6' },
            textStyle: { color: '#6b7280', fontSize: 10 },
          },
        ],
        series: [
          {
            name: 'TSB (Form)',
            type: 'bar',
            data: tsbFull.map(v => ({
              value: v,
              itemStyle: {
                color: v == null ? 'transparent' : v >= 15 ? 'rgba(59,130,246,0.35)' : v >= -10 ? 'rgba(34,197,94,0.35)' : v >= -30 ? 'rgba(234,179,8,0.35)' : 'rgba(239,68,68,0.35)',
              },
            })),
            barWidth: '80%',
            z: 0,
            markArea: {
              silent: true,
              data: [
                [{ yAxis: 15, itemStyle: { color: 'rgba(59,130,246,0.06)' } }, { yAxis: 999 }],
                [{ yAxis: -10, itemStyle: { color: 'rgba(34,197,94,0.06)' } }, { yAxis: 15 }],
                [{ yAxis: -30, itemStyle: { color: 'rgba(234,179,8,0.06)' } }, { yAxis: -10 }],
                [{ yAxis: -999, itemStyle: { color: 'rgba(239,68,68,0.06)' } }, { yAxis: -30 }],
              ],
            },
          },
          {
            name: 'CTL (Fitness)',
            type: 'line',
            data: ctlFull,
            showSymbol: false,
            lineStyle: { width: 2.5, color: '#3b82f6' },
            itemStyle: { color: '#3b82f6' },
            z: 2,
            connectNulls: false,
            markLine: changepoints.length > 0 ? { silent: true, data: changepointsToMarkLines(changepoints) } : undefined,
          },
          {
            name: 'ATL (Ermüdung)',
            type: 'line',
            data: atlFull,
            showSymbol: false,
            lineStyle: { width: 2, color: '#f97316' },
            itemStyle: { color: '#f97316' },
            z: 2,
            connectNulls: false,
          },
          {
            name: 'CTL Prognose',
            type: 'line',
            data: ctlProjFull,
            showSymbol: false,
            lineStyle: { width: 1.5, color: '#3b82f6', type: 'dashed', opacity: 0.5 },
            itemStyle: { color: '#3b82f6', opacity: 0.5 },
            z: 1,
            connectNulls: false,
          },
          // ATL Prognose
          {
            name: 'ATL Prognose',
            type: 'line',
            data: atlProjFull,
            showSymbol: false,
            lineStyle: { width: 1.5, color: '#ef4444', type: 'dashed', opacity: 0.5 },
            itemStyle: { color: '#ef4444', opacity: 0.5 },
            z: 1,
            connectNulls: false,
          },
          // TSB Prognose
          {
            name: 'TSB Prognose',
            type: 'bar',
            data: tsbProjFull,
            showSymbol: false,
            itemStyle: { color: 'rgba(107,114,128,0.2)' },
            z: 0,
            connectNulls: false,
          },
        ],
      }
    },
    [data],
    (chart) => {
      chartInstanceRef.current = chart

      chart.dispatchAction({
        type: 'takeGlobalCursor',
        key: 'brush',
        brushOption: { brushType: 'lineX', brushMode: 'single' },
      })

      type BrushArea = { range?: [number, number]; coordRange?: [number, number] }
      let brushZoomLock = false
      const allDates = [
        ...(data?.days ?? []).map(d => d.date),
        ...(data?.projection ?? []).map(d => d.date),
      ]

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
        endIdx = Math.min(allDates.length - 1, endIdx)

        chart.dispatchAction({
          type: 'dataZoom',
          dataZoomIndex: 0,
          startValue: startIdx,
          endValue: endIdx,
        })
        chart.dispatchAction({ type: 'dataZoom', dataZoomIndex: 1, startValue: startIdx, endValue: endIdx })
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

  const latest = data?.days && data.days.length > 0 ? data.days[data.days.length - 1] : null

  return (
    <div className="space-y-3">
      {/* Header row */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          Performance Management (PMC)
        </h2>
        <div className="flex items-center gap-2 flex-wrap">
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
                  r === range && !isCustomRange
                    ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                    : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'
                }
              >
                {rangeLabels[r]}
              </button>
            ))}
          </div>
          <input
            type="date"
            value={isCustomRange ? localFrom : from}
            max={isCustomRange ? localTo : to}
            onChange={e => handleDateFromChange(e.target.value)}
            className="border border-neutral-200 dark:border-neutral-700 rounded px-2 py-1 text-xs
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
          <span className="text-neutral-400 text-xs">–</span>
          <input
            type="date"
            value={isCustomRange ? localTo : to}
            min={isCustomRange ? localFrom : from}
            max={today}
            onChange={e => handleDateToChange(e.target.value)}
            className="border border-neutral-200 dark:border-neutral-700 rounded px-2 py-1 text-xs
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
        </div>
      </div>

      {/* Metrics row */}
      {(latest || data) && (() => {
        const lastDate = data?.days[data.days.length - 1]?.date
        const todayISO = new Date().toISOString().slice(0, 10)
        const isHistorical = lastDate && lastDate !== todayISO
        const dateSuffix = isHistorical ? ` (${new Date(lastDate).toLocaleDateString('de-DE')})` : ''
        return (
          <div className="flex items-center gap-2 flex-wrap">
            {/* CTL / ATL / TSB badges */}
            {latest && (
              <>
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                  <span className="w-2 h-2 rounded-full bg-blue-500 inline-block" />
                  CTL {latest.ctl.toFixed(1)}{dateSuffix}
                </span>
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-accent-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300">
                  <span className="w-2 h-2 rounded-full bg-accent-DEFAULT inline-block" />
                  ATL {latest.atl.toFixed(1)}{dateSuffix}
                </span>
                <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${latest.tsb >= 0 ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'}`}>
                  <span className={`w-2 h-2 rounded-full inline-block ${latest.tsb >= 0 ? 'bg-emerald-500' : 'bg-rose-500'}`} />
                  TSB {latest.tsb >= 0 ? '+' : ''}{latest.tsb.toFixed(1)}{dateSuffix}
                </span>
              </>
            )}
            {/* ACWR / RampRate / Monotony / Strain */}
            {data && (
              <>
                {data.acwr_status && (
                  <span
                    className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium ${acwrStatusColor(data.acwr_status)}`}
                    title={`${acwrStatusLabel(data.acwr_status)} — Rolling 7:28 uncoupled${data.acwr_status === 'reduced' ? '. Kann geplanten Taper, Erholung oder Detraining anzeigen.' : ''}`}
                  >
                    ACWR {data.acwr.toFixed(2)}{dateSuffix}
                  </span>
                )}
                <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
                  Rampe {data.ramp_rate >= 0 ? '+' : ''}{data.ramp_rate.toFixed(1)}/Wo{dateSuffix}
                </span>
                <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
                  Mono {data.monotony.toFixed(2)}{dateSuffix}
                </span>
                <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
                  Strain {data.strain.toFixed(0)}{dateSuffix}
                </span>
              </>
            )}
          </div>
        )
      })()}

      {/* Fallback warning banner */}
      {data?.using_default_thresholds && (
        <div className="mb-4 px-4 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 text-amber-800 dark:text-amber-200 text-sm">
          TSS-Berechnung nutzt geschätzte Schwellenwerte
          {data.default_threshold_fields?.length ? (
            <> für: <strong>{data.default_threshold_fields.map(f => f.toUpperCase()).join(', ')}</strong></>
          ) : null}.{' '}
          Für genauere Ergebnisse im Profil oder pro Sportart hinterlegen.
        </div>
      )}

      {/* Chart */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        {isLoading ? (
          <div className="flex items-center justify-center h-[340px] text-sm text-neutral-400">
            Lade PMC-Daten…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[340px] text-sm text-neutral-400">
            Fehler beim Laden der PMC-Daten
          </div>
        ) : !data || (data.days?.length ?? 0) === 0 ? (
          <div className="flex flex-col items-center justify-center h-[340px] gap-2 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
            </svg>
            <span className="text-sm">Keine Daten für diesen Zeitraum</span>
          </div>
        ) : (
          <div ref={containerRef} className="h-[340px] w-full" />
        )}
      </div>

      {/* TSB zone legend */}
      <div className="flex items-center gap-3 flex-wrap text-xs text-neutral-500 dark:text-neutral-400">
        <span className="flex items-center gap-1.5" title="Hoher TSB kann Erholung ODER Formverlust durch Trainingsunterbrechung bedeuten.">
          <span className="w-3 h-3 rounded-sm inline-block bg-blue-400/30" />
          Frisch / Detrained (&gt;+15)
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm inline-block bg-emerald-400/30" />
          Optimal (−10…+15)
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm inline-block bg-yellow-400/30" />
          Overreach (−30…−10)
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm inline-block bg-red-400/30" />
          Überlastung (&lt;−30)
        </span>
      </div>
    </div>
  )
}
