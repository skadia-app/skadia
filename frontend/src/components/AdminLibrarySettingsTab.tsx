// frontend/src/components/AdminLibrarySettingsTab.tsx
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useToast } from '@/lib/toast'
import {
  getAdminLibrarySettings,
  updateAdminLibrarySettings,
  adminRenameBulk,
  API_STATUS,
  type AdminLibrarySettings,
  type FileType,
} from '@/api/shadowfs'
import { FILE_TYPE_ORDER } from '@/lib/libraryLabels'
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
  const { t } = useTranslation()
  const [localValue, setLocalValue] = useState<string | null>(null)
  const [bulkConfirm, setBulkConfirm] = useState(false)

  const currentValue = localValue ?? savedValue
  const isDirty = localValue !== null && localValue !== savedValue
  const fileTypeLabel = t(`admin.storage.fileTypes.${fileType}`)

  const saveMut = useMutation({
    mutationFn: () =>
      updateAdminLibrarySettings({
        ...effective,
        default_schemas: { ...effective.default_schemas, [fileType]: currentValue },
      }),
    onSuccess: () => {
      setLocalValue(null)
      onSaved()
      addToast({ type: 'success', title: t('admin.library.toastSaved') })
    },
    onError: (e: Error) =>
      addToast({ type: 'error', title: t('common.error'), message: e.message, duration: 0 }),
  })

  const bulkMut = useMutation({
    mutationFn: () => adminRenameBulk(fileType),
    onSuccess: (res) => {
      setBulkConfirm(false)
      if (res.status === API_STATUS.bulkRenameInProgress) {
        addToast({ type: 'warning', title: t('admin.library.toastBulkAlreadyRunning') })
      } else {
        addToast({
          type: 'success',
          title: t('admin.library.toastRenameStarted', {
            count: res.candidate_count ?? 0,
            fileType: fileTypeLabel,
          }),
        })
      }
    },
    onError: (e: Error) =>
      addToast({ type: 'error', title: t('common.error'), message: e.message, duration: 0 }),
  })

  const bulkRenameButton = (
    <>
      <button
        type="button"
        onClick={() => setBulkConfirm(true)}
        disabled={bulkMut.isPending}
        aria-label={t('admin.library.buttonBulkRename')}
        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg
                   border border-amber-300 dark:border-amber-600
                   text-amber-700 dark:text-amber-400
                   bg-amber-50 dark:bg-amber-900/20
                   hover:bg-amber-100 dark:hover:bg-amber-900/40
                   disabled:opacity-50 disabled:cursor-not-allowed
                   cursor-pointer transition-colors"
      >
        <TriangleAlert className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
        <span>{t('admin.library.buttonBulkRename')}</span>
        <span className="text-amber-500 dark:text-amber-500 font-normal">{t('admin.library.affectsAllUsers')}</span>
      </button>

      {bulkConfirm && (
        <ConfirmModal
          title={t('admin.library.bulkRenameConfirmTitle')}
          message={t('admin.library.bulkRenameConfirmMessage', { fileType: fileTypeLabel })}
          confirmLabel={t('admin.library.bulkRenameConfirmLabel')}
          isPending={bulkMut.isPending}
          onConfirm={() => bulkMut.mutate()}
          onClose={() => setBulkConfirm(false)}
        />
      )}
    </>
  )

  const saveError = saveMut.isError
    ? (saveMut.error instanceof Error ? saveMut.error.message : t('common.unknown'))
    : null

  return (
    <FileSchemaAccordionRow
      fileType={fileType}
      label={fileTypeLabel}
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
  const { t } = useTranslation()

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
      addToast({ type: 'success', title: t('admin.library.toastSaved') })
    },
    onError: (e: Error) =>
      addToast({ type: 'error', title: t('common.error'), message: e.message, duration: 0 }),
  })

  function invalidateAfterRowSave() {
    qc.invalidateQueries({ queryKey: ['admin-library-settings'] })
    qc.invalidateQueries({ queryKey: ['file-schemas-context'] })
    qc.invalidateQueries({ queryKey: ['library-dir'] })
  }

  if (!effective) {
    return (
      <div className="text-sm text-neutral-500 dark:text-neutral-400">
        {t('admin.library.loading')}
      </div>
    )
  }

  if (!localActionsAvailable) {
    return (
      <div className="space-y-6">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {t('admin.library.title')}
        </h2>
        <div className="flex items-start gap-2 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-300">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{t('admin.library.schemaS3Inactive')}</span>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
        {t('admin.library.title')}
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
            {t('admin.library.toggleUserCanEditDirName')}
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
            {t('admin.library.toggleUserCanEditSchemas')}
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
            {saveToggles.isPending ? t('common.saving') : t('common.save')}
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
            {t('admin.library.discardButton')}
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
