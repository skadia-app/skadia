import { useState, useCallback } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createRoute, updateRoute, setRouteTags } from '@/api/routes'
import type { PlannedRoute, RouteWaypoint, RouteSegment } from '@/api/routes'
import { useToast } from '@/lib/toast'
import RouteMetadataForm from './RouteMetadataForm'
import type { RouteMetadataValues } from './RouteMetadataForm'

// ─── Types ────────────────────────────────────────────────────────────────────

interface Props {
  /** Currently planned waypoints */
  waypoints: RouteWaypoint[]
  /** Currently planned segments */
  segments: RouteSegment[]
  /** Active BRouter profile in the planner */
  currentProfile: string
  /** When set: we are editing an existing route. Allows "save" + "save as new". */
  editingRouteId?: string | null
  /** Pre-fill metadata from the route being edited */
  existingRoute?: PlannedRoute | null
  /** Called when the dialog should close (cancel or after successful save) */
  onClose: () => void
  /** Called after a successful save with the saved route */
  onSaved: (route: PlannedRoute) => void
}

type SaveMode = 'save' | 'save_as_new'

// ─── RouteSaveDialog ──────────────────────────────────────────────────────────

export default function RouteSaveDialog({
  waypoints,
  segments,
  currentProfile,
  editingRouteId,
  existingRoute,
  onClose,
  onSaved,
}: Props) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const isEditing = editingRouteId != null

  const [metadata, setMetadata] = useState<RouteMetadataValues>({
    name: existingRoute?.name ?? '',
    description: existingRoute?.description ?? '',
    sport_type: existingRoute?.sport_type ?? 'hiking',
    difficulty: existingRoute?.difficulty ?? '',
    tag_ids: existingRoute?.tags?.map((t) => t.id) ?? [],
    auto_tag: existingRoute?.auto_tag ?? true,
    profile_override: existingRoute?.profile ?? currentProfile,
  })

  const [saveMode, setSaveMode] = useState<SaveMode>('save')

  // ── Mutation ────────────────────────────────────────────────────────────────

  const saveMutation = useMutation({
    mutationFn: async () => {
      const payload: Partial<PlannedRoute> = {
        name: metadata.name.trim(),
        description: metadata.description.trim() || undefined,
        sport_type: metadata.sport_type,
        difficulty: metadata.difficulty.trim() || undefined,
        auto_tag: metadata.auto_tag,
        profile: metadata.profile_override,
        waypoints,
        segments,
      }

      let saved: PlannedRoute

      if (isEditing && saveMode === 'save') {
        saved = await updateRoute(editingRouteId!, payload)
      } else {
        // Both "create" and "save as new" use createRoute
        saved = await createRoute(payload)
      }

      // Sync tags separately
      if (metadata.tag_ids.length > 0) {
        await setRouteTags(saved.id, metadata.tag_ids)
      }

      return saved
    },
    onSuccess: (saved) => {
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      queryClient.invalidateQueries({ queryKey: ['route', saved.id] })
      addToast({
        type: 'success',
        title: saveMode === 'save_as_new' ? 'Als neue Route gespeichert' : 'Route gespeichert',
        message: saved.name,
      })
      onSaved(saved)
      onClose()
    },
    onError: (err) => {
      addToast({
        type: 'error',
        title: 'Fehler beim Speichern',
        message: err instanceof Error ? err.message : 'Unbekannter Fehler',
        duration: 0,
      })
    },
  })

  // ── Handlers ────────────────────────────────────────────────────────────────

  const handleSave = useCallback(
    (mode: SaveMode) => {
      if (!metadata.name.trim()) return
      setSaveMode(mode)
      // Use requestAnimationFrame to let setState flush before mutate
      requestAnimationFrame(() => saveMutation.mutate())
    },
    [metadata.name, saveMutation],
  )

  const isPending = saveMutation.isPending
  const isValid = metadata.name.trim().length > 0

  // ── Render ──────────────────────────────────────────────────────────────────

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/50 backdrop-blur-sm"
        onClick={!isPending ? onClose : undefined}
      />

      {/* Dialog panel */}
      <div className="relative bg-white dark:bg-neutral-900 rounded-2xl shadow-2xl border border-neutral-200 dark:border-neutral-800 w-full max-w-lg flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-200 dark:border-neutral-800 flex-shrink-0">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
            {isEditing ? 'Route speichern' : 'Route speichern'}
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isPending}
            className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 transition-colors disabled:opacity-50 cursor-pointer"
            aria-label="Schließen"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Body — scrollable */}
        <div className="flex-1 overflow-y-auto px-6 py-5">
          <RouteMetadataForm
            initialValues={metadata}
            currentProfile={currentProfile}
            onChange={setMetadata}
            isNew={saveMode === 'save_as_new'}
          />
        </div>

        {/* Footer actions */}
        <div className="flex items-center justify-end gap-3 px-6 py-4 border-t border-neutral-200 dark:border-neutral-800 flex-shrink-0">
          <button
            type="button"
            onClick={onClose}
            disabled={isPending}
            className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors disabled:opacity-50 cursor-pointer"
          >
            Abbrechen
          </button>

          {isEditing && (
            <button
              type="button"
              onClick={() => handleSave('save_as_new')}
              disabled={isPending || !isValid}
              className="px-4 py-2 rounded-lg text-sm font-medium text-accent-700 dark:text-accent-300 border border-accent-300 dark:border-accent-700 hover:bg-accent-50 dark:hover:bg-accent-900/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
            >
              {isPending && saveMode === 'save_as_new' ? (
                <span className="flex items-center gap-2">
                  <Spinner /> Als neue Route…
                </span>
              ) : (
                'Als neue Route speichern'
              )}
            </button>
          )}

          <button
            type="button"
            onClick={() => handleSave('save')}
            disabled={isPending || !isValid}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            {isPending && saveMode === 'save' ? (
              <span className="flex items-center gap-2">
                <Spinner /> Speichert…
              </span>
            ) : (
              'Speichern'
            )}
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── Spinner ──────────────────────────────────────────────────────────────────

function Spinner() {
  return (
    <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
    </svg>
  )
}
