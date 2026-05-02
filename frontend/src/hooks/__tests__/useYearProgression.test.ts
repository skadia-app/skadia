import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'

// ── API mocks ─────────────────────────────────────────────────────────────────

const mockFetchYearProgression = vi.fn()
const mockFetchYearProgressionRolling = vi.fn()

vi.mock('@/api/analytics', () => ({
  fetchYearProgression: (params: string) => mockFetchYearProgression(params),
  fetchYearProgressionRolling: (params: string) => mockFetchYearProgressionRolling(params),
}))

// ── Subject ───────────────────────────────────────────────────────────────────

import { useYearProgression } from '../useYearProgression'

// ── Test wrapper ──────────────────────────────────────────────────────────────

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('useYearProgression', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchYearProgression.mockResolvedValue({
      metric: 'distance_m',
      best_year: 2024,
      years: [{ year: 2024, monthly: [], cumulative: [], total: 0 }],
    })
    mockFetchYearProgressionRolling.mockResolvedValue({
      metric: 'distance_m',
      period_days: 30,
      granularity: 'day',
      years: [],
      best_year: 0,
    })
  })

  it('calls fetchYearProgression when mode=progress', async () => {
    const wrapper = makeWrapper()
    const { result } = renderHook(
      () => useYearProgression({ mode: 'progress', metric: 'distance_m', queryParams: '' }),
      { wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mockFetchYearProgression).toHaveBeenCalledTimes(1)
    expect(mockFetchYearProgressionRolling).not.toHaveBeenCalled()
  })

  it('calls fetchYearProgressionRolling when mode=rolling', async () => {
    const wrapper = makeWrapper()
    const { result } = renderHook(
      () =>
        useYearProgression({
          mode: 'rolling',
          metric: 'distance_m',
          periodDays: 30,
          granularity: 'day',
          queryParams: '',
        }),
      { wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(mockFetchYearProgressionRolling).toHaveBeenCalledTimes(1)
    expect(mockFetchYearProgression).not.toHaveBeenCalled()
  })

  it('cache keys are disjoint between progress and rolling modes', async () => {
    // Both hooks are rendered in the same QueryClient. They must not share cache
    // entries even when queryParams and metric are identical.
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children)

    const { result: progressResult } = renderHook(
      () => useYearProgression({ mode: 'progress', metric: 'distance_m', queryParams: 'x=1' }),
      { wrapper },
    )
    const { result: rollingResult } = renderHook(
      () =>
        useYearProgression({
          mode: 'rolling',
          metric: 'distance_m',
          periodDays: 30,
          granularity: 'day',
          queryParams: 'x=1',
        }),
      { wrapper },
    )

    await waitFor(() => {
      expect(progressResult.current.isLoading).toBe(false)
      expect(rollingResult.current.isLoading).toBe(false)
    })

    // Both fetchers must have been called — if they shared the same cache key,
    // only one would be called.
    expect(mockFetchYearProgression).toHaveBeenCalledTimes(1)
    expect(mockFetchYearProgressionRolling).toHaveBeenCalledTimes(1)

    // The returned data shapes must be distinct (progress has YearProgressionResponse).
    const progressData = progressResult.current.data as { years: unknown[] } | undefined
    expect(progressData?.years).toHaveLength(1)

    const rollingData = rollingResult.current.data as { period_days: number } | undefined
    expect(rollingData?.period_days).toBe(30)
  })

  it('F013: progress mode injects metric into fetch params even when absent from queryParams', async () => {
    const wrapper = makeWrapper()
    renderHook(
      () => useYearProgression({
        mode: 'progress',
        metric: 'elevation_gain_m',
        queryParams: 'sport_type=cycling',
      }),
      { wrapper },
    )

    await waitFor(() => expect(mockFetchYearProgression).toHaveBeenCalledTimes(1))

    const calledWith: string = mockFetchYearProgression.mock.calls[0][0]
    const params = new URLSearchParams(calledWith)
    expect(params.get('metric')).toBe('elevation_gain_m')
    expect(params.get('sport_type')).toBe('cycling')
  })

  it('F013: progress cache key changes when metric changes (no stale cross-metric cache)', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children)

    const { rerender } = renderHook(
      ({ metric }: { metric: string }) =>
        useYearProgression({ mode: 'progress', metric: metric as 'distance_m' | 'elevation_gain_m', queryParams: '' }),
      { wrapper, initialProps: { metric: 'distance_m' } },
    )

    await waitFor(() => expect(mockFetchYearProgression).toHaveBeenCalledTimes(1))

    rerender({ metric: 'elevation_gain_m' })

    await waitFor(() => expect(mockFetchYearProgression).toHaveBeenCalledTimes(2))

    const calls = mockFetchYearProgression.mock.calls
    const firstMetric = new URLSearchParams(calls[0][0]).get('metric')
    const secondMetric = new URLSearchParams(calls[1][0]).get('metric')
    expect(firstMetric).toBe('distance_m')
    expect(secondMetric).toBe('elevation_gain_m')
  })

  it('passes period_days and granularity into the rolling fetch params', async () => {
    const wrapper = makeWrapper()
    renderHook(
      () =>
        useYearProgression({
          mode: 'rolling',
          metric: 'elevation_gain_m',
          periodDays: 7,
          granularity: 'week',
          queryParams: 'sport_type=cycling',
        }),
      { wrapper },
    )

    await waitFor(() => expect(mockFetchYearProgressionRolling).toHaveBeenCalledTimes(1))

    const calledWith: string = mockFetchYearProgressionRolling.mock.calls[0][0]
    const params = new URLSearchParams(calledWith)
    expect(params.get('period_days')).toBe('7')
    expect(params.get('granularity')).toBe('week')
    expect(params.get('metric')).toBe('elevation_gain_m')
    expect(params.get('sport_type')).toBe('cycling')
  })
})
