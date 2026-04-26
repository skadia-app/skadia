import React from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { useMapLayers } from './useMapLayers'

vi.mock('@/api/mapLayers', () => ({
  fetchMapLayers: vi.fn().mockResolvedValue({
    layers: [
      {
        id: 1,
        key: 'osm',
        role: 'base',
        name: 'OSM',
        source_kind: 'raster_tiles',
        url_template: 'x',
        default_opacity: 100,
        is_system: true,
        enabled: true,
        sort_order: 1,
        proxy_through_backend: false,
        created_at: '',
        updated_at: '',
      },
      {
        id: 2,
        key: 'offline',
        role: 'base',
        name: 'Offline',
        source_kind: 'protomaps_offline',
        default_opacity: 100,
        is_system: true,
        enabled: true,
        sort_order: 7,
        proxy_through_backend: false,
        requires_capability: 'offline_map',
        created_at: '',
        updated_at: '',
      },
      {
        id: 3,
        key: 'hillshade',
        role: 'overlay',
        name: 'Hillshade',
        source_kind: 'raster_tiles',
        url_template: 'y',
        default_opacity: 80,
        is_system: true,
        enabled: true,
        sort_order: 1,
        proxy_through_backend: false,
        created_at: '',
        updated_at: '',
      },
    ],
    system_visibility: {},
  }),
}))

vi.mock('./useCapabilities', () => ({
  useCapabilities: vi.fn(),
}))

import { useCapabilities } from './useCapabilities'

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('useMapLayers', () => {
  it('filters layers by capability when caps loaded — offline absent when offline_map=false', async () => {
    ;(useCapabilities as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { offline_map: false },
      isLoading: false,
    })
    const { result } = renderHook(() => useMapLayers(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.base.map((l) => l.key)).toEqual(['osm'])
    expect(result.current.overlay.map((l) => l.key)).toEqual(['hillshade'])
  })

  it('includes offline layer when offline_map=true', async () => {
    ;(useCapabilities as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { offline_map: true },
      isLoading: false,
    })
    const { result } = renderHook(() => useMapLayers(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.base.map((l) => l.key)).toContain('offline')
  })

  it('filters out capability-gated layers while caps still loading (restrictive default)', async () => {
    ;(useCapabilities as ReturnType<typeof vi.fn>).mockReturnValue({
      data: undefined,
      isLoading: true,
    })
    const { result } = renderHook(() => useMapLayers(), { wrapper })
    // Wait until the map-layers query has resolved (osm + offline + hillshade in raw)
    await waitFor(() => expect(result.current.raw).toHaveLength(3))
    // capabilitiesLoading is still true (mock never resolves caps)
    expect(result.current.capabilitiesLoading).toBe(true)
    // osm has no requires_capability → passes; offline is filtered out because caps=undefined
    expect(result.current.base.map((l) => l.key)).toEqual(['osm'])
    expect(result.current.overlay.map((l) => l.key)).toEqual(['hillshade'])
  })

  it('raw includes all layers regardless of capability filter', async () => {
    ;(useCapabilities as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { offline_map: false },
      isLoading: false,
    })
    const { result } = renderHook(() => useMapLayers(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.raw).toHaveLength(3)
  })
})
