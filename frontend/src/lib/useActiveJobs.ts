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
}

export type { ActiveJob }

const POLL_INTERVAL_MS = 5000

export function useActiveJobs() {
  const queryClient = useQueryClient()
  const prevJobsRef = useRef<ActiveJob[]>([])

  const { data } = useQuery({
    queryKey: ['active-jobs'],
    queryFn: fetchActiveJobs,
    refetchInterval: (query) => {
      const data = query.state.data
      if (!data) return POLL_INTERVAL_MS
      const hasActive = data.jobs.some(j => j.status === 'running')
      return hasActive ? POLL_INTERVAL_MS : 30_000
    },
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
    staleTime: POLL_INTERVAL_MS - 500,
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
