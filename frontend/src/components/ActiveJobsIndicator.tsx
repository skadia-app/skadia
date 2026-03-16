import { useState, useRef, useEffect } from 'react'
import { RefreshCw, Check, X } from 'lucide-react'
import { useActiveJobs } from '@/lib/useActiveJobs'
import type { ActiveJob } from '@/lib/useActiveJobs'
import { useJobStream } from '@/lib/useJobStream'
import {
  useEnrichmentProgress,
  ENRICHMENT_MODULES,
  type EnrichmentActivityProgress,
} from '@/lib/useEnrichmentProgress'

function formatElapsed(startedAt: string): string {
  const diff = Date.now() - new Date(startedAt).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'gerade gestartet'
  const hours = Math.floor(mins / 60)
  const remainMins = mins % 60
  if (hours === 0) return `seit ${mins}min`
  return `seit ${hours}h ${remainMins.toString().padStart(2, '0')}min`
}

function JobRow({ job }: { job: ActiveJob }) {
  const isOSMJob = job.job_type === 'osm_import' || job.job_type === 'osm_enrichment'
  const isDataImport = job.job_type === 'garmin_import' || job.job_type === 'strava_import' || job.job_type === 'komoot_import'

  const JOB_LABELS: Record<string, string> = {
    osm_import:              'OSM-Import',
    osm_enrichment:          'OSM-Anreicherung',
    garmin_import:           'Garmin-Synchronisierung',
    strava_import:           'Strava-Synchronisierung',
    komoot_import:           'Komoot-Import',
    activity_recompute_all:  'Alle Aktivitäten neu berechnen',
    activity_recompute:      'Aktivität neu berechnen',
    recompute_multisport:    'Multisport neu berechnen',
    photo_preview_recompute: 'Foto-Vorschaubilder neu erstellen',
    photo_thumb_recompute:   'Foto-Thumbnails neu erstellen',
    photo_clip_embed:        'Foto-Indexierung',
    photo_embed_backfill:    'Foto-Einbettungen berechnen',
    phash_backfill:          'Foto-Duplikaterkennung berechnen',
    elevation_reset:         'Höhenkorrektur zurücksetzen',
    elevation_correction:    'Höhenkorrektur anwenden',
    distance_correction:     'Distanzkorrektur anwenden',
    distance_reset:          'Distanzkorrektur zurücksetzen',
    dem_import:              'Höhenmodell importieren',
    apply_rules:             'Regeln anwenden',
    routing_graph_build:     'Routing-Graph erstellen',
    weather_backfill:        'Wetter-Daten nachfüllen',
  }

  const label = job.job_type === 'segment_recompute'
    ? `Segment „${job.entity_name ?? 'Segment'}"`
    : JOB_LABELS[job.job_type] ?? job.entity_name ?? job.job_type

  const pct =
    !isOSMJob && job.progress_total > 0
      ? Math.round((job.progress_current / job.progress_total) * 100)
      : null

  const subLabel =
    !isOSMJob && !isDataImport && job.status === 'running' && job.progress_total > 0
      ? job.job_type === 'segment_recompute'
        ? `${job.progress_current} / ${job.progress_total} Aktivitäten neu berechnen`
        : job.job_type === 'photo_preview_recompute' || job.job_type === 'photo_thumb_recompute'
        ? `${job.progress_current} / ${job.progress_total} Fotos verarbeitet`
        : job.job_type === 'photo_clip_embed' || job.job_type === 'photo_embed_backfill'
        ? `${job.progress_current} / ${job.progress_total} Fotos indexiert`
        : job.job_type === 'phash_backfill'
        ? `${job.progress_current} / ${job.progress_total} Fotos analysiert`
        : `${job.progress_current} / ${job.progress_total} Aktivitäten`
      : (job.job_type === 'garmin_import' || job.job_type === 'strava_import') && job.status === 'running' && job.progress_total > 0
      ? `${job.progress_current} / ${job.progress_total} Kategorien`
      : job.job_type === 'komoot_import' && job.status === 'running' && job.progress_total > 0
      ? `${job.progress_current} / ${job.progress_total} Touren`
      : null

  const downloadPct =
    isOSMJob && job.phase === 'download' && job.progress_total > 0
      ? Math.round((job.progress_current / job.progress_total) * 100)
      : null

  const isEnrichmentJob = job.job_type === 'osm_enrichment'
  const phaseLabel = isEnrichmentJob
    ? (job.phase === 'enrichment'  ? 'Phase 1/2 — Anreichern' :
       job.phase === 'splitting'   ? 'Phase 2/2 — Aufteilen' : null)
    : (job.phase === 'download'          ? 'Herunterladen' :
       job.phase === 'filter'            ? 'Filtern' :
       job.phase === 'import'            ? 'Importieren' :
       job.phase === 'merge'             ? 'Zusammenführen' :
       job.phase === 'analyze'           ? 'Analysieren' :
       job.phase === 'brouter_segments'  ? 'BRouter-Segmente' :
       job.phase?.startsWith('region_')  ? `Region ${job.phase.split('_')[1]}` : null)

  return (
    <div className="flex items-center gap-3 py-2 border-b border-neutral-100 dark:border-neutral-800 last:border-0">
      <div className="flex-shrink-0">
        {job.status === 'running' && (
          <RefreshCw size={13} className="animate-spin text-accent-DEFAULT" />
        )}
        {job.status === 'completed' && (
          <Check size={13} className="text-green-500" />
        )}
        {job.status === 'failed' && (
          <X size={13} className="text-red-500" />
        )}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium text-neutral-800 dark:text-neutral-200 truncate">
          {label}
          {isOSMJob && job.entity_name && (
            <span className="font-normal text-neutral-400"> ({job.entity_name})</span>
          )}
        </p>

        {/* OSM-Import: Phasen-Anzeige */}
        {isOSMJob && job.status === 'running' && (
          <>
            {phaseLabel && (
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">{phaseLabel}</p>
            )}
            <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
              {formatElapsed(job.started_at)}
            </p>
            {downloadPct !== null ? (
              <div className="mt-1 h-1 bg-neutral-100 dark:bg-neutral-700 rounded-full overflow-hidden">
                <div
                  className="h-full bg-accent-DEFAULT rounded-full transition-all duration-500"
                  style={{ width: `${downloadPct}%` }}
                />
              </div>
            ) : (
              <div className="mt-1 h-1 bg-neutral-100 dark:bg-neutral-700 rounded-full overflow-hidden relative">
                <div className="h-full w-full rounded-full bg-accent-DEFAULT/40 animate-[shimmer_1.5s_ease-in-out_infinite]" style={{ backgroundImage: 'linear-gradient(90deg, transparent 0%, var(--color-accent-DEFAULT) 50%, transparent 100%)', backgroundSize: '200% 100%' }} />
              </div>
            )}
          </>
        )}

        {/* Normale Jobs: Fortschrittsbalken + SubLabel */}
        {!isOSMJob && (
          <>
            {job.status === 'running' && (
              <div className="mt-1 h-1 bg-neutral-100 dark:bg-neutral-700 rounded-full overflow-hidden relative">
                {pct !== null ? (
                  <div
                    className="h-full bg-accent-DEFAULT rounded-full transition-all duration-500"
                    style={{ width: `${pct}%` }}
                  />
                ) : (
                  <div className="h-full w-full rounded-full bg-accent-DEFAULT/40 animate-[shimmer_1.5s_ease-in-out_infinite]" style={{ backgroundImage: 'linear-gradient(90deg, transparent 0%, var(--color-accent-DEFAULT) 50%, transparent 100%)', backgroundSize: '200% 100%' }} />
                )}
              </div>
            )}
            <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
              {job.status === 'running'
                ? (subLabel ? `${subLabel} · ${formatElapsed(job.started_at)}` : formatElapsed(job.started_at))
                : job.status === 'completed'
                ? 'Abgeschlossen'
                : job.status === 'failed'
                ? 'Fehler'
                : null}
            </p>
          </>
        )}

        {/* Abgeschlossen/Fehler für OSM */}
        {isOSMJob && job.status !== 'running' && (
          <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
            {job.status === 'completed' ? 'Abgeschlossen' : 'Fehler'}
          </p>
        )}
      </div>
      {isOSMJob && job.status === 'running' && (
        <button
          onClick={async (e) => {
            e.stopPropagation()
            try {
              const { apiClient } = await import('@/api/client')
              await apiClient(`/admin/osm-import/cancel/${job.id}`, { method: 'POST' })
            } catch (err) {
              console.error('Cancel job failed:', err)
            }
          }}
          className="flex-shrink-0 p-1 rounded hover:bg-red-100 dark:hover:bg-red-900/30 text-neutral-400 hover:text-red-500 transition-colors"
          title="Import abbrechen"
        >
          <X size={12} />
        </button>
      )}
    </div>
  )
}

function EnrichmentRow({ entry }: { entry: EnrichmentActivityProgress }) {
  const doneCount = Object.values(entry.modules).filter((s) => s === 'done').length
  const errorCount = Object.values(entry.modules).filter((s) => s === 'error').length
  const total = ENRICHMENT_MODULES.length
  const pct = Math.round(((doneCount + errorCount) / total) * 100)
  const currentModule = ENRICHMENT_MODULES.find((m) => entry.modules[m] === 'running')
  const allDone = doneCount + errorCount === total

  return (
    <div className="flex items-center gap-3 py-2 border-b border-neutral-100 dark:border-neutral-800 last:border-0">
      <div className="flex-shrink-0">
        {allDone ? (
          errorCount > 0 ? (
            <X size={13} className="text-amber-500" />
          ) : (
            <Check size={13} className="text-green-500" />
          )
        ) : (
          <RefreshCw size={13} className="animate-spin text-accent-DEFAULT" />
        )}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium text-neutral-800 dark:text-neutral-200 truncate">
          Enrichment #{entry.activityId.slice(0, 8)}
        </p>
        <div className="flex flex-wrap gap-0.5 mt-1">
          {ENRICHMENT_MODULES.map((m) => {
            const s = entry.modules[m]
            return (
              <span
                key={m}
                className={`inline-block w-2 h-2 rounded-full ${
                  s === 'done'
                    ? 'bg-green-500'
                    : s === 'running'
                    ? 'bg-accent-DEFAULT animate-pulse'
                    : s === 'error'
                    ? 'bg-red-500'
                    : 'bg-neutral-300 dark:bg-neutral-600'
                }`}
                title={`${m}: ${s}`}
              />
            )
          })}
        </div>
        <div className="mt-1 h-1 bg-neutral-100 dark:bg-neutral-700 rounded-full overflow-hidden">
          <div
            className="h-full bg-accent-DEFAULT rounded-full transition-all duration-500"
            style={{ width: `${pct}%` }}
          />
        </div>
        <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
          {allDone
            ? errorCount > 0
              ? `Fertig (${errorCount} Fehler)`
              : 'Abgeschlossen'
            : currentModule
            ? `${doneCount}/${total} — ${currentModule}`
            : `${doneCount}/${total} Module`}
        </p>
      </div>
    </div>
  )
}

export default function ActiveJobsIndicator() {
  useJobStream()
  const { jobs, isAnyActive } = useActiveJobs()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const enrichmentMap = useEnrichmentProgress((activities) => activities)
  const enrichmentEntries = Array.from(enrichmentMap.values())
  const hasEnrichment = enrichmentEntries.length > 0
  const runningEnrichmentCount = enrichmentEntries.filter((e) => {
    const vals = Object.values(e.modules)
    return vals.some((s) => s === 'running' || s === 'pending') && vals.some((s) => s !== 'pending')
  }).length

  // Schließen bei Klick außerhalb
  useEffect(() => {
    if (!open) return
    function handle(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [open])

  // Nicht rendern wenn keine Jobs vorhanden (auch keine kürzlich abgeschlossenen)
  if (!isAnyActive && jobs.length === 0 && !hasEnrichment) return null

  const runningJobCount = jobs.filter(j => j.status === 'running').length
  const totalRunning = runningJobCount + runningEnrichmentCount

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(prev => !prev)}
        className="flex items-center gap-1.5 px-2 py-1.5 rounded-lg text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors text-xs"
        title="Laufende Jobs"
      >
        <RefreshCw
          size={14}
          className={
            isAnyActive || runningEnrichmentCount > 0
              ? 'animate-spin text-accent-DEFAULT'
              : 'text-neutral-400'
          }
        />
        {totalRunning > 0 && (
          <span className="text-accent-DEFAULT font-medium tabular-nums">
            {totalRunning}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 w-64 rounded-xl border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg z-50 p-3">
          <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
            Jobs
          </p>

          {enrichmentEntries.map((entry) => (
            <EnrichmentRow key={entry.activityId} entry={entry} />
          ))}

          {jobs.map(job => (
            <JobRow key={job.id} job={job} />
          ))}

          {jobs.length === 0 && !hasEnrichment && (
            <p className="text-xs text-neutral-400 dark:text-neutral-500">
              Keine laufenden Jobs
            </p>
          )}
        </div>
      )}
    </div>
  )
}
