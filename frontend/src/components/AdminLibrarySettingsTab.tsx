// frontend/src/components/AdminLibrarySettingsTab.tsx
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { TriangleAlert } from 'lucide-react'
import { useToast } from '@/lib/toast'
import {
  getAdminLibrarySettings,
  updateAdminLibrarySettings,
  adminRenameBulk,
  FILE_TYPE_LABELS,
  API_STATUS,
  type AdminLibrarySettings,
  type FileType,
} from '@/api/shadowfs'
import { LIB_LABELS, FILE_TYPE_ORDER } from '@/lib/libraryLabels'
import { FileSchemaAccordionRow } from '@/components/FileSchemaAccordionRow'
import ConfirmModal from '@/components/ConfirmModal'

// ── AdminSchemaRow — accordion row with per-row save + bulk-rename ────────

interface AdminSchemaRowProps {
  fileType: FileType
  /** Current saved value from server (used as baseline for dirty detection). */
  savedValue: string
  /** Effective settings object (used when saving — keeps toggles in sync). */
  effective: AdminLibrarySettings
  expanded: boolean
  onToggle: () => void
  /** Called after a successful save so parent can refresh query data. */
  onSaved: () => void
}

function AdminSchemaRow({
  fileType,
  savedValue,
  effective,
  expanded,
  onToggle,
  onSaved,
}: AdminSchemaRowProps) {
  const { addToast } = useToast()
  const [localValue, setLocalValue] = useState<string | null>(null)
  const [bulkConfirm, setBulkConfirm] = useState(false)

  const currentValue = localValue ?? savedValue
  const isDirty = localValue !== null && localValue !== savedValue

  const saveMut = useMutation({
    mutationFn: () =>
      updateAdminLibrarySettings({
        ...effective,
        default_schemas: { ...effective.default_schemas, [fileType]: currentValue },
      }),
    onSuccess: () => {
      setLocalValue(null)
      onSaved()
      addToast({ type: 'success', title: LIB_LABELS.toastSaved })
    },
    onError: (e: Error) =>
      addToast({ type: 'error', title: LIB_LABELS.toastError, message: e.message, duration: 0 }),
  })

  const bulkMut = useMutation({
    mutationFn: () => adminRenameBulk(fileType),
    onSuccess: (res) => {
      setBulkConfirm(false)
      if (res.status === API_STATUS.bulkRenameInProgress) {
        addToast({ type: 'warning', title: LIB_LABELS.toastBulkAlreadyRunning })
      } else {
        addToast({
          type: 'success',
          title: `${LIB_LABELS.toastRenameStarted} (${res.candidate_count ?? 0} User, ${FILE_TYPE_LABELS[fileType] ?? fileType})`,
        })
      }
    },
    onError: (e: Error) =>
      addToast({ type: 'error', title: LIB_LABELS.toastError, message: e.message, duration: 0 }),
  })

  const bulkRenameButton = (
    <>
      <button
        type="button"
        onClick={() => setBulkConfirm(true)}
        disabled={bulkMut.isPending}
        aria-label={LIB_LABELS.buttonBulkRename}
        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg
                   border border-amber-300 dark:border-amber-600
                   text-amber-700 dark:text-amber-400
                   bg-amber-50 dark:bg-amber-900/20
                   hover:bg-amber-100 dark:hover:bg-amber-900/40
                   disabled:opacity-50 disabled:cursor-not-allowed
                   cursor-pointer transition-colors"
      >
        <TriangleAlert className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
        <span>{LIB_LABELS.buttonBulkRename}</span>
        <span className="text-amber-500 dark:text-amber-500 font-normal">— betrifft alle User</span>
      </button>

      {bulkConfirm && (
        <ConfirmModal
          title={LIB_LABELS.bulkRenameConfirmTitle}
          message={LIB_LABELS.bulkRenameConfirmMessage.replace('{ft}', fileType)}
          confirmLabel={LIB_LABELS.bulkRenameConfirmLabel}
          isPending={bulkMut.isPending}
          onConfirm={() => bulkMut.mutate()}
          onClose={() => setBulkConfirm(false)}
        />
      )}
    </>
  )

  const saveError = saveMut.isError
    ? (saveMut.error instanceof Error ? saveMut.error.message : 'Unbekannt')
    : null

  return (
    <FileSchemaAccordionRow
      fileType={fileType}
      label={FILE_TYPE_LABELS[fileType] ?? fileType}
      currentValue={currentValue}
      onChange={setLocalValue}
      onSave={() => saveMut.mutate()}
      isSaving={saveMut.isPending}
      isDirty={isDirty}
      expanded={expanded}
      onToggle={onToggle}
      extraActions={bulkRenameButton}
      saveError={saveError}
    />
  )
}

// ── Main component ─────────────────────────────────────────────────────────

export function AdminLibrarySettingsTab() {
  const qc = useQueryClient()
  const { addToast } = useToast()

  const { data } = useQuery({
    queryKey: ['admin-library-settings'],
    queryFn: getAdminLibrarySettings,
  })

  // Local state only for the boolean toggles (schemas save per-row).
  const [localToggles, setLocalToggles] = useState<Pick<
    AdminLibrarySettings,
    'user_can_edit_dir_name' | 'user_can_edit_schemas'
  > | null>(null)

  const [expandedType, setExpandedType] = useState<FileType | null>(null)

  const effective = data
    ? { ...data, ...(localToggles ?? {}) }
    : null

  const toggleDirty = localToggles !== null
  const localActionsAvailable = effective
    ? effective.backend !== 's3' && effective.local_actions_available !== false
    : false

  const saveToggles = useMutation({
    mutationFn: () => updateAdminLibrarySettings(effective!),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-library-settings'] })
      qc.invalidateQueries({ queryKey: ['file-schemas-context'] })
      qc.invalidateQueries({ queryKey: ['library-dir'] })
      setLocalToggles(null)
      addToast({ type: 'success', title: LIB_LABELS.toastSaved })
    },
    onError: (e: Error) =>
      addToast({ type: 'error', title: LIB_LABELS.toastError, message: e.message, duration: 0 }),
  })

  function invalidateAfterRowSave() {
    qc.invalidateQueries({ queryKey: ['admin-library-settings'] })
    qc.invalidateQueries({ queryKey: ['file-schemas-context'] })
    qc.invalidateQueries({ queryKey: ['library-dir'] })
  }

  if (!effective) {
    return (
      <div className="text-sm text-neutral-500 dark:text-neutral-400">
        {LIB_LABELS.adminTabTitle}…
      </div>
    )
  }

  if (!localActionsAvailable) {
    return (
      <div className="space-y-6">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {LIB_LABELS.adminTabTitle}
        </h2>
        <div className="flex items-start gap-2 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-300">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{LIB_LABELS.schemaS3Inactive}</span>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
        {LIB_LABELS.adminTabTitle}
      </h2>

      {/* ── Boolean toggles ──────────────────────────────────────────────── */}
      <div className="space-y-3">
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={effective.user_can_edit_dir_name}
            onChange={(e) =>
              setLocalToggles({
                user_can_edit_dir_name: e.target.checked,
                user_can_edit_schemas: effective.user_can_edit_schemas,
              })
            }
            className="h-4 w-4 rounded border-neutral-300 accent-accent-DEFAULT focus:ring-accent-DEFAULT"
          />
          <span className="text-sm text-neutral-700 dark:text-neutral-300">
            {LIB_LABELS.toggleUserCanEditDirName}
          </span>
        </label>

        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={effective.user_can_edit_schemas}
            onChange={(e) =>
              setLocalToggles({
                user_can_edit_dir_name: effective.user_can_edit_dir_name,
                user_can_edit_schemas: e.target.checked,
              })
            }
            className="h-4 w-4 rounded border-neutral-300 accent-accent-DEFAULT focus:ring-accent-DEFAULT"
          />
          <span className="text-sm text-neutral-700 dark:text-neutral-300">
            {LIB_LABELS.toggleUserCanEditSchemas}
          </span>
        </label>
      </div>

      {/* Toggle save/discard bar — shown only when toggles have local edits */}
      {toggleDirty && (
        <div className="flex gap-2 pt-2 border-t border-neutral-200 dark:border-neutral-700">
          <button
            type="button"
            onClick={() => saveToggles.mutate()}
            disabled={saveToggles.isPending}
            className="cursor-pointer rounded-lg px-4 py-2 text-sm font-medium text-white
                       bg-accent-DEFAULT hover:bg-accent-600
                       disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {saveToggles.isPending ? LIB_LABELS.savingButton : LIB_LABELS.saveButton}
          </button>
          <button
            type="button"
            onClick={() => setLocalToggles(null)}
            disabled={saveToggles.isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2
                       text-sm text-neutral-600 dark:text-neutral-400
                       hover:bg-neutral-100 dark:hover:bg-neutral-800
                       disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {LIB_LABELS.discardButton}
          </button>
        </div>
      )}

      {/* ── Schema accordion rows ────────────────────────────────────────── */}
      <div className="space-y-2">
        {FILE_TYPE_ORDER.map(ft => (
          <AdminSchemaRow
            key={ft}
            fileType={ft}
            savedValue={effective.default_schemas[ft] ?? ''}
            effective={effective}
            expanded={expandedType === ft}
            onToggle={() => setExpandedType(prev => prev === ft ? null : ft)}
            onSaved={invalidateAfterRowSave}
          />
        ))}
      </div>
    </div>
  )
}
