import { useRef, useMemo, useState, useEffect } from 'react'
import { ChevronDown } from 'lucide-react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getFileSchemas,
  getFileSchemasContext,
  updateFileSchema,
  getLibraryDir,
  updateLibraryDir,
  FILE_TYPE_LABELS,
  API_STATUS,
  type FileType,
  type UpdateLibraryDirResponse,
} from '@/api/shadowfs'
import { useToast } from '@/lib/toast'
import { LIB_LABELS } from '@/lib/libraryLabels'
import { SettingsSection } from '@/components/ui/SettingsSection'

// ── Variable definitions per type ─────────────────────────────────────────
// IMPORTANT: Keep in sync with backend variable list defined in
// services/api/internal/shadowfs/schema.go (SchemaRenderer.Render) and
// services/api/internal/shadowfs/legacy_template.go.
// Any new variable added on the backend MUST also be added here.

interface VarDef {
  key: string
  label: string
}

const TYPE_VARIABLES: Record<FileType, VarDef[]> = {
  activity: [
    { key: '{year}',        label: 'Jahr'         },
    { key: '{month}',       label: 'Monat'        },
    { key: '{day}',         label: 'Tag'          },
    { key: '{time}',        label: 'Uhrzeit'      },
    { key: '{sport}',       label: 'Sportart'     },
    { key: '{subCategory}', label: 'Unterkategorie'},
    { key: '{city}',        label: 'Stadt'        },
    { key: '{country}',     label: 'Land'         },
    { key: '{title}',       label: 'Titel'        },
    { key: '{distance_km}', label: 'Distanz (km)' },
    { key: '{duration_h}',  label: 'Dauer (h)'    },
    { key: '{elevation_m}', label: 'Höhenmeter'   },
  ],
  media: [
    { key: '{year}',        label: 'Jahr'          },
    { key: '{month}',       label: 'Monat'         },
    { key: '{day}',         label: 'Tag'           },
    { key: '{time}',        label: 'Uhrzeit'       },
    { key: '{title}',       label: 'Titel'         },
    { key: '{parent_type}', label: 'Eltern-Typ'    },
    { key: '{parent_name}', label: 'Eltern-Name'   },
    { key: '{camera}',      label: 'Kamera'        },
    { key: '{width}',       label: 'Breite (px)'   },
    { key: '{height}',      label: 'Höhe (px)'     },
  ],
  document: [
    { key: '{year}',        label: 'Jahr'        },
    { key: '{month}',       label: 'Monat'       },
    { key: '{category}',    label: 'Kategorie'   },
    { key: '{title}',       label: 'Titel'       },
    { key: '{parent_type}', label: 'Eltern-Typ'  },
    { key: '{parent_name}', label: 'Eltern-Name' },
  ],
  health: [
    { key: '{year}',  label: 'Jahr'  },
    { key: '{month}', label: 'Monat' },
    { key: '{day}',   label: 'Tag'   },
    { key: '{type}',  label: 'Typ'   },
  ],
  route: [
    { key: '{sport}',       label: 'Sportart'     },
    { key: '{title}',       label: 'Titel'        },
    { key: '{distance_km}', label: 'Distanz (km)' },
    { key: '{elevation_m}', label: 'Höhenmeter'   },
    { key: '{difficulty}',  label: 'Schwierigkeit'},
  ],
}

const EXAMPLE_VALUES: Record<string, string> = {
  '{year}':        '2024',
  '{month}':       '03',
  '{day}':         '15',
  '{time}':        '14-30',
  '{sport}':       'cycling',
  '{subCategory}': 'mountain',
  '{city}':        'Berlin',
  '{country}':     'Deutschland',
  '{title}':       'Feierabendrunde',
  '{distance_km}': '42',
  '{duration_h}':  '2',
  '{elevation_m}': '850',
  '{parent_type}': 'activity',
  '{parent_name}': 'Feierabendrunde',
  '{camera}':      'Nikon-Z6',
  '{width}':       '5504',
  '{height}':      '3096',
  '{category}':    'rechnung',
  '{type}':        'hrv',
  '{difficulty}':  'T3',
}

const FILE_TYPE_ORDER: FileType[] = ['activity', 'media', 'document', 'health', 'route']

// ── SchemaSection — one collapsible per file type ─────────────────────────

interface SchemaSectionProps {
  fileType: FileType
  initialSchema: string
  effectiveSchema: string
  isUserOverride: boolean
  disabled?: boolean
  disabledHint?: string
}

function SchemaSection({ fileType, initialSchema, effectiveSchema, isUserOverride, disabled = false, disabledHint }: SchemaSectionProps) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const inputRef = useRef<HTMLInputElement>(null)
  const pendingCursorRef = useRef<number | null>(null)
  const [open, setOpen] = useState(false)
  const [localValue, setLocalValue] = useState<string | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])

  const currentValue = localValue ?? initialSchema
  const isDirty = localValue !== null && localValue !== initialSchema

  const preview = useMemo(() => {
    let result = currentValue
    for (const [key, val] of Object.entries(EXAMPLE_VALUES)) {
      result = result.replaceAll(key, val)
    }
    return result || '(leer)'
  }, [currentValue])

  const mut = useMutation({
    mutationFn: () => updateFileSchema(fileType, currentValue),
    onSuccess: (data) => {
      setWarnings(data.warnings ?? [])
      // Invalidate first, then clear localValue only after the cache reflects
      // the newly saved schema — prevents a flash where initialSchema still
      // shows the old value while localValue is already null.
      queryClient.invalidateQueries({ queryKey: ['file-schemas'] }).then(() => {
        if (data.schema === currentValue) {
          setLocalValue(null)
        }
      })
      addToast({ type: 'success', title: LIB_LABELS.fileSchemaToastSaved })
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'Unbekannter Fehler'
      addToast({ type: 'error', title: LIB_LABELS.fileSchemaErrorSave, message: msg, duration: 0 })
    },
  })

  // After insertVariable sets localValue, restore cursor position.
  // Dependency on localValue ensures this only runs after insertVariable
  // (or manual typing), not on every parent re-render.
  useEffect(() => {
    if (pendingCursorRef.current !== null && inputRef.current) {
      const pos = pendingCursorRef.current
      pendingCursorRef.current = null
      inputRef.current.setSelectionRange(pos, pos)
      inputRef.current.focus()
    }
  }, [localValue])

  function insertVariable(variable: string) {
    const input = inputRef.current
    if (!input) return
    const start = input.selectionStart ?? currentValue.length
    const end = input.selectionEnd ?? currentValue.length
    const next = currentValue.slice(0, start) + variable + currentValue.slice(end)
    pendingCursorRef.current = start + variable.length
    setLocalValue(next)
  }

  const vars = TYPE_VARIABLES[fileType]

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg overflow-hidden">
      {/* Header / toggle */}
      <button
        onClick={() => setOpen(prev => !prev)}
        className="w-full flex items-center justify-between px-4 py-3 text-left cursor-pointer
                   bg-neutral-50 dark:bg-neutral-800/50
                   hover:bg-neutral-100 dark:hover:bg-neutral-800
                   transition-colors"
        aria-expanded={open}
        aria-controls={`schema-body-${fileType}`}
      >
        <span className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
          {FILE_TYPE_LABELS[fileType] ?? fileType}
        </span>
        <div className="flex items-center gap-3">
          {!open && (initialSchema || effectiveSchema) && (
            <span className="text-xs font-mono text-neutral-400 dark:text-neutral-500 truncate max-w-60">
              {initialSchema || effectiveSchema}
            </span>
          )}
          <ChevronDown className={`w-4 h-4 text-neutral-400 transition-transform ${open ? 'rotate-180' : ''}`} aria-hidden="true" />
        </div>
      </button>

      {/* Body */}
      {open && (
        <div id={`schema-body-${fileType}`} className="px-4 py-4 space-y-3 bg-white dark:bg-neutral-900/30">
          {/* Input + Save */}
          <div>
            <label
              htmlFor={`schema-input-${fileType}`}
              className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1"
            >
              {LIB_LABELS.fileSchemaTemplateLabel}
            </label>
            <div className="flex gap-2">
              <input
                ref={inputRef}
                id={`schema-input-${fileType}`}
                type="text"
                value={disabled ? (currentValue || effectiveSchema) : currentValue}
                onChange={e => !disabled && setLocalValue(e.target.value)}
                disabled={disabled}
                readOnly={disabled}
                className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2
                           bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100
                           placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                           focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT text-sm font-mono
                           disabled:opacity-60 disabled:cursor-not-allowed disabled:bg-neutral-50
                           dark:disabled:bg-neutral-800/50"
              />
              <button
                onClick={() => mut.mutate()}
                disabled={!isDirty || mut.isPending || disabled}
                className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium text-white
                           bg-accent-DEFAULT hover:bg-accent-600
                           disabled:opacity-40 disabled:cursor-not-allowed
                           transition-colors"
              >
                {mut.isPending ? LIB_LABELS.savingButton : LIB_LABELS.saveButton}
              </button>
            </div>
            {disabled && disabledHint && (
              <p className="mt-1.5 text-xs text-neutral-500 dark:text-neutral-400">
                {disabledHint}
              </p>
            )}
            {!disabled && !isUserOverride && effectiveSchema && (
              <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                {LIB_LABELS.schemaAdminDefaultHint}<code className="rounded bg-neutral-100 px-1 py-0.5 font-mono dark:bg-neutral-800">{effectiveSchema}</code>
              </p>
            )}
          </div>

          {/* Variable chips */}
          <div className="space-y-1.5">
            <span className="text-xs text-neutral-500 dark:text-neutral-500">
              {LIB_LABELS.fileSchemaAvailableVars}
            </span>
            <div className="flex flex-wrap gap-1.5">
              {vars.map(v => (
                <button
                  key={v.key}
                  onClick={() => insertVariable(v.key)}
                  disabled={disabled}
                  aria-label={`${LIB_LABELS.fileSchemaInsertVariable}: ${v.key}`}
                  className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-md
                             border border-neutral-200 dark:border-neutral-700
                             bg-neutral-50 dark:bg-neutral-800
                             text-neutral-600 dark:text-neutral-400
                             hover:bg-orange-50 hover:border-orange-300 hover:text-accent-600
                             dark:hover:bg-orange-900/20 dark:hover:border-orange-700 dark:hover:text-orange-400
                             cursor-pointer transition-colors
                             disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-neutral-50
                             dark:disabled:hover:bg-neutral-800 disabled:hover:border-neutral-200
                             dark:disabled:hover:border-neutral-700 disabled:hover:text-neutral-600
                             dark:disabled:hover:text-neutral-400"
                >
                  <span className="font-mono">{v.key}</span>
                  <span className="text-neutral-400 dark:text-neutral-600">{v.label}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Live preview */}
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 p-3">
            <span className="text-xs text-neutral-500 dark:text-neutral-500 block mb-1">
              {LIB_LABELS.fileSchemaPreview}
            </span>
            <span className="text-sm font-mono text-neutral-800 dark:text-neutral-200 break-all">
              {preview}
            </span>
          </div>

          {/* Warnings from API */}
          {warnings.length > 0 && (
            <div className="rounded-lg border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20 p-3 space-y-1">
              {warnings.map((w) => (
                <p key={w} className="text-xs text-amber-700 dark:text-amber-400">{w}</p>
              ))}
            </div>
          )}

          {/* Save error inline */}
          {mut.isError && (
            <p className="text-xs text-rose-500">
              {LIB_LABELS.fileSchemaErrorSave}:{' '}
              {mut.error instanceof Error ? mut.error.message : 'Unbekannt'}
            </p>
          )}
        </div>
      )}
    </div>
  )
}

// ── Library directory section ─────────────────────────────────────────────

function LibraryDirSection() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [localValue, setLocalValue] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['library-dir'],
    queryFn: getLibraryDir,
    staleTime: 60_000,
  })

  const editable = data?.editable !== false
  const effectiveName = data?.effective_name ?? ''
  const adminManagedReason = data?.admin_managed_reason ?? null

  const currentValue = localValue ?? data?.library_dir_name ?? ''
  const isDirty = localValue !== null && localValue !== (data?.library_dir_name ?? '')

  const readOnlyHint =
    adminManagedReason === 's3_backend'
      ? LIB_LABELS.libraryDirNotApplicableS3
      : LIB_LABELS.libraryDirManagedByAdmin

  const mut = useMutation<UpdateLibraryDirResponse>({
    mutationFn: () => updateLibraryDir(currentValue),
    onSuccess: (res) => {
      if (res.status === API_STATUS.libraryDirNameTaken) {
        addToast({ type: 'warning', title: LIB_LABELS.toastLibraryDirTaken })
        return
      }
      if (res.status === API_STATUS.libraryDirNameClashesUUID) {
        addToast({ type: 'warning', title: LIB_LABELS.toastLibraryDirClashesUUID })
        return
      }
      queryClient.invalidateQueries({ queryKey: ['library-dir'] }).then(() => {
        setLocalValue(null)
      })
      addToast({ type: 'success', title: LIB_LABELS.libraryDirToastSaved })
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'Unbekannter Fehler'
      addToast({ type: 'error', title: LIB_LABELS.fileSchemaErrorSave, message: msg, duration: 0 })
    },
  })

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg p-4 space-y-3">
      <div>
        <h3 className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
          {LIB_LABELS.libraryDirSectionTitle}
        </h3>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          {LIB_LABELS.libraryDirSectionDesc}
        </p>
      </div>

      {isLoading ? (
        <div className="h-9 rounded-lg bg-neutral-100 dark:bg-neutral-800 animate-pulse" />
      ) : !editable ? (
        /* Read-only view */
        <div className="space-y-1.5">
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg
                          border border-neutral-200 dark:border-neutral-700
                          bg-neutral-50 dark:bg-neutral-800/50">
            <span className="text-xs text-neutral-500 dark:text-neutral-400 shrink-0">
              {LIB_LABELS.libraryDirPrefix}
            </span>
            <code className="text-sm font-mono text-neutral-800 dark:text-neutral-200 break-all">
              {effectiveName}
            </code>
          </div>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {readOnlyHint}
          </p>
        </div>
      ) : (
        /* Editable view */
        <div className="flex gap-2">
          <label htmlFor="library-dir-input" className="sr-only">
            {LIB_LABELS.libraryDirSectionTitle}
          </label>
          <input
            id="library-dir-input"
            type="text"
            value={currentValue}
            onChange={e => setLocalValue(e.target.value)}
            placeholder={LIB_LABELS.libraryDirInputPlaceholder}
            maxLength={100}
            className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2
                       bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100
                       placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                       focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT text-sm font-mono"
          />
          <button
            onClick={() => mut.mutate()}
            disabled={!isDirty || mut.isPending}
            className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium text-white
                       bg-accent-DEFAULT hover:bg-accent-600
                       disabled:opacity-40 disabled:cursor-not-allowed
                       transition-colors"
          >
            {mut.isPending ? LIB_LABELS.savingButton : LIB_LABELS.saveButton}
          </button>
        </div>
      )}

      {mut.isError && (
        <p className="text-xs text-rose-500">
          {LIB_LABELS.fileSchemaErrorSave}:{' '}
          {mut.error instanceof Error ? mut.error.message : 'Unbekannt'}
        </p>
      )}
    </div>
  )
}

// ── Main component ─────────────────────────────────────────────────────────

export default function FileSchemaSettings() {
  const { data: schemas, isLoading, isError } = useQuery({
    queryKey: ['file-schemas'],
    queryFn: getFileSchemas,
    staleTime: 60_000,
  })

  const { data: context } = useQuery({
    queryKey: ['file-schemas-context'],
    queryFn: getFileSchemasContext,
    staleTime: 60_000,
  })

  // Build lookup by file_type for O(1) access
  const schemaMap = useMemo(() => {
    const map: Partial<Record<FileType, { schema: string; effective_schema: string; is_user_override: boolean }>> = {}
    for (const s of schemas ?? []) {
      map[s.file_type] = { schema: s.schema, effective_schema: s.effective_schema, is_user_override: s.is_user_override }
    }
    return map
  }, [schemas])

  const schemasDisabled = context?.editable === false
  const schemasHint = schemasDisabled
    ? context?.admin_managed_reason === 's3_backend'
      ? LIB_LABELS.schemaS3Inactive
      : LIB_LABELS.schemaAdminManaged
    : undefined

  return (
    <SettingsSection title={LIB_LABELS.fileSchemaSectionTitle} description={LIB_LABELS.fileSchemaSectionDescription}>

      {isError && (
        <p className="text-sm text-rose-500">{LIB_LABELS.fileSchemaErrorLoad}</p>
      )}

      {isLoading ? (
        <div className="space-y-2">
          {FILE_TYPE_ORDER.map(ft => (
            <div key={ft} className="h-12 rounded-lg bg-neutral-100 dark:bg-neutral-800 animate-pulse" />
          ))}
        </div>
      ) : (
        <div className="space-y-2">
          {FILE_TYPE_ORDER.map(ft => (
            <SchemaSection
              key={ft}
              fileType={ft}
              initialSchema={schemaMap[ft]?.schema ?? ''}
              effectiveSchema={schemaMap[ft]?.effective_schema ?? ''}
              isUserOverride={schemaMap[ft]?.is_user_override ?? false}
              disabled={schemasDisabled}
              disabledHint={schemasHint}
            />
          ))}
        </div>
      )}

      <LibraryDirSection />
    </SettingsSection>
  )
}
