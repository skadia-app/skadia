import { useState, useEffect, useRef } from 'react'
import { apiClient } from '@/api/client'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface DetectedModule {
  name: string
  description: string
  available: boolean
  item_count: number
}

interface ImportJob {
  id: string
  status: string
  source_path: string
  total_modules: number
  completed_modules: number
  current_module: string | null
  started_at: string | null
  completed_at: string | null
  created_at: string
}

// ---------------------------------------------------------------------------
// API helpers
// ---------------------------------------------------------------------------

async function detectGarminModules(sourcePath: string): Promise<DetectedModule[]> {
  const res = await apiClient<{ modules: DetectedModule[] }>('/import/garmin/detect', {
    method: 'POST',
    body: { source_path: sourcePath },
  })
  return res.modules ?? []
}

async function startGarminImport(sourcePath: string, modules: string[]): Promise<string> {
  const res = await apiClient<{ job_id: string }>('/import/garmin/start', {
    method: 'POST',
    body: { source_path: sourcePath, modules },
  })
  return res.job_id
}

async function getImportJob(jobId: string): Promise<ImportJob> {
  return apiClient<ImportJob>(`/import/garmin/jobs/${jobId}`)
}

// ---------------------------------------------------------------------------
// Default prefill
// ---------------------------------------------------------------------------

const DEFAULT_PATH = '/home/user/Projekte/Skadia/garmin_export/DI_CONNECT'

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function ImportPage() {
  const [sourcePath, setSourcePath] = useState(DEFAULT_PATH)
  const [modules, setModules] = useState<DetectedModule[]>([])
  const [selectedModules, setSelectedModules] = useState<Set<string>>(new Set())
  const [detecting, setDetecting] = useState(false)
  const [detectError, setDetectError] = useState<string | null>(null)
  const [jobId, setJobId] = useState<string | null>(null)
  const [job, setJob] = useState<ImportJob | null>(null)
  const [importing, setImporting] = useState(false)
  const [importError, setImportError] = useState<string | null>(null)

  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Stop polling when job is done
  useEffect(() => {
    if (job && (job.status === 'completed' || job.status === 'failed')) {
      if (pollingRef.current) {
        clearInterval(pollingRef.current)
        pollingRef.current = null
      }
      setImporting(false)
    }
  }, [job])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [])

  async function handleDetect() {
    if (!sourcePath.trim()) return
    setDetecting(true)
    setDetectError(null)
    setModules([])
    setSelectedModules(new Set())
    try {
      const detected = await detectGarminModules(sourcePath.trim())
      setModules(detected)
      // Pre-select all available modules
      setSelectedModules(new Set(detected.filter(m => m.available).map(m => m.name)))
    } catch (e) {
      setDetectError(e instanceof Error ? e.message : String(e))
    } finally {
      setDetecting(false)
    }
  }

  function toggleModule(name: string) {
    setSelectedModules((prev: Set<string>) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  async function handleStartImport() {
    if (selectedModules.size === 0) return
    setImporting(true)
    setImportError(null)
    setJob(null)
    setJobId(null)
    try {
      const id = await startGarminImport(sourcePath.trim(), Array.from(selectedModules))
      setJobId(id)
      // Start polling
      pollingRef.current = setInterval(async () => {
        try {
          const j = await getImportJob(id)
          setJob(j)
        } catch {
          // ignore transient errors
        }
      }, 2000)
    } catch (e) {
      setImportError(e instanceof Error ? e.message : String(e))
      setImporting(false)
    }
  }

  const progress =
    job && job.total_modules > 0
      ? Math.round((job.completed_modules / job.total_modules) * 100)
      : 0

  return (
    <div className="max-w-3xl mx-auto space-y-8 py-6 px-4">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Datenimport
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Importiere Aktivitaten, Ausrustung und Gesundheitsdaten aus einem Garmin-Export.
        </p>
      </div>

      {/* Source path input */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
            Garmin-Export-Verzeichnis
          </h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Gib den absoluten Pfad zum entpackten Garmin-Export an (z.B. DI_CONNECT-Verzeichnis).
          </p>
        </div>
        <div className="flex gap-3">
          <input
            type="text"
            value={sourcePath}
            onChange={e => setSourcePath(e.target.value)}
            placeholder="/pfad/zum/garmin/export"
            className="flex-1 rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
          />
          <button
            onClick={handleDetect}
            disabled={detecting || !sourcePath.trim()}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {detecting ? 'Erkenne...' : 'Erkennen'}
          </button>
        </div>
        {detectError && (
          <p className="text-sm text-red-600 dark:text-red-400">{detectError}</p>
        )}
      </section>

      {/* Module selection */}
      {modules.length > 0 && (
        <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
            Verfugbare Module
          </h2>
          <div className="space-y-2">
            {modules.map(m => (
              <label
                key={m.name}
                className={`flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                  m.available
                    ? 'border-neutral-200 dark:border-neutral-700 hover:border-accent-DEFAULT dark:hover:border-accent-DEFAULT'
                    : 'border-neutral-100 dark:border-neutral-800 opacity-50 cursor-not-allowed'
                }`}
              >
                <input
                  type="checkbox"
                  checked={selectedModules.has(m.name)}
                  onChange={() => toggleModule(m.name)}
                  disabled={!m.available}
                  className="mt-0.5 h-4 w-4 rounded border-neutral-300 text-accent-DEFAULT focus:ring-accent-DEFAULT"
                />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 capitalize">
                      {m.name}
                    </span>
                    {m.available && m.item_count > 0 && (
                      <span className="text-xs text-neutral-400">
                        ({m.item_count.toLocaleString()} Eintr.)
                      </span>
                    )}
                    {!m.available && (
                      <span className="text-xs text-neutral-400">nicht verfugbar</span>
                    )}
                  </div>
                  <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                    {m.description}
                  </p>
                </div>
              </label>
            ))}
          </div>

          <div className="flex items-center gap-3 pt-2">
            <button
              onClick={handleStartImport}
              disabled={importing || selectedModules.size === 0}
              className="px-6 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {importing ? 'Importiere...' : 'Import starten'}
            </button>
            <span className="text-xs text-neutral-500">
              {selectedModules.size} von {modules.filter(m => m.available).length} Modulen ausgewahlt
            </span>
          </div>
          {importError && (
            <p className="text-sm text-red-600 dark:text-red-400">{importError}</p>
          )}
        </section>
      )}

      {/* Progress */}
      {jobId && (
        <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
            Import-Fortschritt
          </h2>

          {!job && (
            <p className="text-sm text-neutral-500 dark:text-neutral-400">Starte Job...</p>
          )}

          {job && (
            <div className="space-y-3">
              <div className="flex items-center justify-between text-sm">
                <span className="text-neutral-700 dark:text-neutral-300">
                  {job.current_module
                    ? `Modul: ${job.current_module}`
                    : job.status === 'completed'
                    ? 'Abgeschlossen'
                    : job.status === 'failed'
                    ? 'Fehlgeschlagen'
                    : 'Lauft...'}
                </span>
                <span className="text-neutral-500 text-xs font-mono">Job {jobId.slice(0, 8)}</span>
              </div>

              {/* Progress bar */}
              <div className="w-full h-2 bg-neutral-200 dark:bg-neutral-700 rounded-full overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all duration-500 ${
                    job.status === 'completed'
                      ? 'bg-green-500'
                      : job.status === 'failed'
                      ? 'bg-red-500'
                      : 'bg-accent-DEFAULT'
                  }`}
                  style={{
                    width:
                      job.status === 'completed'
                        ? '100%'
                        : job.status === 'running' && job.total_modules === 0
                        ? '10%'
                        : `${progress}%`,
                  }}
                />
              </div>

              {job.status === 'completed' && (
                <div className="rounded-lg bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 p-3">
                  <p className="text-sm font-medium text-green-800 dark:text-green-300">
                    Import erfolgreich abgeschlossen.
                  </p>
                  <p className="text-xs text-green-700 dark:text-green-400 mt-1">
                    Die importierten Aktivitaten sind nun in der Aktivitatenliste sichtbar.
                  </p>
                </div>
              )}

              {job.status === 'failed' && (
                <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-3">
                  <p className="text-sm font-medium text-red-800 dark:text-red-300">
                    Import fehlgeschlagen.
                  </p>
                </div>
              )}
            </div>
          )}
        </section>
      )}
    </div>
  )
}
