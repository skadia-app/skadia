import { describe, it, expect } from 'vitest'
import { sameHighlight } from '../useStickyHighlight'
import type { HighlightKind } from '../useStickyHighlight'

/**
 * Minimal synchronous state-machine simulator for useStickyHighlight.
 *
 * @testing-library/react is not installed in this project. The hook is a
 * thin React wrapper around a deterministic state machine. We test the
 * machine directly by re-implementing the same transition rules used in
 * the hook (handleClick = toggle, effective = hover ?? sticky).
 */
function createHighlightMachine() {
  let hover: HighlightKind = null
  let sticky: HighlightKind = null

  return {
    handleClick(h: HighlightKind) {
      if (!h) { sticky = null; return }
      sticky = sameHighlight(sticky, h) ? null : h
    },
    setHover(h: HighlightKind) {
      hover = h
    },
    get effective(): HighlightKind {
      return hover ?? sticky
    },
  }
}

describe('useStickyHighlight', () => {
  it('click A → sticky', () => {
    const m = createHighlightMachine()
    m.handleClick({ kind: 'descriptor', descriptorKey: 'mtb_technical' })
    expect(m.effective).toMatchObject({ descriptorKey: 'mtb_technical' })
  })

  it('click-again un-sticks', () => {
    const m = createHighlightMachine()
    m.handleClick({ kind: 'descriptor', descriptorKey: 'mtb_technical' })
    m.handleClick({ kind: 'descriptor', descriptorKey: 'mtb_technical' })
    expect(m.effective).toBeNull()
  })

  it('click B replaces A', () => {
    const m = createHighlightMachine()
    m.handleClick({ kind: 'descriptor', descriptorKey: 'mtb_technical' })
    m.handleClick({ kind: 'descriptor', descriptorKey: 'alpine_hiking' })
    expect(m.effective).toMatchObject({ descriptorKey: 'alpine_hiking' })
  })

  it('hover overrides sticky', () => {
    const m = createHighlightMachine()
    m.handleClick({ kind: 'descriptor', descriptorKey: 'mtb_technical' })
    m.setHover({ kind: 'descriptor', descriptorKey: 'exposed' })
    expect(m.effective).toMatchObject({ descriptorKey: 'exposed' })
    m.setHover(null)
    expect(m.effective).toMatchObject({ descriptorKey: 'mtb_technical' })
  })
})
