import React from 'react'

export const ENRICHMENT_MODULES = [
  'summits', 'lifts', 'settlements', 'commute', 'borders',
  'water', 'forests', 'surfaces', 'stops', 'intervals',
  'ascents', 'embeddings',
] as const

export type EnrichmentModule = typeof ENRICHMENT_MODULES[number]
export type ModuleStatus = 'pending' | 'running' | 'done' | 'error'

export interface EnrichmentActivityProgress {
  activityId: string
  modules: Record<EnrichmentModule, ModuleStatus>
  startedAt: number
  lastUpdate: number
}

// Modul-level state (singleton)
const _activities = new Map<string, EnrichmentActivityProgress>()
const _listeners = new Set<() => void>()

function notify() {
  _listeners.forEach(fn => fn())
}

export function updateModule(activityId: string, module: string, status: ModuleStatus) {
  let entry = _activities.get(activityId)
  if (!entry) {
    const modules = {} as Record<EnrichmentModule, ModuleStatus>
    for (const m of ENRICHMENT_MODULES) modules[m] = 'pending'
    entry = { activityId, modules, startedAt: Date.now(), lastUpdate: Date.now() }
  }
  if (module in entry.modules) {
    entry = {
      ...entry,
      modules: { ...entry.modules, [module]: status as ModuleStatus },
      lastUpdate: Date.now(),
    }
  }
  _activities.set(activityId, entry)
  notify()
}

export function clearCompleted() {
  let changed = false
  for (const [id, entry] of _activities) {
    const allDone = Object.values(entry.modules).every(s => s === 'done' || s === 'error')
    if (allDone && Date.now() - entry.lastUpdate > 10_000) {
      _activities.delete(id)
      changed = true
    }
  }
  if (changed) notify()
}

export function clearAll() {
  _activities.clear()
  notify()
}

export function getActivities() {
  return new Map(_activities)
}

export function useEnrichmentProgress<T>(
  selector: (activities: Map<string, EnrichmentActivityProgress>) => T
): T {
  const selectorRef = React.useRef(selector)
  selectorRef.current = selector
  const [value, setValue] = React.useState(() => selector(new Map(_activities)))
  React.useEffect(() => {
    // Pass a new Map snapshot so React always sees a new reference and re-renders.
    const fn = () => setValue(selectorRef.current(new Map(_activities)))
    _listeners.add(fn)
    return () => {
      _listeners.delete(fn)
    }
  }, [])
  return value
}
