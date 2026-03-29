import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { fetchActiveJobs } from '@/api/jobs'
import type { ActiveJob } from '@/api/jobs'

const jobTypeLabels: Record<string, string> = {
  osm_import:              'OSM-Import',
  osm_enrichment:          'OSM-Anreicherung',
  garmin_import:           'Garmin-Import',
  strava_import:           'Strava-Import',
  komoot_import:           'Komoot-Import',
  segment_recompute:       'Segment-Neuberechnung',
  activity_recompute:      'Aktivität-Neuberechnung',
  activity_recompute_all:  'Alle Aktivitäten neu berechnen',
  photo_preview_recompute: 'Foto-Vorschauen',
  photo_clip_embed:        'Foto-Indexierung',
  phash_backfill:          'pHash-Berechnung',
  recompute_multisport:    'Multisport-Neuberechnung',
  tile_build:              'Routing-Tiles erstellen',
  activity_re_enrich:      'Enrichment-Update',
  thumbnail_backfill:      'Thumbnails regenerieren',
  shadowfs_reconcile:      'Dateispeicher-Prüfung',
  fusion_queue_compute:    'Fusionskandidaten berechnen',
}

export type { ActiveJob }

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
          const label = jobTypeLabels[job.job_type] ?? job.job_type
          const suffix = job.entity_name ? ` — ${job.entity_name}` : ''
          const title = job.status === 'completed'
            ? `${label}${suffix} abgeschlossen`
            : `${label}${suffix} fehlgeschlagen`
          new Notification(title, { icon: '/favicon.ico' })
        }
      }
      if (wasRunning && job.status === 'completed') {
        if (job.job_type === 'segment_recompute' && job.entity_id) {
          queryClient.invalidateQueries({ queryKey: ['segment', job.entity_id] })
          queryClient.invalidateQueries({ queryKey: ['segment-performances', job.entity_id] })
          queryClient.invalidateQueries({ queryKey: ['segments'] })
        }
        if (job.job_type === 'activity_recompute_all') {
          queryClient.invalidateQueries({ queryKey: ['activities'] })
        }
        if (job.job_type === 'osm_import') {
          queryClient.invalidateQueries({ queryKey: ['osm-status'] })
          queryClient.invalidateQueries({ queryKey: ['osm-regions-status'] })
        }
        if (job.job_type === 'osm_enrichment') {
          queryClient.invalidateQueries({ queryKey: ['osm-status'] })
          queryClient.invalidateQueries({ queryKey: ['osm-regions'] })
        }
        if (job.job_type === 'garmin_import' || job.job_type === 'strava_import') {
          queryClient.invalidateQueries({ queryKey: ['activities'] })
        }
        if (job.job_type === 'fusion_queue_compute') {
          queryClient.invalidateQueries({ queryKey: ['fusion-queue'] })
        }
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
