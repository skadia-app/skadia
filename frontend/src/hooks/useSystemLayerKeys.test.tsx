import React from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { useSystemLayerKeys } from './useSystemLayerKeys'

vi.mock('@/api/mapLayers', () => ({
  fetchSystemLayerKeys: vi.fn().mockResolvedValue([
    { key: 'nature', category: 'coverage', composite_group: 'nature', default_label: 'Natur' },
    { key: 'noise', category: 'coverage', composite_group: 'noise', default_label: 'Lärm' },
    { key: 'waters', category: 'vector', default_label: 'Gewässer' },
  ]),
}))

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('useSystemLayerKeys', () => {
  it('returns keys array with all descriptors', async () => {
    const { result } = renderHook(() => useSystemLayerKeys(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.keys).toHaveLength(3)
  })

  it('returns byKey lookup for O(1) access', async () => {
    const { result } = renderHook(() => useSystemLayerKeys(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.byKey['noise'].default_label).toBe('Lärm')
    expect(result.current.byKey['waters'].category).toBe('vector')
  })

  it('returns empty arrays while loading', () => {
    const { result } = renderHook(() => useSystemLayerKeys(), { wrapper })
    // Immediately after mount — data not yet resolved
    expect(result.current.keys).toEqual([])
    expect(result.current.byKey).toEqual({})
  })

  it('groups descriptors correctly by category', async () => {
    const { result } = renderHook(() => useSystemLayerKeys(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const coverage = result.current.keys.filter((k) => k.category === 'coverage')
    const vector = result.current.keys.filter((k) => k.category === 'vector')
    expect(coverage).toHaveLength(2)
    expect(vector).toHaveLength(1)
  })
})
