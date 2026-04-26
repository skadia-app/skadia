import { describe, it, expect, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { useHoverPopover } from '../useHoverPopover'

describe('useHoverPopover', () => {
  it('starts closed', () => {
    const { result } = renderHook(() => useHoverPopover())
    expect(result.current.open).toBe(false)
  })

  it('onMouseEnter sets open=true and calls onOpen', () => {
    const onOpen = vi.fn()
    const { result } = renderHook(() => useHoverPopover({ onOpen }))
    act(() => result.current.triggerProps.onMouseEnter())
    expect(result.current.open).toBe(true)
    expect(onOpen).toHaveBeenCalledTimes(1)
  })

  it('onFocus sets open=true', () => {
    const { result } = renderHook(() => useHoverPopover())
    act(() => result.current.triggerProps.onFocus())
    expect(result.current.open).toBe(true)
  })

  it('Escape closes and calls onClose', () => {
    const onClose = vi.fn()
    const { result } = renderHook(() => useHoverPopover({ onClose }))
    act(() => result.current.triggerProps.onMouseEnter())
    expect(result.current.open).toBe(true)
    act(() => {
      result.current.triggerProps.onKeyDown({ key: 'Escape' } as React.KeyboardEvent)
    })
    expect(result.current.open).toBe(false)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('Escape does NOT fire when closeOnEscape=false', () => {
    const { result } = renderHook(() => useHoverPopover({ closeOnEscape: false }))
    act(() => result.current.triggerProps.onMouseEnter())
    act(() => {
      result.current.triggerProps.onKeyDown({ key: 'Escape' } as React.KeyboardEvent)
    })
    expect(result.current.open).toBe(true)
  })

  it('popoverProps and sentinelAttr share the same attribute name', () => {
    const { result } = renderHook(() => useHoverPopover())
    const { sentinelAttr, popoverProps } = result.current
    expect(popoverProps[sentinelAttr]).toBe('')
  })

  it('onMouseLeave with relatedTarget inside sentinel keeps open=true', () => {
    const { result } = renderHook(() => useHoverPopover())
    // Open first
    act(() => result.current.triggerProps.onMouseEnter())
    expect(result.current.open).toBe(true)

    // Create a real DOM element with the sentinel attribute and simulate leaving into it
    const { sentinelAttr } = result.current
    const inner = document.createElement('div')
    inner.setAttribute(sentinelAttr, '')
    document.body.appendChild(inner)

    act(() => {
      result.current.triggerProps.onMouseLeave({
        relatedTarget: inner,
      } as unknown as React.MouseEvent)
    })
    expect(result.current.open).toBe(true)
    document.body.removeChild(inner)
  })

  it('onMouseLeave with relatedTarget outside sentinel closes', () => {
    const { result } = renderHook(() => useHoverPopover())
    act(() => result.current.triggerProps.onMouseEnter())
    act(() => {
      result.current.triggerProps.onMouseLeave({
        relatedTarget: document.body,
      } as unknown as React.MouseEvent)
    })
    expect(result.current.open).toBe(false)
  })

  it('multiple instances generate unique sentinelAttrs (no cross-trigger)', () => {
    const { result: a } = renderHook(() => useHoverPopover())
    const { result: b } = renderHook(() => useHoverPopover())
    expect(a.current.sentinelAttr).not.toBe(b.current.sentinelAttr)
  })

  it('integration: trigger+popover render correctly with sentinel guard', () => {
    function Fixture() {
      const { open, triggerProps, popoverProps } = useHoverPopover()
      return (
        <div {...popoverProps}>
          <button {...triggerProps}>Hover me</button>
          {open && <div {...popoverProps} data-testid="content">Content</div>}
        </div>
      )
    }
    render(<Fixture />)
    expect(screen.queryByTestId('content')).not.toBeInTheDocument()
    fireEvent.mouseEnter(screen.getByRole('button'))
    expect(screen.getByTestId('content')).toBeInTheDocument()
    // mouseLeave into the content div (same sentinel) → stays open
    const content = screen.getByTestId('content')
    fireEvent.mouseLeave(screen.getByRole('button'), { relatedTarget: content })
    expect(screen.getByTestId('content')).toBeInTheDocument()
  })
})
