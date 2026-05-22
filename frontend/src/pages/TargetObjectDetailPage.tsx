import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
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
import TargetOverlayTabs from '@/components/target-overlays/TargetOverlayTabs'
import { useToast } from '@/lib/toast'

const PLACE_KIND_LABELS: Record<string, string> = {
  custom: 'Eigener Punkt',
  private_place: 'Privater Ort',
  poi: 'POI',
  entrance: 'Einstieg',
  parking: 'Parkplatz',
  summit: 'Gipfel',
  pass: 'Pass',
  water: 'Gewässer',
  settlement: 'Ort',
  amenity: 'Infrastruktur',
}

const REGION_KIND_LABELS: Record<string, string> = {
  custom: 'Eigener Bereich',
  private_area: 'Privater Bereich',
  public_pool: 'Öffentlicher Pool',
  venue_area: 'Gelände',
  privacy_zone_mirror: 'Privacy-Zonen-Referenz',
  admin_area: 'Admin-Bereich',
  protected_area: 'Schutzgebiet',
}

function asPointGeometry(value: PlaceRegionEditorValue): PointGeometry {
  if (value.geometry.type !== 'Point') {
    throw new Error('Orte benötigen eine Punkt-Geometrie.')
  }
  return value.geometry
}

function asRegionGeometry(value: PlaceRegionEditorValue): PolygonGeometry | MultiPolygonGeometry {
  if (value.geometry.type !== 'Polygon' && value.geometry.type !== 'MultiPolygon') {
    throw new Error('Bereiche benötigen eine Polygon- oder MultiPolygon-Geometrie.')
  }
  return value.geometry
}

const VISIBILITY_LABELS: Record<string, string> = {
  private: 'Privat',
  shared: 'Geteilt',
  public: 'Öffentlich',
}

type DetailKind = 'place' | 'region'

export default function TargetObjectDetailPage() {
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
      addToast({ type: 'success', title: detailKind === 'region' ? 'Bereich archiviert' : 'Ort archiviert' })
    },
    onError: error => addToast({ type: 'error', title: detailKind === 'region' ? 'Bereich konnte nicht archiviert werden' : 'Ort konnte nicht archiviert werden', message: errorMessage(error), duration: 0 }),
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
      addToast({ type: 'success', title: detailKind === 'region' ? 'Bereich gespeichert' : 'Ort gespeichert' })
    },
    onError: error => addToast({ type: 'error', title: detailKind === 'region' ? 'Bereich konnte nicht gespeichert werden' : 'Ort konnte nicht gespeichert werden', message: errorMessage(error), duration: 0 }),
  })

  const object = detail.data
  if (detail.isLoading) return <p className="text-sm text-neutral-500">Lädt...</p>
  if (!object) return <p className="text-sm text-neutral-500">Objekt nicht gefunden.</p>

  const permissions = new Set(object.effective_permissions ?? [])
  const ownerEdit = Boolean(me?.is_admin || me?.user_id === object.owner_user_id)
  const canArchive = Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('hide_or_archive'))
  const canEdit = Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('edit_geometry'))
  const canManageReports = Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('manage_reports'))
  const parsedGeometry = parseGeometry(object.geometry)
  const subjectKind = detailKind
  const kindLabel = detailKind === 'region'
    ? REGION_KIND_LABELS[(object as TargetRegion).region_kind] ?? (object as TargetRegion).region_kind
    : PLACE_KIND_LABELS[(object as TargetPlace).place_kind] ?? (object as TargetPlace).place_kind

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <Link to="/objects" className="mb-3 inline-flex items-center gap-1.5 text-sm text-neutral-500 hover:text-neutral-900 dark:hover:text-neutral-100">
            <ArrowLeft size={15} />
            Orte & Bereiche
          </Link>
          <h1 className="text-2xl font-semibold tracking-normal">{object.name}</h1>
          <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
            {kindLabel} · {VISIBILITY_LABELS[object.visibility] ?? object.visibility}
          </p>
          {object.overlay_summary && (
            <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
              {object.overlay_summary.public_open_report_count} offene Meldungen · {object.overlay_summary.public_comment_count} Kommentare
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
                {editing ? 'Bearbeiten schließen' : 'Bearbeiten'}
              </button>
            )}
            {canArchive && !object.archived_at && (
              <button
                type="button"
                onClick={() => archiveMutation.mutate()}
                disabled={archiveMutation.isPending}
                className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
              >
                {archiveMutation.isPending ? 'Archiviert...' : 'Archivieren'}
              </button>
            )}
          </div>
        )}
      </div>

      {editing && parsedGeometry && (
        <PlaceRegionEditor
          mode={detailKind}
          busy={updateMutation.isPending}
          submitLabel={detailKind === 'region' ? 'Bereich aktualisieren' : 'Ort aktualisieren'}
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
        <h2 className="text-lg font-semibold">Geometrie</h2>
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

function PermissionAudit({ permissions, isOwner, isAdmin }: { permissions: string[]; isOwner: boolean; isAdmin: boolean }) {
  return (
    <section className="space-y-2 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <h2 className="text-lg font-semibold">Berechtigungsstatus</h2>
      <p className="text-sm text-neutral-600 dark:text-neutral-300">
        {isAdmin
          ? 'Admin-Zugriff'
          : isOwner
            ? 'Eigene Referenz'
            : permissions.length > 0
              ? permissions.join(', ')
              : 'Keine delegierten Rechte in der Detailantwort'}
      </p>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        Governance-Verwaltung ist hier noch nicht vollständig verfügbar; angezeigt werden nur die effektiven Rechte aus der Detailantwort.
      </p>
    </section>
  )
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Bitte versuche es erneut.'
}
