import { describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useCapabilities } from './useCapabilities'

vi.mock('@/api/routes', () => ({
  getCapabilities: vi.fn().mockResolvedValue({
    routing: false,
    brouter: false,
    badrouter: false,
    offline_map: true,
    surface_enrichment: false,
    dem_correction: false,
    isochrone: false,
    route_suggestion: false,
    ml_service: false,
    import_path_enabled: false,
  }),
}))

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('useCapabilities', () => {
  it('returns capabilities from the endpoint', async () => {
    const { result } = renderHook(() => useCapabilities(), { wrapper })
    await waitFor(() => expect(result.current.data).toBeTruthy())
    expect(result.current.data?.offline_map).toBe(true)
  })
})
