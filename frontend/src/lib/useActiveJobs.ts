import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { fetchActiveJobs } from '@/api/jobs'
import type { ActiveJob } from '@/api/jobs'
import { JOB_TYPE_LABELS } from '@/lib/jobLabels'

export type { ActiveJob }

export function invalidateQueriesForCompletedJob(queryClient: QueryClient, job: ActiveJob) {
  if ((job.job_type === 'segment_recompute' || job.job_type === 'segment_recalculation') && job.entity_id) {
    queryClient.invalidateQueries({ queryKey: ['segment', job.entity_id] })
    queryClient.invalidateQueries({ queryKey: ['segment-performances', job.entity_id] })
    queryClient.invalidateQueries({ queryKey: ['segments'] })
    return
  }

  if (job.job_type === 'activity_recompute' && job.entity_id) {
    queryClient.invalidateQueries({ queryKey: ['activity', job.entity_id] })
    queryClient.invalidateQueries({ queryKey: ['activity-streams', job.entity_id] })
    queryClient.invalidateQueries({ queryKey: ['activity-sensors', job.entity_id] })
    queryClient.invalidateQueries({ queryKey: ['activity-track-summary', job.entity_id] })
    // Lift candidates may change when reprocess includes the lift module — refresh
    // so the activity-detail review modal/banner reacts to newly detected rides.
    queryClient.invalidateQueries({ queryKey: ['lift-candidates', job.entity_id] })
    return
  }

  if (job.job_type === 'lift_recompute' && job.entity_id) {
    queryClient.invalidateQueries({ queryKey: ['activity', job.entity_id] })
    queryClient.invalidateQueries({ queryKey: ['lift-candidates', job.entity_id] })
    queryClient.invalidateQueries({ queryKey: ['lift-candidates-inspector', job.entity_id] })
    return
  }

  if (job.job_type === 'recompute_multisport') {
    queryClient.invalidateQueries({ queryKey: ['activities'] })
    if (job.entity_id) {
      queryClient.invalidateQueries({ queryKey: ['activity', job.entity_id] })
      queryClient.invalidateQueries({ queryKey: ['activity-children', job.entity_id] })
      // Multisport re-processing reruns lift detection on the parent activity.
      queryClient.invalidateQueries({ queryKey: ['lift-candidates', job.entity_id] })
    } else {
      // No entity_id — broadcast-invalidate all lift-candidate lists.
      queryClient.invalidateQueries({ queryKey: ['lift-candidates'] })
    }
    queryClient.invalidateQueries({ queryKey: ['activity-track-summary'] })
    return
  }

  if (
    job.job_type === 'activity_recompute_all' ||
    job.job_type === 'backfill_activity_enrichments' ||
    job.job_type === 'data_import_enrichment'
  ) {
    queryClient.invalidateQueries({ queryKey: ['activities'] })
    queryClient.invalidateQueries({ queryKey: ['activity-track-summary'] })
    // Bulk reprocessing reruns lift detection on all activities — broadcast-invalidate.
    queryClient.invalidateQueries({ queryKey: ['lift-candidates'] })
    if (job.entity_id) {
      queryClient.invalidateQueries({ queryKey: ['activity', job.entity_id] })
      queryClient.invalidateQueries({ queryKey: ['activity-streams', job.entity_id] })
      queryClient.invalidateQueries({ queryKey: ['activity-sensors', job.entity_id] })
      queryClient.invalidateQueries({ queryKey: ['activity-track-summary', job.entity_id] })
      queryClient.invalidateQueries({ queryKey: ['lift-candidates', job.entity_id] })
      queryClient.invalidateQueries({ queryKey: ['lift-candidates-inspector', job.entity_id] })
    }
    return
  }

  if (job.job_type === 'osm_import') {
    queryClient.invalidateQueries({ queryKey: ['osm-status'] })
    queryClient.invalidateQueries({ queryKey: ['osm-regions-status'] })
    return
  }

  if (job.job_type === 'osm_enrichment') {
    queryClient.invalidateQueries({ queryKey: ['osm-status'] })
    queryClient.invalidateQueries({ queryKey: ['osm-regions'] })
    return
  }

  if (job.job_type === 'garmin_import' || job.job_type === 'strava_import' || job.job_type === 'komoot_import') {
    queryClient.invalidateQueries({ queryKey: ['activities'] })
    queryClient.invalidateQueries({ queryKey: ['activity-track-summary'] })
    return
  }

  if (job.job_type === 'fusion_queue_compute') {
    queryClient.invalidateQueries({ queryKey: ['fusion-queue'] })
    return
  }

  if (job.job_type === 'gdpr_export') {
    queryClient.invalidateQueries({ queryKey: ['data-export-latest'] })
    return
  }

  if (job.job_type === 'data_import') {
    queryClient.invalidateQueries({ queryKey: ['activities'] })
    queryClient.invalidateQueries({ queryKey: ['tags'] })
    queryClient.invalidateQueries({ queryKey: ['gear'] })
    queryClient.invalidateQueries({ queryKey: ['segments'] })
    queryClient.invalidateQueries({ queryKey: ['activity-track-summary'] })
  }
}

export function useActiveJobs() {
  const queryClient = useQueryClient()
  const prevJobsRef = useRef<ActiveJob[]>([])

  const { data } = useQuery({
    queryKey: ['active-jobs'],
    queryFn: fetchActiveJobs,
    // SSE delivers data every 5s via setQueryData. This 120s poll is a safety net
    // for edge cases (suspended tabs, SSE failure, leader crash).
    refetchInterval: 120_000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
    staleTime: 30_000,
  })

  // Query-Invalidierung wenn Jobs abgeschlossen werden
  useEffect(() => {
    if (!data) return
    const prev = prevJobsRef.current
    const current = data.jobs

    for (const job of current) {
      const prevJob = prev.find(j => j.id === job.id)
      const wasRunning = prevJob?.status === 'running'
      if (wasRunning && (job.status === 'completed' || job.status === 'failed')) {
        if (document.hidden && 'Notification' in window && Notification.permission === 'granted') {
          const label = JOB_TYPE_LABELS[job.job_type] ?? job.job_type
          const suffix = job.entity_name ? ` — ${job.entity_name}` : ''
          const title = job.status === 'completed'
            ? `${label}${suffix} abgeschlossen`
            : `${label}${suffix} fehlgeschlagen`
          new Notification(title, { icon: '/favicon.ico' })
        }
      }
      if (wasRunning && job.status === 'completed') {
        invalidateQueriesForCompletedJob(queryClient, job)
      }
    }

    prevJobsRef.current = current
  }, [data, queryClient])

  const jobs = data?.jobs ?? []
  const isAnyActive = jobs.some(j => j.status === 'running')

  function isJobActiveForEntity(entityId: string): boolean {
    return jobs.some(j => j.entity_id === entityId && j.status === 'running')
  }

  function getJobForEntity(entityId: string): ActiveJob | undefined {
    return jobs.find(j => j.entity_id === entityId)
  }

  return { jobs, isAnyActive, isJobActiveForEntity, getJobForEntity }
}
