// frontend/src/hooks/useLayerOrder.ts
import { useCallback, useState } from 'react'
import type maplibregl from 'maplibre-gl'

export interface LayerEntry {
  key: string
  zone: 'above' | 'below'
  maplibreIds: string[]
}

const LS_KEY = 'skadia-layer-order'

const SYSTEM_DEFAULTS: LayerEntry[] = [
  { key: 'summits',          zone: 'above', maplibreIds: ['summits-layer'] },
  { key: 'waters',           zone: 'above', maplibreIds: ['waters-layer'] },
  { key: 'forests',          zone: 'above', maplibreIds: ['forests-layer'] },
  { key: 'visitedPois',      zone: 'above', maplibreIds: ['visited-pois-layer'] },
  { key: 'amenities',        zone: 'above', maplibreIds: ['pois-layer'] },
  { key: 'coverage-nature',  zone: 'below', maplibreIds: [] },
  { key: 'coverage-noise',   zone: 'below', maplibreIds: [] },
  { key: 'coverage-traffic', zone: 'below', maplibreIds: [] },
]

// Fix 2: validate saved items and merge missing SYSTEM_DEFAULTS keys;
// prune stale keys (renamed system keys, deleted user layers not cleaned up via removeUserEntry)
function mergeWithDefaults(saved: LayerEntry[]): LayerEntry[] {
  const systemKeys = new Set(SYSTEM_DEFAULTS.map(d => d.key))
  const valid = saved.filter(
    e =>
      typeof e.key === 'string' &&
      Array.isArray(e.maplibreIds) &&
      (e.zone === 'above' || e.zone === 'below') &&
      (systemKeys.has(e.key) || e.key.startsWith('user-')),
  )
  const savedKeys = new Set(valid.map(e => e.key))
  const missing = SYSTEM_DEFAULTS.filter(d => !savedKeys.has(d.key))
  return [...valid, ...missing]
}

function loadFromStorage(): LayerEntry[] | null {
  try {
    const raw = localStorage.getItem(LS_KEY)
    return raw ? mergeWithDefaults(JSON.parse(raw) as LayerEntry[]) : null
  } catch {
    return null
  }
}

function persist(entries: LayerEntry[]) {
  localStorage.setItem(LS_KEY, JSON.stringify(entries))
}

export function useLayerOrder() {
  const [entries, setEntries] = useState<LayerEntry[]>(
    () => loadFromStorage() ?? SYSTEM_DEFAULTS,
  )

  // Fix 3: registerIds now calls persist
  const registerIds = useCallback((key: string, ids: string[]) => {
    setEntries(prev => {
      const next = prev.map(e => (e.key === key ? { ...e, maplibreIds: ids } : e))
      persist(next)
      return next
    })
  }, [])

  // Add a user layer entry if not present
  const addUserEntry = useCallback((key: string, zone: 'above' | 'below', maplibreIds: string[]) => {
    setEntries(prev => {
      if (prev.some(e => e.key === key)) return prev
      const next = [...prev, { key, zone, maplibreIds }]
      persist(next)
      return next
    })
  }, [])

  // Remove a user layer entry
  const removeUserEntry = useCallback((key: string) => {
    setEntries(prev => {
      const next = prev.filter(e => e.key !== key)
      persist(next)
      return next
    })
  }, [])

  // Fix 4: skip admin-hidden entries when visibleKeys is provided so the user
  // always moves to the next visible neighbour in the same zone.
  const moveUp = useCallback((key: string, visibleKeys?: ReadonlySet<string>) => {
    setEntries(prev => {
      const idx = prev.findIndex(e => e.key === key)
      if (idx <= 0) return prev
      const zone = prev[idx].zone
      let targetIdx = idx - 1
      while (targetIdx >= 0) {
        if (prev[targetIdx].zone !== zone) return prev // zone boundary
        if (!visibleKeys || visibleKeys.has(prev[targetIdx].key)) break
        targetIdx--
      }
      if (targetIdx < 0) return prev
      const next = [...prev]
      const [item] = next.splice(idx, 1)
      next.splice(targetIdx, 0, item)
      persist(next)
      return next
    })
  }, [])

  const moveDown = useCallback((key: string, visibleKeys?: ReadonlySet<string>) => {
    setEntries(prev => {
      const idx = prev.findIndex(e => e.key === key)
      if (idx < 0 || idx >= prev.length - 1) return prev
      const zone = prev[idx].zone
      let targetIdx = idx + 1
      while (targetIdx < prev.length) {
        if (prev[targetIdx].zone !== zone) return prev // zone boundary
        if (!visibleKeys || visibleKeys.has(prev[targetIdx].key)) break
        targetIdx++
      }
      if (targetIdx >= prev.length) return prev
      const next = [...prev]
      const [item] = next.splice(idx, 1)
      next.splice(targetIdx, 0, item)
      persist(next)
      return next
    })
  }, [])

  // Reorder to drop position. The dragged item inherits the target's zone (cross-zone drops allowed).
  // toKey === null places at the end of the given fallbackZone.
  const moveTo = useCallback((key: string, toKey: string | null, fallbackZone?: 'above' | 'below') => {
    setEntries(prev => {
      const fromIdx = prev.findIndex(e => e.key === key)
      if (fromIdx < 0) return prev
      const next = [...prev]
      const [item] = next.splice(fromIdx, 1)
      if (toKey === null) {
        const zone = fallbackZone ?? item.zone
        item.zone = zone
        let insertIdx = next.length
        for (let i = next.length - 1; i >= 0; i--) {
          if (next[i].zone === zone) { insertIdx = i + 1; break }
          if (i === 0) insertIdx = 0
        }
        next.splice(insertIdx, 0, item)
      } else {
        const toIdx = next.findIndex(e => e.key === toKey)
        if (toIdx < 0) {
          next.splice(fromIdx, 0, item)
          return prev
        }
        item.zone = next[toIdx].zone
        next.splice(toIdx, 0, item)
      }
      persist(next)
      return next
    })
  }, [])

  const changeZone = useCallback((key: string, zone: 'above' | 'below') => {
    setEntries(prev => {
      const next = prev.map(e => (e.key === key ? { ...e, zone } : e))
      persist(next)
      return next
    })
  }, [])

  // Fix 1: zone-aware applyOrder — below-zone entries placed before belowAnchorId,
  // above-zone entries pushed to the top of the stack.
  // Guard: skip below-zone loop when belowAnchorId is undefined (vector tiles not yet loaded)
  // to avoid moveLayer(id, undefined) which would silently move layers to the top.
  const applyOrder = useCallback((map: maplibregl.Map, belowAnchorId?: string) => {
    const aboveEntries = [...entries].filter(e => e.zone === 'above').reverse()

    if (belowAnchorId) {
      const belowEntries = [...entries].filter(e => e.zone === 'below').reverse()
      for (const entry of belowEntries) {
        for (const id of entry.maplibreIds) {
          if (map.getLayer(id)) {
            try { map.moveLayer(id, belowAnchorId) } catch { /* best-effort */ }
          }
        }
      }
    }

    for (const entry of aboveEntries) {
      for (const id of entry.maplibreIds) {
        if (map.getLayer(id)) {
          try { map.moveLayer(id) } catch { /* best-effort */ }
        }
      }
    }
  }, [entries])

  return {
    layerEntries: entries,
    registerIds,
    addUserEntry,
    removeUserEntry,
    moveUp,
    moveDown,
    moveTo,
    changeZone,
    applyOrder,
  }
}
