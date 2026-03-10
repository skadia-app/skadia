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

function JobRow({ job }: { job: ActiveJob }) {
  const isOSMImport = job.job_type === 'osm_import'
  const isDataImport = job.job_type === 'garmin_import' || job.job_type === 'strava_import'

  const label = isOSMImport
    ? 'OSM-Import'
    : job.job_type === 'garmin_import'
    ? 'Garmin-Import'
    : job.job_type === 'strava_import'
    ? 'Strava-Import'
    : job.job_type === 'activity_recompute_all'
    ? 'Alle Aktivitäten neu berechnen'
    : job.job_type === 'photo_preview_recompute'
    ? 'Foto-Vorschauen'
    : job.job_type === 'photo_clip_embed'
    ? 'Foto-Indexierung (KI)'
    : `Segment „${job.entity_name ?? 'Segment'}`

  const pct =
    !isOSMImport && job.progress_total > 0
      ? Math.round((job.progress_current / job.progress_total) * 100)
      : null

  const subLabel =
    !isOSMImport && !isDataImport && job.status === 'running' && job.progress_total > 0
      ? job.job_type === 'segment_recompute'
        ? `${job.progress_current} / ${job.progress_total} Aktivitäten neu berechnen`
        : job.job_type === 'photo_preview_recompute'
        ? `${job.progress_current} / ${job.progress_total} Thumbnails generiert`
        : job.job_type === 'photo_clip_embed'
        ? `${job.progress_current} / ${job.progress_total} Fotos indexiert`
        : `${job.progress_current} / ${job.progress_total} Aktivitäten`
      : isDataImport && job.status === 'running' && job.progress_total > 0
      ? `${job.progress_current} / ${job.progress_total} Kategorien`
      : null

  const downloadPct =
    isOSMImport && job.phase === 'download' && job.progress_total > 0
      ? Math.round((job.progress_current / job.progress_total) * 100)
      : null

  const phaseLabel =
    job.phase === 'download' ? 'Phase 1/3 — Herunterladen' :
    job.phase === 'import'   ? 'Phase 2/3 — Importieren' :
    job.phase === 'index'    ? 'Phase 3/3 — Indizieren' : null

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
          {isOSMImport && job.entity_name && (
            <span className="font-normal text-neutral-400"> ({job.entity_name})</span>
          )}
        </p>

        {/* OSM-Import: Phasen-Anzeige */}
        {isOSMImport && job.status === 'running' && (
          <>
            {phaseLabel && (
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">{phaseLabel}</p>
            )}
            {downloadPct !== null && (
              <div className="mt-1 h-1 bg-neutral-100 dark:bg-neutral-700 rounded-full overflow-hidden">
                <div
                  className="h-full bg-accent-DEFAULT rounded-full transition-all duration-500"
                  style={{ width: `${downloadPct}%` }}
                />
              </div>
            )}
            {job.phase === 'import' && (
              <>
                <div className="mt-1 h-1 bg-neutral-100 dark:bg-neutral-700 rounded-full overflow-hidden">
                  <div className="h-full w-full bg-accent-DEFAULT rounded-full animate-pulse" />
                </div>
                <p className="text-xs text-amber-600 dark:text-amber-400 mt-0.5">
                  Kann mehrere Stunden dauern
                </p>
              </>
            )}
            {job.phase === 'index' && job.progress_total > 0 && (
              <div className="mt-1 h-1 bg-neutral-100 dark:bg-neutral-700 rounded-full overflow-hidden">
                <div
                  className="h-full bg-accent-DEFAULT rounded-full transition-all duration-500"
                  style={{ width: `${Math.round((job.progress_current / job.progress_total) * 100)}%` }}
                />
              </div>
            )}
          </>
        )}

        {/* Normale Jobs: Fortschrittsbalken + SubLabel */}
        {!isOSMImport && (
          <>
            {pct !== null && job.status === 'running' && (
              <div className="mt-1 h-1 bg-neutral-100 dark:bg-neutral-700 rounded-full overflow-hidden">
                <div
                  className="h-full bg-accent-DEFAULT rounded-full transition-all duration-500"
                  style={{ width: `${pct}%` }}
                />
              </div>
            )}
            <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
              {job.status === 'running' && subLabel
                ? subLabel
                : job.status === 'completed'
                ? 'Abgeschlossen'
                : job.status === 'failed'
                ? 'Fehler'
                : null}
            </p>
          </>
        )}

        {/* Abgeschlossen/Fehler für OSM */}
        {isOSMImport && job.status !== 'running' && (
          <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
            {job.status === 'completed' ? 'Abgeschlossen' : 'Fehler'}
          </p>
        )}
      </div>
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
        title="Laufende Berechnungen"
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
            Berechnungen
          </p>

          {enrichmentEntries.map((entry) => (
            <EnrichmentRow key={entry.activityId} entry={entry} />
          ))}

          {jobs.map(job => (
            <JobRow key={job.id} job={job} />
          ))}

          {jobs.length === 0 && !hasEnrichment && (
            <p className="text-xs text-neutral-400 dark:text-neutral-500">
              Keine laufenden Berechnungen
            </p>
          )}
        </div>
      )}
    </div>
  )
}
