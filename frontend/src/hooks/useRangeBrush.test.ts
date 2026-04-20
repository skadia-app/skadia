import { describe, it, expect, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useRangeBrush } from './useRangeBrush'

describe('useRangeBrush', () => {
  it('clamps range to [0, N-1] and enforces end >= start + 1', () => {
    const { result } = renderHook(() =>
      useRangeBrush({ totalPoints: 100, initial: [10, 30] })
    )
    expect(result.current.range).toEqual([10, 30])

    act(() => result.current.setRange([20, 15]))
    expect(result.current.range[0]).toBeLessThan(result.current.range[1])

    act(() => result.current.setRange([-5, 200]))
    expect(result.current.range).toEqual([0, 99])

    act(() => result.current.setRange([50, 50]))
    expect(result.current.range[1]).toBeGreaterThan(result.current.range[0])
  })

  it('setStart and setEnd clamp against each other', () => {
    const { result } = renderHook(() =>
      useRangeBrush({ totalPoints: 100, initial: [10, 30] })
    )
    act(() => result.current.setStart(50))   // would exceed end=30
    expect(result.current.range[0]).toBe(29) // clamped to end-1

    act(() => result.current.setEnd(5))      // would be below start=29
    expect(result.current.range[1]).toBe(30) // clamped to start+1
  })

  it('onBrushChange fires with new range', () => {
    const cb = vi.fn()
    const { result } = renderHook(() =>
      useRangeBrush({ totalPoints: 100, initial: [0, 99], onBrushChange: cb })
    )
    act(() => result.current.setRange([10, 20]))
    expect(cb).toHaveBeenCalledWith([10, 20])
  })

  it('clampRange rejects total < 2 — returns [0, 0] defensive fallback', () => {
    // Modal must guard against this at call site (trackPoints.length >= 2),
    // but the hook must not crash when total=1 or total=0.
    const { result: r1 } = renderHook(() =>
      useRangeBrush({ totalPoints: 1, initial: [0, 0] })
    )
    expect(r1.current.range).toEqual([0, 0])

    const { result: r0 } = renderHook(() =>
      useRangeBrush({ totalPoints: 0, initial: [0, 0] })
    )
    expect(r0.current.range).toEqual([0, 0])

    // setRange on total=1 hook still returns [0, 0] — no crash
    act(() => r1.current.setRange([0, 0]))
    expect(r1.current.range).toEqual([0, 0])
  })
})
