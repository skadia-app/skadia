import { useRef, useEffect } from 'react'
import * as echarts from 'echarts/core'

export interface EChartsTheme {
  isDark: boolean
  textColor: string
  gridLineColor: string
}

export function getTheme(): EChartsTheme {
  const isDark = document.documentElement.classList.contains('dark')
  return {
    isDark,
    textColor: isDark ? '#9ca3af' : '#6b7280',
    gridLineColor: isDark ? '#262626' : '#f5f5f5',
  }
}

/**
 * Manages an ECharts instance: init, resize, dark-mode, dispose.
 *
 * @param getOption - Called with theme to produce chart options.
 *   IMPORTANT: If this function closes over external state, include that state in `deps`.
 *   If `getOption` is an inline arrow, it works automatically (new ref on each render).
 *   If `getOption` is a stable/memoized function, external state changes won't trigger re-render.
 * @param deps - Re-render chart when these change.
 * @param onChart - Optional callback receiving the chart instance after init.
 *   Use it to register event handlers, call dispatchAction, set group, etc.
 *   Return an optional cleanup function (called before dispose).
 * @param options - Optional configuration object.
 *   - `onChart`: same as the positional `onChart` parameter; prefer the options form for new callers.
 *   - `enabled`: when `false`, the chart is not initialised (and any existing instance is
 *     disposed). Useful to express "still loading, don't init yet" without conditionally
 *     rendering the container div, which would trigger the removeChild crash on re-mount.
 *     Defaults to `true`.
 *
 * Backwards-compatible: passing `onChart` as the third positional argument still works
 * — all existing callers continue to function unchanged.
 */
export function useECharts(
  getOption: (theme: EChartsTheme) => echarts.EChartsCoreOption,
  deps: React.DependencyList,
  onChartOrOptions?:
    | ((chart: echarts.ECharts) => (() => void) | void)
    | { onChart?: (chart: echarts.ECharts) => (() => void) | void; enabled?: boolean },
) {
  const containerRef = useRef<HTMLDivElement>(null)

  // Resolve the two call-styles into a unified shape.
  const onChart =
    typeof onChartOrOptions === 'function' ? onChartOrOptions : onChartOrOptions?.onChart
  const enabled =
    typeof onChartOrOptions === 'object' && onChartOrOptions !== null && !('call' in onChartOrOptions)
      ? (onChartOrOptions.enabled ?? true)
      : true

  useEffect(() => {
    // Container not yet mounted or caller explicitly disabled init.
    if (!containerRef.current || !enabled) return

    const theme = getTheme()
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chart.setOption(getOption(theme))

    const chartCleanup = onChart?.(chart)

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      chartCleanup?.()
      observer.disconnect()

      // Guard against the case where the container div was already removed from the DOM
      // by a conditional render (e.g. isLoading swaps the div away). ECharts internally
      // calls removeChild during dispose, which throws if the canvas parent is already
      // gone. React has already cleaned up the DOM node, so the chart instance is orphaned
      // and GC will reclaim it — we only need to avoid the crash.
      if (containerRef.current && document.contains(containerRef.current)) {
        chart.dispose()
      } else {
        try {
          chart.dispose()
        } catch (err) {
          console.warn('[useECharts] dispose skipped — container already detached from DOM', err)
        }
      }
    }
  }, deps) // eslint-disable-line react-hooks/exhaustive-deps — NICHT ENTFERNEN: deps werden vom Caller übergeben, getOption ist absichtlich ausgeschlossen (sonst infinite Re-Renders bei inline Arrows)

  return containerRef
}
