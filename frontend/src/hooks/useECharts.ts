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
 * @param getOption - Called with theme to produce chart options.
 *   IMPORTANT: If this function closes over external state, include that state in `deps`.
 *   If `getOption` is an inline arrow, it works automatically (new ref on each render).
 *   If `getOption` is a stable/memoized function, external state changes won't trigger re-render.
 * @param deps - Re-render chart when these change.
 * @param onChart - Optional callback receiving the chart instance after init.
 *   Use it to register event handlers, call dispatchAction, set group, etc.
 *   Return an optional cleanup function (called before dispose).
 */
export function useECharts(
  getOption: (theme: EChartsTheme) => echarts.EChartsCoreOption,
  deps: React.DependencyList,
  onChart?: (chart: echarts.ECharts) => (() => void) | void,
) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const theme = getTheme()
    const chart = echarts.init(containerRef.current, undefined, { renderer: 'canvas' })
    chart.setOption(getOption(theme))

    const chartCleanup = onChart?.(chart)

    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(containerRef.current)

    return () => {
      chartCleanup?.()
      observer.disconnect()
      chart.dispose()
    }
  }, deps) // eslint-disable-line react-hooks/exhaustive-deps — NICHT ENTFERNEN: deps werden vom Caller übergeben, getOption ist absichtlich ausgeschlossen (sonst infinite Re-Renders bei inline Arrows)

  return containerRef
}
