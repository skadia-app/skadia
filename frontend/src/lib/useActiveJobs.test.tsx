import { describe, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import type { ActiveJob } from '@/api/jobs'
import { invalidateQueriesForCompletedJob } from './useActiveJobs'

function makeJob(overrides: Partial<ActiveJob>): ActiveJob {
  return {
    id: 'job-1',
    job_type: 'activity_recompute',
    status: 'completed',
    progress_current: 1,
    progress_total: 1,
    started_at: '2026-04-26T00:00:00Z',
    ...overrides,
  }
}

describe('invalidateQueriesForCompletedJob', () => {
  it('invalidates the finished activity summary for single-activity recomputes', async () => {
    const queryClient = new QueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    invalidateQueriesForCompletedJob(queryClient, makeJob({
      job_type: 'activity_recompute',
      entity_id: 'activity-1',
    }))

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['activity', 'activity-1'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['activity-track-summary', 'activity-1'] })
  })

  it('invalidates multisport child lookups and all track summaries after multisport recompute', async () => {
    const queryClient = new QueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    invalidateQueriesForCompletedJob(queryClient, makeJob({
      job_type: 'recompute_multisport',
      entity_id: 'parent-1',
    }))

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['activity-children', 'parent-1'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['activity-track-summary'] })
  })

  it('invalidates all track summaries after enrichment backfill and imports', async () => {
    const queryClient = new QueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    invalidateQueriesForCompletedJob(queryClient, makeJob({ job_type: 'backfill_activity_enrichments' }))
    invalidateQueriesForCompletedJob(queryClient, makeJob({ job_type: 'data_import' }))

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['activity-track-summary'] })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['activities'] })
  })
})
