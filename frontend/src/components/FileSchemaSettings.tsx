import { useState, useMemo } from 'react'
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
import { LIB_LABELS, FILE_TYPE_ORDER } from '@/lib/libraryLabels'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { FileSchemaAccordionRow } from '@/components/FileSchemaAccordionRow'

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : LIB_LABELS.unknownErrorFallback
}

// ── SchemaSection — one collapsible per file type ─────────────────────────

interface SchemaSectionProps {
  fileType: FileType
  initialSchema: string
  effectiveSchema: string
  isUserOverride: boolean
  disabled?: boolean
  disabledHint?: string
  expanded: boolean
  onToggle: () => void
}

function SchemaSection({
  fileType,
  initialSchema,
  effectiveSchema,
  isUserOverride,
  disabled = false,
  disabledHint,
  expanded,
  onToggle,
}: SchemaSectionProps) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [localValue, setLocalValue] = useState<string | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])

  const currentValue = localValue ?? initialSchema
  const isDirty = localValue !== null && localValue !== initialSchema

  const mut = useMutation({
    mutationFn: () => updateFileSchema(fileType, currentValue),
    onSuccess: (data) => {
      setWarnings(data.warnings ?? [])
      queryClient.invalidateQueries({ queryKey: ['file-schemas'] }).then(() => {
        if (data.schema === currentValue) {
          setLocalValue(null)
        }
      })
      addToast({ type: 'success', title: LIB_LABELS.fileSchemaToastSaved })
    },
    onError: (err: unknown) => {
      const msg = errorMessage(err)
      addToast({ type: 'error', title: LIB_LABELS.fileSchemaErrorSave, message: msg, duration: 0 })
    },
  })

  const adminDefaultHint =
    !disabled && !isUserOverride && effectiveSchema ? effectiveSchema : undefined

  const saveError = mut.isError
    ? errorMessage(mut.error)
    : null

  return (
    <FileSchemaAccordionRow
      fileType={fileType}
      label={FILE_TYPE_LABELS[fileType] ?? fileType}
      currentValue={disabled ? (currentValue || effectiveSchema) : currentValue}
      onChange={setLocalValue}
      onSave={() => mut.mutate()}
      isSaving={mut.isPending}
      isDirty={isDirty}
      disabled={disabled}
      disabledHint={disabled ? disabledHint : undefined}
      adminDefaultHint={adminDefaultHint}
      expanded={expanded}
      onToggle={onToggle}
      warnings={warnings}
      saveError={saveError}
    />
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
      const msg = errorMessage(err)
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
          {errorMessage(mut.error)}
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

  const [expandedType, setExpandedType] = useState<FileType | null>(null)

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
              expanded={expandedType === ft}
              onToggle={() => setExpandedType(prev => prev === ft ? null : ft)}
            />
          ))}
        </div>
      )}

      <LibraryDirSection />
    </SettingsSection>
  )
}
