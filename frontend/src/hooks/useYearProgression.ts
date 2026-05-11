import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchYearProgression, fetchYearProgressionRolling } from '@/api/analytics'
import type { YearProgressionResponse, YearProgressionRollingResponse } from '@/api/analytics'

export type YearProgressionMode = 'progress' | 'rolling'

// Metric keys shared between chart and modal.
export type MetricKey =
  | 'distance_m'
  | 'elevation_gain_m'
  | 'elevation_loss_m'
  | 'moving_time_s'
  | 'tss'
  | 'total_calories'

interface UseYearProgressionArgs {
  mode: YearProgressionMode
  metric: MetricKey
  periodDays?: number
  granularity?: 'day' | 'week'
  queryParams: string
}

interface UseYearProgressionResult<T> {
  data: T | undefined
  isLoading: boolean
  isError: boolean
}

// Overload for 'progress' mode — returns YearProgressionResponse.
export function useYearProgression(args: UseYearProgressionArgs & { mode: 'progress' }): UseYearProgressionResult<YearProgressionResponse>
// Overload for 'rolling' mode — returns YearProgressionRollingResponse.
export function useYearProgression(args: UseYearProgressionArgs & { mode: 'rolling' }): UseYearProgressionResult<YearProgressionRollingResponse>
// Implementation signature (not visible to callers) handles both modes with
// disjoint cache keys so React Query never conflates the two response shapes.
export function useYearProgression(args: UseYearProgressionArgs): UseYearProgressionResult<unknown> {
  const { mode, metric, periodDays = 30, granularity = 'day', queryParams } = args

  // Progress-mode params: ensure `metric` is always present in the query string.
  // Callers may omit metric from queryParams (they pass it separately for cache
  // keying); we normalise here so the API always receives it.
  const progressParams = useMemo(() => {
    const p = new URLSearchParams(queryParams)
    p.set('metric', metric)
    return p.toString()
  }, [queryParams, metric])

  const progressResult = useQuery({
    queryKey: ['analytics', 'year-progression', 'progress', metric, queryParams],
    queryFn: () => fetchYearProgression(progressParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
    enabled: mode === 'progress',
  })

  const rollingParams = buildRollingParams(queryParams, metric, periodDays, granularity)
  const rollingResult = useQuery({
    queryKey: ['analytics', 'year-progression', 'rolling', rollingParams],
    queryFn: () => fetchYearProgressionRolling(rollingParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
    enabled: mode === 'rolling',
  })

  if (mode === 'progress') {
    return {
      data: progressResult.data,
      isLoading: progressResult.isLoading,
      isError: progressResult.isError,
    }
  }

  return {
    data: rollingResult.data,
    isLoading: rollingResult.isLoading,
    isError: rollingResult.isError,
  }
}

function buildRollingParams(
  base: string,
  metric: MetricKey,
  periodDays: number,
  granularity: 'day' | 'week',
): string {
  const params = new URLSearchParams(base)
  params.set('metric', metric)
  params.set('period_days', String(periodDays))
  params.set('granularity', granularity)
  return params.toString()
}
