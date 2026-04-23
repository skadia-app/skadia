// frontend/src/components/segment-editor/TrackModeBottomBar.tsx
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  BrushComponent,
  ToolboxComponent,
  TooltipComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { ArrowLeftRight, ZoomIn } from 'lucide-react'
import { useRangeBrush } from '@/hooks/useRangeBrush'
import { cn } from '@/lib/utils'

echarts.use([LineChart, GridComponent, BrushComponent, ToolboxComponent, TooltipComponent, DataZoomComponent, CanvasRenderer])

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
  /** Resets only the geometry selection (range). Form values are preserved. */
  onReset: () => void
  /** Resets everything: geometry selection + all form fields (Fix #12). */
  onResetAll: () => void
  onPreviewOrSave: () => void
}

export function TrackModeBottomBar(props: TrackModeBottomBarProps) {
  const { trackPoints, range, onRangeChange, onCommitRange, onReverse, onReset, onResetAll, onPreviewOrSave } = props
  const chartRef = useRef<HTMLDivElement | null>(null)
  const echartsInstanceRef = useRef<echarts.ECharts | null>(null)
  // Inline confirm state for "Alles zurücksetzen" (Fix #12)
  const [confirmResetAll, setConfirmResetAll] = useState(false)
  // Whether X-axis is currently zoomed (for "Zoom zurücksetzen" button visibility)
  const [isZoomed, setIsZoomed] = useState(false)

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

  // Stable ref for onCommitRange so the brushEnd closure inside useEffect doesn't
  // capture a stale version of the prop. Must be updated every render.
  const onCommitRangeRefCapture = useRef(onCommitRange)
  onCommitRangeRefCapture.current = onCommitRange

  // coordToIndex: converts km X-axis value → nearest track-point index.
  // Binary-search on the cumulativeKm array for O(log n) lookup.
  const coordToIndex = useCallback((coord: number): number => {
    const km = cumulativeKm
    if (km.length === 0) return 0
    if (coord <= km[0]) return 0
    if (coord >= km[km.length - 1]) return km.length - 1
    let lo = 0; let hi = km.length - 1
    while (lo < hi - 1) {
      const mid = (lo + hi) >> 1
      if (km[mid] <= coord) lo = mid; else hi = mid
    }
    // Return whichever of lo/hi is closer
    return (coord - km[lo]) < (km[hi] - coord) ? lo : hi
  }, [cumulativeKm])

  const { bindEcharts, setStart, setEnd } = useRangeBrush({
    totalPoints: trackPoints.length,
    initial: range,
    onBrushChange: (r) => {
      onRangeChange(r)
      // NOTE: map sync (onCommitRange) is triggered separately:
      //   - brush: called after brushEnd (which is debounced at 30 ms)
      //   - slider: called on pointerUp via the DualRangeSlider onCommit prop
      //   - keyboard: called explicitly in handleKeyDown
    },
    coordToIndex,
  })

  // Init ECharts — polished per RouteProfileChart reference.
  useEffect(() => {
    if (!chartRef.current) return
    const ec = echarts.init(chartRef.current, undefined, { renderer: 'canvas' })
    echartsInstanceRef.current = ec

    // Y-axis range: tight auto-scale so small elevation changes are visible.
    // Padding = 5 % of the actual elevation range (min 2 m) so even flat tracks
    // get a sensible band instead of a 40-m fixed window.
    const validElev = elevationSeries.filter((e): e is number => e != null)
    const rawMin = validElev.length > 0 ? Math.min(...validElev) : 0
    const rawMax = validElev.length > 0 ? Math.max(...validElev) : 10
    const rangePad = Math.max(2, (rawMax - rawMin) * 0.05)
    const yMin = validElev.length > 0 ? Math.floor(rawMin - rangePad) : 0
    const yMax = validElev.length > 0 ? Math.ceil(rawMax + rangePad) : 10
    const totalKm = trackPoints.length > 0 ? cumulativeKm[trackPoints.length - 1] : 1

    ec.setOption({
      backgroundColor: 'transparent',
      animation: false,
      grid: { top: 8, right: 12, bottom: 22, left: 44, containLabel: false },
      xAxis: {
        type: 'value',
        min: 0,
        max: totalKm,
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
        // transformable: true allows the brush selection to be dragged and resized
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
      // DataZoom component: allows X-axis to zoom into the selected brush region.
      // The 'inside' type enables mouse-wheel / touchpad zoom on the chart area.
      dataZoom: [
        {
          type: 'inside',
          xAxisIndex: 0,
          filterMode: 'none',
          zoomOnMouseWheel: false,
          moveOnMouseMove: false,
          moveOnMouseWheel: false,
        },
      ],
      toolbox: { show: false },
      series: [{
        type: 'line',
        data: elevationSeries.map((y, i) => [cumulativeKm[i] ?? i, y ?? 0]),
        showSymbol: false,
        smooth: 0.2,
        // D2: orange accent line to match sub-segment color on map
        lineStyle: { color: '#F97316', width: 2.0 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(249,115,22,0.35)' },
            { offset: 0.6, color: 'rgba(249,115,22,0.12)' },
            { offset: 1, color: 'rgba(249,115,22,0.02)' },
          ]),
        },
        emphasis: { disabled: true },
      }],
    })

    // After brush ends: (1) zoom X-axis to selection, (2) sync map via onCommitRange.
    // Note: brushEnd fires after the debounce delay (30 ms) so this is not called
    // on every frame during drag — only when the user lifts the pointer.
    const onCommitRangeRef = onCommitRangeRefCapture
    ec.on('brushEnd', (p: unknown) => {
      const params = p as { areas?: Array<{ coordRange?: [number, number] }> }
      const area = params?.areas?.[0]
      if (area?.coordRange) {
        const [startKm, endKm] = area.coordRange
        // Add a small padding around the selection so the brush border is visible
        const pad = (endKm - startKm) * 0.08
        const zStart = ((startKm - pad) / totalKm) * 100
        const zEnd = ((endKm + pad) / totalKm) * 100
        ec.dispatchAction({
          type: 'dataZoom',
          dataZoomIndex: 0,
          start: Math.max(0, zStart),
          end: Math.min(100, zEnd),
        })
        setIsZoomed(true)
        // Sync map: convert km values back to indices and fitBounds
        const startIdx = coordToIndex(startKm)
        const endIdx = coordToIndex(endKm)
        onCommitRangeRef.current?.([startIdx, endIdx])
      }
    })

    bindEcharts({
      on: ec.on.bind(ec),
      off: ec.off.bind(ec),
      dispatchAction: ec.dispatchAction.bind(ec) as (a: unknown) => void,
    })
    return () => { ec.dispose(); echartsInstanceRef.current = null }
  }, [trackPoints, hasElevation, elevationSeries, cumulativeKm, bindEcharts, coordToIndex])

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

  // Reset X-axis zoom back to full view
  const handleZoomReset = useCallback(() => {
    const ec = echartsInstanceRef.current
    if (!ec) return
    ec.dispatchAction({ type: 'dataZoom', dataZoomIndex: 0, start: 0, end: 100 })
    setIsZoomed(false)
  }, [])

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

  return (
    <div className="border-t border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900 px-3 pt-2 pb-2">
      {/* Row 1 — Elevation brush chart (D1: height 116px, section heading) */}
      <div className="mb-1.5">
        <div className="flex items-center gap-2 px-0 pb-1">
          <span className="text-[10px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">
            Höhenprofil
          </span>
          <span className="text-[10px] text-neutral-400 dark:text-neutral-600">
            · Brush ziehen = Auswahl · Brush verschieben = Position · Scroll = Zoom
          </span>
          <div className="flex-1" />
          {isZoomed && (
            <button
              type="button"
              onClick={handleZoomReset}
              title="Zoom zurücksetzen"
              className={cn(
                'flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px]',
                'bg-neutral-200 dark:bg-neutral-700 text-neutral-600 dark:text-neutral-300',
                'hover:bg-neutral-300 dark:hover:bg-neutral-600 transition-colors',
              )}
            >
              <ZoomIn className="size-2.5" />
              <span>Zoom zurück</span>
            </button>
          )}
        </div>
        <div className="relative">
          <div ref={chartRef} className="h-[116px] w-full" />
          {!hasElevation && (
            <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
              <span className="text-xs text-neutral-400 dark:text-neutral-600">Keine Höhendaten</span>
            </div>
          )}
        </div>
      </div>

      {/* Row 2 — Pointer-capture dual-thumb range slider (fix #14: no stacked inputs) */}
      {/* setStart/setEnd from useRangeBrush fire onBrushChange → onRangeChange (state update).
          onCommitRange is called on pointerUp via DualRangeSlider.onCommit (fitBounds). */}
      <div className="mb-2 flex items-center gap-2">
        <span className="w-14 shrink-0 font-mono text-xs text-neutral-500 dark:text-neutral-500">{startKm} km</span>
        <DualRangeSlider
          min={0}
          max={trackPoints.length - 1}
          start={range[0]}
          end={range[1]}
          onStartChange={setStart}
          onEndChange={setEnd}
          onCommit={onCommitRange}
        />
        <span className="w-14 shrink-0 text-right font-mono text-xs text-neutral-500 dark:text-neutral-500">{endKm} km</span>
      </div>

      {/* Row 3 — Stepper controls + action buttons */}
      <div className="flex items-center gap-2">
        <PillGroup
          label="Start"
          value={range[0]}
          totalPoints={trackPoints.length}
          onPrev={() => setStart(range[0] - 1)}
          onNext={() => setStart(range[0] + 1)}
          handleRef={startRef}
          onKeyDown={handleKeyDown('start')}
        />
        <PillGroup
          label="Ende"
          value={range[1]}
          totalPoints={trackPoints.length}
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

        {/* Reset geometry only (Fix #12 — separate from full reset) */}
        <button
          type="button"
          onClick={onReset}
          title="Nur Geometrie-Auswahl zurücksetzen (Formularwerte bleiben)"
          className={cn(
            'rounded-md px-2 py-1 text-xs',
            'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400',
            'border border-neutral-200 dark:border-neutral-700',
            'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-700 dark:hover:text-neutral-300',
          )}
        >
          Geometrie
        </button>

        {/* Reset all: geometry + form fields, with inline confirm (Fix #12) */}
        {confirmResetAll ? (
          <span className="flex items-center gap-1 text-xs">
            <span className="shrink-0 text-neutral-600 dark:text-neutral-300">Alles zurücksetzen?</span>
            <button
              type="button"
              onClick={() => { setConfirmResetAll(false); onResetAll() }}
              className="rounded-md border border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/40 px-2 py-1 text-xs font-medium text-red-700 dark:text-red-300 transition-colors hover:bg-red-100 dark:hover:bg-red-900/60"
            >
              Ja
            </button>
            <button
              type="button"
              onClick={() => setConfirmResetAll(false)}
              className="rounded-md border border-neutral-200 dark:border-neutral-700 bg-neutral-100 dark:bg-neutral-800 px-2 py-1 text-xs text-neutral-600 dark:text-neutral-300 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700"
            >
              Nein
            </button>
          </span>
        ) : (
          <button
            type="button"
            onClick={() => setConfirmResetAll(true)}
            title="Geometrie und alle Formularwerte zurücksetzen"
            className={cn(
              'rounded-md px-2 py-1 text-xs',
              'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400',
              'border border-neutral-200 dark:border-neutral-700',
              'transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-700 dark:hover:text-neutral-300',
            )}
          >
            Alles
          </button>
        )}

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
          Speichern
        </button>
      </div>
    </div>
  )
}

// ── DualRangeSlider ───────────────────────────────────────────────────────────
// Pure pointer-capture dual-thumb slider. No stacked <input> elements.
// Each thumb tracks its own pointer via setPointerCapture so thumbs are
// independently draggable even when overlapping (#14 root cause fix).
//
// Design choices:
// - onStartChange / onEndChange fire continuously during drag (for live preview)
// - onCommit fires once on pointerUp with the final [start, end] pair (for map fitBounds)
// - Both thumbs use setPointerCapture so they stay under the finger even if the
//   pointer temporarily leaves the thumb element during fast movement.
interface DualRangeSliderProps {
  min: number
  max: number
  start: number
  end: number
  onStartChange: (v: number) => void
  onEndChange: (v: number) => void
  /** Called once on pointerUp with the committed [start, end] pair. Used to trigger map fitBounds. */
  onCommit?: (r: [number, number]) => void
}

function DualRangeSlider({ min, max, start, end, onStartChange, onEndChange, onCommit }: DualRangeSliderProps) {
  const trackRef = useRef<HTMLDivElement>(null)
  // Keep refs to latest start/end so onUp closure can read committed values
  const startRef = useRef(start)
  const endRef = useRef(end)
  startRef.current = start
  endRef.current = end

  const total = Math.max(1, max - min)
  const startPct = ((start - min) / total) * 100
  const endPct = ((end - min) / total) * 100

  // Convert pixel offset within the track to a clamped integer value.
  const pxToValue = useCallback((clientX: number): number => {
    const el = trackRef.current
    if (!el) return min
    const rect = el.getBoundingClientRect()
    const pct = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width))
    return Math.round(min + pct * total)
  }, [min, total])

  // Start thumb pointer handling
  const handleStartPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault()
    const el = e.currentTarget
    el.setPointerCapture(e.pointerId)
    const onMove = (ev: PointerEvent) => {
      const v = pxToValue(ev.clientX)
      // start must stay < end
      onStartChange(Math.min(v, endRef.current - 1))
    }
    const onUp = () => {
      el.removeEventListener('pointermove', onMove)
      el.removeEventListener('pointerup', onUp)
      onCommit?.([startRef.current, endRef.current])
    }
    el.addEventListener('pointermove', onMove)
    el.addEventListener('pointerup', onUp)
  }, [onStartChange, onCommit, pxToValue])

  // End thumb pointer handling
  const handleEndPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault()
    const el = e.currentTarget
    el.setPointerCapture(e.pointerId)
    const onMove = (ev: PointerEvent) => {
      const v = pxToValue(ev.clientX)
      // end must stay > start
      onEndChange(Math.max(v, startRef.current + 1))
    }
    const onUp = () => {
      el.removeEventListener('pointermove', onMove)
      el.removeEventListener('pointerup', onUp)
      onCommit?.([startRef.current, endRef.current])
    }
    el.addEventListener('pointermove', onMove)
    el.addEventListener('pointerup', onUp)
  }, [onEndChange, onCommit, pxToValue])

  return (
    <div ref={trackRef} className="relative flex-1 h-5 select-none touch-none">
      {/* Track background */}
      <div className="absolute top-1/2 left-0 right-0 -translate-y-1/2 h-1.5 rounded-full bg-neutral-200 dark:bg-neutral-700" />
      {/* Selected fill */}
      <div
        className="absolute top-1/2 -translate-y-1/2 h-1.5 rounded-full bg-accent-DEFAULT"
        style={{ left: `${startPct}%`, right: `${100 - endPct}%` }}
      />
      {/* Start thumb — positioned absolutely, uses pointer capture for independent drag */}
      <div
        role="slider"
        aria-label="Start-Punkt"
        aria-valuemin={min}
        aria-valuemax={max}
        aria-valuenow={start}
        tabIndex={0}
        className={cn(
          'absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-4 h-4',
          'rounded-full bg-accent-DEFAULT border-2 border-white dark:border-neutral-900',
          'shadow-md cursor-grab active:cursor-grabbing',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1',
          'transition-shadow hover:shadow-lg',
        )}
        style={{ left: `${startPct}%`, zIndex: 10 }}
        onPointerDown={handleStartPointerDown}
        onKeyDown={(e) => {
          if (e.key === 'ArrowLeft') { e.preventDefault(); const v = Math.max(min, start - (e.shiftKey ? 10 : 1)); onStartChange(v); onCommit?.([v, endRef.current]) }
          if (e.key === 'ArrowRight') { e.preventDefault(); const v = Math.min(start + (e.shiftKey ? 10 : 1), end - 1); onStartChange(v); onCommit?.([v, endRef.current]) }
        }}
      />
      {/* End thumb */}
      <div
        role="slider"
        aria-label="Ende-Punkt"
        aria-valuemin={min}
        aria-valuemax={max}
        aria-valuenow={end}
        tabIndex={0}
        className={cn(
          'absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-4 h-4',
          'rounded-full bg-accent-DEFAULT border-2 border-white dark:border-neutral-900',
          'shadow-md cursor-grab active:cursor-grabbing',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1',
          'transition-shadow hover:shadow-lg',
        )}
        style={{ left: `${endPct}%`, zIndex: 10 }}
        onPointerDown={handleEndPointerDown}
        onKeyDown={(e) => {
          if (e.key === 'ArrowLeft') { e.preventDefault(); const v = Math.max(start + 1, end - (e.shiftKey ? 10 : 1)); onEndChange(v); onCommit?.([startRef.current, v]) }
          if (e.key === 'ArrowRight') { e.preventDefault(); const v = Math.min(max, end + (e.shiftKey ? 10 : 1)); onEndChange(v); onCommit?.([startRef.current, v]) }
        }}
      />
    </div>
  )
}

function PillGroup({ label, value, totalPoints, onPrev, onNext, handleRef, onKeyDown }: {
  label: string
  value: number
  totalPoints: number
  onPrev: () => void
  onNext: () => void
  handleRef: React.RefObject<HTMLButtonElement | null>
  onKeyDown: (e: React.KeyboardEvent) => void
}) {
  return (
    <div className="flex items-center gap-0.5 rounded-md border border-neutral-200 dark:border-neutral-700 bg-neutral-100 dark:bg-neutral-800 px-1.5 py-1">
      <span className="mr-1 text-xs text-neutral-500 dark:text-neutral-500">{label}</span>
      <button
        type="button"
        onClick={onPrev}
        aria-label={`${label} minus 1`}
        className="flex size-5 items-center justify-center rounded text-neutral-500 dark:text-neutral-400 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-white focus-visible:ring-1 focus-visible:ring-accent-DEFAULT focus-visible:outline-none"
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
        <span className="text-neutral-400 dark:text-neutral-600"> / {totalPoints}</span>
      </button>
      <button
        type="button"
        onClick={onNext}
        aria-label={`${label} plus 1`}
        className="flex size-5 items-center justify-center rounded text-neutral-500 dark:text-neutral-400 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-white focus-visible:ring-1 focus-visible:ring-accent-DEFAULT focus-visible:outline-none"
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
