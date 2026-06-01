import { useState, useMemo, useRef, useEffect } from 'react'
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
import { useTranslation } from 'react-i18next'
import { fetchAnalyticsPMC, fetchAnalyticsPMCSplit, type PMCDay } from '@/api/analytics'
import { toLocalISO } from '@/lib/dateUtils'
import { useChangepoints, changepointsToMarkLines } from '@/components/analytics/ChangepointMarkers'
import { useECharts } from '@/hooks/useECharts'
import { cn } from '@/lib/utils'

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

const TIME_RANGES: TimeRange[] = ['1m', '3m', '6m', '1y', 'ly', 'all']

const SPORT_GROUP_COLORS: Record<string, string> = {
  cycling: '#3b82f6',    // blue-500
  running: '#10b981',    // emerald-500
  swimming: '#06b6d4',   // cyan-500
  strength: '#a855f7',   // purple-500
  hiking: '#22c55e',     // green-500  // see NOTE above
  other: '#f59e0b',      // amber-500
}
const KNOWN_SPORT_GROUPS = new Set(Object.keys(SPORT_GROUP_COLORS))

const COMBINED_CTL_COLOR = '#3b82f6'
const SPARSE_THRESHOLD = 5

const STORAGE_KEY_SPORT_GROUPS = 'pmc_selected_sport_groups'

function getSportGroupLabel(key: string, t: ReturnType<typeof useTranslation>['t']): string {
  if (!KNOWN_SPORT_GROUPS.has(key)) return t('analyticsPmc.sportGroups.unknown')
  return t(`analyticsPmc.sportGroups.${key}`)
}

function getSportGroupColor(key: string): string {
  return SPORT_GROUP_COLORS[key] ?? '#6b7280'
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

function acwrStatusLabel(status: string, t: ReturnType<typeof useTranslation>['t']): string {
  switch (status) {
    case 'optimal':   return t('analyticsPmc.acwrStatus.optimal')
    case 'reduced':   return t('analyticsPmc.acwrStatus.reduced')
    case 'overreach': return t('analyticsPmc.acwrStatus.overreach')
    case 'danger':    return t('analyticsPmc.acwrStatus.danger')
    default:          return '—'
  }
}

// Read Set<string> from localStorage, ignore parse errors
function readStoredGroups(): Set<string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_SPORT_GROUPS)
    if (!raw) return new Set()
    const parsed: unknown = JSON.parse(raw)
    if (Array.isArray(parsed)) return new Set(parsed.filter(x => typeof x === 'string'))
  } catch {
    // ignore
  }
  return new Set()
}

function writeStoredGroups(groups: Set<string>): void {
  try {
    localStorage.setItem(STORAGE_KEY_SPORT_GROUPS, JSON.stringify([...groups]))
  } catch {
    // ignore
  }
}

// Build a date-indexed lookup for a given series
function buildDateMap(days: PMCDay[]): Map<string, PMCDay> {
  const map = new Map<string, PMCDay>()
  for (const d of days) map.set(d.date, d)
  return map
}

export default function PMCChartExtended() {
  const { t, i18n } = useTranslation()
  const locale = i18n.language || 'de'
  const formatZeroDecimals = useMemo(() => new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }), [locale])
  const formatOneDecimal = useMemo(() => new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }), [locale])
  const formatTwoDecimals = useMemo(() => new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 }), [locale])
  const [range, setRange] = useState<TimeRange>('6m')
  const changepoints = useChangepoints('ctl', 180)
  const [localFrom, setLocalFrom] = useState<string>('')
  const [localTo, setLocalTo] = useState<string>('')
  const [isCustomRange, setIsCustomRange] = useState(false)
  const [isZoomed, setIsZoomed] = useState(false)
  const chartInstanceRef = useRef<echarts.ECharts | null>(null)
  const today = toLocalISO(new Date())
  const ctlCombinedLabel = t('analyticsPmc.series.ctlCombined')
  const ctlFitnessLabel = t('analyticsPmc.series.ctlFitness')
  const atlFatigueLabel = t('analyticsPmc.series.atlFatigue')
  const tsbFormLabel = t('analyticsPmc.series.tsbForm')
  const ctlProjectionLabel = t('analyticsPmc.series.ctlProjection')
  const atlProjectionLabel = t('analyticsPmc.series.atlProjection')
  const tsbProjectionLabel = t('analyticsPmc.series.tsbProjection')

  // Sport group selection — persisted in localStorage
  const [selectedSportGroups, setSelectedSportGroups] = useState<Set<string>>(() => readStoredGroups())

  const hasSportGroupsActive = selectedSportGroups.size > 0

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

  const toggleSportGroup = (key: string) => {
    setSelectedSportGroups(prev => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      writeStoredGroups(next)
      return next
    })
  }

  // Build base params with only from/to — pure aggregate, no activity filter.
  const baseQueryParams = useMemo(() => {
    const params = new URLSearchParams()
    if (!isCustomRange && range === 'all') {
      // no from/to — backend returns all data
    } else {
      params.set('from', from)
      params.set('to', to)
    }
    return params.toString()
  }, [from, to, isCustomRange, range])

  // Standard fetch — only loaded when NO sport groups are active.
  // When sport groups are active the split response already contains __combined__
  // (with days) and all top-level analytics fields (acwr, ramp_rate, etc.).
  const { data: combinedData, isLoading: combinedLoading, isError: combinedError } = useQuery({
    queryKey: ['analytics', 'pmc-extended', baseQueryParams],
    queryFn: () => fetchAnalyticsPMC(baseQueryParams),
    enabled: !hasSportGroupsActive,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  // Split fetch — includes all analytics filters via baseQueryParams.
  // Enabled whenever sport groups are active so the __combined__ series
  // drives the metrics row and chart base lines.
  const { data: splitData, isLoading: splitLoading } = useQuery({
    queryKey: ['analytics', 'pmc-split', baseQueryParams],
    queryFn: () =>
      fetchAnalyticsPMCSplit(
        Object.fromEntries(new URLSearchParams(baseQueryParams)) as { from: string; to: string } & Record<string, string>,
      ),
    enabled: hasSportGroupsActive,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  // Unified data view: when sport groups are active, derive combined data from
  // the __combined__ series + top-level fields of the split response.
  const effectiveCombinedData = useMemo(() => {
    if (!hasSportGroupsActive) return combinedData ?? null
    if (!splitData) return null
    const combinedDays = splitData.series['__combined__'] ?? []
    return {
      days: combinedDays,
      acwr: splitData.acwr,
      acwr_status: splitData.acwr_status,
      acwr_trend: splitData.acwr_trend,
      ramp_rate: splitData.ramp_rate,
      monotony: splitData.monotony,
      strain: splitData.strain,
      projection: splitData.projection,
      using_default_thresholds: splitData.using_default_thresholds,
      default_threshold_fields: splitData.default_threshold_fields,
    }
  }, [hasSportGroupsActive, combinedData, splitData])

  const isLoading = hasSportGroupsActive ? splitLoading : combinedLoading
  const isError = hasSportGroupsActive ? false : combinedError

  // Derive available sport groups from split data (exclude __combined__)
  const availableSportGroups = useMemo<Array<{ key: string; count: number }>>(() => {
    if (!splitData?.series) return []
    return Object.entries(splitData.series)
      .filter(([key]) => key !== '__combined__')
      .map(([key, days]) => ({ key, count: days.length }))
      .sort((a, b) => b.count - a.count)
  }, [splitData])

  // Sync: remove selected groups that are no longer in the data
  useEffect(() => {
    if (availableSportGroups.length === 0) return
    const validKeys = new Set(availableSportGroups.map(g => g.key))
    setSelectedSportGroups(prev => {
      const next = new Set([...prev].filter(k => validKeys.has(k)))
      if (next.size !== prev.size) {
        writeStoredGroups(next)
        return next
      }
      return prev
    })
  }, [availableSportGroups])

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
      if (!effectiveCombinedData) return { series: [] }

      const days = effectiveCombinedData.days ?? []
      const projection = effectiveCombinedData.projection ?? []

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

      // Build sport-group CTL series (when active)
      const activeSportGroupSeries: ReturnType<typeof buildSportGroupSeries>[] = []
      if (hasSportGroupsActive && splitData?.series) {
        for (const key of selectedSportGroups) {
          const sportDays = splitData.series[key]
          if (!sportDays) continue
          const dateMap = buildDateMap(sportDays)
          const ctlValues = allDates.map(d => dateMap.get(d)?.ctl ?? null)
          activeSportGroupSeries.push(buildSportGroupSeries(key, ctlValues, t))
        }
      }

      const legendData = hasSportGroupsActive
        ? [
            ctlCombinedLabel,
            ...activeSportGroupSeries.map(s => s.name),
            atlFatigueLabel,
            tsbFormLabel,
            ctlProjectionLabel,
            atlProjectionLabel,
            tsbProjectionLabel,
          ]
        : [ctlFitnessLabel, atlFatigueLabel, tsbFormLabel, ctlProjectionLabel, atlProjectionLabel, tsbProjectionLabel]

      const ctlCombinedSeries = hasSportGroupsActive
        ? {
            name: ctlCombinedLabel,
            type: 'line' as const,
            data: ctlFull,
            showSymbol: false,
            lineStyle: { width: 1.5, color: COMBINED_CTL_COLOR, type: 'dashed' as const, opacity: 0.5 },
            itemStyle: { color: COMBINED_CTL_COLOR, opacity: 0.5 },
            z: 1,
            connectNulls: false,
          }
        : {
            name: ctlFitnessLabel,
            type: 'line' as const,
            data: ctlFull,
            showSymbol: false,
            lineStyle: { width: 2.5, color: COMBINED_CTL_COLOR },
            itemStyle: { color: COMBINED_CTL_COLOR },
            z: 2,
            connectNulls: false,
            markLine: changepoints.length > 0 ? { silent: true, data: changepointsToMarkLines(changepoints) } : undefined,
          }

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
              title: { zoom: t('analyticsPmc.toolbox.zoom'), back: t('analyticsPmc.toolbox.zoomReset') },
              brushStyle: { borderWidth: 1, color: 'rgba(59,130,246,0.1)', borderColor: 'rgba(59,130,246,0.6)' },
            },
            restore: { title: t('analyticsPmc.reset') },
            brush: { show: false },
            saveAsImage: { title: t('analyticsPmc.toolbox.savePng'), pixelRatio: 2 },
          },
          iconStyle: { borderColor: '#6b7280' },
          emphasis: { iconStyle: { borderColor: '#d1d5db' } },
        },
        legend: {
          data: legendData,
          top: 6,
          right: 80,
          textStyle: { fontSize: 11, color: '#9ca3af' },
        },
        tooltip: {
          confine: true,
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
                <strong style="color:#f9fafb">${formatOneDecimal.format(p.value)}</strong>
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
            name: tsbFormLabel,
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
          ctlCombinedSeries,
          {
            name: atlFatigueLabel,
            type: 'line',
            data: atlFull,
            showSymbol: false,
            lineStyle: { width: 2, color: '#f97316' },
            itemStyle: { color: '#f97316' },
            z: 2,
            connectNulls: false,
          },
          ...activeSportGroupSeries,
          {
            name: ctlProjectionLabel,
            type: 'line',
            data: ctlProjFull,
            showSymbol: false,
            lineStyle: { width: 1.5, color: '#3b82f6', type: 'dashed', opacity: 0.5 },
            itemStyle: { color: '#3b82f6', opacity: 0.5 },
            z: 1,
            connectNulls: false,
          },
          {
            name: atlProjectionLabel,
            type: 'line',
            data: atlProjFull,
            showSymbol: false,
            lineStyle: { width: 1.5, color: '#ef4444', type: 'dashed', opacity: 0.5 },
            itemStyle: { color: '#ef4444', opacity: 0.5 },
            z: 1,
            connectNulls: false,
          },
          {
            name: tsbProjectionLabel,
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
    [effectiveCombinedData, splitData, selectedSportGroups, hasSportGroupsActive, changepoints, ctlCombinedLabel, ctlFitnessLabel, atlFatigueLabel, tsbFormLabel, ctlProjectionLabel, atlProjectionLabel, tsbProjectionLabel, formatOneDecimal, t],
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
        ...(effectiveCombinedData?.days ?? []).map(d => d.date),
        ...(effectiveCombinedData?.projection ?? []).map(d => d.date),
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

  const latest = effectiveCombinedData?.days && effectiveCombinedData.days.length > 0 ? effectiveCombinedData.days[effectiveCombinedData.days.length - 1] : null

  return (
    <div className="space-y-3">
      {/* Header row */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {t('analyticsPmc.title')}
        </h2>
        <div className="flex items-center gap-2 flex-wrap">
          {isZoomed && (
            <button onClick={handleResetZoom}
              className="cursor-pointer text-sm px-3 py-1 rounded-md bg-accent-100 text-orange-700 hover:bg-orange-200
                         dark:bg-orange-900/30 dark:text-orange-300 dark:hover:bg-orange-900/50 transition-colors">
              {t('analyticsPmc.zoomReset')}
            </button>
          )}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
            {TIME_RANGES.map(r => (
              <button
                key={r}
                onClick={() => handleRangeClick(r)}
                title={t(`analyticsPmc.rangeTooltips.${r}`, { year: new Date().getFullYear() - 1 })}
                className={
                  r === range && !isCustomRange
                    ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                    : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors'
                }
              >
                {t(`analyticsPmc.rangeLabels.${r}`)}
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

      {/* Sport group chips — only shown when split data is available */}
      {availableSportGroups.length > 0 && (
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className="text-xs text-neutral-400 dark:text-neutral-500 mr-0.5">{t('analyticsPmc.ctlBySport')}</span>
          {availableSportGroups.map(({ key, count }) => {
            const isSparse = count < SPARSE_THRESHOLD
            const isActive = selectedSportGroups.has(key)
            const color = getSportGroupColor(key)
            const label = getSportGroupLabel(key, t)
            return (
              <button
                key={key}
                onClick={() => !isSparse && toggleSportGroup(key)}
                title={isSparse ? t('analyticsPmc.sparseData') : label}
                disabled={isSparse}
                className={[
                  'inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium transition-all border cursor-pointer',
                  isSparse
                    ? 'opacity-40 cursor-not-allowed border-neutral-200 dark:border-neutral-700 text-neutral-500 dark:text-neutral-400 bg-transparent'
                    : isActive
                      ? 'border-transparent text-white shadow-sm'
                      : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 bg-transparent hover:border-neutral-300 dark:hover:border-neutral-600',
                ].join(' ')}
                style={isActive && !isSparse ? { backgroundColor: color, borderColor: color } : {}}
              >
                <span
                  className="w-2 h-2 rounded-full inline-block flex-shrink-0"
                  style={{ backgroundColor: isSparse ? '#9ca3af' : color }}
                />
                {label}
                {isSparse && (
                  <span className="ml-0.5 text-neutral-400 dark:text-neutral-500">{t('analyticsPmc.sparseDataShort')}</span>
                )}
              </button>
            )
          })}
          {selectedSportGroups.size > 0 && (
            <button
              onClick={() => {
                setSelectedSportGroups(new Set())
                writeStoredGroups(new Set())
              }}
              className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 transition-colors cursor-pointer ml-1"
              title={t('analyticsPmc.resetSportFilter')}
            >
              {t('analyticsPmc.reset')}
            </button>
          )}
        </div>
      )}

      {/* Metrics row */}
      {(latest || effectiveCombinedData) && (() => {
        const lastDate = effectiveCombinedData?.days[effectiveCombinedData.days.length - 1]?.date
        const todayISO = new Date().toISOString().slice(0, 10)
        const isHistorical = lastDate && lastDate !== todayISO
        const dateSuffix = isHistorical ? ` (${new Date(lastDate).toLocaleDateString(locale)})` : ''
        return (
          <div className="flex items-center gap-2 flex-wrap">
            {/* CTL / ATL / TSB badges */}
            {latest && (
              <>
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                  <span className="w-2 h-2 rounded-full bg-blue-500 inline-block" />
                  CTL {formatOneDecimal.format(latest.ctl)}{dateSuffix}
                </span>
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-accent-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300">
                  <span className="w-2 h-2 rounded-full bg-accent-DEFAULT inline-block" />
                  ATL {formatOneDecimal.format(latest.atl)}{dateSuffix}
                </span>
                <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${latest.tsb >= 0 ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'}`}>
                  <span className={`w-2 h-2 rounded-full inline-block ${latest.tsb >= 0 ? 'bg-emerald-500' : 'bg-rose-500'}`} />
                  TSB {latest.tsb >= 0 ? '+' : ''}{formatOneDecimal.format(latest.tsb)}{dateSuffix}
                </span>
              </>
            )}
            {/* ACWR / RampRate / Monotony / Strain */}
            {effectiveCombinedData && (
              <>
                {effectiveCombinedData.acwr_status && (
                  <span
                    className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium ${acwrStatusColor(effectiveCombinedData.acwr_status)}`}
                    title={[
                      acwrStatusLabel(effectiveCombinedData.acwr_status, t),
                      t('analyticsPmc.acwrRollingHint'),
                      effectiveCombinedData.acwr_status === 'reduced' ? t('analyticsPmc.acwrReducedHint') : null,
                    ].filter(Boolean).join(' - ')}
                  >
                    ACWR {formatTwoDecimals.format(effectiveCombinedData.acwr)}{dateSuffix}
                  </span>
                )}
                <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
                  {t('analyticsPmc.ramp')} {effectiveCombinedData.ramp_rate >= 0 ? '+' : ''}{formatOneDecimal.format(effectiveCombinedData.ramp_rate)}/{t('analyticsPmc.weekShort')}{dateSuffix}
                </span>
                <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
                  {t('analyticsPmc.monotony')} {formatTwoDecimals.format(effectiveCombinedData.monotony)}{dateSuffix}
                </span>
                <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
                  {t('analyticsPmc.strain')} {formatZeroDecimals.format(effectiveCombinedData.strain)}{dateSuffix}
                </span>
              </>
            )}
          </div>
        )
      })()}

      {/* Fallback warning banner */}
      {effectiveCombinedData?.using_default_thresholds && (
        <div className="mb-4 px-4 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 text-amber-800 dark:text-amber-200 text-sm">
          {t('analyticsPmc.defaultThresholds')}
          {effectiveCombinedData.default_threshold_fields?.length ? (
            <> {t('analyticsPmc.forFields')} <strong>{effectiveCombinedData.default_threshold_fields.map(f => f.toUpperCase()).join(', ')}</strong></>
          ) : null}.{' '}
          {t('analyticsPmc.thresholdsHint')}
        </div>
      )}

      {/* Chart */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        <div className="relative h-[340px]">
          {isLoading && (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
              {t('analyticsPmc.loading')}
            </div>
          )}
          {!isLoading && isError && (
            <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
              {t('analyticsPmc.loadFailed')}
            </div>
          )}
          {!isLoading && !isError && (!effectiveCombinedData || (effectiveCombinedData.days?.length ?? 0) === 0) && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-neutral-400">
              <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                  d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
              </svg>
              <span className="text-sm">{t('analyticsPmc.noDataForRange')}</span>
            </div>
          )}
          <div
            ref={containerRef}
            className={cn("w-full h-full", {
              invisible: isLoading || isError || !effectiveCombinedData || (effectiveCombinedData.days?.length ?? 0) === 0,
            })}
          />
        </div>
      </div>

      {/* TSB zone legend */}
      <div className="flex items-center gap-3 flex-wrap text-xs text-neutral-500 dark:text-neutral-400">
        <span className="flex items-center gap-1.5" title={t('analyticsPmc.tsbFreshHint')}>
          <span className="w-3 h-3 rounded-sm inline-block bg-blue-400/30" />
          {t('analyticsPmc.tsbZones.fresh')}
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm inline-block bg-emerald-400/30" />
          Optimal (−10…+15)
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm inline-block bg-yellow-400/30" />
          {t('analyticsPmc.tsbZones.overload')}
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm inline-block bg-red-400/30" />
          {t('analyticsPmc.tsbZones.severeOverload')}
        </span>
      </div>
    </div>
  )
}

// Helper: build a single sport-group CTL series config
function buildSportGroupSeries(key: string, ctlValues: (number | null)[], t: ReturnType<typeof useTranslation>['t']) {
  const color = getSportGroupColor(key)
  const label = getSportGroupLabel(key, t)
  return {
    name: `${t('analyticsPmc.series.ctl')} ${label}`,
    type: 'line' as const,
    data: ctlValues,
    showSymbol: false,
    lineStyle: { width: 2.5, color },
    itemStyle: { color },
    z: 3,
    connectNulls: false,
  }
}
