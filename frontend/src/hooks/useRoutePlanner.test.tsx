import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ToastProvider } from '@/lib/toast'
import { useRoutePlanner } from './useRoutePlanner'

// Mock API layer that useRoutePlanner depends on at module-init time.
vi.mock('@/api/routes', async () => {
  const actual = await vi.importActual<typeof import('@/api/routes')>('@/api/routes')
  return {
    ...actual,
    getCapabilities: vi.fn().mockResolvedValue({ has_badrouter: true }),
    getProfileDefaults: vi.fn().mockResolvedValue({
      profile: 'hiking',
      uphill_cost: 7,
    }),
  }
})

describe('useRoutePlanner (P6 null-initial-state)', () => {
  let qc: QueryClient

  beforeEach(() => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  })

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </ToastProvider>
  )

  it('starts with profileConfig === null before server fetch', () => {
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    expect(result.current.profileConfig).toBeNull()
  })

  it('populates profileConfig after server fetch resolves', async () => {
    const { result } = renderHook(() => useRoutePlanner(), { wrapper })
    await waitFor(() => {
      expect(result.current.profileConfig).not.toBeNull()
    })
    expect(result.current.profileConfig?.profile).toBe('hiking')
  })
})
