import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeft } from 'lucide-react'
import { getMe } from '@/api/auth'
import {
  deleteTargetPlace,
  deleteTargetRegion,
  getTargetPlace,
  getTargetRegion,
  updateTargetPlace,
  updateTargetRegion,
  type MultiPolygonGeometry,
  type PointGeometry,
  type PolygonGeometry,
  type TargetGeometry,
  type TargetPlace,
  type TargetPlaceResponse,
  type TargetRegion,
  type TargetRegionResponse,
} from '@/api/targetObjects'
import PlaceRegionEditor, { type PlaceRegionEditorValue } from '@/components/target-objects/PlaceRegionEditor'
import { placeKindLabel, regionKindLabel, targetVisibilityLabel } from '@/components/target-objects/targetLabels'
import TargetOverlayTabs from '@/components/target-overlays/TargetOverlayTabs'
import { useToast } from '@/lib/toast'

function asPointGeometry(value: PlaceRegionEditorValue): PointGeometry {
  if (value.geometry.type !== 'Point') {
    throw new Error('invalidPointGeometry')
  }
  return value.geometry
}

function asRegionGeometry(value: PlaceRegionEditorValue): PolygonGeometry | MultiPolygonGeometry {
  if (value.geometry.type !== 'Polygon' && value.geometry.type !== 'MultiPolygon') {
    throw new Error('invalidRegionGeometry')
  }
  return value.geometry
}

type DetailKind = 'place' | 'region'

const TARGET_OBJECT_PERMISSION_LABEL_KEYS: Record<string, string> = {
  edit_metadata: 'targetObjects.permissions.edit_metadata',
  edit_geometry: 'targetObjects.permissions.edit_geometry',
  publish: 'targetObjects.permissions.publish',
  hide_or_archive: 'targetObjects.permissions.hide_or_archive',
  disable_leaderboard: 'targetObjects.permissions.disable_leaderboard',
  disable_gps_download: 'targetObjects.permissions.disable_gps_download',
  manage_reports: 'targetObjects.permissions.manage_reports',
  manage_events: 'targetObjects.permissions.manage_events',
  resolve_claims: 'targetObjects.permissions.resolve_claims',
  manage_members: 'targetObjects.permissions.manage_members',
  manage_roles: 'targetObjects.permissions.manage_roles',
  link_external_refs: 'targetObjects.permissions.link_external_refs',
}

export default function TargetObjectDetailPage() {
  const { t } = useTranslation()
  const { kind = 'place', id = '' } = useParams()
  const detailKind: DetailKind = kind === 'region' ? 'region' : 'place'
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [editing, setEditing] = useState(false)
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 60_000 })
  const detail = useQuery({
    queryKey: ['target-object', detailKind, id],
    queryFn: async () => {
      if (detailKind === 'region') return (await getTargetRegion(id, { include_overlay_summary: true })).region
      return (await getTargetPlace(id, { include_overlay_summary: true })).place
    },
    enabled: Boolean(id),
  })
  const archiveMutation = useMutation({
    mutationFn: () => detailKind === 'region' ? deleteTargetRegion(id) : deleteTargetPlace(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-object', detailKind, id] })
      queryClient.invalidateQueries({ queryKey: detailKind === 'region' ? ['target-regions'] : ['target-places'] })
      addToast({ type: 'success', title: detailKind === 'region' ? t('targetObjects.regionArchived') : t('targetObjects.placeArchived') })
    },
    onError: error => addToast({ type: 'error', title: detailKind === 'region' ? t('targetObjects.regionArchiveFailed') : t('targetObjects.placeArchiveFailed'), message: errorMessage(error, t), duration: 0 }),
  })
  const updateMutation = useMutation<TargetPlaceResponse | TargetRegionResponse, Error, PlaceRegionEditorValue>({
    mutationFn: (value: PlaceRegionEditorValue): Promise<TargetPlaceResponse | TargetRegionResponse> => {
      const ownerEdit = Boolean(me?.is_admin || me?.user_id === object?.owner_user_id)
      const sharedPatch = {
        name: value.name,
        visibility: ownerEdit ? value.visibility : undefined,
        access_scope_id: ownerEdit ? value.access_scope_id : undefined,
      }
      if (detailKind === 'region') {
        return updateTargetRegion(id, { ...sharedPatch, geometry: asRegionGeometry(value), region_kind: value.kind as TargetRegion['region_kind'] })
      }
      return updateTargetPlace(id, { ...sharedPatch, geometry: asPointGeometry(value), place_kind: value.kind as TargetPlace['place_kind'] })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-object', detailKind, id] })
      queryClient.invalidateQueries({ queryKey: detailKind === 'region' ? ['target-regions'] : ['target-places'] })
      setEditing(false)
      addToast({ type: 'success', title: detailKind === 'region' ? t('targetObjects.regionSaved') : t('targetObjects.placeSaved') })
    },
    onError: error => addToast({ type: 'error', title: detailKind === 'region' ? t('targetObjects.regionSaveFailed') : t('targetObjects.placeSaveFailed'), message: errorMessage(error, t), duration: 0 }),
  })

  const object = detail.data
  if (detail.isLoading) return <p className="text-sm text-neutral-500">{t('targetObjects.loading')}</p>
  if (!object) return <p className="text-sm text-neutral-500">{t('targetObjects.detail.notFound')}</p>

  const permissions = new Set(object.effective_permissions ?? [])
  const ownerEdit = Boolean(me?.is_admin || me?.user_id === object.owner_user_id)
  const canArchive = Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('hide_or_archive'))
  const canEdit = Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('edit_geometry'))
  const canManageReports = Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('manage_reports'))
  const parsedGeometry = parseGeometry(object.geometry)
  const subjectKind = detailKind
  const kindLabel = detailKind === 'region'
    ? targetObjectKindLabel('region', (object as TargetRegion).region_kind, t)
    : targetObjectKindLabel('place', (object as TargetPlace).place_kind, t)

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <Link to="/objects" className="mb-3 inline-flex items-center gap-1.5 text-sm text-neutral-500 hover:text-neutral-900 dark:hover:text-neutral-100">
            <ArrowLeft size={15} />
            {t('targetObjects.title')}
          </Link>
          <h1 className="text-2xl font-semibold tracking-normal">{object.name}</h1>
          <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
            {kindLabel} · {targetObjectVisibilityLabel(object.visibility, t)}
          </p>
          {object.overlay_summary && (
            <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
              {t('targetObjects.detail.overlaySummary', { reports: object.overlay_summary.public_open_report_count, comments: object.overlay_summary.public_comment_count })}
            </p>
          )}
        </div>
        {(canEdit || (canArchive && !object.archived_at)) && (
          <div className="flex gap-2">
            {canEdit && (
              <button
                type="button"
                onClick={() => setEditing(current => !current)}
                disabled={updateMutation.isPending}
                className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
              >
                {editing ? t('targetObjects.detail.closeEdit') : t('targetObjects.detail.edit')}
              </button>
            )}
            {canArchive && !object.archived_at && (
              <button
                type="button"
                onClick={() => archiveMutation.mutate()}
                disabled={archiveMutation.isPending}
                className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
              >
                {archiveMutation.isPending ? t('targetObjects.archiving') : t('targetObjects.archive')}
              </button>
            )}
          </div>
        )}
      </div>

      {editing && parsedGeometry && (
        <PlaceRegionEditor
          mode={detailKind}
          busy={updateMutation.isPending}
          submitLabel={detailKind === 'region' ? t('targetObjects.detail.updateRegion') : t('targetObjects.detail.updatePlace')}
          showVisibility={ownerEdit}
          initialValue={{
            name: object.name,
            kind: detailKind === 'region' ? (object as TargetRegion).region_kind : (object as TargetPlace).place_kind,
            geometry: parsedGeometry,
            visibility: object.visibility,
            access_scope_id: object.access_scope_id,
          }}
          onSubmit={value => updateMutation.mutate(value)}
        />
      )}

      {object.description && (
        <p className="text-sm text-neutral-700 dark:text-neutral-300">{object.description}</p>
      )}

      <section className="space-y-2">
        <h2 className="text-lg font-semibold">{t('targetObjects.detail.geometry')}</h2>
        <pre className="overflow-auto rounded-md border border-neutral-200 bg-neutral-50 p-3 text-xs dark:border-neutral-800 dark:bg-neutral-900">
          {JSON.stringify(parsedGeometry, null, 2)}
        </pre>
      </section>

      <PermissionAudit permissions={object.effective_permissions ?? []} isOwner={Boolean(me?.user_id === object.owner_user_id)} isAdmin={Boolean(me?.is_admin)} />

      <TargetOverlayTabs
        subjectKind={subjectKind}
        subjectId={object.id}
        canWrite={Boolean(me)}
        canManageReports={canManageReports}
      />
    </div>
  )
}

function parseGeometry(geometry: TargetPlace['geometry'] | TargetRegion['geometry']): TargetGeometry | null {
  if (typeof geometry !== 'string') return geometry ?? null
  try {
    return JSON.parse(geometry)
  } catch {
    return null
  }
}

export function targetObjectKindLabel(kind: DetailKind, value: string, t: (key: string) => string): string {
  return kind === 'region' ? regionKindLabel(value, t) : placeKindLabel(value, t)
}

export function targetObjectVisibilityLabel(value: string, t: (key: string) => string): string {
  return targetVisibilityLabel(value, t)
}

export function targetObjectPermissionLabel(value: string, t: (key: string) => string): string {
  return t(TARGET_OBJECT_PERMISSION_LABEL_KEYS[value] ?? 'targetObjects.permissions.unknown')
}

function PermissionAudit({ permissions, isOwner, isAdmin }: { permissions: string[]; isOwner: boolean; isAdmin: boolean }) {
  const { t } = useTranslation()
  const permissionLabels = permissions.map(permission => targetObjectPermissionLabel(permission, t)).join(', ')

  return (
    <section className="space-y-2 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <h2 className="text-lg font-semibold">{t('targetObjects.detail.permissionStatus')}</h2>
      <p className="text-sm text-neutral-600 dark:text-neutral-300">
        {isAdmin
          ? t('targetObjects.detail.adminAccess')
          : isOwner
            ? t('targetObjects.detail.ownReference')
            : permissions.length > 0
              ? permissionLabels
              : t('targetObjects.detail.noDelegatedPermissions')}
      </p>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        {t('targetObjects.detail.permissionNote')}
      </p>
    </section>
  )
}

function errorMessage(error: unknown, t: (key: string) => string): string {
  if (error instanceof Error && error.message === 'invalidPointGeometry') return t('targetObjects.invalidPointGeometry')
  if (error instanceof Error && error.message === 'invalidRegionGeometry') return t('targetObjects.invalidRegionGeometry')
  return error instanceof Error ? error.message : t('targetObjects.tryAgain')
}
