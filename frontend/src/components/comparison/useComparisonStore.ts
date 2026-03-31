// High-frequency comparison state (hover position, zoom range).
// Uses useSyncExternalStore to avoid a dependency on zustand.

import { useSyncExternalStore } from 'react'

export interface HoverGpsPoint {
  lat: number
  lng: number
  color: string
}

interface ComparisonState {
  hoverTimestamp: number | null
  hoverPoints: HoverGpsPoint[] | null
  timeRange: [number, number] | null
}

type Listener = () => void

const state: ComparisonState = {
  hoverTimestamp: null,
  hoverPoints: null,
  timeRange: null,
}

const listeners = new Set<Listener>()

function subscribe(listener: Listener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function notify(): void {
  listeners.forEach(l => l())
}

function getSnapshot(): ComparisonState {
  return state
}

// ── Setters ──────────────────────────────────────────────────────────────────

export function setHoverTimestamp(ts: number | null): void {
  state.hoverTimestamp = ts
  notify()
}

export function setHoverPoints(points: HoverGpsPoint[] | null): void {
  state.hoverPoints = points
  notify()
}

export function setTimeRange(range: [number, number] | null): void {
  state.timeRange = range
  notify()
}

// ── Hooks ─────────────────────────────────────────────────────────────────────

/**
 * Fine-grained selector hook — only the selected slice triggers a re-render.
 * Example: const hoverPoints = useComparisonStore(s => s.hoverPoints)
 */
export function useComparisonStore<T>(selector: (s: ComparisonState) => T): T {
  return useSyncExternalStore(
    subscribe,
    () => selector(getSnapshot()),
    () => selector(getSnapshot()),
  )
}

/**
 * Convenience: returns the full action object for callers that mutate state.
 */
export const comparisonStoreActions = {
  setHoverTimestamp,
  setHoverPoints,
  setTimeRange,
}
