import { describe, expect, it, vi } from 'vitest'
import { normalizeDataZoomPercentRange, resolveBrushAreaDataRange } from '../ActivityCharts'

describe('ActivityCharts zoom event helpers', () => {
  it('prefers brush pixel range so brushEnd can use pending brush areas', () => {
    const chart = {
      convertFromPixel: vi.fn()
        .mockReturnValueOnce(1.25)
        .mockReturnValueOnce(4.5),
    }

    expect(resolveBrushAreaDataRange(chart, { range: [10, 50], coordRange: [9, 99] })).toEqual([1.25, 4.5])
    expect(chart.convertFromPixel).toHaveBeenCalledTimes(2)
  })

  it('falls back to coordRange when no pixel range is available', () => {
    expect(resolveBrushAreaDataRange({ convertFromPixel: vi.fn() }, { coordRange: [2, 5] })).toEqual([2, 5])
  })

  it('normalizes dataZoom start/end percentages', () => {
    expect(normalizeDataZoomPercentRange({ batch: [{ start: 12, end: 34 }] }, 100)).toEqual([12, 34])
    expect(normalizeDataZoomPercentRange({ start: 20, end: 80 }, 100)).toEqual([20, 80])
  })

  it('normalizes dataZoom value ranges when ECharts omits start/end percentages', () => {
    expect(normalizeDataZoomPercentRange({ batch: [{ startValue: 25, endValue: 75 }] }, 100)).toEqual([25, 75])
  })
})
