import { useState, useEffect, useCallback, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { Map as MaplibreMap } from 'maplibre-gl'
import { apiClient } from '@/api/client'

export interface CoverageCompositeState {
  enabled: boolean
  opacity: number
  layers: Record<string, boolean>
}

export interface CoverageOverlayState {
  nature: CoverageCompositeState
  quiet: CoverageCompositeState
  trafficUrban: CoverageCompositeState
}

const COMPOSITE_LAYERS: Record<keyof CoverageOverlayState, string[]> = {
  nature: ['landuse', 'forest', 'water', 'river_proximity', 'protected'],
  quiet: ['quiet'],
  trafficUrban: ['urbanization', 'traffic'],
}

const LAYER_KEY_TO_API: Record<string, string> = {
  forest: 'forest',
  water: 'water',
  riverProximity: 'river_proximity',
  protected: 'protected',
  landuse: 'landuse',
  traffic: 'traffic',
  urbanization: 'urbanization',
}

const STORAGE_KEY = 'skadia-coverage-overlays'

const defaultState: CoverageOverlayState = {
  nature: {
    enabled: false,
    opacity: 50,
    layers: { forest: true, water: true, riverProximity: true, protected: true, landuse: false },
  },
  quiet: {
    enabled: false,
    opacity: 50,
    layers: {},
  },
  trafficUrban: {
    enabled: false,
    opacity: 50,
    layers: { traffic: true, urbanization: true },
  },
}

function mergeComposite(
  defaults: CoverageCompositeState,
  saved?: Partial<CoverageCompositeState>,
): CoverageCompositeState {
  if (!saved) return defaults
  return {
    enabled: saved.enabled ?? defaults.enabled,
    opacity: saved.opacity ?? defaults.opacity,
    layers: { ...defaults.layers, ...saved.layers },
  }
}

function loadState(): CoverageOverlayState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const saved = JSON.parse(raw) as Partial<CoverageOverlayState>
      return {
        nature: mergeComposite(defaultState.nature, saved.nature),
        quiet: mergeComposite(defaultState.quiet, saved.quiet),
        trafficUrban: mergeComposite(defaultState.trafficUrban, saved.trafficUrban),
      }
    }
  } catch { /* ignore */ }
  return defaultState
}

function saveState(state: CoverageOverlayState) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
}

async function fetchCoverageInfo(): Promise<{ buildTimestamp: string }> {
  return apiClient<{ buildTimestamp: string }>('/coverage/info')
}

function syncLayerVisibility(map: MaplibreMap, state: CoverageOverlayState) {
  const compositeKeys = Object.keys(COMPOSITE_LAYERS) as (keyof CoverageOverlayState)[]
  for (const compositeKey of compositeKeys) {
    const compositeState = state[compositeKey]
    const apiLayers = COMPOSITE_LAYERS[compositeKey]
    for (const apiLayer of apiLayers) {
      const layerId = `coverage-${apiLayer}-layer`
      if (!map.getLayer(layerId)) continue

      const subLayerKey = Object.entries(LAYER_KEY_TO_API).find(([, v]) => v === apiLayer)?.[0]
      const subLayerEnabled = subLayerKey ? compositeState.layers[subLayerKey] ?? true : true
      const shouldShow = compositeState.enabled && subLayerEnabled

      map.setLayoutProperty(layerId, 'visibility', shouldShow ? 'visible' : 'none')
      if (shouldShow) {
        map.setPaintProperty(layerId, 'raster-opacity', compositeState.opacity / 100)
      }
    }
  }
}

export function useCoverageOverlays(mapInstance: MaplibreMap | null, styleVersion: number) {
  const [state, setState] = useState<CoverageOverlayState>(loadState)
  const stateRef = useRef(state)
  stateRef.current = state

  const { data: coverageInfo } = useQuery({
    queryKey: ['coverage-info'],
    queryFn: fetchCoverageInfo,
    staleTime: 5 * 60 * 1000,
    retry: false,
  })
  const buildTimestamp = coverageInfo?.buildTimestamp ?? ''

  useEffect(() => {
    saveState(state)
  }, [state])

  // Effect 1: Manage sources — only re-create when buildTimestamp or style changes
  useEffect(() => {
    if (!mapInstance || styleVersion === 0) return

    // Add all possible sources (one per API layer)
    const allApiLayers = [...new Set(Object.values(COMPOSITE_LAYERS).flat())]
    for (const apiLayer of allApiLayers) {
      const sourceId = `coverage-${apiLayer}`
      if (!mapInstance.getSource(sourceId)) {
        const tileUrl = buildTimestamp
          ? `/api/v1/coverage/tiles/${apiLayer}/{z}/{x}/{y}.png?v=${buildTimestamp}`
          : `/api/v1/coverage/tiles/${apiLayer}/{z}/{x}/{y}.png`
        mapInstance.addSource(sourceId, {
          type: 'raster',
          tiles: [tileUrl],
          tileSize: 256,
          minzoom: 8,
          maxzoom: 15,
        })
      }
      const layerId = `coverage-${apiLayer}-layer`
      if (!mapInstance.getLayer(layerId)) {
        const beforeId = findFirstVectorOverlayLayer(mapInstance)
        mapInstance.addLayer(
          {
            id: layerId,
            type: 'raster',
            source: sourceId,
            layout: { visibility: 'none' },
            paint: { 'raster-opacity': 0.5 },
          },
          beforeId,
        )
      }
    }

    // Apply current state
    syncLayerVisibility(mapInstance, stateRef.current)

    return () => {
      if (!mapInstance) return
      for (const apiLayer of allApiLayers) {
        const layerId = `coverage-${apiLayer}-layer`
        const sourceId = `coverage-${apiLayer}`
        if (mapInstance.getLayer(layerId)) mapInstance.removeLayer(layerId)
        if (mapInstance.getSource(sourceId)) mapInstance.removeSource(sourceId)
      }
    }
  }, [mapInstance, styleVersion, buildTimestamp])

  // Effect 2: Sync visibility + opacity — no source/layer add/remove
  useEffect(() => {
    if (!mapInstance || styleVersion === 0) return
    syncLayerVisibility(mapInstance, state)
  }, [mapInstance, state, styleVersion])

  const updateComposite = useCallback(
    (key: keyof CoverageOverlayState, update: Partial<CoverageCompositeState>) => {
      setState((prev) => ({
        ...prev,
        [key]: { ...prev[key], ...update },
      }))
    },
    [],
  )

  const toggleLayer = useCallback(
    (composite: keyof CoverageOverlayState, layer: string, enabled: boolean) => {
      setState((prev) => ({
        ...prev,
        [composite]: {
          ...prev[composite],
          layers: { ...prev[composite].layers, [layer]: enabled },
        },
      }))
    },
    [],
  )

  return { coverageState: state, updateComposite, toggleLayer }
}

function findFirstVectorOverlayLayer(map: MaplibreMap): string | undefined {
  const layers = map.getStyle()?.layers || []
  for (const l of layers) {
    if (
      l.id.startsWith('heatmap-') ||
      l.id.startsWith('activity-') ||
      l.id.startsWith('summit') ||
      l.id.startsWith('waters-') ||
      l.id.startsWith('forests-') ||
      l.id.startsWith('visited-pois-')
    ) {
      return l.id
    }
  }
  return undefined
}
