import { useState, useRef, useEffect } from 'react'
import { RefreshCw, Check, X } from 'lucide-react'
import { useActiveJobs } from '@/lib/useActiveJobs'
import type { ActiveJob } from '@/lib/useActiveJobs'

function JobRow({ job }: { job: ActiveJob }) {
  const isOSMImport = job.job_type === 'osm_import'

  const label = isOSMImport
    ? 'OSM-Import'
    : job.job_type === 'activity_recompute_all'
    ? 'Alle Aktivitäten neu berechnen'
    : `Segment „${job.entity_name ?? 'Segment'}`

  const pct =
    !isOSMImport && job.progress_total > 0
      ? Math.round((job.progress_current / job.progress_total) * 100)
      : null

  const subLabel =
    !isOSMImport && job.status === 'running' && job.progress_total > 0
      ? job.job_type === 'segment_recompute'
        ? `${job.progress_current} / ${job.progress_total} Aktivitäten neu berechnen`
        : `${job.progress_current} / ${job.progress_total} Aktivitäten`
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

export default function ActiveJobsIndicator() {
  const { jobs, isAnyActive } = useActiveJobs()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

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
  if (!isAnyActive && jobs.length === 0) return null

  const runningCount = jobs.filter(j => j.status === 'running').length

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
            isAnyActive
              ? 'animate-spin text-accent-DEFAULT'
              : 'text-neutral-400'
          }
        />
        {runningCount > 0 && (
          <span className="text-accent-DEFAULT font-medium tabular-nums">
            {runningCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 w-64 rounded-xl border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg z-50 p-3">
          <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
            Berechnungen
          </p>

          {jobs.map(job => (
            <JobRow key={job.id} job={job} />
          ))}

          {jobs.length === 0 && (
            <p className="text-xs text-neutral-400 dark:text-neutral-500">
              Keine laufenden Berechnungen
            </p>
          )}
        </div>
      )}
    </div>
  )
}
