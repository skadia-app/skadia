import { useCallback, useEffect, useState } from 'react'
import type { DescriptorKey } from '@/lib/descriptorFilter'

// The full kind union used by the route-planner (MapPage).
// Activity-Detail callers parameterise with a narrower union that omits 'descriptor'.
export type HighlightKind =
  | { kind: 'highway'; key: string; overflowRawTypes?: string[] }
  | { kind: 'surface'; key: string; overflowRawTypes?: string[] }
  | { kind: 'descriptor'; descriptorKey: DescriptorKey }
  | null

function sameHighlight<H>(a: H | null, b: H | null): boolean {
  if (!a || !b) return false
  // Cast via unknown to satisfy strict structural overlap check.
  // At runtime H is always a subtype of HighlightKind, enforced by call-sites.
  const ah = a as unknown as HighlightKind
  const bh = b as unknown as HighlightKind
  if (!ah || !bh) return false
  if (ah.kind !== bh.kind) return false
  if (ah.kind === 'descriptor' && bh.kind === 'descriptor') return ah.descriptorKey === bh.descriptorKey
  if ((ah.kind === 'highway' || ah.kind === 'surface') && (bh.kind === 'highway' || bh.kind === 'surface')) {
    return ah.key === bh.key
  }
  return false
}

export interface StickyHighlightApi<H = HighlightKind> {
  effective: H | null
  sticky: H | null
  setHover: (h: H | null) => void
  handleClick: (h: H | null) => void
  clearSticky: () => void
}

/**
 * Generic sticky-highlight hook.
 *
 * Parameterise H with the subset of HighlightKind that the call-site cares about.
 * - MapPage uses the full HighlightKind union (default).
 * - ActivityDetailPage uses `{ kind: 'highway'; key: string } | { kind: 'surface'; key: string }`,
 *   removing the need to filter out `descriptor` manually after the call.
 */
export function useStickyHighlight<H = HighlightKind>(): StickyHighlightApi<H> {
  const [hover, setHoverState] = useState<H | null>(null)
  const [sticky, setSticky] = useState<H | null>(null)

  const setHover = useCallback((h: H | null) => setHoverState(h), [])
  const clearSticky = useCallback(() => setSticky(null), [])

  const handleClick = useCallback((h: H | null) => {
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
