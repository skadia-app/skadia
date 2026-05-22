import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { getMe } from '@/api/auth'
import {
  deleteTargetPlace,
  deleteTargetRegion,
  getTargetPlace,
  getTargetRegion,
  type TargetPlace,
  type TargetRegion,
} from '@/api/targetObjects'
import TargetOverlayTabs from '@/components/target-overlays/TargetOverlayTabs'

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
    },
  })

  const object = detail.data
  if (detail.isLoading) return <p className="text-sm text-neutral-500">Lädt...</p>
  if (!object) return <p className="text-sm text-neutral-500">Objekt nicht gefunden.</p>

  const permissions = new Set(object.effective_permissions ?? [])
  const canArchive = Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('hide_or_archive'))
  const canManageReports = Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('manage_reports'))
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
        {canArchive && !object.archived_at && (
          <button
            type="button"
            onClick={() => archiveMutation.mutate()}
            disabled={archiveMutation.isPending}
            className="rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
          >
            Archivieren
          </button>
        )}
      </div>

      {object.description && (
        <p className="text-sm text-neutral-700 dark:text-neutral-300">{object.description}</p>
      )}

      <section className="space-y-2">
        <h2 className="text-lg font-semibold">Geometrie</h2>
        <pre className="overflow-auto rounded-md border border-neutral-200 bg-neutral-50 p-3 text-xs dark:border-neutral-800 dark:bg-neutral-900">
          {JSON.stringify(parseGeometry(object.geometry), null, 2)}
        </pre>
      </section>

      <TargetOverlayTabs
        subjectKind={subjectKind}
        subjectId={object.id}
        canWrite={Boolean(me)}
        canManageReports={canManageReports}
      />
    </div>
  )
}

function parseGeometry(geometry: TargetPlace['geometry'] | TargetRegion['geometry']) {
  if (typeof geometry !== 'string') return geometry ?? null
  try {
    return JSON.parse(geometry)
  } catch {
    return geometry
  }
}
