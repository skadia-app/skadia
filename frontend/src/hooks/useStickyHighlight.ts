import { useCallback, useEffect, useState } from 'react'

export type HighlightKind =
  | { kind: 'highway'; key: string }
  | { kind: 'surface'; key: string }
  | { kind: 'descriptor'; descriptorKey: 'mtb_technical' | 'alpine_hiking' | 'exposed' | 'low_visibility' }
  | null

function sameHighlight(a: HighlightKind, b: HighlightKind): boolean {
  if (!a || !b) return false
  if (a.kind !== b.kind) return false
  if (a.kind === 'descriptor' && b.kind === 'descriptor') return a.descriptorKey === b.descriptorKey
  if ((a.kind === 'highway' || a.kind === 'surface') && (b.kind === 'highway' || b.kind === 'surface')) {
    return a.key === b.key
  }
  return false
}

export interface StickyHighlightApi {
  effective: HighlightKind
  sticky: HighlightKind
  setHover: (h: HighlightKind) => void
  handleClick: (h: HighlightKind) => void
  clearSticky: () => void
}

export function useStickyHighlight(): StickyHighlightApi {
  const [hover, setHoverState] = useState<HighlightKind>(null)
  const [sticky, setSticky] = useState<HighlightKind>(null)

  const setHover = useCallback((h: HighlightKind) => setHoverState(h), [])
  const clearSticky = useCallback(() => setSticky(null), [])

  const handleClick = useCallback((h: HighlightKind) => {
    if (!h) { setSticky(null); return }
    setSticky(prev => (sameHighlight(prev, h) ? null : h))
  }, [])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setSticky(null)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  const effective = hover ?? sticky
  return { effective, sticky, setHover, handleClick, clearSticky }
}
