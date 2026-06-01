import type * as echarts from 'echarts/core'
import type { TFunction } from 'i18next'

type EChartsInstance = echarts.ECharts

type BrushArea = {
  range?: [number, number]
  coordRange?: [number, number]
}

interface BrushZoomOptions {
  dataZoomIndexes?: number[]
  minValue?: number
  maxValue?: number
  roundValues?: boolean
}

export const lineXBrush = {
  toolbox: [],
  brushType: 'lineX',
  brushStyle: {
    borderWidth: 1,
    color: 'rgba(59,130,246,0.12)',
    borderColor: 'rgba(59,130,246,0.6)',
  },
  throttleType: 'debounce',
  throttleDelay: 100,
}

// The toolbox action must emit brushEnd; the handler below converts that range
// to a dataZoom action for the chart's x-axis.
export function lineXBrushToolboxFeature(t: TFunction) {
  return {
    show: true,
    type: ['lineX', 'clear'],
    title: {
      lineX: t('analyticsTrainingStats.chartBrushZoom.brushTitle'),
      clear: t('analyticsTrainingStats.chartBrushZoom.clearTitle'),
    },
  }
}

export function lineXBrushRestoreToolboxFeature(t: TFunction) {
  return {
    title: t('analyticsTrainingStats.chartBrushZoom.restoreTitle'),
  }
}

export function lineXBrushToolboxFeatures(t: TFunction) {
  return {
    brush: lineXBrushToolboxFeature(t),
    restore: lineXBrushRestoreToolboxFeature(t),
  }
}

export function activateLineXBrush(chart: EChartsInstance) {
  chart.dispatchAction({
    type: 'takeGlobalCursor',
    key: 'brush',
    brushOption: { brushType: 'lineX', brushMode: 'single' },
  })
}

export function bindLineXBrushZoom(
  chart: EChartsInstance,
  options: BrushZoomOptions = {},
): () => void {
  // Activate brush on mount so the chart starts in zoom-ready state — drag
  // immediately works and the cursor shows crosshair. Without this the user
  // would have to click the toolbox brush icon first.
  activateLineXBrush(chart)

  const dataZoomIndexes = options.dataZoomIndexes ?? [0]
  const roundValues = options.roundValues ?? true
  let brushZoomLock = false
  let disposed = false

  const rearmLineXBrush = () => {
    requestAnimationFrame(() => {
      if (!disposed) activateLineXBrush(chart)
    })
  }

  const clamp = (value: number) => {
    let next = value
    if (options.minValue != null) next = Math.max(options.minValue, next)
    if (options.maxValue != null) next = Math.min(options.maxValue, next)
    return next
  }

  const handleBrushEnd = (params: unknown) => {
    if (brushZoomLock) return
    brushZoomLock = true

    const p = params as { areas?: BrushArea[]; batch?: Array<{ areas?: BrushArea[] }> }
    const areas = p.areas ?? p.batch?.[0]?.areas
    const area = areas?.[0]
    if (!area) {
      brushZoomLock = false
      return
    }

    let startValue: number
    let endValue: number

    if (area.coordRange) {
      ;[startValue, endValue] = area.coordRange
    } else if (area.range) {
      const start = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[0])
      const end = chart.convertFromPixel({ xAxisIndex: 0 }, area.range[1])
      if (start == null || end == null) {
        brushZoomLock = false
        return
      }
      startValue = Number(start)
      endValue = Number(end)
    } else {
      brushZoomLock = false
      return
    }

    if (!Number.isFinite(startValue) || !Number.isFinite(endValue)) {
      brushZoomLock = false
      return
    }
    if (roundValues) {
      startValue = Math.round(startValue)
      endValue = Math.round(endValue)
    }
    if (endValue < startValue) {
      ;[startValue, endValue] = [endValue, startValue]
    }

    startValue = clamp(startValue)
    endValue = clamp(endValue)
    if (endValue <= startValue) {
      brushZoomLock = false
      return
    }

    for (const dataZoomIndex of dataZoomIndexes) {
      chart.dispatchAction({
        type: 'dataZoom',
        dataZoomIndex,
        startValue,
        endValue,
      })
    }
    chart.dispatchAction({ type: 'brush', command: 'clear', areas: [] })
    rearmLineXBrush()

    requestAnimationFrame(() => {
      brushZoomLock = false
    })
  }

  const handleDataZoom = () => {
    if (!brushZoomLock) rearmLineXBrush()
  }

  // Restore (toolbox reset button) emits its own event without a dataZoom
  // dispatch, so we rearm explicitly here too — otherwise the cursor stays
  // on the default pointer after reset.
  const handleRestore = () => {
    rearmLineXBrush()
  }

  chart.on('brushEnd', handleBrushEnd)
  chart.on('dataZoom', handleDataZoom)
  chart.on('restore', handleRestore)

  return () => {
    disposed = true
    chart.off('brushEnd', handleBrushEnd)
    chart.off('dataZoom', handleDataZoom)
    chart.off('restore', handleRestore)
  }
}
