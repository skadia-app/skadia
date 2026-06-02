import { useState, useEffect, useRef, Suspense, lazy } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { apiClient, apiUploadWithProgress } from '@/api/client'
import deLocale from '@/i18n/locales/de.json'
import {
  scanGarminExport,
  scanStravaExport,
  getFusionQueue,
  type ScanResult,
  type ImportOptions,
  type ConflictStrategy,
} from '@/api/importWizard'
import { fetchImportQueueCount } from '@/api/importQueue'
import { getCapabilities } from '@/api/routes'
import {
  komootLogin,
  komootScan,
  komootStartImport,
  komootCredentialsStatus,
  komootDeleteCredentials,
  getKomootImportJob,
  type KomootScanResult,
  type KomootJob,
} from '@/api/komoot'

const ImportQueueTab = lazy(() => import('@/components/ImportQueueTab'))
const DEFAULT_IMPORTS_PAGE_COPY = deLocale.importsPage

function interpolate(template: string, values: Record<string, string | number>): string {
  return template.replace(/\{\{(\w+)\}\}/g, (match, key: string) => {
    const value = values[key]
    return value === undefined ? match : String(value)
  })
}

function safeBackendIdentifier(value: string): string {
  return /^[A-Za-z0-9_.:-]+$/.test(value) ? value : 'unknown'
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

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
  kind?: 'info' | 'error' | 'completed'
}

type WizardPhase = 'scan' | 'config' | 'import'
const KOMOOT_PHASE_KEYS = new Set(['login', 'scan', 'config', 'import'])
const IMPORT_CATEGORY_KEYS = new Set(['activities', 'gear', 'profile', 'privacy_zones', 'media', 'health', 'monitoring'])

export interface KomootImportSelection {
  importRecorded: boolean
  importPlanned: boolean
  importPhotos: boolean
}

function komootSelectionUnit(selection: KomootImportSelection, t?: TFunction): { singular: string; plural: string } {
  if (selection.importPlanned && !selection.importRecorded && !selection.importPhotos) {
    return {
      singular: t ? t('importsPage.komoot.units.route_one') : DEFAULT_IMPORTS_PAGE_COPY.komoot.units.route_one,
      plural: t ? t('importsPage.komoot.units.route_other') : DEFAULT_IMPORTS_PAGE_COPY.komoot.units.route_other,
    }
  }
  if (selection.importRecorded && !selection.importPlanned && !selection.importPhotos) {
    return {
      singular: t ? t('importsPage.komoot.units.activity_one') : DEFAULT_IMPORTS_PAGE_COPY.komoot.units.activity_one,
      plural: t ? t('importsPage.komoot.units.activity_other') : DEFAULT_IMPORTS_PAGE_COPY.komoot.units.activity_other,
    }
  }
  return {
    singular: t ? t('importsPage.komoot.units.entry_one') : DEFAULT_IMPORTS_PAGE_COPY.komoot.units.entry_one,
    plural: t ? t('importsPage.komoot.units.entry_other') : DEFAULT_IMPORTS_PAGE_COPY.komoot.units.entry_other,
  }
}

export function komootProgressLabel(current: number, total: number, selection: KomootImportSelection, t?: TFunction): string {
  const unit = komootSelectionUnit(selection, t)
  return `${unit.singular} ${current} / ${total}`
}

export function komootCompletionLog(importedCount: number, selection: KomootImportSelection, t?: TFunction): string {
  if (!t) {
    if (importedCount <= 0) return DEFAULT_IMPORTS_PAGE_COPY.komoot.log.completedEmpty
    const unit = komootSelectionUnit(selection)
    return interpolate(DEFAULT_IMPORTS_PAGE_COPY.komoot.log.completedImported, { count: importedCount, unit: unit.plural })
  }
  if (importedCount <= 0) return t('importsPage.komoot.log.completedEmpty')
  const unit = komootSelectionUnit(selection, t)
  return t('importsPage.komoot.log.completedImported', { count: importedCount, unit: unit.plural })
}

export function komootAvailabilityText(selection: KomootImportSelection, t?: TFunction): string {
  if (selection.importPlanned && !selection.importRecorded && !selection.importPhotos) {
    return t ? t('importsPage.komoot.availability.routes') : DEFAULT_IMPORTS_PAGE_COPY.komoot.availability.routes
  }
  if (selection.importRecorded && !selection.importPlanned && !selection.importPhotos) {
    return t ? t('importsPage.komoot.availability.activities') : DEFAULT_IMPORTS_PAGE_COPY.komoot.availability.activities
  }
  return t ? t('importsPage.komoot.availability.activitiesAndRoutes') : DEFAULT_IMPORTS_PAGE_COPY.komoot.availability.activitiesAndRoutes
}

export function komootPhaseLabel(phase: string | null | undefined, t: TFunction): string {
  if (!phase) return ''
  return KOMOOT_PHASE_KEYS.has(phase) ? t(`importsPage.komoot.phases.${phase}`) : t('importsPage.komoot.phases.unknown')
}

// ---------------------------------------------------------------------------
// API helpers — Garmin
// ---------------------------------------------------------------------------

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

async function uploadGarminZIPScan(
  file: File,
  onUploaded?: () => void,
): Promise<{ source_path: string; scan_result: ScanResult }> {
  const fd = new FormData()
  fd.append('file', file)
  return apiUploadWithProgress('/import/garmin/upload-zip-scan', fd, onUploaded)
}

// ---------------------------------------------------------------------------
// API helpers — Strava
// ---------------------------------------------------------------------------

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

async function uploadStravaZIPScan(
  file: File,
  onUploaded?: () => void,
): Promise<{ source_path: string; scan_result: ScanResult }> {
  const fd = new FormData()
  fd.append('file', file)
  return apiUploadWithProgress('/import/strava/upload-zip-scan', fd, onUploaded)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatTimestamp(d: Date, language = 'de'): string {
  return d.toLocaleTimeString(language, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export function importCategoryLabel(cat: string, t: TFunction): string {
  const safeCategory = safeBackendIdentifier(cat)
  return IMPORT_CATEGORY_KEYS.has(safeCategory) ? t(`importsPage.categories.${safeCategory}`) : t('importsPage.categories.unknown')
}

export function importFoundLabel(count: number, t: TFunction): string {
  return t('importsPage.found', { count })
}

export function importJobStatusLabel(status: string | null | undefined, t: TFunction): string {
  switch (status) {
    case 'completed':
      return t('importsPage.status.completed')
    case 'failed':
      return t('importsPage.status.failed')
    case 'queued':
    case 'pending':
      return t('importsPage.status.queued')
    case 'running':
    case null:
    case undefined:
      return t('importsPage.status.running')
    default:
      return t('importsPage.status.unknown')
  }
}

export function importCurrentModuleLabel(module: string, t: TFunction): string {
  return t('importsPage.currentModule', {
    module: importCategoryLabel(module, t),
  })
}

export function importConflictStrategyOptions(t: TFunction): Array<{ value: ConflictStrategy; label: string; desc: string }> {
  return [
    {
      value: 'enrich_missing',
      label: t('importsPage.conflict.enrichMissing.label'),
      desc: t('importsPage.conflict.enrichMissing.desc'),
    },
    {
      value: 'replace_all',
      label: t('importsPage.conflict.replaceAll.label'),
      desc: t('importsPage.conflict.replaceAll.desc'),
    },
    {
      value: 'skip',
      label: t('importsPage.conflict.skip.label'),
      desc: t('importsPage.conflict.skip.desc'),
    },
  ]
}

// Max log entries to keep
const MAX_LOG_ENTRIES = 200

// ---------------------------------------------------------------------------
// Default prefill paths
// ---------------------------------------------------------------------------

const DEFAULT_GARMIN_PATH = ''
const DEFAULT_STRAVA_PATH = ''

// ---------------------------------------------------------------------------
// Step indicator
// ---------------------------------------------------------------------------

interface StepIndicatorProps {
  phase: WizardPhase
}

function StepIndicator({ phase }: StepIndicatorProps) {
  const { t } = useTranslation()
  const steps: { id: WizardPhase; label: string; num: number }[] = [
    { id: 'scan', label: t('importsPage.steps.scan'), num: 1 },
    { id: 'config', label: t('importsPage.steps.config'), num: 2 },
    { id: 'import', label: t('importsPage.steps.import'), num: 3 },
  ]
  const currentIdx = steps.findIndex(s => s.id === phase)
  return (
    <div className="flex items-center gap-0 mb-6">
      {steps.map((step, idx) => {
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
            {idx < steps.length - 1 && (
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
// Phase 1: Scan — results display (scan is triggered from card 1)
// ---------------------------------------------------------------------------

interface ScanResultsProps {
  result: ScanResult
  onContinue: (result: ScanResult) => void
}

function ScanResultsView({ result, onContinue }: ScanResultsProps) {
  const { t } = useTranslation()
  const hasAny = Object.values(result).some(v => v !== undefined)
  if (!hasAny) {
    return (
      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        {t('importsPage.noImportableData')}
      </p>
    )
  }
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3">
        {Object.entries(result).map(([cat, stats]) =>
          stats ? (
            <div
              key={cat}
              className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-3"
            >
              <div className="font-medium text-sm">{importCategoryLabel(cat, t)}</div>
              <div className="text-xs text-neutral-500 mt-0.5">
                {importFoundLabel(stats.found, t)}
              </div>
            </div>
          ) : null
        )}
      </div>
      <button
        onClick={() => onContinue(result)}
        className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
      >
        {t('importsPage.continueToConfig')}
      </button>
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
  const { t } = useTranslation()
  const categories = Object.entries(scanResult)
    .filter(([, stats]) => stats !== undefined)
    .map(([cat]) => cat)

  const selectedCategories = options.categories ?? categories

  const showDateRange =
    options.categories === undefined || options.categories.includes('activities')

  return (
    <div className="space-y-6">
      {/* Category toggles */}
      <div className="space-y-2">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          {t('importsPage.whatToImport')}
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
                className="w-4 h-4 accent-accent-DEFAULT"
              />
              <span className="flex-1 text-sm">{importCategoryLabel(cat, t)}</span>
              <span className="text-xs text-neutral-400">
                {importFoundLabel(stats.found, t)}
              </span>
            </label>
          )
        })}
      </div>

      {/* Date range */}
      {showDateRange && (
        <div>
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
            {t('importsPage.dateRangeOptional')}
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

      {/* Conflict strategy */}
      <div>
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          {t('importsPage.conflict.title')}
        </h3>
        <div className="space-y-2">
          {importConflictStrategyOptions(t).map(opt => (
            <label key={opt.value} className="flex gap-3 cursor-pointer">
              <input
                type="radio"
                name={`strategy-${platform}`}
                value={opt.value}
                checked={options.conflict_strategy === opt.value}
                onChange={() => onChange({ ...options, conflict_strategy: opt.value })}
                className="mt-0.5 accent-accent-DEFAULT"
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
        className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
      >
        {t('importsPage.startImport')}
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
  fusionCount: number | null
}

function ImportPhase({
  jobId,
  job,
  importing,
  importError,
  log,
  logEndRef,
  progress,
  fusionCount,
}: ImportPhaseProps) {
  const { t, i18n } = useTranslation()
  return (
    <div className="space-y-4">
      {importing && !jobId && (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">{t('importsPage.startingJob')}</p>
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
              {importJobStatusLabel(job?.status, t)}
            </span>
            <span className="text-xs text-neutral-500 tabular-nums font-mono">
              {t('importsPage.jobShort', { id: jobId.slice(0, 8) })}
              {job && job.total_modules > 0 &&
                t('importsPage.progress.categories', {
                  completed: job.completed_modules,
                  total: job.total_modules,
                  progress,
                })}
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
              {importCurrentModuleLabel(job.current_module, t)}
            </p>
          )}

          {/* Terminal log */}
          {log.length > 0 && (
            <div
              className="rounded-lg bg-neutral-900 border border-neutral-800 p-3 h-48 overflow-y-auto font-mono text-xs leading-relaxed"
              aria-label={t('importsPage.log.label')}
            >
              {log.map((entry, i) => (
                <div key={i} className="flex gap-3">
                  <span className="shrink-0 text-neutral-500 select-none">
                    {formatTimestamp(entry.timestamp, i18n.resolvedLanguage ?? i18n.language ?? 'de')}
                  </span>
                  <span
                    className={
                      entry.kind === 'error'
                        ? 'text-red-400'
                        : entry.kind === 'completed'
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
            <div className={`rounded-lg p-3 border ${
              job.errors && job.errors.length > 0
                ? 'bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800'
                : 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800'
            }`}>
              <p className={`text-sm font-medium ${
                job.errors && job.errors.length > 0
                  ? 'text-yellow-800 dark:text-yellow-300'
                  : 'text-green-800 dark:text-green-300'
              }`}>
                {t('importsPage.summary.completed', {
                  completed: job.completed_modules,
                  total: job.total_modules,
                })}
                {job.errors && job.errors.length > 0 && t('importsPage.summary.errorCount', { count: job.errors.length })}
              </p>
              <p className={`text-xs mt-1 ${
                job.errors && job.errors.length > 0
                  ? 'text-yellow-700 dark:text-yellow-400'
                  : 'text-green-700 dark:text-green-400'
              }`}>
                {t('importsPage.summary.activitiesVisible')}
              </p>
              {job.errors && job.errors.length > 0 && (
                <details className="mt-2">
                  <summary className="text-xs text-yellow-700 dark:text-yellow-400 cursor-pointer hover:underline">
                    {t('importsPage.summary.showErrors')}
                  </summary>
                  <ul className="mt-1 space-y-0.5 max-h-32 overflow-y-auto">
                    {job.errors.map((err, i) => (
                      <li key={i} className="text-xs text-yellow-700 dark:text-yellow-400 font-mono break-all">
                        {err}
                      </li>
                    ))}
                  </ul>
                </details>
              )}
            </div>
          )}

          {/* Fusion candidates hint */}
          {job?.status === 'completed' && fusionCount !== null && fusionCount > 0 && (
            <div className="rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 p-3 mt-3">
              <p className="text-sm font-medium text-blue-800 dark:text-blue-300">
                {t('importsPage.fusion.found', {
                  count: fusionCount,
                  displayCount: fusionCount >= 5 ? t('importsPage.fusion.several') : fusionCount,
                })}
              </p>
              <p className="text-xs text-blue-700 dark:text-blue-400 mt-1">
                {t('importsPage.fusion.description')}
              </p>
              <a
                href="/workshop/fusion"
                className="inline-block mt-2 text-xs font-medium text-blue-600 dark:text-blue-400 hover:underline"
              >
                {t('importsPage.fusion.review')}
              </a>
            </div>
          )}

          {/* Failure summary */}
          {job?.status === 'failed' && (
            <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-3 space-y-1">
              <p className="text-sm font-medium text-red-800 dark:text-red-300">
                {t('importsPage.summary.failed')}
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
  startFn: (path: string, modules: string[], options: ImportOptions) => Promise<string>
  pollFn: (jobId: string) => Promise<ImportJob>
  scanZipFn: (file: File, onUploaded?: () => void) => Promise<{ source_path: string; scan_result: ScanResult }>
  pathImportEnabled: boolean
  capsLoading: boolean
}

function ImportPanel({
  platform,
  defaultPath,
  dirLabel,
  dirHint,
  dirPlaceholder,
  startFn,
  pollFn,
  scanZipFn,
  pathImportEnabled,
  capsLoading,
}: ImportPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const STORAGE_KEY = `skadia-import-job-${platform}`

  const [sourcePath, setSourcePath] = useState(defaultPath)
  const [uploadStatus, setUploadStatus] = useState<'idle' | 'uploading' | 'scanning'>('idle')
  const [scanning, setScanning] = useState(false)
  const [scanError, setScanError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)

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

  const [fusionCount, setFusionCount] = useState<number | null>(null)

  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const logEndRef = useRef<HTMLDivElement | null>(null)
  const lastModuleRef = useRef<string | null>(null)

  // Reusable polling logic for reconnecting after navigation
  function startPolling(id: string) {
    if (pollingRef.current) clearInterval(pollingRef.current)
    pollingRef.current = setInterval(async () => {
      try {
        const j = await pollFn(id)
        setJob(j)

        if (j.current_module && j.current_module !== lastModuleRef.current) {
          lastModuleRef.current = j.current_module
          appendLog(importCurrentModuleLabel(j.current_module, t))
        }

        if (j.status === 'completed' && lastModuleRef.current !== '__completed__') {
          lastModuleRef.current = '__completed__'
          appendLog(t('importsPage.log.completedModules', { count: j.completed_modules }), 'completed')
        }
        if (j.status === 'failed' && lastModuleRef.current !== '__failed__') {
          lastModuleRef.current = '__failed__'
          appendLog(t('importsPage.log.importFailed'), 'error')
          if (j.errors && j.errors.length > 0) {
            j.errors.forEach(err => appendLog(t('importsPage.log.errorWithMessage', { message: err }), 'error'))
          }
        }
      } catch {
        // Ignore transient polling errors.
      }
    }, 2000)
  }

  // Reconnect: restore a running job from localStorage
  // Check job status first; reconnect only when it is still running.
  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (!saved) return
    ;(async () => {
      try {
        const { jobId: savedId } = JSON.parse(saved) as { jobId: string }
        if (!savedId) { localStorage.removeItem(STORAGE_KEY); return }

        const j = await pollFn(savedId)
        if (j.status === 'completed' || j.status === 'failed') {
          // Job already finished; clean up localStorage and keep the UI in scan phase.
          localStorage.removeItem(STORAGE_KEY)
          return
        }
        // Job is still running; reconnect.
        setJobId(savedId)
        setPhase('import')
        setImporting(true)
        lastModuleRef.current = null
        appendLog(t('importsPage.log.reconnecting'))
        startPolling(savedId)
      } catch {
        // Job is no longer in the DB or the network failed; clear localStorage without reconnecting.
        localStorage.removeItem(STORAGE_KEY)
      }
    })()
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Auto-scroll log to bottom
  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [log])

  // Stop polling when job is done + check fusion candidates on completion
  useEffect(() => {
    if (job && (job.status === 'completed' || job.status === 'failed')) {
      if (pollingRef.current) {
        clearInterval(pollingRef.current)
        pollingRef.current = null
      }
      setImporting(false)
      localStorage.removeItem(STORAGE_KEY)

      if (job.status === 'completed') {
        getFusionQueue(5)
          .then(resp => setFusionCount(resp.items.length))
          .catch(() => setFusionCount(null))
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [job])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [])

  function appendLog(message: string, kind: LogEntry['kind'] = 'info') {
    setLog(prev => {
      const next = [...prev, { timestamp: new Date(), message, kind }]
      if (next.length > MAX_LOG_ENTRIES) return next.slice(next.length - MAX_LOG_ENTRIES)
      return next
    })
  }

  async function handleAnalyse() {
    const path = sourcePath.trim()
    if (!path) return
    setScanning(true)
    setScanError(null)
    setScanResult(null)
    try {
      const fn = platform === 'garmin' ? scanGarminExport : scanStravaExport
      const result = await fn(path)
      setScanResult(result)
      // Phase stays "scan"; the user reviews results and continues explicitly.
    } catch (e) {
      setScanError(e instanceof Error ? e.message : String(e))
    } finally {
      setScanning(false)
    }
  }

  async function handleZIPUpload(file: File) {
    setUploadStatus('uploading')
    setScanError(null)
    setScanResult(null)
    setLog([])

    try {
      const { source_path, scan_result } = await scanZipFn(file, () => setUploadStatus('scanning'))
      setSourcePath(source_path)
      setScanResult(scan_result)
      const cats = Object.entries(scan_result)
        .filter(([, stats]) => stats !== undefined)
        .map(([cat]) => cat)
      setImportOptions(prev => ({ ...prev, categories: cats }))
      setPhase('config')
    } catch (e) {
      setScanError(e instanceof Error ? e.message : String(e))
    } finally {
      setUploadStatus('idle')
    }
  }

  async function handleStartImport() {
    const categoriesToImport = importOptions.categories ?? []

    setPhase('import')
    setImporting(true)
    setImportError(null)
    setJob(null)
    setJobId(null)
    setLog([])
    setFusionCount(null)
    lastModuleRef.current = null

    try {
      // Derive module names from scanResult — avoids a redundant detectFn call
      // (which could fail if a temp dir was cleaned up between scan and start).
      const modulesToRun = scanResult
        ? Object.entries(scanResult)
            .filter(([cat, stats]) =>
              stats !== undefined &&
              (categoriesToImport.length === 0 || categoriesToImport.includes(cat))
            )
            .map(([cat]) => cat)
        : categoriesToImport

      const id = await startFn(sourcePath.trim(), modulesToRun, importOptions)
      setJobId(id)
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ jobId: id }))
      appendLog(t('importsPage.log.jobStarted', { id: id.slice(0, 8) }))

      // Refresh the active-job indicator immediately.
      queryClient.invalidateQueries({ queryKey: ['active-jobs'] })

      startPolling(id)
    } catch (e) {
      setImportError(e instanceof Error ? e.message : String(e))
      setImporting(false)
      // Stay on import phase so the error is visible — do NOT go back to config silently
    }
  }

  const progress =
    job && job.total_modules > 0
      ? Math.round((job.completed_modules / job.total_modules) * 100)
      : 0

  return (
    <div className="space-y-6">
      {/* Hidden file input for ZIP upload */}
      <input
        ref={fileInputRef}
        type="file"
        accept=".zip"
        className="hidden"
        onChange={e => {
          const file = e.target.files?.[0]
          if (file) handleZIPUpload(file)
          // Reset so same file can be re-selected
          e.target.value = ''
        }}
      />

      {/* Source section */}
      {capsLoading ? (
        <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <div className="animate-pulse space-y-3">
            <div className="h-5 bg-neutral-200 dark:bg-neutral-700 rounded w-48" />
            <div className="h-4 bg-neutral-200 dark:bg-neutral-700 rounded w-3/4" />
            <div className="h-10 bg-neutral-200 dark:bg-neutral-700 rounded" />
          </div>
        </section>
      ) : (
        <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
          <div>
            <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
              {pathImportEnabled ? dirLabel : t('importsPage.uploadExport')}
            </h2>
            <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
              {pathImportEnabled
                ? dirHint
                : t('importsPage.uploadExportHint')}
            </p>
          </div>

          {/* Path input — only shown when server has IMPORT_ALLOWED_DIRS configured */}
          {pathImportEnabled && (
            <input
              type="text"
              value={sourcePath}
              onChange={e => {
                setSourcePath(e.target.value)
                if (phase !== 'scan') {
                  setPhase('scan')
                  setScanResult(null)
                  setScanError(null)
                }
              }}
              placeholder={dirPlaceholder}
              disabled={(phase === 'import' && importing) || uploadStatus !== 'idle'}
              className="w-full rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed"
            />
          )}

          {/* Action buttons — only in scan phase before results are shown */}
          {phase === 'scan' && uploadStatus === 'idle' && !scanResult && (
            <div className="flex flex-col items-stretch gap-3">
              {pathImportEnabled && (
                <>
                  <button
                    onClick={handleAnalyse}
                    disabled={scanning || !sourcePath.trim()}
                    className="flex items-center justify-center gap-2 px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                  >
                    {scanning ? (
                      <>
                        <svg className="w-4 h-4 animate-spin shrink-0" fill="none" viewBox="0 0 24 24">
                          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
                        </svg>
                        {t('importsPage.analyzing')}
                      </>
                    ) : (
                      t('importsPage.analyze')
                    )}
                  </button>
                  {scanError && <p className="text-sm text-red-600 dark:text-red-400">{scanError}</p>}
                  <div className="flex items-center gap-3">
                    <div className="flex-1 h-px bg-neutral-200 dark:bg-neutral-700" />
                    <span className="text-xs text-neutral-400">{t('importsPage.orUploadZip')}</span>
                    <div className="flex-1 h-px bg-neutral-200 dark:bg-neutral-700" />
                  </div>
                </>
              )}
              {scanError && !pathImportEnabled && (
                <p className="text-sm text-red-600 dark:text-red-400">{scanError}</p>
              )}
              <button
                onClick={() => fileInputRef.current?.click()}
                className="cursor-pointer flex items-center justify-center gap-2 px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
              >
                <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a2 2 0 002 2h12a2 2 0 002-2v-1M12 12V4m0 0l-3 3m3-3l3 3" />
                </svg>
                {t('importsPage.uploadZip')}
              </button>
            </div>
          )}

          {/* Upload / scan in progress indicator */}
          {uploadStatus !== 'idle' && (
            <div className="flex items-center gap-2 text-sm text-neutral-500 dark:text-neutral-400">
              <svg className="w-4 h-4 animate-spin shrink-0" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              {uploadStatus === 'uploading' ? t('importsPage.uploadingZip') : t('importsPage.scanning')}
            </div>
          )}
        </section>
      )}

      {/* Wizard panel */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <StepIndicator phase={phase} />

        {phase === 'scan' && !scanning && scanResult && (
          <ScanResultsView
            result={scanResult}
            onContinue={result => {
              const cats = Object.entries(result)
                .filter(([, stats]) => stats !== undefined)
                .map(([cat]) => cat)
              setImportOptions(prev => ({ ...prev, categories: cats }))
              setPhase('config')
            }}
          />
        )}

        {phase === 'scan' && scanning && (
          <p className="text-sm text-neutral-500 dark:text-neutral-400">{t('importsPage.analyzing')}</p>
        )}

        {phase === 'config' && scanResult && (
          <ConfigPhase
            platform={platform}
            scanResult={scanResult}
            options={importOptions}
            onChange={setImportOptions}
            onStart={handleStartImport}
          />
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
            fusionCount={fusionCount}
          />
        )}

        {/* Back navigation */}
        {phase === 'config' && (
          <button
            onClick={() => { setPhase('scan'); setScanResult(null); setScanError(null); setScanning(false) }}
            className="cursor-pointer mt-4 text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
          >
            {t('importsPage.backToAnalysis')}
          </button>
        )}
        {/* New import button — only after import completes */}
        {phase === 'import' &&
          job &&
          (job.status === 'completed' || job.status === 'failed') && (
            <button
              onClick={() => {
                localStorage.removeItem(STORAGE_KEY)
                setPhase('scan')
                setScanResult(null)
                setScanError(null)
                setJob(null)
                setJobId(null)
                setLog([])
                setImportOptions({ conflict_strategy: 'enrich_missing' })
              }}
              className="cursor-pointer mt-4 text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
            >
              {t('importsPage.startNewImport')}
            </button>
          )}
      </section>
    </div>
  )
}

// ---------------------------------------------------------------------------
// KomootImportPanel — Login → Scan → Config → Import
// ---------------------------------------------------------------------------

type KomootPhase = 'login' | 'scan' | 'config' | 'import'

function KomootImportPanel() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const STORAGE_KEY = 'skadia-import-job-komoot'

  const [phase, setPhase] = useState<KomootPhase>('login')

  // Login state
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loginLoading, setLoginLoading] = useState(false)
  const [loginError, setLoginError] = useState<string | null>(null)

  // Scan state
  const [scanResult, setScanResult] = useState<KomootScanResult | null>(null)
  const [scanLoading, setScanLoading] = useState(false)
  const [scanError, setScanError] = useState<string | null>(null)

  // Credential persistence
  const [saveCredentials, setSaveCredentials] = useState(false)
  const [hasStoredCredentials, setHasStoredCredentials] = useState<boolean | null>(null)
  const [storedEmail, setStoredEmail] = useState('')
  const [deletingCredentials, setDeletingCredentials] = useState(false)

  // Track whether credentials were saved during this session (for cleanup on unmount)
  const loggedInThisSessionRef = useRef(false)
  const saveCredentialsRef = useRef(false)
  const importStartedRef = useRef(false)

  // Config state
  const [importRecorded, setImportRecorded] = useState(true)
  const [importPlanned, setImportPlanned] = useState(true)
  const [importPhotos, setImportPhotos] = useState(true)
  const [conflictStrategy, setConflictStrategy] = useState<ConflictStrategy>('enrich_missing')
  const selectionRef = useRef<KomootImportSelection>({ importRecorded: true, importPlanned: true, importPhotos: true })

  // Import state
  const [jobId, setJobId] = useState<string | null>(null)
  const [job, setJob] = useState<KomootJob | null>(null)
  const [importing, setImporting] = useState(false)
  const [importError, setImportError] = useState<string | null>(null)
  const [log, setLog] = useState<LogEntry[]>([])

  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const logEndRef = useRef<HTMLDivElement | null>(null)
  const lastPhaseRef = useRef<string | null>(null)
  const lastProgressRef = useRef<number>(-1)

  function appendLog(message: string, kind: LogEntry['kind'] = 'info') {
    setLog(prev => {
      const next = [...prev, { timestamp: new Date(), message, kind }]
      if (next.length > MAX_LOG_ENTRIES) return next.slice(next.length - MAX_LOG_ENTRIES)
      return next
    })
  }

  function startPolling(id: string) {
    if (pollingRef.current) clearInterval(pollingRef.current)
    pollingRef.current = setInterval(async () => {
      try {
        const j = await getKomootImportJob(id)
        setJob(j)

        if (j.phase && j.phase !== lastPhaseRef.current) {
          lastPhaseRef.current = j.phase
          appendLog(t('importsPage.komoot.log.phase', { phase: komootPhaseLabel(j.phase, t) }))
        }

        const cur = j.progress_current ?? 0
        const total = j.progress_total ?? 0
        if (j.phase === 'import' && total > 0 && cur !== lastProgressRef.current) {
          lastProgressRef.current = cur
          appendLog(komootProgressLabel(cur, total, selectionRef.current, t))
        }

        if (j.status === 'completed' && lastPhaseRef.current !== '__completed__') {
          lastPhaseRef.current = '__completed__'
          appendLog(komootCompletionLog(lastProgressRef.current, selectionRef.current, t), 'completed')
        }
        if (j.status === 'failed' && lastPhaseRef.current !== '__failed__') {
          lastPhaseRef.current = '__failed__'
          appendLog(t('importsPage.log.importFailed'), 'error')
        }
      } catch {
        // Ignore transient polling errors.
      }
    }, 2000)
  }

  // Keep refs in sync for cleanup effect
  useEffect(() => { saveCredentialsRef.current = saveCredentials }, [saveCredentials])
  useEffect(() => {
    selectionRef.current = { importRecorded, importPlanned, importPhotos }
  }, [importRecorded, importPlanned, importPhotos])

  // Check if stored credentials exist
  useEffect(() => {
    komootCredentialsStatus().then(res => {
      setHasStoredCredentials(res.has_credentials)
      if (res.email) setStoredEmail(res.email)
    }).catch(() => setHasStoredCredentials(false))
  }, [])

  // Cleanup: delete temp credentials on unmount if user logged in but didn't persist
  useEffect(() => {
    return () => {
      if (loggedInThisSessionRef.current && !saveCredentialsRef.current && !importStartedRef.current) {
        komootDeleteCredentials().catch(() => {})
      }
    }
  }, [])

  // Reconnect: stored running job
  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (!saved) return
    ;(async () => {
      try {
        const { jobId: savedId, selection } = JSON.parse(saved) as { jobId: string; selection?: KomootImportSelection }
        if (!savedId) { localStorage.removeItem(STORAGE_KEY); return }
        if (selection) {
          selectionRef.current = selection
          setImportRecorded(selection.importRecorded)
          setImportPlanned(selection.importPlanned)
          setImportPhotos(selection.importPhotos)
        }
        const j = await getKomootImportJob(savedId)
        if (j.status === 'completed' || j.status === 'failed') {
          localStorage.removeItem(STORAGE_KEY)
          return
        }
        setJobId(savedId)
        setPhase('import')
        setImporting(true)
        lastPhaseRef.current = null
        lastProgressRef.current = -1
        appendLog(t('importsPage.log.reconnecting'))
        startPolling(savedId)
      } catch {
        localStorage.removeItem(STORAGE_KEY)
      }
    })()
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [log])

  useEffect(() => {
    if (job && (job.status === 'completed' || job.status === 'failed')) {
      if (pollingRef.current) {
        clearInterval(pollingRef.current)
        pollingRef.current = null
      }
      setImporting(false)
      localStorage.removeItem(STORAGE_KEY)
    }
  }, [job])

  useEffect(() => {
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [])

  async function handleLogin() {
    if (!email.trim() || !password) return
    setLoginLoading(true)
    setLoginError(null)
    try {
      const res = await komootLogin(email.trim(), password, saveCredentials)
      if (res.status !== 'ok') {
        setLoginError(res.error ?? t('importsPage.komoot.invalidCredentials'))
        return
      }
      loggedInThisSessionRef.current = true
      if (res.scan) {
        setScanResult(res.scan)
        setPhase('config')
      } else {
        setPhase('scan')
        await handleScan()
      }
    } catch (e) {
      setLoginError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoginLoading(false)
    }
  }

  async function handleScan() {
    setScanLoading(true)
    setScanError(null)
    try {
      const result = await komootScan()
      if (!result.has_credentials) {
        setPhase('login')
        setScanError(t('importsPage.komoot.missingStoredCredentials'))
        return
      }
      setSaveCredentials(true) // Credentials exist in DB, so treat this as "save" mode.
      setScanResult(result)
      setPhase('config')
    } catch (e) {
      setScanError(e instanceof Error ? e.message : String(e))
    } finally {
      setScanLoading(false)
    }
  }

  async function handleDeleteCredentials() {
    setDeletingCredentials(true)
    try {
      await komootDeleteCredentials()
      setHasStoredCredentials(false)
      setStoredEmail('')
    } catch {
      // ignore — user can retry
    } finally {
      setDeletingCredentials(false)
    }
  }

  async function handleStartImport() {
    setPhase('import')
    setImporting(true)
    setImportError(null)
    setJob(null)
    setJobId(null)
    setLog([])
    lastPhaseRef.current = null
    importStartedRef.current = true

    try {
      const id = await komootStartImport({
        import_recorded: importRecorded,
        import_planned: importPlanned,
        import_photos: importPhotos,
        options: { conflict_strategy: conflictStrategy },
        delete_credentials_after: !saveCredentials,
      })
      setJobId(id)
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ jobId: id, selection: selectionRef.current }))
      appendLog(t('importsPage.log.jobStarted', { id: id.slice(0, 8) }))
      queryClient.invalidateQueries({ queryKey: ['active-jobs'] })
      startPolling(id)
    } catch (e) {
      setImportError(e instanceof Error ? e.message : String(e))
      setImporting(false)
    }
  }

  const progress =
    job && job.progress_total && job.progress_total > 0 && job.progress_current != null
      ? Math.round((job.progress_current / job.progress_total) * 100)
      : 0
  const currentKomootSelection = { importRecorded, importPlanned, importPhotos }
  const komootUnit = komootSelectionUnit(currentKomootSelection, t)
  return (
    <div className="space-y-6">
      {/* Step indicator */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        {/* Progress steps */}
        <div className="flex items-center gap-0 mb-6">
          {(['login', 'config', 'import'] as const).map((step, idx) => {
            const labels = {
              login: t('importsPage.komoot.phases.login'),
              config: t('importsPage.komoot.phases.config'),
              import: t('importsPage.steps.import'),
            }
            const currentIdx = ['login', 'config', 'import'].indexOf(phase === 'scan' ? 'login' : phase)
            const isActive = step === (phase === 'scan' ? 'login' : phase)
            const isDone = idx < currentIdx
            return (
              <div key={step} className="flex items-center flex-1 last:flex-none">
                <div className="flex items-center gap-2">
                  <div className={`w-7 h-7 rounded-full flex items-center justify-center text-xs font-semibold shrink-0 transition-colors ${
                    isDone ? 'bg-green-500 text-white' : isActive ? 'bg-accent-DEFAULT text-white' : 'bg-neutral-200 dark:bg-neutral-700 text-neutral-500 dark:text-neutral-400'
                  }`}>
                    {isDone ? (
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                      </svg>
                    ) : idx + 1}
                  </div>
                  <span className={`text-sm font-medium whitespace-nowrap ${isActive ? 'text-neutral-900 dark:text-neutral-100' : isDone ? 'text-neutral-600 dark:text-neutral-400' : 'text-neutral-400 dark:text-neutral-500'}`}>
                    {labels[step]}
                  </span>
                </div>
                {idx < 2 && (
                  <div className={`flex-1 h-px mx-3 transition-colors ${isDone ? 'bg-green-400' : 'bg-neutral-200 dark:bg-neutral-700'}`} />
                )}
              </div>
            )
          })}
        </div>

        {/* Phase: Login */}
        {(phase === 'login' || phase === 'scan') && (
          <div className="space-y-4">
            <p className="text-sm text-neutral-500 dark:text-neutral-400">
              {t('importsPage.komoot.loginDescription')}
            </p>
            <div className="space-y-3 max-w-sm">
              <div>
                <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
                  {t('importsPage.komoot.email')}
                </label>
                <input
                  type="email"
                  value={email}
                  onChange={e => setEmail(e.target.value)}
                  placeholder={t('importsPage.komoot.emailPlaceholder')}
                  disabled={loginLoading || scanLoading}
                  className="w-full rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
                  {t('importsPage.komoot.password')}
                </label>
                <input
                  type="password"
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter') handleLogin() }}
                  disabled={loginLoading || scanLoading}
                  className="w-full rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed"
                />
              </div>
              <label className="flex items-start gap-2.5 cursor-pointer">
                <input
                  type="checkbox"
                  checked={saveCredentials}
                  onChange={e => setSaveCredentials(e.target.checked)}
                  disabled={loginLoading || scanLoading}
                  className="mt-0.5 w-4 h-4 accent-accent-DEFAULT"
                />
                <div>
                  <span className="text-sm text-neutral-700 dark:text-neutral-300">{t('importsPage.komoot.saveCredentials')}</span>
                  <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
                    {saveCredentials
                      ? t('importsPage.komoot.saveCredentialsOn')
                      : t('importsPage.komoot.saveCredentialsOff')}
                  </p>
                </div>
              </label>
            </div>
            {loginError && <p className="text-sm text-red-600 dark:text-red-400">{loginError}</p>}
            {scanError && <p className="text-sm text-red-600 dark:text-red-400">{scanError}</p>}
            <div className="flex flex-wrap items-center gap-3">
              <button
                onClick={handleLogin}
                disabled={loginLoading || !email.trim() || !password}
                className="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
              >
                {loginLoading ? (
                  <>
                    <svg className="w-4 h-4 animate-spin shrink-0" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
                    </svg>
                    {t('importsPage.komoot.connecting')}
                  </>
                ) : t('importsPage.komoot.loginAndScan')}
              </button>
              {hasStoredCredentials && (
                <button
                  onClick={handleScan}
                  disabled={scanLoading || loginLoading}
                  className="px-4 py-2 rounded-lg text-sm font-medium border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                >
                  {scanLoading ? t('importsPage.komoot.scanning') : t('importsPage.komoot.useStoredCredentials')}
                </button>
              )}
            </div>
            {hasStoredCredentials && (
              <div className="flex items-center gap-2 mt-1">
                <p className="text-xs text-neutral-400 dark:text-neutral-500">
                  {t('importsPage.komoot.storedCredentials')} <span className="font-medium text-neutral-500 dark:text-neutral-400">{storedEmail}</span>
                </p>
                <button
                  onClick={handleDeleteCredentials}
                  disabled={deletingCredentials}
                  className="text-xs text-red-500 hover:text-red-600 dark:text-red-400 dark:hover:text-red-300 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                >
                  {deletingCredentials ? t('importsPage.komoot.deleting') : t('importsPage.komoot.delete')}
                </button>
              </div>
            )}
          </div>
        )}

        {/* Phase: Config */}
        {phase === 'config' && scanResult && (
          <div className="space-y-5">
            {/* Scan summary */}
            <div className="grid grid-cols-3 gap-3">
              {[
                { label: t('importsPage.komoot.summary.recordedTours'), value: scanResult.recorded_count, new_: null },
                { label: t('importsPage.komoot.summary.plannedRoutes'), value: scanResult.planned_count, new_: null },
                { label: t('importsPage.komoot.summary.newItems'), value: scanResult.new_count, new_: null },
              ].map(item => (
                <div key={item.label} className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-3">
                  <div className="text-lg font-semibold tabular-nums">{item.value}</div>
                  <div className="text-xs text-neutral-500 mt-0.5">{item.label}</div>
                </div>
              ))}
            </div>

            {/* Import options */}
            <div className="space-y-2">
              <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('importsPage.komoot.whatToImport')}</h3>
              <label className="flex items-center gap-3 cursor-pointer">
                <input type="checkbox" checked={importRecorded} onChange={e => setImportRecorded(e.target.checked)} className="w-4 h-4 accent-accent-DEFAULT" />
                <span className="text-sm">{t('importsPage.komoot.recordedActivities')}</span>
                <span className="text-xs text-neutral-400">{t('importsPage.komoot.tourCount', { count: scanResult.recorded_count })}</span>
              </label>
              <label className="flex items-center gap-3 cursor-pointer">
                <input type="checkbox" checked={importPlanned} onChange={e => setImportPlanned(e.target.checked)} className="w-4 h-4 accent-accent-DEFAULT" />
                <span className="text-sm">{t('importsPage.komoot.plannedRoutes')}</span>
                <span className="text-xs text-neutral-400">{t('importsPage.komoot.tourCount', { count: scanResult.planned_count })}</span>
              </label>
              <label className="flex items-center gap-3 cursor-pointer">
                <input type="checkbox" checked={importPhotos} onChange={e => setImportPhotos(e.target.checked)} className="w-4 h-4 accent-accent-DEFAULT" disabled={!importRecorded} />
                <span className={`text-sm ${!importRecorded ? 'text-neutral-400' : ''}`}>{t('importsPage.komoot.importPhotos')}</span>
              </label>
              {importPhotos && importRecorded && (
                <p className="text-xs text-neutral-400 pl-7">{t('importsPage.komoot.photoCountHint')}</p>
              )}
            </div>

            {/* Conflict strategy */}
            <div className="space-y-2">
              <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('importsPage.conflict.title')}</h3>
              {importConflictStrategyOptions(t).map(opt => (
                <label key={opt.value} className="flex gap-3 cursor-pointer">
                  <input type="radio" name="komoot-strategy" value={opt.value} checked={conflictStrategy === opt.value} onChange={() => setConflictStrategy(opt.value)} className="mt-0.5 accent-accent-DEFAULT" />
                  <div>
                    <div className="text-sm font-medium">{opt.label}</div>
                    <div className="text-xs text-neutral-500">
                      {opt.value === 'enrich_missing' ? t('importsPage.komoot.conflictKomootDesc') : opt.desc}
                    </div>
                  </div>
                </label>
              ))}
            </div>

            <button
              onClick={handleStartImport}
              disabled={!importRecorded && !importPlanned}
              className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
            >
              {t('importsPage.startImport')}
            </button>

            <button
              onClick={() => { setPhase('login'); setScanResult(null); setLoginError(null); setScanError(null) }}
              className="cursor-pointer block mt-2 text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
            >
              {t('importsPage.komoot.loginAgain')}
            </button>
          </div>
        )}

        {/* Phase: Import */}
        {phase === 'import' && (
          <div className="space-y-4">
            {importing && !jobId && (
              <p className="text-sm text-neutral-500 dark:text-neutral-400">{t('importsPage.startingJob')}</p>
            )}
            {importError && (
              <p className="text-sm text-red-600 dark:text-red-400">{importError}</p>
            )}
            {jobId && (
              <div className="space-y-4">
                <div className="flex items-center justify-between text-sm">
                  <span className={`font-medium ${
                    job?.status === 'completed' ? 'text-green-600 dark:text-green-400'
                    : job?.status === 'failed' ? 'text-red-600 dark:text-red-400'
                    : 'text-neutral-700 dark:text-neutral-300'
                  }`}>
                    {importJobStatusLabel(job?.status, t)}
                  </span>
                  <span className="text-xs text-neutral-500 tabular-nums font-mono">
                    {t('importsPage.jobShort', { id: jobId.slice(0, 8) })}
                    {job?.progress_total && job.progress_total > 0 && t('importsPage.komoot.progress', {
                      current: job.progress_current ?? 0,
                      total: job.progress_total,
                      unit: komootUnit.plural,
                      progress,
                    })}
                  </span>
                </div>

                <div className="w-full h-2 bg-neutral-200 dark:bg-neutral-700 rounded-full overflow-hidden">
                  <div
                    className={`h-full rounded-full transition-all duration-500 ${
                      job?.status === 'completed' ? 'bg-green-500' : job?.status === 'failed' ? 'bg-red-500' : 'bg-accent-DEFAULT'
                    }`}
                    style={{
                      width: job?.status === 'completed' ? '100%'
                        : job?.status === 'running' && (!job.progress_total || job.progress_total === 0) ? '10%'
                        : `${progress}%`,
                    }}
                  />
                </div>

                {job?.status === 'running' && job.phase && (
                  <p className="text-xs text-neutral-500 dark:text-neutral-400 font-mono truncate">
                    {t('importsPage.komoot.log.phase', { phase: komootPhaseLabel(job.phase, t) })}
                  </p>
                )}

                {log.length > 0 && (
                  <div className="rounded-lg bg-neutral-900 border border-neutral-800 p-3 h-40 overflow-y-auto font-mono text-xs leading-relaxed" aria-label={t('importsPage.log.label')}>
                    {log.map((entry, i) => (
                      <div key={i} className="flex gap-3">
                        <span className="shrink-0 text-neutral-500 select-none">{formatTimestamp(entry.timestamp, i18n.resolvedLanguage ?? i18n.language ?? 'de')}</span>
                        <span className={
                          entry.kind === 'error'
                            ? 'text-red-400'
                            : entry.kind === 'completed'
                            ? 'text-green-400'
                            : 'text-neutral-200'
                        }>
                          {entry.message}
                        </span>
                      </div>
                    ))}
                    <div ref={logEndRef} />
                  </div>
                )}

                {job?.status === 'completed' && (
                  <div className="rounded-lg bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 p-3">
                    <p className="text-sm font-medium text-green-800 dark:text-green-300">
                      {t('importsPage.komoot.importCompleted')}
                    </p>
                    <p className="text-xs text-green-700 dark:text-green-400 mt-1">
                      {komootAvailabilityText(currentKomootSelection, t)}
                    </p>
                    {!saveCredentials && (
                      <p className="text-xs text-green-700 dark:text-green-400 mt-1">
                        {t('importsPage.komoot.credentialsDeleted')}
                      </p>
                    )}
                  </div>
                )}

                {job?.status === 'failed' && (
                  <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-3">
                    <p className="text-sm font-medium text-red-800 dark:text-red-300">{t('importsPage.summary.failed')}</p>
                  </div>
                )}
              </div>
            )}

            {job && (job.status === 'completed' || job.status === 'failed') && (
              <button
                onClick={() => {
                  localStorage.removeItem(STORAGE_KEY)
                  setPhase('login')
                  setScanResult(null)
                  setJob(null)
                  setJobId(null)
                  setLog([])
                }}
                className="cursor-pointer text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
              >
                {t('importsPage.startNewImport')}
              </button>
            )}
          </div>
        )}
      </section>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main page with tabs
// ---------------------------------------------------------------------------

export type ImportSource = 'garmin' | 'strava' | 'komoot'
type TabId = ImportSource | 'queue'

export function ImportSourcePanel({ source }: { source: ImportSource }) {
  const { t } = useTranslation()
  const { data: capabilities, isPending: capsLoading } = useQuery({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 30_000,
  })
  const pathImportEnabled = capabilities?.import_path_enabled ?? false

  if (source === 'garmin') {
    return (
      <ImportPanel
        platform="garmin"
        defaultPath={DEFAULT_GARMIN_PATH}
        dirLabel={t('importsPage.sources.garmin.dirLabel')}
        dirHint={t('importsPage.sources.garmin.dirHint')}
        dirPlaceholder={t('importsPage.sources.garmin.dirPlaceholder')}
        startFn={startGarminImportJob}
        pollFn={getGarminImportJob}
        scanZipFn={uploadGarminZIPScan}
        pathImportEnabled={pathImportEnabled}
        capsLoading={capsLoading}
      />
    )
  }

  if (source === 'strava') {
    return (
      <ImportPanel
        platform="strava"
        defaultPath={DEFAULT_STRAVA_PATH}
        dirLabel={t('importsPage.sources.strava.dirLabel')}
        dirHint={t('importsPage.sources.strava.dirHint')}
        dirPlaceholder={t('importsPage.sources.strava.dirPlaceholder')}
        startFn={startStravaImportJob}
        pollFn={getStravaImportJob}
        scanZipFn={uploadStravaZIPScan}
        pathImportEnabled={pathImportEnabled}
        capsLoading={capsLoading}
      />
    )
  }

  return <KomootImportPanel />
}

export default function ImportPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const [activeTab, setActiveTab] = useState<TabId>(
    (['garmin', 'strava', 'komoot', 'queue'].includes(tabParam ?? '') ? tabParam : 'garmin') as TabId
  )

  // Sync tab when URL query param changes (e.g. from ImportHub navigation)
  useEffect(() => {
    if (tabParam && ['garmin', 'strava', 'komoot', 'queue'].includes(tabParam)) {
      setActiveTab(tabParam as TabId)
    }
  }, [tabParam])

  // Queue count for badge
  const { data: queueCount = 0 } = useQuery({
    queryKey: ['import-queue-count'],
    queryFn: fetchImportQueueCount,
    staleTime: 30_000,
    retry: false,
  })

  return (
    <div className="max-w-3xl mx-auto space-y-8 py-6 px-4">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          {t('importsPage.title')}
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          {t('importsPage.description')}
        </p>
      </div>

      {/* Tab bar */}
      <div className="flex gap-1 border-b border-neutral-200 dark:border-neutral-800">
        {(['garmin', 'strava', 'komoot', 'queue'] as TabId[]).map(tabId => (
          <button
            key={tabId}
            onClick={() => setActiveTab(tabId)}
            className={`cursor-pointer relative px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px ${
              activeTab === tabId
                ? 'border-accent-DEFAULT text-accent-DEFAULT'
                : 'border-transparent text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100'
            }`}
          >
            {tabId === 'garmin' && 'Garmin'}
            {tabId === 'strava' && 'Strava'}
            {tabId === 'komoot' && 'Komoot'}
            {tabId === 'queue' && (
              <span className="flex items-center gap-1.5">
                {t('importsPage.tabs.queue')}
                {queueCount > 0 && (
                  <span className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-orange-500 text-white text-[10px] font-bold leading-none">
                    {queueCount > 9 ? '9+' : queueCount}
                  </span>
                )}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Tab content — panels are always mounted to preserve state during tab switches */}
      <div className={activeTab !== 'garmin' ? 'hidden' : ''}>
        <ImportSourcePanel source="garmin" />
      </div>

      <div className={activeTab !== 'strava' ? 'hidden' : ''}>
        <ImportSourcePanel source="strava" />
      </div>

      <div className={activeTab !== 'komoot' ? 'hidden' : ''}>
        <ImportSourcePanel source="komoot" />
      </div>

      {activeTab === 'queue' && (
        <Suspense fallback={
          <div className="flex items-center justify-center py-12 text-neutral-400 text-sm">{t('importsPage.loading')}</div>
        }>
          <ImportQueueTab />
        </Suspense>
      )}
    </div>
  )
}
