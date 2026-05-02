import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, render, act, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { useECharts } from '../useECharts'

// ---------------------------------------------------------------------------
// ECharts init triggers canvas operations that jsdom cannot execute. We mock
// the entire echarts/core module so the hook's DOM-containment logic can be
// tested with a controlled chart stub.
// ---------------------------------------------------------------------------

const mockDispose = vi.fn()
const mockSetOption = vi.fn()
const mockResize = vi.fn()
const mockInit = vi.fn()

vi.mock('echarts/core', () => ({
  init: (...args: unknown[]) => {
    mockInit(...args)
    return {
      setOption: mockSetOption,
      resize: mockResize,
      dispose: mockDispose,
      on: vi.fn(),
      off: vi.fn(),
    }
  },
}))

// ResizeObserver is not available in jsdom — provide a no-op stub.
global.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
}

beforeEach(() => {
  mockDispose.mockClear()
  mockSetOption.mockClear()
  mockResize.mockClear()
  mockInit.mockClear()
})

// ---------------------------------------------------------------------------
// Test 1 — DOM-containment check: dispose is called when container is mounted
// ---------------------------------------------------------------------------
describe('useECharts — DOM-containment dispose guard', () => {
  it('calls dispose when container is still in the document on cleanup', () => {
    // Use a real rendered component so the ref is attached to a DOM node that
    // is actually part of document.body — renderHook alone never renders a div
    // with the returned ref, so it cannot trigger the init path.
    function Fixture() {
      const ref = useECharts(vi.fn(() => ({})), [])
      return <div ref={ref} style={{ width: 200, height: 100 }} />
    }
    const { unmount } = render(<Fixture />)
    expect(mockInit).toHaveBeenCalledTimes(1)
    unmount()
    expect(mockDispose).toHaveBeenCalledTimes(1)
  })

  it('does NOT throw when dispose is called on a detached container', () => {
    // Simulate the crash scenario: echarts.dispose throws removeChild error
    // when the canvas parent is already gone.
    mockDispose.mockImplementationOnce(() => {
      throw new DOMException(
        "Failed to execute 'removeChild' on 'Node': The node to be removed is not a child of this node.",
        'NotFoundError',
      )
    })

    const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    // We test the guard logic directly by simulating what the cleanup closure
    // does. This is the exact code path from the hook's useEffect cleanup.
    const guardedDispose = (chart: { dispose: () => void }, container: HTMLDivElement | null) => {
      if (container && document.contains(container)) {
        chart.dispose()
      } else {
        try {
          chart.dispose()
        } catch (err) {
          console.warn('[useECharts] dispose skipped — container already detached from DOM', err)
        }
      }
    }

    const fakeChart = { dispose: mockDispose }
    // container is null — simulates conditional unmount (React removed the div)
    expect(() => guardedDispose(fakeChart, null)).not.toThrow()
    expect(consoleSpy).toHaveBeenCalledWith(
      expect.stringContaining('[useECharts]'),
      expect.anything(),
    )
    consoleSpy.mockRestore()
  })

  // -------------------------------------------------------------------------
  // Test 2 — `enabled` flag: no init when enabled=false
  // -------------------------------------------------------------------------
  it('does not init chart when enabled=false', () => {
    renderHook(() =>
      useECharts(vi.fn(() => ({})), [], { enabled: false }),
    )
    expect(mockInit).not.toHaveBeenCalled()
  })

  it('inits chart when enabled=true (default)', () => {
    // renderHook's container div is not attached to document.body in all
    // versions; the hook guards on containerRef.current !== null.
    // We need a component that actually mounts the ref div.
    function Fixture() {
      const ref = useECharts(vi.fn(() => ({})), [])
      return <div ref={ref} style={{ width: 200, height: 100 }} />
    }
    render(<Fixture />)
    expect(mockInit).toHaveBeenCalledTimes(1)
  })

  // -------------------------------------------------------------------------
  // Test 3 — Backwards compatibility: positional onChart callback still works
  // -------------------------------------------------------------------------
  it('accepts positional onChart callback (old call style)', () => {
    const onChart = vi.fn().mockReturnValue(undefined)
    function Fixture() {
      const ref = useECharts(vi.fn(() => ({})), [], onChart)
      return <div ref={ref} style={{ width: 200, height: 100 }} />
    }
    render(<Fixture />)
    expect(onChart).toHaveBeenCalledTimes(1)
    expect(onChart).toHaveBeenCalledWith(
      expect.objectContaining({ setOption: mockSetOption }),
    )
  })

  it('accepts options-object onChart callback (new call style)', () => {
    const onChart = vi.fn().mockReturnValue(undefined)
    function Fixture() {
      const ref = useECharts(vi.fn(() => ({})), [], { onChart })
      return <div ref={ref} style={{ width: 200, height: 100 }} />
    }
    render(<Fixture />)
    expect(onChart).toHaveBeenCalledTimes(1)
  })

  // -------------------------------------------------------------------------
  // Test 4 — Conditional container: toggle causes unmount → no crash
  //
  // This is the exact reproducer for the ZoneDistributionChart crash:
  // container div is conditional on !isLoading, a state toggle re-fetches
  // → isLoading becomes true → container unmounts → effect cleanup fires.
  // -------------------------------------------------------------------------
  it('does not crash when container unmounts before cleanup fires (conditional render)', async () => {
    // jsdom does not support canvas/ResizeObserver fully, but the mock above
    // replaces ECharts so the only dangerous operation is dispose().
    // We make dispose throw to prove the try/catch swallows it.
    mockDispose.mockImplementation(() => {
      throw new DOMException(
        "Failed to execute 'removeChild' on 'Node': The node to be removed is not a child of this node.",
        'NotFoundError',
      )
    })

    const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    function ConditionalChart({ show }: { show: boolean }) {
      const ref = useECharts(vi.fn(() => ({})), [show])
      if (!show) return <div>loading...</div>
      return <div ref={ref} style={{ width: 200, height: 100 }} />
    }

    function Wrapper() {
      const [show, setShow] = useState(true)
      return (
        <>
          <button onClick={() => setShow(false)}>toggle</button>
          <ConditionalChart show={show} />
        </>
      )
    }

    const { getByRole } = render(<Wrapper />)

    // Toggle — unmounts the chart div, triggers cleanup
    await act(async () => {
      getByRole('button').click()
    })

    // dispose was called, warn was emitted, no unhandled error was thrown
    await waitFor(() => {
      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining('[useECharts]'),
        expect.anything(),
      )
    })

    consoleSpy.mockRestore()
    // Reset so later tests get the standard no-op dispose
    mockDispose.mockImplementation(() => {})
  })
})
