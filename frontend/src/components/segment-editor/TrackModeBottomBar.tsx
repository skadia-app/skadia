// frontend/src/components/segment-editor/TrackModeBottomBar.tsx
import { useEffect, useMemo, useRef } from 'react'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  BrushComponent,
  ToolboxComponent,
  TooltipComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { ArrowLeftRight } from 'lucide-react'
import { useRangeBrush } from '@/hooks/useRangeBrush'
import { cn } from '@/lib/utils'

echarts.use([LineChart, GridComponent, BrushComponent, ToolboxComponent, TooltipComponent, CanvasRenderer])

export interface TrackPoint {
  lat: number
  lng: number
  time?: string
  elevation?: number | null
}

export interface TrackModeBottomBarProps {
  trackPoints: TrackPoint[]
  range: [number, number]
  onRangeChange: (r: [number, number]) => void
  onCommitRange: (r: [number, number]) => void   // fires on mouse-up / keyboard-commit, triggers map fitBounds
  onReverse: () => void
  onReset: () => void
  onPreviewOrSave: () => void
  isPreview: boolean
}

export function TrackModeBottomBar(props: TrackModeBottomBarProps) {
  const { trackPoints, range, onRangeChange, onCommitRange, onReverse, onReset, onPreviewOrSave, isPreview } = props
  const chartRef = useRef<HTMLDivElement | null>(null)
  const echartsInstanceRef = useRef<echarts.ECharts | null>(null)

  const { bindEcharts, setStart, setEnd } = useRangeBrush({
    totalPoints: trackPoints.length,
    initial: range,
    onBrushChange: onRangeChange,
  })

  // Compute elevation series and cumulative distance-per-index for km labels.
  const { elevationSeries, hasElevation, cumulativeKm } = useMemo(() => {
    const elev = trackPoints.map(p => (p.elevation ?? null))
    const has = elev.some(e => e != null)
    const km: number[] = [0]
    for (let i = 1; i < trackPoints.length; i++) {
      const a = trackPoints[i - 1]; const b = trackPoints[i]
      const d = haversineKm(a.lat, a.lng, b.lat, b.lng)
      km.push(km[i - 1] + d)
    }
    return { elevationSeries: elev, hasElevation: has, cumulativeKm: km }
  }, [trackPoints])

  // Init ECharts — polished per RouteProfileChart reference.
  useEffect(() => {
    if (!chartRef.current) return
    const ec = echarts.init(chartRef.current, undefined, { renderer: 'canvas' })
    echartsInstanceRef.current = ec

    // Y-axis range with padding (same approach as RouteProfileChart)
    const validElev = elevationSeries.filter((e): e is number => e != null)
    const yMin = validElev.length > 0 ? Math.floor(Math.min(...validElev) - 20) : 0
    const yMax = validElev.length > 0 ? Math.ceil(Math.max(...validElev) + 20) : 1

    ec.setOption({
      backgroundColor: 'transparent',
      animation: false,
      grid: { top: 8, right: 12, bottom: 22, left: 44, containLabel: false },
      xAxis: {
        type: 'value',
        min: 0,
        max: trackPoints.length > 0 ? cumulativeKm[trackPoints.length - 1] : 1,
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          formatter: (v: number) => `${v.toFixed(1)} km`,
          showMinLabel: true,
          showMaxLabel: true,
        },
        axisLine: { lineStyle: { color: '#374151' } },
        splitLine: { show: false },
        axisTick: { show: false },
      },
      yAxis: {
        type: 'value',
        show: hasElevation,
        min: yMin,
        max: yMax,
        axisLabel: {
          fontSize: 10,
          color: '#6b7280',
          formatter: (v: number) => `${v} m`,
          showMinLabel: false,
          showMaxLabel: true,
        },
        splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
        axisTick: { show: false },
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: {
          type: 'line',
          lineStyle: { color: '#6b7280', width: 1, type: 'dashed' },
        },
        backgroundColor: 'rgba(17,24,39,0.92)',
        borderColor: '#374151',
        borderWidth: 1,
        textStyle: { color: '#f3f4f6', fontSize: 11 },
        formatter: (params: unknown) => {
          const p = params as Array<{ dataIndex: number; value: [number, number] }>
          if (!p || !p[0]) return ''
          const idx = p[0].dataIndex
          const elevVal = elevationSeries[idx]
          const kmVal = cumulativeKm[idx]
          const kmStr = kmVal != null ? kmVal.toFixed(2) : '—'
          const elevStr = elevVal != null ? `${Math.round(elevVal)} m` : '—'
          return `<div style="font-size:11px;line-height:1.7"><div style="color:#9ca3af">${kmStr} km</div><div><strong style="color:#f9fafb">${elevStr}</strong></div></div>`
        },
      },
      brush: {
        xAxisIndex: 0,
        throttleType: 'debounce',
        throttleDelay: 30,
        transformable: true,
        brushType: 'lineX',
        brushMode: 'single',
        inBrush: { colorAlpha: 1 },
        outOfBrush: { colorAlpha: 0.3 },
        brushStyle: {
          borderWidth: 1,
          borderColor: 'rgba(249,115,22,0.6)',
          color: 'rgba(249,115,22,0.12)',
        },
      },
      toolbox: { show: false },
      series: [{
        type: 'line',
        data: elevationSeries.map((y, i) => [cumulativeKm[i] ?? i, y ?? 0]),
        showSymbol: false,
        smooth: 0.2,
        lineStyle: { color: '#22c55e', width: 1.5 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(34,197,94,0.35)' },
            { offset: 0.6, color: 'rgba(34,197,94,0.12)' },
            { offset: 1, color: 'rgba(34,197,94,0.02)' },
          ]),
        },
        emphasis: { disabled: true },
      }],
    })
    bindEcharts({
      on: ec.on.bind(ec),
      off: ec.off.bind(ec),
      dispatchAction: ec.dispatchAction.bind(ec),
    })
    return () => { ec.dispose(); echartsInstanceRef.current = null }
  }, [trackPoints, hasElevation, elevationSeries, cumulativeKm, bindEcharts])

  // Reflect external range changes back to the brush visual.
  // lastDispatchedRangeRef prevents a dispatch loop when the brush change itself
  // triggers an onRangeChange callback that sets the same range value. (#9)
  const lastDispatchedRangeRef = useRef<[number, number] | null>(null)
  useEffect(() => {
    const ec = echartsInstanceRef.current
    if (!ec) return
    const last = lastDispatchedRangeRef.current
    if (last && last[0] === range[0] && last[1] === range[1]) return
    lastDispatchedRangeRef.current = [range[0], range[1]]
    ec.dispatchAction({
      type: 'brush',
      areas: [{ brushType: 'lineX', coordRange: [cumulativeKm[range[0]] ?? range[0], cumulativeKm[range[1]] ?? range[1]], xAxisIndex: 0 }],
    })
  }, [range, cumulativeKm])

  const startKm = cumulativeKm[range[0]]?.toFixed(1) ?? '0.0'
  const endKm = cumulativeKm[range[1]]?.toFixed(1) ?? '0.0'

  // Keyboard handler: arrow-keys on focused handle buttons.
  const startRef = useRef<HTMLButtonElement>(null)
  const endRef = useRef<HTMLButtonElement>(null)
  const handleKeyDown = (which: 'start' | 'end') => (e: React.KeyboardEvent) => {
    const cur = which === 'start' ? range[0] : range[1]
    const setter = which === 'start' ? setStart : setEnd
    const step = e.shiftKey ? 10 : (e.key === 'PageUp' || e.key === 'PageDown' ? 100 : 1)
    if (e.key === 'ArrowLeft' || e.key === 'PageUp') { e.preventDefault(); setter(cur - step); onCommitRange(which === 'start' ? [cur - step, range[1]] : [range[0], cur - step]) }
    if (e.key === 'ArrowRight' || e.key === 'PageDown') { e.preventDefault(); setter(cur + step); onCommitRange(which === 'start' ? [cur + step, range[1]] : [range[0], cur + step]) }
    if (e.key === 'Home') { e.preventDefault(); setter(0) }
    if (e.key === 'End') { e.preventDefault(); setter(trackPoints.length - 1) }
  }

  // Dual-range slider fill positioning (start% → end%)
  const total = Math.max(1, trackPoints.length - 1)
  const startPct = (range[0] / total) * 100
  const endPct = (range[1] / total) * 100

  return (
    <div className="border-t border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900 px-3 pt-2 pb-2">
      {/* Row 1 — Elevation brush chart */}
      <div className="mb-1.5 relative">
        <div ref={chartRef} className="h-[72px] w-full" />
        {!hasElevation && (
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <span className="text-xs text-neutral-400 dark:text-neutral-600">Keine Höhendaten</span>
          </div>
        )}
      </div>

      {/* Row 2 — Range slider with correct fill positioning */}
      <div className="mb-2 flex items-center gap-2">
        <span className="w-14 shrink-0 font-mono text-xs text-neutral-500 dark:text-neutral-500">{startKm} km</span>
        <div className="relative flex-1 h-5">
          {/* Track background */}
          <div className="absolute top-1/2 left-0 right-0 -translate-y-1/2 h-1.5 rounded-full bg-neutral-200 dark:bg-neutral-700" />
          {/* Selected fill — positioned from startPct% to endPct% */}
          <div
            className="absolute top-1/2 -translate-y-1/2 h-1.5 rounded-full bg-accent-DEFAULT"
            style={{ left: `${startPct}%`, right: `${100 - endPct}%` }}
          />
          {/* Start handle */}
          <input
            type="range"
            min={0} max={trackPoints.length - 1}
            value={range[0]}
            onChange={e => setStart(Number(e.target.value))}
            onMouseUp={() => onCommitRange(range)}
            onKeyUp={() => onCommitRange(range)}
            style={{ '--thumb-color': '#F97316' } as React.CSSProperties}
            className="absolute inset-0 w-full cursor-pointer appearance-none bg-transparent [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:h-4 [&::-webkit-slider-thumb]:w-4 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-accent-DEFAULT [&::-webkit-slider-thumb]:shadow-sm [&::-webkit-slider-thumb]:cursor-pointer [&::-moz-range-thumb]:h-4 [&::-moz-range-thumb]:w-4 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:bg-accent-DEFAULT [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:cursor-pointer [&::-webkit-slider-runnable-track]:appearance-none [&::-webkit-slider-runnable-track]:bg-transparent [&::-moz-range-track]:bg-transparent"
            aria-label="Start-Punkt"
            aria-valuemin={0} aria-valuemax={trackPoints.length - 1} aria-valuenow={range[0]}
          />
          {/* End handle */}
          <input
            type="range"
            min={0} max={trackPoints.length - 1}
            value={range[1]}
            onChange={e => setEnd(Number(e.target.value))}
            onMouseUp={() => onCommitRange(range)}
            onKeyUp={() => onCommitRange(range)}
            className="absolute inset-0 w-full cursor-pointer appearance-none bg-transparent [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:h-4 [&::-webkit-slider-thumb]:w-4 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-accent-DEFAULT [&::-webkit-slider-thumb]:shadow-sm [&::-webkit-slider-thumb]:cursor-pointer [&::-moz-range-thumb]:h-4 [&::-moz-range-thumb]:w-4 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:bg-accent-DEFAULT [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:cursor-pointer [&::-webkit-slider-runnable-track]:appearance-none [&::-webkit-slider-runnable-track]:bg-transparent [&::-moz-range-track]:bg-transparent"
            aria-label="Ende-Punkt"
            aria-valuemin={0} aria-valuemax={trackPoints.length - 1} aria-valuenow={range[1]}
          />
        </div>
        <span className="w-14 shrink-0 text-right font-mono text-xs text-neutral-500 dark:text-neutral-500">{endKm} km</span>
      </div>

      {/* Row 3 — Stepper controls + action buttons */}
      <div className="flex items-center gap-2">
        <PillGroup
          label="Start"
          value={range[0]}
          onPrev={() => setStart(range[0] - 1)}
          onNext={() => setStart(range[0] + 1)}
          handleRef={startRef}
          onKeyDown={handleKeyDown('start')}
        />
        <PillGroup
          label="Ende"
          value={range[1]}
          onPrev={() => setEnd(range[1] - 1)}
          onNext={() => setEnd(range[1] + 1)}
          handleRef={endRef}
          onKeyDown={handleKeyDown('end')}
        />
        <span className="ml-1 text-xs text-neutral-400 dark:text-neutral-600">← → fein · ⇧ ×10</span>

        <div className="flex-1" />

        {/* Reverse */}
        <button
          type="button"
          onClick={onReverse}
          title="Richtung umkehren"
          className={cn(
            'flex items-center gap-1.5 rounded-md px-2 py-1 text-xs',
            'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300',
            'border border-neutral-200 dark:border-neutral-700',
            'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-white',
          )}
        >
          <ArrowLeftRight className="size-3.5" />
          <span>Umkehren</span>
        </button>

        {/* Reset */}
        <button
          type="button"
          onClick={onReset}
          title="Auf gespeicherten Stand zurücksetzen"
          className={cn(
            'rounded-md px-2 py-1 text-xs',
            'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400',
            'border border-neutral-200 dark:border-neutral-700',
            'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-700 dark:hover:text-neutral-300',
          )}
        >
          Reset
        </button>

        {/* Primary CTA — orange accent */}
        <button
          type="button"
          onClick={onPreviewOrSave}
          className={cn(
            'rounded-md px-3 py-1 text-xs font-medium text-white transition-colors shadow-sm',
            'bg-accent-DEFAULT hover:bg-accent-600',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1',
          )}
        >
          {isPreview ? 'Speichern' : 'Vorschau →'}
        </button>
      </div>
    </div>
  )
}

function PillGroup({ label, value, onPrev, onNext, handleRef, onKeyDown }: {
  label: string
  value: number
  onPrev: () => void
  onNext: () => void
  handleRef: React.RefObject<HTMLButtonElement>
  onKeyDown: (e: React.KeyboardEvent) => void
}) {
  return (
    <div className="flex items-center gap-0.5 rounded-md border border-neutral-200 dark:border-neutral-700 bg-neutral-100 dark:bg-neutral-800 px-1.5 py-1">
      <span className="mr-1 text-xs text-neutral-500 dark:text-neutral-500">{label}</span>
      <button
        type="button"
        onClick={onPrev}
        aria-label={`${label} minus 1`}
        className="flex size-5 items-center justify-center rounded text-neutral-500 dark:text-neutral-400 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-white"
      >
        ‹
      </button>
      <button
        ref={handleRef}
        type="button"
        onKeyDown={onKeyDown}
        className="min-w-[3.5ch] px-0.5 text-center font-mono text-xs text-neutral-700 dark:text-neutral-200 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent-DEFAULT"
      >
        {value}
      </button>
      <button
        type="button"
        onClick={onNext}
        aria-label={`${label} plus 1`}
        className="flex size-5 items-center justify-center rounded text-neutral-500 dark:text-neutral-400 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-white"
      >
        ›
      </button>
    </div>
  )
}

function haversineKm(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const R = 6371
  const dLat = (lat2 - lat1) * Math.PI / 180
  const dLng = (lng2 - lng1) * Math.PI / 180
  const a = Math.sin(dLat/2)**2 + Math.cos(lat1*Math.PI/180)*Math.cos(lat2*Math.PI/180)*Math.sin(dLng/2)**2
  return 2 * R * Math.asin(Math.min(1, Math.sqrt(a)))
}
