import { useCallback, useRef, useState } from 'react'

export interface UseRangeBrushOptions {
  totalPoints: number
  initial?: [number, number]
  onBrushChange?: (range: [number, number]) => void
  /**
   * Optional function that converts an ECharts X-axis coordinate value
   * (e.g. cumulative km) to a track-point index.
   * When provided, brushEnd coordRange values are converted via this function
   * instead of being rounded directly (which would be wrong when X = km).
   * dataZoom startValue/endValue are also converted via this function.
   */
  coordToIndex?: (coord: number) => number
}

export interface UseRangeBrushResult {
  range: [number, number]
  setRange: (r: [number, number]) => void
  setStart: (i: number) => void
  setEnd: (i: number) => void
  bindEcharts: (ec: EchartsLike | null) => void  // attaches brush + dataZoom handlers
}

// Declare the ECharts type loosely — we only use event-binding + dispatchAction.
type EchartsLike = {
  on: (ev: string, cb: (p: unknown) => void) => void
  off: (ev: string) => void
  dispatchAction: (a: unknown) => void
}

// NOTE (Batch-5-Kritiker MIN22): clampRange für total<=1 liefert [0,0], also ein
// "Punkt"-Range — kein UI-Sinn. Die Modal-Ebene MUSS den Editor bei trackPoints.length < 2
// gar nicht erst öffnen (Caller-Precondition in ActivitySegments/SegmentEditorModal:
// `if (trackPoints.length < 2) { addToast({type: 'warning', title: 'Zu wenig Trackpunkte'}); return }`).
// clampRange defensiv auf [0,0] fallen lassen, damit der Hook nicht crasht.
function clampRange(r: [number, number], total: number): [number, number] {
  if (total <= 1) return [0, 0]
  let [s, e] = r
  s = Math.max(0, Math.min(s, total - 2))
  e = Math.max(s + 1, Math.min(e, total - 1))
  return [s, e]
}

export function useRangeBrush(opts: UseRangeBrushOptions): UseRangeBrushResult {
  const { totalPoints, initial, onBrushChange, coordToIndex } = opts
  const [range, setRangeState] = useState<[number, number]>(
    clampRange(initial ?? [0, totalPoints - 1], totalPoints),
  )
  const echartsRef = useRef<EchartsLike | null>(null)
  const onBrushChangeRef = useRef(onBrushChange)
  onBrushChangeRef.current = onBrushChange
  const coordToIndexRef = useRef(coordToIndex)
  coordToIndexRef.current = coordToIndex

  const setRange = useCallback(
    (r: [number, number]) => {
      const clamped = clampRange(r, totalPoints)
      setRangeState(clamped)
      onBrushChangeRef.current?.(clamped)
    },
    [totalPoints],
  )

  const setStart = useCallback(
    (i: number) => {
      setRangeState(prev => {
        // Clamp i to valid range, but preserve prev[1] (end does not move).
        // start must be in [0, end-1].
        const end = prev[1]
        const s = Math.max(0, Math.min(i, end - 1))
        const next: [number, number] = [s, end]
        onBrushChangeRef.current?.(next)
        return next
      })
    },
    [],
  )

  const setEnd = useCallback(
    (i: number) => {
      setRangeState(prev => {
        // Clamp i to valid range, but preserve prev[0] (start does not move).
        // end must be in [start+1, totalPoints-1].
        const start = prev[0]
        const e = Math.max(start + 1, Math.min(i, totalPoints - 1))
        const next: [number, number] = [start, e]
        onBrushChangeRef.current?.(next)
        return next
      })
    },
    [totalPoints],
  )

  const bindEcharts = useCallback(
    (ec: EchartsLike | null) => {
      // Cleanup previous binding.
      if (echartsRef.current && echartsRef.current !== ec) {
        echartsRef.current.off('brushEnd')
        echartsRef.current.off('dataZoom')
      }
      echartsRef.current = ec
      if (!ec) return

      ec.on('brushEnd', (p: unknown) => {
        const params = p as { areas?: Array<{ coordRange?: [number, number] }> }
        const area = params?.areas?.[0]
        if (area?.coordRange) {
          const toIdx = coordToIndexRef.current
          if (toIdx) {
            // X-axis uses a custom coordinate (e.g. km) — convert to index.
            setRange([toIdx(area.coordRange[0]), toIdx(area.coordRange[1])])
          } else {
            // X-axis uses indices directly.
            setRange([Math.round(area.coordRange[0]), Math.round(area.coordRange[1])])
          }
        }
      })

      ec.on('dataZoom', (p: unknown) => {
        const params = p as { batch?: Array<{ startValue?: number; endValue?: number }> }
        const first = params?.batch?.[0]
        if (first && first.startValue != null && first.endValue != null) {
          const toIdx = coordToIndexRef.current
          if (toIdx) {
            setRange([toIdx(first.startValue), toIdx(first.endValue)])
          } else {
            setRange([Math.round(first.startValue), Math.round(first.endValue)])
          }
        }
      })
    },
    [setRange],
  )

  return { range, setRange, setStart, setEnd, bindEcharts }
}
