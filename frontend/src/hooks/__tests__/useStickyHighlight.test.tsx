import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { fireEvent } from '@testing-library/react'
import { useStickyHighlight } from '../useStickyHighlight'
import type { HighlightKind } from '../useStickyHighlight'

const MTB: HighlightKind = { kind: 'descriptor', descriptorKey: 'mtb_technical' }
const ALPINE: HighlightKind = { kind: 'descriptor', descriptorKey: 'alpine_hiking' }
const EXPOSED: HighlightKind = { kind: 'descriptor', descriptorKey: 'exposed' }
const HW_PRIM: HighlightKind = { kind: 'highway', key: 'primary' }
const HW_PRIM2: HighlightKind = { kind: 'highway', key: 'primary' }
const SURF_ASPH: HighlightKind = { kind: 'surface', key: 'asphalt' }

describe('useStickyHighlight — real React hook via renderHook', () => {
  it('click A → sticky (effective = A)', () => {
    const { result } = renderHook(() => useStickyHighlight())
    act(() => result.current.handleClick(MTB))
    expect(result.current.effective).toMatchObject({ descriptorKey: 'mtb_technical' })
  })

  it('click-again un-sticks (effective = null)', () => {
    const { result } = renderHook(() => useStickyHighlight())
    act(() => result.current.handleClick(MTB))
    act(() => result.current.handleClick(MTB))
    expect(result.current.effective).toBeNull()
  })

  it('click B replaces A (effective = B)', () => {
    const { result } = renderHook(() => useStickyHighlight())
    act(() => result.current.handleClick(MTB))
    act(() => result.current.handleClick(ALPINE))
    expect(result.current.effective).toMatchObject({ descriptorKey: 'alpine_hiking' })
  })

  it('hover overrides sticky, hover-clear restores sticky', () => {
    const { result } = renderHook(() => useStickyHighlight())
    act(() => result.current.handleClick(MTB))
    act(() => result.current.setHover(EXPOSED))
    expect(result.current.effective).toMatchObject({ descriptorKey: 'exposed' })
    act(() => result.current.setHover(null))
    expect(result.current.effective).toMatchObject({ descriptorKey: 'mtb_technical' })
  })

  it('ESC clears sticky (keydown event on document)', () => {
    const { result } = renderHook(() => useStickyHighlight())
    act(() => result.current.handleClick(MTB))
    expect(result.current.effective).toMatchObject({ descriptorKey: 'mtb_technical' })
    act(() => {
      fireEvent.keyDown(document, { key: 'Escape' })
    })
    expect(result.current.effective).toBeNull()
  })

  it('handleClick(null) clears sticky', () => {
    const { result } = renderHook(() => useStickyHighlight())
    act(() => result.current.handleClick(MTB))
    act(() => result.current.handleClick(null))
    expect(result.current.effective).toBeNull()
  })

  it('highway/surface sameHighlight: identical keys match → un-sticks', () => {
    const { result } = renderHook(() => useStickyHighlight())
    // Two separate objects with same kind+key must be treated as equal
    act(() => result.current.handleClick(HW_PRIM))
    act(() => result.current.handleClick(HW_PRIM2))
    expect(result.current.effective).toBeNull()
  })

  it('highway and surface with same key string are NOT the same highlight', () => {
    const { result } = renderHook(() => useStickyHighlight())
    // highway key='asphalt' click, then surface key='asphalt' click — different kinds
    const hwAsphalt: HighlightKind = { kind: 'highway', key: 'asphalt' }
    act(() => result.current.handleClick(hwAsphalt))
    act(() => result.current.handleClick(SURF_ASPH))
    // Not same → B replaces A, effective = surface/asphalt
    expect(result.current.effective).toMatchObject({ kind: 'surface', key: 'asphalt' })
  })

  it('exposes sticky separate from effective (hover doesnt leak into sticky)', () => {
    const { result } = renderHook(() => useStickyHighlight())
    act(() => result.current.setHover({ kind: 'highway', key: 'path' }))
    expect(result.current.effective).toMatchObject({ kind: 'highway', key: 'path' })
    expect(result.current.sticky).toBeNull()  // hover darf NICHT in sticky leaken
  })

  it('hover→click-same toggle: hover sets effective, click makes it sticky, click-again clears', () => {
    const { result } = renderHook(() => useStickyHighlight())
    // Hover shows MTB (no sticky)
    act(() => result.current.setHover(MTB))
    expect(result.current.effective).toMatchObject({ descriptorKey: 'mtb_technical' })
    // Click MTB while hovering → sticky=MTB
    act(() => result.current.handleClick(MTB))
    // Clear hover — sticky should remain
    act(() => result.current.setHover(null))
    expect(result.current.effective).toMatchObject({ descriptorKey: 'mtb_technical' })
    // Click same MTB again → un-sticks
    act(() => result.current.handleClick(MTB))
    expect(result.current.effective).toBeNull()
  })
})
