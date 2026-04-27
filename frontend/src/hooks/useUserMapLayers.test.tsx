import React from 'react'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { useUserMapLayers, useUserMapLayersMutations } from './useUserMapLayers'

// All mock implementations must live inside vi.mock factory (hoisting constraint).
vi.mock('@/api/mapLayers', () => {
  return {
    fetchUserMapLayers: vi.fn().mockResolvedValue([
      {
        id: 1,
        name: 'Mein Layer',
        url_template: 'https://t.example.org/{z}/{x}/{y}.png',
        default_opacity: 80,
        enabled: true,
        sort_order: 0,
        created_at: '',
        updated_at: '',
      },
    ]),
    createUserMapLayer: vi.fn().mockResolvedValue({ id: 2, name: 'Neu', url_template: 'x', default_opacity: 100, enabled: true, sort_order: 1, created_at: '', updated_at: '' }),
    patchUserMapLayer: vi.fn().mockResolvedValue(undefined),
    deleteUserMapLayer: vi.fn().mockResolvedValue(undefined),
    bulkSortUserMapLayers: vi.fn().mockResolvedValue(undefined),
  }
})

const makeWrapper = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  )
  return { qc, Wrapper }
}

describe('useUserMapLayers', () => {
  it('fetches user map layers and exposes them', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useUserMapLayers(), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toHaveLength(1)
    expect(result.current.data![0].name).toBe('Mein Layer')
  })
})

describe('useUserMapLayersMutations', () => {
  it('invalidates user-map-layers after create', async () => {
    const { Wrapper, qc } = makeWrapper()
    const spy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useUserMapLayersMutations(), { wrapper: Wrapper })
    await act(async () => {
      await result.current.create.mutateAsync({ name: 'Neu' })
    })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['user-map-layers', 'anon'] })
  })

  it('invalidates user-map-layers after remove', async () => {
    const { Wrapper, qc } = makeWrapper()
    const spy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useUserMapLayersMutations(), { wrapper: Wrapper })
    await act(async () => {
      await result.current.remove.mutateAsync(1)
    })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['user-map-layers', 'anon'] })
  })

  it('invalidates user-map-layers after update', async () => {
    const { Wrapper, qc } = makeWrapper()
    const spy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useUserMapLayersMutations(), { wrapper: Wrapper })
    await act(async () => {
      await result.current.update.mutateAsync({ id: 1, body: { name: 'Geändert' } })
    })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['user-map-layers', 'anon'] })
  })

  it('invalidates user-map-layers after sort', async () => {
    const { Wrapper, qc } = makeWrapper()
    const spy = vi.spyOn(qc, 'invalidateQueries')
    const { result } = renderHook(() => useUserMapLayersMutations(), { wrapper: Wrapper })
    await act(async () => {
      await result.current.sort.mutateAsync([{ id: 1, sort_order: 0 }])
    })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['user-map-layers', 'anon'] })
  })
})
