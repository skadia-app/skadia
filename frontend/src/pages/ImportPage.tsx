import { useState, useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import {
  scanGarminExport,
  scanStravaExport,
  getFusionQueue,
  type ScanResult,
  type ImportOptions,
  type ConflictStrategy,
  type FusionQueueEntry,
} from '@/api/importWizard'
import DuplicateReviewPanel from '@/components/DuplicateReviewPanel'

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
  errors?: string[] | null
}

interface LogEntry {
  timestamp: Date
  message: string
}

type WizardPhase = 'scan' | 'config' | 'review' | 'import'

// ---------------------------------------------------------------------------
// API helpers — Garmin
// ---------------------------------------------------------------------------

async function detectGarminModules(sourcePath: string): Promise<DetectedModule[]> {
  const res = await apiClient<{ modules: DetectedModule[] }>('/import/garmin/detect', {
    method: 'POST',
    body: { source_path: sourcePath },
  })
  return res.modules ?? []
}

async function startGarminImportJob(sourcePath: string, modules: string[], options: ImportOptions): Promise<string> {
  const res = await apiClient<{ job_id: string }>('/import/garmin/start', {
    method: 'POST',
    body: { source_path: sourcePath, modules, options },
  })
  return res.job_id
}

async function getGarminImportJob(jobId: string): Promise<ImportJob> {
  return apiClient<ImportJob>(`/import/garmin/jobs/${jobId}`)
}

// ---------------------------------------------------------------------------
// API helpers — Strava
// ---------------------------------------------------------------------------

async function detectStravaModules(sourcePath: string): Promise<DetectedModule[]> {
  const res = await apiClient<{ modules: DetectedModule[] }>('/import/strava/detect', {
    method: 'POST',
    body: { source_path: sourcePath },
  })
  return res.modules ?? []
}

async function startStravaImportJob(sourcePath: string, modules: string[], options: ImportOptions): Promise<string> {
  const res = await apiClient<{ job_id: string }>('/import/strava/start', {
    method: 'POST',
    body: { source_path: sourcePath, modules, options },
  })
  return res.job_id
}

async function getStravaImportJob(jobId: string): Promise<ImportJob> {
  return apiClient<ImportJob>(`/import/strava/jobs/${jobId}`)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatTimestamp(d: Date): string {
  return d.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function categoryLabel(cat: string): string {
  const labels: Record<string, string> = {
    activities: 'Aktivitäten',
    gear: 'Ausrüstung',
    profile: 'Profil',
    privacy_zones: 'Privacy-Zonen',
    health: 'Gesundheit',
  }
  return labels[cat] ?? cat
}

// Max log entries to keep
const MAX_LOG_ENTRIES = 200

// ---------------------------------------------------------------------------
// Default prefill paths
// ---------------------------------------------------------------------------

const DEFAULT_GARMIN_PATH = '/home/user/Projekte/Skadia/garmin_export/DI_CONNECT'
const DEFAULT_STRAVA_PATH = '/home/user/Projekte/Skadia/strava_export'

// ---------------------------------------------------------------------------
// Step indicator
// ---------------------------------------------------------------------------

interface StepIndicatorProps {
  phase: WizardPhase
}

const STEPS: { id: WizardPhase; label: string; num: number }[] = [
  { id: 'scan', label: 'Analysieren', num: 1 },
  { id: 'config', label: 'Konfigurieren', num: 2 },
  { id: 'review', label: 'Prüfen', num: 3 },
  { id: 'import', label: 'Importieren', num: 4 },
]

function StepIndicator({ phase }: StepIndicatorProps) {
  const currentIdx = STEPS.findIndex(s => s.id === phase)
  return (
    <div className="flex items-center gap-0 mb-6">
      {STEPS.map((step, idx) => {
        const isActive = step.id === phase
        const isDone = idx < currentIdx
        return (
          <div key={step.id} className="flex items-center flex-1 last:flex-none">
            <div className="flex items-center gap-2">
              <div
                className={`w-7 h-7 rounded-full flex items-center justify-center text-xs font-semibold shrink-0 transition-colors ${
                  isDone
                    ? 'bg-green-500 text-white'
                    : isActive
                    ? 'bg-accent-DEFAULT text-white'
                    : 'bg-neutral-200 dark:bg-neutral-700 text-neutral-500 dark:text-neutral-400'
                }`}
              >
                {isDone ? (
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                ) : (
                  step.num
                )}
              </div>
              <span
                className={`text-sm font-medium whitespace-nowrap ${
                  isActive
                    ? 'text-neutral-900 dark:text-neutral-100'
                    : isDone
                    ? 'text-neutral-600 dark:text-neutral-400'
                    : 'text-neutral-400 dark:text-neutral-500'
                }`}
              >
                {step.label}
              </span>
            </div>
            {idx < STEPS.length - 1 && (
              <div
                className={`flex-1 h-px mx-3 transition-colors ${
                  isDone ? 'bg-green-400' : 'bg-neutral-200 dark:bg-neutral-700'
                }`}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Phase 1: Scan
// ---------------------------------------------------------------------------

interface ScanPhaseProps {
  platform: 'garmin' | 'strava'
  path: string
  onDone: (result: ScanResult) => void
}

function ScanPhase({ platform, path, onDone }: ScanPhaseProps) {
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<ScanResult | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function handleScan() {
    setLoading(true)
    setError(null)
    try {
      const fn = platform === 'garmin' ? scanGarminExport : scanStravaExport
      const r = await fn(path)
      setResult(r)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const hasResults = result && Object.values(result).some(v => v !== undefined)

  return (
    <div className="space-y-4">
      <button
        onClick={handleScan}
        disabled={loading || !path}
        className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        {loading ? 'Analysiere...' : 'Export analysieren'}
      </button>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      {hasResults && result && (
        <>
          <div className="grid grid-cols-2 gap-3">
            {Object.entries(result).map(([cat, stats]) =>
              stats ? (
                <div
                  key={cat}
                  className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-3"
                >
                  <div className="font-medium text-sm">{categoryLabel(cat)}</div>
                  <div className="text-xs text-neutral-500 mt-0.5">
                    {stats.new} neu · {stats.existing} vorhanden · {stats.found} gesamt
                  </div>
                </div>
              ) : null
            )}
          </div>
          <button
            onClick={() => onDone(result)}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
          >
            Weiter zur Konfiguration →
          </button>
        </>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Phase 2: Config
// ---------------------------------------------------------------------------

interface ConfigPhaseProps {
  platform: 'garmin' | 'strava'
  scanResult: ScanResult
  options: ImportOptions
  onChange: (o: ImportOptions) => void
  onStart: () => void
}

function ConfigPhase({ platform, scanResult, options, onChange, onStart }: ConfigPhaseProps) {
  const categories = Object.entries(scanResult)
    .filter(([, stats]) => stats !== undefined)
    .map(([cat]) => cat)

  const selectedCategories = options.categories ?? categories

  const showDateRange =
    options.categories === undefined || options.categories.includes('activities')

  return (
    <div className="space-y-6">
      {/* Kategorie-Toggles */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          Was importieren?
        </h3>
        {categories.map(cat => {
          const stats = scanResult[cat]!
          const selected = selectedCategories.includes(cat)
          return (
            <label key={cat} className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={selected}
                onChange={e => {
                  const next = e.target.checked
                    ? [...selectedCategories.filter(c => c !== cat), cat]
                    : selectedCategories.filter(c => c !== cat)
                  onChange({ ...options, categories: next })
                }}
                className="w-4 h-4 accent-orange-500"
              />
              <span className="flex-1 text-sm">{categoryLabel(cat)}</span>
              <span className="text-xs text-neutral-400">
                {stats.new} neu · {stats.existing} vorhanden
              </span>
            </label>
          )
        })}
      </div>

      {/* Datumsbereich */}
      {showDateRange && (
        <div>
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
            Datumsbereich (optional)
          </h3>
          <div className="flex items-center gap-2">
            <input
              type="date"
              value={options.date_from ?? ''}
              onChange={e => onChange({ ...options, date_from: e.target.value || undefined })}
              className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100"
            />
            <span className="text-neutral-400">–</span>
            <input
              type="date"
              value={options.date_to ?? ''}
              onChange={e => onChange({ ...options, date_to: e.target.value || undefined })}
              className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100"
            />
          </div>
        </div>
      )}

      {/* Konflikt-Strategie */}
      <div>
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          Bei bereits vorhandenen Aktivitäten:
        </h3>
        <div className="space-y-2">
          {(
            [
              {
                value: 'enrich_missing' as ConflictStrategy,
                label: 'Fehlende Felder ergänzen',
                desc: 'Leere Werte (HR, Power, etc.) aus neuer Quelle auffüllen',
              },
              {
                value: 'replace_all' as ConflictStrategy,
                label: 'Alles ersetzen',
                desc: 'Bestehende Aktivität vollständig überschreiben',
              },
              {
                value: 'skip' as ConflictStrategy,
                label: 'Überspringen',
                desc: 'Bereits vorhandene Aktivitäten ignorieren',
              },
            ] satisfies Array<{ value: ConflictStrategy; label: string; desc: string }>
          ).map(opt => (
            <label key={opt.value} className="flex gap-3 cursor-pointer">
              <input
                type="radio"
                name={`strategy-${platform}`}
                value={opt.value}
                checked={options.conflict_strategy === opt.value}
                onChange={() => onChange({ ...options, conflict_strategy: opt.value })}
                className="mt-0.5 accent-orange-500"
              />
              <div>
                <div className="text-sm font-medium">{opt.label}</div>
                <div className="text-xs text-neutral-500">{opt.desc}</div>
              </div>
            </label>
          ))}
        </div>
      </div>

      <button
        onClick={onStart}
        disabled={selectedCategories.length === 0}
        className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Import starten →
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Phase 3: Import (progress + log)
// ---------------------------------------------------------------------------

interface ImportPhaseProps {
  jobId: string | null
  job: ImportJob | null
  importing: boolean
  importError: string | null
  log: LogEntry[]
  logEndRef: React.RefObject<HTMLDivElement | null>
  progress: number
}

function ImportPhase({
  jobId,
  job,
  importing,
  importError,
  log,
  logEndRef,
  progress,
}: ImportPhaseProps) {
  return (
    <div className="space-y-4">
      {importing && !jobId && (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">Starte Job...</p>
      )}

      {importError && (
        <p className="text-sm text-red-600 dark:text-red-400">{importError}</p>
      )}

      {jobId && (
        <div className="space-y-4">
          <div className="flex items-center justify-between text-sm">
            <span
              className={`font-medium ${
                job?.status === 'completed'
                  ? 'text-green-600 dark:text-green-400'
                  : job?.status === 'failed'
                  ? 'text-red-600 dark:text-red-400'
                  : 'text-neutral-700 dark:text-neutral-300'
              }`}
            >
              {job?.status === 'completed'
                ? 'Abgeschlossen'
                : job?.status === 'failed'
                ? 'Fehlgeschlagen'
                : 'Lauft...'}
            </span>
            <span className="text-xs text-neutral-500 tabular-nums font-mono">
              Job {jobId.slice(0, 8)}
              {job && job.total_modules > 0 &&
                ` · ${job.completed_modules}/${job.total_modules} Module (${progress} %)`}
            </span>
          </div>

          {/* Progress bar */}
          <div className="w-full h-2 bg-neutral-200 dark:bg-neutral-700 rounded-full overflow-hidden">
            <div
              className={`h-full rounded-full transition-all duration-500 ${
                job?.status === 'completed'
                  ? 'bg-green-500'
                  : job?.status === 'failed'
                  ? 'bg-red-500'
                  : 'bg-accent-DEFAULT'
              }`}
              style={{
                width:
                  job?.status === 'completed'
                    ? '100%'
                    : job?.status === 'running' && job.total_modules === 0
                    ? '10%'
                    : `${progress}%`,
              }}
            />
          </div>

          {/* Current module indicator */}
          {job?.status === 'running' && job.current_module && (
            <p className="text-xs text-neutral-500 dark:text-neutral-400 font-mono truncate">
              {job.current_module}
            </p>
          )}

          {/* Terminal log */}
          {log.length > 0 && (
            <div
              className="rounded-lg bg-neutral-900 border border-neutral-800 p-3 h-48 overflow-y-auto font-mono text-xs leading-relaxed"
              aria-label="Import-Log"
            >
              {log.map((entry, i) => (
                <div key={i} className="flex gap-3">
                  <span className="shrink-0 text-neutral-500 select-none">
                    {formatTimestamp(entry.timestamp)}
                  </span>
                  <span
                    className={
                      entry.message.startsWith('Fehler:')
                        ? 'text-red-400'
                        : entry.message.startsWith('Abgeschlossen')
                        ? 'text-green-400'
                        : 'text-neutral-200'
                    }
                  >
                    {entry.message}
                  </span>
                </div>
              ))}
              <div ref={logEndRef} />
            </div>
          )}

          {/* Completion summary */}
          {job?.status === 'completed' && (
            <div className="rounded-lg bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 p-3">
              <p className="text-sm font-medium text-green-800 dark:text-green-300">
                Import abgeschlossen — {job.completed_modules} Module verarbeitet.
              </p>
              <p className="text-xs text-green-700 dark:text-green-400 mt-1">
                Die importierten Aktivitäten sind nun in der Aktivitätenliste sichtbar.
              </p>
            </div>
          )}

          {/* Failure summary */}
          {job?.status === 'failed' && (
            <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-3 space-y-1">
              <p className="text-sm font-medium text-red-800 dark:text-red-300">
                Import fehlgeschlagen.
              </p>
              {job.errors && job.errors.length > 0 && (
                <ul className="mt-1 space-y-0.5">
                  {job.errors.map((err, i) => (
                    <li key={i} className="text-xs text-red-700 dark:text-red-400 font-mono">
                      {err}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// ImportPanel — wizard with 3 phases
// ---------------------------------------------------------------------------

interface ImportPanelProps {
  platform: 'garmin' | 'strava'
  defaultPath: string
  dirLabel: string
  dirHint: string
  dirPlaceholder: string
  detectFn: (path: string) => Promise<DetectedModule[]>
  startFn: (path: string, modules: string[], options: ImportOptions) => Promise<string>
  pollFn: (jobId: string) => Promise<ImportJob>
}

function ImportPanel({
  platform,
  defaultPath,
  dirLabel,
  dirHint,
  dirPlaceholder,
  detectFn,
  startFn,
  pollFn,
}: ImportPanelProps) {
  const [sourcePath, setSourcePath] = useState(defaultPath)

  // Wizard state
  const [phase, setPhase] = useState<WizardPhase>('scan')
  const [scanResult, setScanResult] = useState<ScanResult | null>(null)
  const [importOptions, setImportOptions] = useState<ImportOptions>({
    conflict_strategy: 'enrich_missing',
  })

  // Import job state
  const [jobId, setJobId] = useState<string | null>(null)
  const [job, setJob] = useState<ImportJob | null>(null)
  const [importing, setImporting] = useState(false)
  const [importError, setImportError] = useState<string | null>(null)
  const [log, setLog] = useState<LogEntry[]>([])

  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const logEndRef = useRef<HTMLDivElement | null>(null)
  const lastModuleRef = useRef<string | null>(null)

  // Auto-scroll log to bottom
  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [log])

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

  function appendLog(message: string) {
    setLog(prev => {
      const next = [...prev, { timestamp: new Date(), message }]
      if (next.length > MAX_LOG_ENTRIES) return next.slice(next.length - MAX_LOG_ENTRIES)
      return next
    })
  }

  function handleGoToReview() {
    setPhase('review')
  }

  async function handleStartImport() {
    // Determine which modules to import based on selected categories
    // Map categories back to modules via detectFn, or use categories directly as module names
    const categoriesToImport = importOptions.categories ?? []

    setPhase('import')
    setImporting(true)
    setImportError(null)
    setJob(null)
    setJobId(null)
    setLog([])
    lastModuleRef.current = null

    try {
      // Use the detect API to get actual module names for the selected categories,
      // then filter to only those that match selected categories.
      // If categories array is empty (all selected / no filter set), detect all available.
      const allModules = await detectFn(sourcePath.trim())
      const availableModules = allModules.filter(m => m.available)
      const modulesToRun =
        categoriesToImport.length === 0
          ? availableModules.map(m => m.name)
          : availableModules
              .filter(m => categoriesToImport.includes(m.name))
              .map(m => m.name)

      const id = await startFn(sourcePath.trim(), modulesToRun, importOptions)
      setJobId(id)
      appendLog(`Job gestartet (${id.slice(0, 8)})`)

      pollingRef.current = setInterval(async () => {
        try {
          const j = await pollFn(id)
          setJob(j)

          if (j.current_module && j.current_module !== lastModuleRef.current) {
            lastModuleRef.current = j.current_module
            appendLog(j.current_module)
          }

          if (j.status === 'completed' && lastModuleRef.current !== '__completed__') {
            lastModuleRef.current = '__completed__'
            appendLog(`Abgeschlossen — ${j.completed_modules} Module verarbeitet`)
          }
          if (j.status === 'failed' && lastModuleRef.current !== '__failed__') {
            lastModuleRef.current = '__failed__'
            appendLog('Import fehlgeschlagen')
            if (j.errors && j.errors.length > 0) {
              j.errors.forEach(err => appendLog(`Fehler: ${err}`))
            }
          }
        } catch {
          // ignore transient polling errors
        }
      }, 2000)
    } catch (e) {
      setImportError(e instanceof Error ? e.message : String(e))
      setImporting(false)
      setPhase('config')
    }
  }

  const progress =
    job && job.total_modules > 0
      ? Math.round((job.completed_modules / job.total_modules) * 100)
      : 0

  const { data: fusionQueue = [] } = useQuery<FusionQueueEntry[]>({
    queryKey: ['fusion-queue'],
    queryFn: () => getFusionQueue(20),
    enabled: phase === 'review',
  })

  return (
    <div className="space-y-6">
      {/* Source path input — always visible */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
            {dirLabel}
          </h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">{dirHint}</p>
        </div>
        <div className="flex gap-3">
          <input
            type="text"
            value={sourcePath}
            onChange={e => {
              setSourcePath(e.target.value)
              // Reset wizard if path changes
              if (phase !== 'scan') {
                setPhase('scan')
                setScanResult(null)
              }
            }}
            placeholder={dirPlaceholder}
            disabled={phase === 'import' && importing}
            className="flex-1 rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50"
          />
        </div>
      </section>

      {/* Wizard panel */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <StepIndicator phase={phase} />

        {phase === 'scan' && (
          <ScanPhase
            platform={platform}
            path={sourcePath.trim()}
            onDone={result => {
              setScanResult(result)
              // Pre-select all available categories
              const cats = Object.entries(result)
                .filter(([, stats]) => stats !== undefined)
                .map(([cat]) => cat)
              setImportOptions(prev => ({ ...prev, categories: cats }))
              setPhase('config')
            }}
          />
        )}

        {phase === 'config' && scanResult && (
          <ConfigPhase
            platform={platform}
            scanResult={scanResult}
            options={importOptions}
            onChange={setImportOptions}
            onStart={handleGoToReview}
          />
        )}

        {phase === 'review' && (
          <div className="space-y-4">
            <div>
              <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
                Fusionskandidaten prüfen
              </h2>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
                Aktivitäten die möglicherweise mit zwei Geräten aufgezeichnet wurden.
              </p>
            </div>
            <DuplicateReviewPanel
              entries={fusionQueue}
              onDone={handleStartImport}
            />
          </div>
        )}

        {phase === 'import' && (
          <ImportPhase
            jobId={jobId}
            job={job}
            importing={importing}
            importError={importError}
            log={log}
            logEndRef={logEndRef}
            progress={progress}
          />
        )}

        {/* Back navigation */}
        {phase === 'config' && (
          <button
            onClick={() => setPhase('scan')}
            className="mt-4 text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
          >
            ← Zurück zur Analyse
          </button>
        )}
        {phase === 'review' && (
          <button
            onClick={() => setPhase('config')}
            className="mt-4 text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
          >
            ← Zurück zur Konfiguration
          </button>
        )}

        {/* New import button — only after import completes */}
        {phase === 'import' &&
          job &&
          (job.status === 'completed' || job.status === 'failed') && (
            <button
              onClick={() => {
                setPhase('scan')
                setScanResult(null)
                setJob(null)
                setJobId(null)
                setLog([])
                setImportOptions({ conflict_strategy: 'enrich_missing' })
              }}
              className="mt-4 text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
            >
              ← Neuen Import starten
            </button>
          )}
      </section>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main page with tabs
// ---------------------------------------------------------------------------

type TabId = 'garmin' | 'strava'

export default function ImportPage() {
  const [activeTab, setActiveTab] = useState<TabId>('garmin')

  const tabs: { id: TabId; label: string }[] = [
    { id: 'garmin', label: 'Garmin' },
    { id: 'strava', label: 'Strava' },
  ]

  return (
    <div className="max-w-3xl mx-auto space-y-8 py-6 px-4">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Datenimport
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Importiere Aktivitäten, Ausrüstung und Profile aus einem lokalen Export.
        </p>
      </div>

      {/* Tab bar */}
      <div className="flex gap-1 border-b border-neutral-200 dark:border-neutral-800">
        {tabs.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px ${
              activeTab === tab.id
                ? 'border-accent-DEFAULT text-accent-DEFAULT'
                : 'border-transparent text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {activeTab === 'garmin' && (
        <ImportPanel
          platform="garmin"
          defaultPath={DEFAULT_GARMIN_PATH}
          dirLabel="Garmin-Export-Verzeichnis"
          dirHint="Gib den absoluten Pfad zum entpackten Garmin-Export an (z.B. DI_CONNECT-Verzeichnis)."
          dirPlaceholder="/pfad/zum/garmin/export/DI_CONNECT"
          detectFn={detectGarminModules}
          startFn={startGarminImportJob}
          pollFn={getGarminImportJob}
        />
      )}

      {activeTab === 'strava' && (
        <ImportPanel
          platform="strava"
          defaultPath={DEFAULT_STRAVA_PATH}
          dirLabel="Strava-Export-Verzeichnis"
          dirHint="Gib den absoluten Pfad zum entpackten Strava-Export an (Verzeichnis mit activities.csv, profile.csv, bikes.csv etc.)."
          dirPlaceholder="/pfad/zum/strava/export"
          detectFn={detectStravaModules}
          startFn={startStravaImportJob}
          pollFn={getStravaImportJob}
        />
      )}
    </div>
  )
}
