import { useRef, useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiUpload } from '@/api/client'
import { getShadowFSSettings } from '@/api/settings'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface UploadResult {
  status: 'created' | 'replaced' | 'enriched' | 'skipped'
  activity_id?: string
  fields_enriched?: number
  warnings?: string[]
}

type UploadState =
  | { type: 'idle' }
  | { type: 'uploading'; filename: string }
  | { type: 'success'; count: number }
  | { type: 'error'; message: string }

// ---------------------------------------------------------------------------
// Inline SVG icons
// ---------------------------------------------------------------------------

function UploadIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 16v1a2 2 0 002 2h12a2 2 0 002-2v-1M12 12V4m0 0l-3.5 3.5M12 4l3.5 3.5" />
    </svg>
  )
}

function FileIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
      <polyline points="14 2 14 8 20 8" />
    </svg>
  )
}

function FolderIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2z" />
    </svg>
  )
}

function ChevronRightIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="9 18 15 12 9 6" />
    </svg>
  )
}

function SpinnerIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
    </svg>
  )
}

function CheckIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M5 13l4 4L19 7" />
    </svg>
  )
}

// Simple "G" badge for Garmin
function GarminBadge() {
  return (
    <span className="inline-flex items-center justify-center w-5 h-5 rounded bg-[#007CC0] text-white text-[10px] font-bold shrink-0 select-none">
      G
    </span>
  )
}

// Simple "S" badge for Strava
function StravaBadge() {
  return (
    <span className="inline-flex items-center justify-center w-5 h-5 rounded bg-[#FC4C02] text-white text-[10px] font-bold shrink-0 select-none">
      S
    </span>
  )
}

// ---------------------------------------------------------------------------
// Upload helper
// ---------------------------------------------------------------------------

async function uploadActivityFile(file: File): Promise<UploadResult> {
  const fd = new FormData()
  fd.append('file', file)
  return apiUpload<UploadResult>('/upload', fd)
}

// ---------------------------------------------------------------------------
// Drag-and-drop zone
// ---------------------------------------------------------------------------

interface DropZoneProps {
  onFiles: (files: File[]) => void
  disabled: boolean
}

function DropZone({ onFiles, disabled }: DropZoneProps) {
  const [dragging, setDragging] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault()
    if (!disabled) setDragging(true)
  }

  function handleDragLeave() {
    setDragging(false)
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    setDragging(false)
    if (disabled) return
    const files = Array.from(e.dataTransfer.files).filter(f =>
      /\.(fit|gpx|tcx)$/i.test(f.name)
    )
    if (files.length > 0) onFiles(files)
  }

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? [])
    if (files.length > 0) onFiles(files)
    e.target.value = ''
  }

  return (
    <button
      type="button"
      onClick={() => !disabled && inputRef.current?.click()}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      disabled={disabled}
      className={`w-full rounded-lg border-2 border-dashed px-4 py-4 text-center transition-colors cursor-pointer focus:outline-none
        ${dragging
          ? 'border-accent-DEFAULT bg-accent-DEFAULT/10'
          : 'border-neutral-300 dark:border-neutral-700 hover:border-accent-DEFAULT hover:bg-neutral-50 dark:hover:bg-neutral-800/50'
        }
        ${disabled ? 'opacity-50 cursor-not-allowed' : ''}
      `}
    >
      <input
        ref={inputRef}
        type="file"
        accept=".fit,.gpx,.tcx"
        multiple
        className="hidden"
        onChange={handleChange}
      />
      <UploadIcon className="w-5 h-5 mx-auto mb-1 text-neutral-400" />
      <p className="text-xs font-medium text-neutral-600 dark:text-neutral-300">
        FIT · GPX · TCX droppen oder klicken
      </p>
      <p className="text-[11px] text-neutral-400 mt-0.5">Mehrere Dateien gleichzeitig</p>
    </button>
  )
}

// ---------------------------------------------------------------------------
// Main ImportHub component
// ---------------------------------------------------------------------------

export default function ImportHub() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [uploadState, setUploadState] = useState<UploadState>({ type: 'idle' })
  const [hotfolderEnabled, setHotfolderEnabled] = useState<boolean | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Load hotfolder status when popover opens
  useEffect(() => {
    if (open && hotfolderEnabled === null) {
      getShadowFSSettings()
        .then(s => setHotfolderEnabled(s?.hotfolder_enabled ?? false))
        .catch(() => setHotfolderEnabled(false))
    }
  }, [open, hotfolderEnabled])

  // Close on click outside
  useEffect(() => {
    if (!open) return
    function onPointerDown(e: PointerEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [open])

  // Close on Escape
  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open])

  const handleFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return
    let successCount = 0
    for (const file of files) {
      setUploadState({ type: 'uploading', filename: file.name })
      try {
        const result = await uploadActivityFile(file)
        if (result.status !== 'skipped') successCount++
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err)
        setUploadState({ type: 'error', message: msg })
        return
      }
    }
    setUploadState({ type: 'success', count: successCount })
    // Auto-reset after 4 seconds
    setTimeout(() => setUploadState({ type: 'idle' }), 4000)
  }, [])

  // Global drag & drop — opens the popover when files are dragged anywhere on the page
  useEffect(() => {
    const handleWindowDragOver = (e: DragEvent) => {
      if (e.dataTransfer?.types.includes('Files')) {
        e.preventDefault()
        e.stopPropagation()
        setOpen(true)
      }
    }

    const handleWindowDrop = (e: DragEvent) => {
      e.preventDefault()
      e.stopPropagation()
      const files = Array.from(e.dataTransfer?.files ?? []).filter(f =>
        /\.(fit|gpx|tcx)$/i.test(f.name)
      )
      if (files.length > 0) {
        setOpen(true)
        handleFiles(files)
      }
    }

    window.addEventListener('dragover', handleWindowDragOver)
    window.addEventListener('drop', handleWindowDrop)
    return () => {
      window.removeEventListener('dragover', handleWindowDragOver)
      window.removeEventListener('drop', handleWindowDrop)
    }
  }, [handleFiles])

  function handleImporterLink(tab: 'garmin' | 'strava') {
    setOpen(false)
    navigate(`/settings/import?tab=${tab}`)
  }

  const uploading = uploadState.type === 'uploading'

  return (
    <div ref={containerRef} className="relative">
      {/* Trigger button */}
      <button
        onClick={() => setOpen(v => !v)}
        aria-label="Import-Hub öffnen"
        className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors
          ${open
            ? 'bg-accent-DEFAULT text-white'
            : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800'
          }`}
      >
        <UploadIcon className="w-4 h-4 shrink-0" />
        <span className="hidden sm:inline">Upload</span>
      </button>

      {/* Popover */}
      {open && (
        <div
          className="absolute right-0 top-full mt-2 w-80 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 shadow-xl z-50 overflow-hidden"
          role="dialog"
          aria-label="Import-Hub"
        >
          {/* Header */}
          <div className="px-4 pt-3 pb-2 border-b border-neutral-100 dark:border-neutral-800">
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
              Aktivität importieren
            </h2>
          </div>

          <div className="p-4 space-y-4">
            {/* Section 1: Direct file upload */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-neutral-500">
                <FileIcon className="w-3.5 h-3.5" />
                Datei hochladen
              </div>

              {uploadState.type === 'idle' && (
                <DropZone onFiles={handleFiles} disabled={false} />
              )}

              {uploadState.type === 'uploading' && (
                <div className="flex items-center gap-2 rounded-lg border border-neutral-200 dark:border-neutral-800 px-3 py-3 text-sm text-neutral-600 dark:text-neutral-300">
                  <SpinnerIcon className="w-4 h-4 animate-spin shrink-0 text-accent-DEFAULT" />
                  <span className="truncate">{uploadState.filename}</span>
                </div>
              )}

              {uploadState.type === 'success' && (
                <div className="flex items-center gap-2 rounded-lg border border-green-200 dark:border-green-800 bg-green-50 dark:bg-green-900/20 px-3 py-3 text-sm">
                  <CheckIcon className="w-4 h-4 shrink-0 text-green-600 dark:text-green-400" />
                  <span className="text-green-700 dark:text-green-300 font-medium">
                    {uploadState.count === 0
                      ? 'Aktivitat bereits vorhanden'
                      : `${uploadState.count} Aktivität${uploadState.count !== 1 ? 'en' : ''} importiert`}
                  </span>
                  <button
                    onClick={() => setUploadState({ type: 'idle' })}
                    className="ml-auto text-xs text-green-600 dark:text-green-400 hover:underline shrink-0"
                  >
                    Weitere
                  </button>
                </div>
              )}

              {uploadState.type === 'error' && (
                <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 px-3 py-3 text-sm space-y-1.5">
                  <p className="text-red-700 dark:text-red-300 font-medium line-clamp-2">
                    {uploadState.message}
                  </p>
                  <button
                    onClick={() => setUploadState({ type: 'idle' })}
                    className="text-xs text-red-600 dark:text-red-400 hover:underline"
                  >
                    Erneut versuchen
                  </button>
                </div>
              )}
            </div>

            {/* Divider */}
            <div className="h-px bg-neutral-100 dark:bg-neutral-800" />

            {/* Section 2: Importer links */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-neutral-500">
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a2 2 0 002 2h12a2 2 0 002-2v-1M8 12l4-4 4 4M12 8v8" />
                </svg>
                Bulk-Importer
              </div>

              <button
                onClick={() => handleImporterLink('garmin')}
                className="w-full flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-left text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors group"
              >
                <GarminBadge />
                <span className="flex-1 font-medium">Garmin Connect</span>
                <ChevronRightIcon className="w-3.5 h-3.5 text-neutral-400 group-hover:text-neutral-600 dark:group-hover:text-neutral-200 transition-colors" />
              </button>

              <button
                onClick={() => handleImporterLink('strava')}
                className="w-full flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-left text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors group"
              >
                <StravaBadge />
                <span className="flex-1 font-medium">Strava</span>
                <ChevronRightIcon className="w-3.5 h-3.5 text-neutral-400 group-hover:text-neutral-600 dark:group-hover:text-neutral-200 transition-colors" />
              </button>
            </div>

            {/* Divider */}
            <div className="h-px bg-neutral-100 dark:bg-neutral-800" />

            {/* Section 3: Hotfolder status */}
            <div className="space-y-2">
              <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-neutral-400 dark:text-neutral-500">
                <FolderIcon className="w-3.5 h-3.5" />
                Hotfolder
              </div>

              <button
                onClick={() => { setOpen(false); navigate('/settings') }}
                className="w-full flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-left text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors group"
              >
                <span className="shrink-0">
                  {hotfolderEnabled === null ? (
                    <span className="w-2 h-2 rounded-full bg-neutral-300 dark:bg-neutral-600 inline-block" />
                  ) : hotfolderEnabled ? (
                    <span className="w-2 h-2 rounded-full bg-emerald-500 inline-block" />
                  ) : (
                    <span className="w-2 h-2 rounded-full bg-neutral-400 dark:bg-neutral-600 inline-block" />
                  )}
                </span>
                <span className="flex-1">
                  {hotfolderEnabled === null
                    ? 'Lade…'
                    : hotfolderEnabled
                    ? 'Hotfolder aktiv'
                    : 'Hotfolder nicht aktiv'}
                </span>
                <ChevronRightIcon className="w-3.5 h-3.5 text-neutral-400 group-hover:text-neutral-600 dark:group-hover:text-neutral-200 transition-colors" />
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
