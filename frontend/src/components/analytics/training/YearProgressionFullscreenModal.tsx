import { useState, useMemo, useEffect, useCallback } from 'react'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { ModalShell } from '@/components/ui/ModalShell'
import { useECharts } from '@/hooks/useECharts'
import { useYearProgression } from '@/hooks/useYearProgression'
import type { MetricKey, YearProgressionMode } from '@/hooks/useYearProgression'
import YearComparisonTable from './YearComparisonTable'
import SportHierarchyPicker from './SportHierarchyPicker'
import { expandSportSelection } from '@/components/analytics/filters/SportOnlyFilterBar'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, CanvasRenderer])

// ECharts palette: must be identical to YearProgressionChart and YearComparisonTable.
const YEAR_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444',
  '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16',
]

const MONTH_LABELS = ['Jan', 'Feb', 'Mär', 'Apr', 'Mai', 'Jun', 'Jul', 'Aug', 'Sep', 'Okt', 'Nov', 'Dez']

// Metric display metadata
const METRIC_META: Record<MetricKey, { label: string; unit: string; factor: number; decimals: number }> = {
  distance_m:       { label: 'Distanz',       unit: 'km',   factor: 0.001,   decimals: 1 },
  elevation_gain_m: { label: 'Höhenmeter',    unit: 'm',    factor: 1,       decimals: 0 },
  moving_time_s:    { label: 'Bewegungszeit', unit: 'h',    factor: 1/3600,  decimals: 1 },
  tss:              { label: 'TSS',           unit: 'TSS',  factor: 1,       decimals: 0 },
  total_calories:   { label: 'Kalorien',      unit: 'kcal', factor: 1,       decimals: 0 },
}

// Metrics shown as active toggles in the modal header (TSS and Kalorien are disabled/hidden).
const METRIC_TOGGLE_KEYS: MetricKey[] = ['distance_m', 'elevation_gain_m', 'moving_time_s']

type PeriodUnit = 'day' | 'week' | 'month'

function periodToDays(multiplier: number, unit: PeriodUnit): number {
  switch (unit) {
    case 'day':   return multiplier
    case 'week':  return multiplier * 7
    case 'month': return multiplier * 30
  }
}

// F007: all state is local; modal-close discards it. Next open re-initialises
// from props + defaults.

interface YearProgressionFullscreenModalProps {
  open: boolean
  onClose: () => void
  initialYears?: number[]
  initialMetric?: MetricKey
}

export default function YearProgressionFullscreenModal({
  open,
  onClose,
  initialYears,
  initialMetric,
}: YearProgressionFullscreenModalProps) {
  // Inner component is remounted on each open cycle → all useState resets to
  // initial values, satisfying F007 without manual reset logic.
  if (!open) return null

  return (
    <ModalStateContainer
      key={String(open)}
      onClose={onClose}
      initialYears={initialYears}
      initialMetric={initialMetric}
    />
  )
}

function ModalStateContainer({
  onClose,
  initialYears,
  initialMetric,
}: Omit<YearProgressionFullscreenModalProps, 'open'>) {
  // ── Core state ──────────────────────────────────────────────────────────────
  const [mode, setMode] = useState<YearProgressionMode>('progress')
  const [metric, setMetric] = useState<MetricKey>(initialMetric ?? 'distance_m')
  // Rolling granularity (X-axis resolution). Progress mode has no granularity param.
  // Granularity stays 'day' across modes; rolling mode is the only consumer.
  // _setGranularity is reserved for the future granularity toggle UI (not in this round).
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const [granularity, _setGranularity] = useState<'day' | 'week'>('day')
  const [selectedYears, setSelectedYears] = useState<number[]>(initialYears ?? [])
  const [selectedSports, setSelectedSports] = useState<string[]>([])
  const [lockedDate, setLockedDate] = useState<Date | null>(null)

  // ── Rolling-mode period state ────────────────────────────────────────────────
  const [periodMultiplier, setPeriodMultiplier] = useState<number>(1)
  const [periodUnit, setPeriodUnit] = useState<PeriodUnit>('month')
  const periodDays = periodToDays(periodMultiplier, periodUnit)

  // ── Mode-switch handler ──────────────────────────────────────────────────────
  const handleModeSwitch = useCallback((next: YearProgressionMode) => {
    if (next === mode) return
    // Keep: metric, selectedYears, selectedSports, granularity
    // Reset: lockedDate (only meaningful in progress mode)
    setLockedDate(null)
    setMode(next)
  }, [mode])

  // ── Query params ─────────────────────────────────────────────────────────────
  const queryParams = useMemo(() => {
    const params = new URLSearchParams()
    params.set('metric', metric)
    if (selectedSports.length > 0) {
      // WORKAROUND until sport-taxonomy-rework: bare-sport tokens collapse subtype
      // filter in backend (filter.go:165-218). We expand client-side.
      const expanded = expandSportSelection(selectedSports)
      params.set('sport_type', expanded.join(','))
    }
    return params.toString()
  }, [metric, selectedSports])

  // ── Data fetching ────────────────────────────────────────────────────────────
  const progressResult = useYearProgression({ mode: 'progress', metric, queryParams })
  const rollingResult = useYearProgression({
    mode: 'rolling',
    metric,
    periodDays,
    granularity,
    queryParams,
  })

  const activeResult = mode === 'progress' ? progressResult : rollingResult
  const { isLoading, isError } = activeResult

  // ── Available years (mode-specific: rolling may cover fewer years than progress) ─
  const availableYears = useMemo(() => {
    const data = mode === 'progress' ? progressResult.data : rollingResult.data
    return data?.years?.map(y => y.year) ?? []
  }, [mode, progressResult.data?.years, rollingResult.data?.years]) // eslint-disable-line react-hooks/exhaustive-deps

  // Year-Set-Mismatch rule: trim selectedYears to available years on load / mode switch.
  useEffect(() => {
    if (availableYears.length === 0) return
    if (selectedYears.length === 0) {
      setSelectedYears(availableYears)
      return
    }
    const available = new Set(availableYears)
    const filtered = selectedYears.filter(y => available.has(y))
    if (filtered.length !== selectedYears.length) {
      setSelectedYears(filtered)
    }
  }, [availableYears.join(',')]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Display data filtered to selected years ──────────────────────────────────
  const displayProgressYears = useMemo(() => {
    if (!progressResult.data?.years) return []
    const sel = new Set(selectedYears)
    return progressResult.data.years.filter(y => sel.has(y.year))
  }, [progressResult.data?.years, selectedYears])

  const yearColorIndex = useMemo(() => {
    const map = new Map<number, number>()
    progressResult.data?.years?.forEach((yd, idx) => map.set(yd.year, idx))
    return map
  }, [progressResult.data?.years])

  const meta = METRIC_META[metric]

  // ── Progress-mode chart option ────────────────────────────────────────────────
  const progressChartOption = useMemo(() => {
    if (!displayProgressYears.length) return null
    const tooltipFormatter = (params: unknown) => {
      const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string }>
      if (!Array.isArray(items) || items.length === 0) return ''
      let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">${items[0].axisValueLabel}</div>`
      for (const p of items) {
        if (p.value == null) continue
        const v = (p.value * meta.factor).toFixed(meta.decimals)
        html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
          <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
          <span style="color:#d1d5db">${p.seriesName}:</span>
          <strong style="color:#f9fafb">${v} ${meta.unit}</strong>
        </div>`
      }
      return html
    }

    const series = displayProgressYears.map((yd, idx) => {
      const colorIdx = yearColorIndex.get(yd.year) ?? idx
      const color = YEAR_COLORS[colorIdx % YEAR_COLORS.length]
      return {
        name: String(yd.year),
        type: 'line' as const,
        data: yd.cumulative,
        showSymbol: false,
        lineStyle: { width: 2, color },
        itemStyle: { color },
        connectNulls: false,
      }
    })

    return {
      backgroundColor: 'transparent',
      grid: { left: 56, right: 24, top: 36, bottom: 48 },
      legend: {
        data: displayProgressYears.map(y => String(y.year)),
        top: 4,
        textStyle: { fontSize: 11, color: '#9ca3af' },
      },
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 12 },
        formatter: tooltipFormatter,
      },
      xAxis: {
        type: 'category',
        data: MONTH_LABELS,
        axisLabel: { fontSize: 10, color: '#6b7280' },
        axisLine: { lineStyle: { color: '#374151' } },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value',
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          formatter: (v: number) => {
            const converted = v * meta.factor
            return converted >= 1000 ? `${(converted / 1000).toFixed(0)}k` : converted.toFixed(0)
          },
        },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      },
      dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
      series,
    }
  }, [displayProgressYears, yearColorIndex, meta])

  // ── Rolling-mode chart option ─────────────────────────────────────────────────
  const rollingChartOption = useMemo(() => {
    const rollingYears = rollingResult.data?.years
    if (!rollingYears?.length) return null

    const sel = new Set(selectedYears)
    const selected = rollingYears.filter(y => sel.has(y.year))
    if (!selected.length) return null

    const tooltipFormatter = (params: unknown) => {
      const items = params as Array<{ seriesName: string; value: number | null; axisValueLabel: string; color: string }>
      if (!Array.isArray(items) || items.length === 0) return ''
      let html = `<div style="font-size:11px;color:#9ca3af;margin-bottom:4px">Tag ${items[0].axisValueLabel}</div>`
      for (const p of items) {
        if (p.value == null) continue
        const v = (p.value * meta.factor).toFixed(meta.decimals)
        html += `<div style="display:flex;align-items:center;gap:6px;margin-bottom:2px">
          <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${p.color}"></span>
          <span style="color:#d1d5db">${p.seriesName}:</span>
          <strong style="color:#f9fafb">${v} ${meta.unit}</strong>
        </div>`
      }
      return html
    }

    // X-axis: day-of-year 1–365. Series are aligned by index (day 1 = Jan 1 for each year).
    const maxLen = Math.max(...selected.map(y => y.points.length))
    const xData = Array.from({ length: maxLen }, (_, i) => String(i + 1))

    const series = selected.map((yd, idx) => {
      const colorIdx = yearColorIndex.get(yd.year) ?? idx
      const color = YEAR_COLORS[colorIdx % YEAR_COLORS.length]
      return {
        name: String(yd.year),
        type: 'line' as const,
        data: yd.points.map(p => p.value),
        showSymbol: false,
        lineStyle: { width: 2, color },
        itemStyle: { color },
        connectNulls: false,
      }
    })

    return {
      backgroundColor: 'transparent',
      grid: { left: 56, right: 24, top: 36, bottom: 48 },
      legend: {
        data: selected.map(y => String(y.year)),
        top: 4,
        textStyle: { fontSize: 11, color: '#9ca3af' },
      },
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        textStyle: { color: '#f3f4f6', fontSize: 12 },
        formatter: tooltipFormatter,
      },
      xAxis: {
        type: 'category',
        data: xData,
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          // Show labels only every 30 days to avoid crowding.
          interval: 29,
        },
        axisLine: { lineStyle: { color: '#374151' } },
        splitLine: { show: false },
        name: 'Tag des Jahres',
        nameTextStyle: { fontSize: 9, color: '#6b7280' },
      },
      yAxis: {
        type: 'value',
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          formatter: (v: number) => {
            const converted = v * meta.factor
            return converted >= 1000 ? `${(converted / 1000).toFixed(0)}k` : converted.toFixed(0)
          },
        },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
      },
      dataZoom: [{ type: 'inside', xAxisIndex: 0, filterMode: 'none' }],
      series,
    }
  }, [rollingResult.data?.years, selectedYears, yearColorIndex, meta])

  const activeOption = mode === 'progress' ? progressChartOption : rollingChartOption

  // Progress-mode click handler: converts the clicked month index (x-axis category index)
  // into a Date set to the last day of that month, then locks the comparison table.
  // Only active in progress mode; rolling mode has no monthly x-axis.
  const handleProgressChartClick = useCallback((chart: echarts.ECharts) => {
    if (mode !== 'progress') return
    const onClick = (params: unknown) => {
      const p = params as { dataIndex?: number }
      if (p.dataIndex == null) return
      const monthIdx = p.dataIndex // 0-based month index from the category x-axis
      const currentYear = new Date().getFullYear()
      // Last day of the clicked month in the current year.
      // new Date(year, month + 1, 0) gives the last day of `month`.
      const lastDay = new Date(currentYear, monthIdx + 1, 0).getDate()
      setLockedDate(new Date(currentYear, monthIdx, lastDay))
    }
    chart.on('click', onClick)
    return () => chart.off('click', onClick)
  }, [mode]) // eslint-disable-line react-hooks/exhaustive-deps

  const chartContainerRef = useECharts(
    () => activeOption ?? { series: [] },
    [activeOption],
    { enabled: !!activeOption, onChart: handleProgressChartClick },
  )

  // ── Toggle helpers ────────────────────────────────────────────────────────────
  const toggleYear = (year: number) => {
    setSelectedYears(prev =>
      prev.includes(year)
        ? prev.length > 1 ? prev.filter(y => y !== year) : prev
        : [...prev, year],
    )
  }

  // ── Render ────────────────────────────────────────────────────────────────────
  const PERIOD_UNIT_LABELS: { value: PeriodUnit; label: string }[] = [
    { value: 'day',   label: 'Tag' },
    { value: 'week',  label: 'Woche' },
    { value: 'month', label: 'Monat' },
  ]

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-7xl">
      {/* ── Header row ── */}
      <div className="flex items-center justify-between gap-3 mb-4 flex-wrap">
        {/* Left: title + mode toggle */}
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            Jahresvergleich
          </h2>
          {/* Mode toggle: Progress | Rolling */}
          <div
            className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs"
            role="group"
            aria-label="Ansichtsmodus"
          >
            {(['progress', 'rolling'] as const).map(m => (
              <button
                key={m}
                type="button"
                aria-pressed={mode === m}
                onClick={() => handleModeSwitch(m)}
                className={`px-3 py-1.5 font-medium cursor-pointer transition-colors
                  ${mode === m
                    ? 'bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900'
                    : 'hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300'
                  }`}
              >
                {m === 'progress' ? 'Progress' : 'Rolling'}
              </button>
            ))}
          </div>
        </div>

        {/* Right: metric toggle pills */}
        <div className="flex items-center gap-1 flex-wrap">
          {METRIC_TOGGLE_KEYS.map(key => {
            const m = METRIC_META[key]
            return (
              <button
                key={key}
                type="button"
                aria-pressed={metric === key}
                onClick={() => setMetric(key)}
                className={`px-3 py-1 rounded-full text-xs font-medium border cursor-pointer transition-colors
                  ${metric === key
                    ? 'bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 border-transparent'
                    : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
                  }`}
              >
                {m.label}
              </button>
            )
          })}
        </div>
      </div>

      {/* ── Rolling period selector (only in rolling mode) ── */}
      {mode === 'rolling' && (
        <div
          className="flex items-center gap-3 mb-3 flex-wrap text-xs"
          data-testid="period-selector"
        >
          <span className="text-neutral-500 dark:text-neutral-400 font-medium">Vielfaches:</span>
          <input
            type="number"
            min={1}
            max={12}
            value={periodMultiplier}
            onChange={e => {
              const v = Math.max(1, Math.min(12, Number(e.target.value)))
              setPeriodMultiplier(v)
            }}
            aria-label="Vielfaches"
            className="w-14 border border-neutral-200 dark:border-neutral-700 rounded px-2 py-1 text-xs
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
          {/* Period unit pills */}
          <div
            className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700"
            role="group"
            aria-label="Zeiteinheit"
          >
            {PERIOD_UNIT_LABELS.map(({ value, label }) => (
              <button
                key={value}
                type="button"
                aria-pressed={periodUnit === value}
                onClick={() => setPeriodUnit(value)}
                className={`px-2.5 py-1 font-medium cursor-pointer transition-colors
                  ${periodUnit === value
                    ? 'bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900'
                    : 'hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300'
                  }`}
              >
                {label}
              </button>
            ))}
          </div>
          <span className="text-neutral-400 dark:text-neutral-600 italic">
            {/* Show computed period in days as a hint */}
            {periodDays} {periodDays === 1 ? 'Tag' : 'Tage'} Fenster
          </span>
        </div>
      )}

      {/* ── Body: chart area + sidebar ── */}
      <div className="flex gap-4" style={{ minHeight: '480px' }}>
        {/* ── Main chart area ── */}
        <div className="flex-1 min-w-0 flex flex-col gap-3">
          {/* Year-selection pills */}
          {availableYears.length > 0 && (
            <div className="flex items-center gap-1.5 flex-wrap" data-testid="year-pills">
              {availableYears.map((year, idx) => {
                const color = YEAR_COLORS[idx % YEAR_COLORS.length]
                const active = selectedYears.includes(year)
                return (
                  <button
                    key={year}
                    type="button"
                    aria-pressed={active}
                    onClick={() => toggleYear(year)}
                    className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border cursor-pointer transition-all
                      ${active
                        ? 'border-transparent text-white'
                        : 'border-neutral-200 dark:border-neutral-700 text-neutral-500 dark:text-neutral-400 bg-transparent'
                      }`}
                    style={active ? { backgroundColor: color } : {}}
                  >
                    {!active && (
                      <span className="w-2 h-2 rounded-full inline-block" style={{ backgroundColor: color }} />
                    )}
                    {year}
                  </button>
                )
              })}
            </div>
          )}

          {/* Chart */}
          <div className="flex-1 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden relative" style={{ minHeight: '400px' }}>
            {isLoading && (
              <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
                Lade Jahresvergleich…
              </div>
            )}
            {isError && !isLoading && (
              <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
                Fehler beim Laden
              </div>
            )}
            {!isLoading && !isError && !activeOption && (
              <div className="absolute inset-0 flex items-center justify-center text-sm text-neutral-400">
                Keine Daten vorhanden
              </div>
            )}
            <div
              ref={chartContainerRef}
              className="w-full h-full"
              data-testid="year-progression-chart"
              style={{ minHeight: '400px' }}
            />
          </div>
        </div>

        {/* ── Sidebar ── */}
        <div className="w-72 shrink-0 flex flex-col gap-4">
          {/* Sport picker */}
          <div>
            <p className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-2">
              Sportarten
            </p>
            <SportHierarchyPicker
              value={selectedSports}
              onChange={setSelectedSports}
            />
          </div>

          {/* Year comparison table (progress mode only — rolling has no cumulative data) */}
          {mode === 'progress' && (
            <div>
              <YearComparisonTable
                years={displayProgressYears}
                yearColorIndex={yearColorIndex}
                metric={metric}
                lockedDate={lockedDate}
                onLockedDateChange={setLockedDate}
              />
            </div>
          )}

          {/* Rolling-mode description */}
          {mode === 'rolling' && (
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 text-xs text-neutral-500 dark:text-neutral-400">
              <p className="font-medium text-neutral-700 dark:text-neutral-300 mb-1">Rolling-Fenster</p>
              <p>
                Jeder Punkt zeigt die Summe der letzten {periodDays} {periodDays === 1 ? 'Tages' : 'Tage'}.
                Die Kurven aller Jahre starten bei Tag&nbsp;1 (1.&nbsp;Januar).
              </p>
            </div>
          )}
        </div>
      </div>
    </ModalShell>
  )
}
