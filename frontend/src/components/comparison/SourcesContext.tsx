// Context for rarely-changing comparison source data: identities, labels, colors,
// activity metadata, stream arrays, and per-source visibility toggles.

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useMemo,
  useCallback,
  type ReactNode,
} from 'react'
import type { Activity, StreamPoint } from '@/api/types'

// ── Types ─────────────────────────────────────────────────────────────────────

export interface SourceInfo {
  /** Stable unique identifier (e.g. activity ID or a synthetic key) */
  id: string
  /** Human-readable label shown in legend, tabs, table headers */
  label: string
  /** Hex color for this source (#RRGGBB) */
  color: string
  /** Optional: full Activity object for summary statistics */
  activity?: Activity
  /** Optional: raw stream points for chart rendering */
  streams?: StreamPoint[]
}

export type XMode = 'distance' | 'time'

interface SourcesContextValue {
  sources: SourceInfo[]
  /** Visibility per source id */
  visibility: Record<string, boolean>
  toggleVisibility: (id: string) => void
  /** X-axis mode for charts */
  xMode: XMode
  setXMode: (mode: XMode) => void
  /** Filtered sources list (only visible ones) */
  visibleSources: SourceInfo[]
}

// ── Context ───────────────────────────────────────────────────────────────────

const SourcesContext = createContext<SourcesContextValue | null>(null)

// ── Provider ─────────────────────────────────────────────────────────────────

interface SourcesProviderProps {
  sources: SourceInfo[]
  children: ReactNode
}

export function SourcesProvider({ sources, children }: SourcesProviderProps) {
  const [visibility, setVisibility] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(sources.map(s => [s.id, true]))
  )
  const [xMode, setXMode] = useState<XMode>('time')

  // Sync visibility when sources array changes:
  // - New sources (not in current visibility map) → default to visible
  // - Removed sources → clean up to avoid stale keys
  useEffect(() => {
    setVisibility(prev => {
      const updated: Record<string, boolean> = {}
      for (const s of sources) {
        updated[s.id] = prev[s.id] ?? true
      }
      return updated
    })
  }, [sources])

  const toggleVisibility = useCallback((id: string) => {
    setVisibility(prev => ({ ...prev, [id]: !prev[id] }))
  }, [])

  const visibleSources = useMemo(
    () => sources.filter(s => visibility[s.id] !== false),
    [sources, visibility]
  )

  const value = useMemo<SourcesContextValue>(
    () => ({ sources, visibility, toggleVisibility, xMode, setXMode, visibleSources }),
    [sources, visibility, toggleVisibility, xMode, visibleSources]
  )

  return (
    <SourcesContext.Provider value={value}>
      {children}
    </SourcesContext.Provider>
  )
}

// ── Hook ──────────────────────────────────────────────────────────────────────

export function useSources(): SourcesContextValue {
  const ctx = useContext(SourcesContext)
  if (!ctx) throw new Error('useSources must be used inside SourcesProvider')
  return ctx
}
