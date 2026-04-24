// frontend/src/components/AdminLibrarySettingsTab.tsx
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useToast } from '@/lib/toast'
import {
  getAdminLibrarySettings,
  updateAdminLibrarySettings,
  adminRenameBulk,
  type AdminLibrarySettings,
} from '@/api/shadowfs'
import { LIB_LABELS } from '@/lib/libraryLabels'
import ConfirmModal from '@/components/ConfirmModal'

const FILE_TYPES = ['activity', 'media', 'document', 'health', 'route'] as const

export function AdminLibrarySettingsTab() {
  const qc = useQueryClient()
  const { addToast } = useToast()

  const { data } = useQuery({
    queryKey: ['admin-library-settings'],
    queryFn: getAdminLibrarySettings,
  })

  const [local, setLocal] = useState<AdminLibrarySettings | null>(null)
  const [bulkConfirmFileType, setBulkConfirmFileType] = useState<string | null>(null)
  const effective = local ?? data

  const save = useMutation({
    mutationFn: updateAdminLibrarySettings,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-library-settings'] })
      qc.invalidateQueries({ queryKey: ['file-schemas-context'] })
      qc.invalidateQueries({ queryKey: ['library-dir'] })
      setLocal(null)
      addToast({ type: 'success', title: LIB_LABELS.toastSaved })
    },
    onError: (e: Error) =>
      addToast({ type: 'error', title: LIB_LABELS.toastError, message: e.message, duration: 0 }),
  })

  const bulk = useMutation({
    mutationFn: adminRenameBulk,
    onSuccess: (res, fileType) => {
      setBulkConfirmFileType(null)
      if (res.status === LIB_LABELS.statusBulkRenameInProgress) {
        addToast({ type: 'warning', title: LIB_LABELS.toastBulkAlreadyRunning })
      } else {
        addToast({
          type: 'success',
          title: `${LIB_LABELS.toastRenameStarted} (${res.candidate_count ?? 0} User, ${fileType})`,
        })
      }
    },
    onError: (e: Error) =>
      addToast({ type: 'error', title: LIB_LABELS.toastError, message: e.message, duration: 0 }),
  })

  if (!effective) {
    return (
      <div className="text-sm text-neutral-500 dark:text-neutral-400">
        {LIB_LABELS.adminTabTitle}…
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
        {LIB_LABELS.adminTabTitle}
      </h2>

      {/* Boolean toggles */}
      <div className="space-y-3">
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={effective.user_can_edit_dir_name}
            onChange={(e) =>
              setLocal({ ...effective, user_can_edit_dir_name: e.target.checked })
            }
            className="h-4 w-4 rounded border-neutral-300 text-blue-600 focus:ring-blue-500"
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
              setLocal({ ...effective, user_can_edit_schemas: e.target.checked })
            }
            className="h-4 w-4 rounded border-neutral-300 text-blue-600 focus:ring-blue-500"
          />
          <span className="text-sm text-neutral-700 dark:text-neutral-300">
            {LIB_LABELS.toggleUserCanEditSchemas}
          </span>
        </label>
      </div>

      {/* File-type schema inputs */}
      <div className="space-y-4">
        {FILE_TYPES.map((ft) => (
          <div key={ft} className="space-y-1">
            <label
              htmlFor={`schema-${ft}`}
              className="block text-sm font-medium text-neutral-700 dark:text-neutral-300"
            >
              {ft}
            </label>
            <input
              id={`schema-${ft}`}
              type="text"
              value={effective.default_schemas[ft] ?? ''}
              onChange={(e) =>
                setLocal({
                  ...effective,
                  default_schemas: { ...effective.default_schemas, [ft]: e.target.value },
                })
              }
              className="w-full rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 px-3 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <button
              type="button"
              className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
              onClick={() => setBulkConfirmFileType(ft)}
              disabled={bulk.isPending}
            >
              {LIB_LABELS.buttonBulkRename}
            </button>
          </div>
        ))}
      </div>

      {/* Save / discard bar — only shown when local edits exist */}
      {local && (
        <div className="flex gap-2 pt-2 border-t border-neutral-200 dark:border-neutral-700">
          <button
            type="button"
            onClick={() => save.mutate(local)}
            disabled={save.isPending}
            className="cursor-pointer rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {save.isPending ? '...' : LIB_LABELS.saveButton}
          </button>
          <button
            type="button"
            onClick={() => setLocal(null)}
            disabled={save.isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {LIB_LABELS.discardButton}
          </button>
        </div>
      )}

      {/* Bulk-rename confirmation modal */}
      {bulkConfirmFileType && (
        <ConfirmModal
          title={LIB_LABELS.bulkRenameConfirmTitle}
          message={LIB_LABELS.bulkRenameConfirmMessage.replace('{ft}', bulkConfirmFileType)}
          confirmLabel={LIB_LABELS.buttonBulkRename}
          isPending={bulk.isPending}
          onConfirm={() => bulk.mutate(bulkConfirmFileType)}
          onClose={() => setBulkConfirmFileType(null)}
        />
      )}
    </div>
  )
}
