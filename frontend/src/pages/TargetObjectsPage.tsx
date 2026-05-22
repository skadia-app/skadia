import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getMe } from '@/api/auth'
import { useToast } from '@/lib/toast'
import {
  createTargetPlace,
  createTargetRegion,
  deleteTargetPlace,
  deleteTargetRegion,
  listTargetPlaces,
  listTargetRegions,
  type MultiPolygonGeometry,
  type PointGeometry,
  type PolygonGeometry,
  type TargetPlaceKind,
  type TargetRegionKind,
  type TargetPlace,
  type TargetRegion,
} from '@/api/targetObjects'
import PlaceRegionEditor, { type PlaceRegionEditorValue } from '@/components/target-objects/PlaceRegionEditor'

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

export default function TargetObjectsPage() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [mode, setMode] = useState<'place' | 'region'>('place')
  const [search, setSearch] = useState('')
  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 60_000 })
  const places = useQuery({ queryKey: ['target-places', search], queryFn: () => listTargetPlaces({ search, limit: 100 }) })
  const regions = useQuery({ queryKey: ['target-regions', search], queryFn: () => listTargetRegions({ search, limit: 100 }) })

  const createPlaceMutation = useMutation({
    mutationFn: (value: PlaceRegionEditorValue) => createTargetPlace({
      name: value.name,
      place_kind: value.kind as TargetPlaceKind,
      geometry: asPointGeometry(value),
      visibility: value.visibility,
      access_scope_id: value.access_scope_id,
      origin_key: 'manual',
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-places'] })
      addToast({ type: 'success', title: 'Ort gespeichert' })
    },
    onError: error => addToast({ type: 'error', title: 'Ort konnte nicht gespeichert werden', message: errorMessage(error), duration: 0 }),
  })
  const createRegionMutation = useMutation({
    mutationFn: (value: PlaceRegionEditorValue) => createTargetRegion({
      name: value.name,
      region_kind: value.kind as TargetRegionKind,
      geometry: asRegionGeometry(value),
      visibility: value.visibility,
      access_scope_id: value.access_scope_id,
      origin_key: 'manual',
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-regions'] })
      addToast({ type: 'success', title: 'Bereich gespeichert' })
    },
    onError: error => addToast({ type: 'error', title: 'Bereich konnte nicht gespeichert werden', message: errorMessage(error), duration: 0 }),
  })
  const archivePlaceMutation = useMutation({
    mutationFn: deleteTargetPlace,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-places'] })
      addToast({ type: 'success', title: 'Ort archiviert' })
    },
    onError: error => addToast({ type: 'error', title: 'Ort konnte nicht archiviert werden', message: errorMessage(error), duration: 0 }),
  })
  const archiveRegionMutation = useMutation({
    mutationFn: deleteTargetRegion,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['target-regions'] })
      addToast({ type: 'success', title: 'Bereich archiviert' })
    },
    onError: error => addToast({ type: 'error', title: 'Bereich konnte nicht archiviert werden', message: errorMessage(error), duration: 0 }),
  })

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-normal">Orte & Bereiche</h1>
          <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">Punkte und Flächen für Karten, Pools und spätere Governance.</p>
        </div>
        <input
          value={search}
          onChange={event => setSearch(event.target.value)}
          placeholder="Suchen"
          className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900 sm:w-72"
        />
      </div>

      <div className="flex gap-2">
        <button className={tabClass(mode === 'place')} onClick={() => setMode('place')}>Ort</button>
        <button className={tabClass(mode === 'region')} onClick={() => setMode('region')}>Bereich</button>
      </div>

      <PlaceRegionEditor
        key={mode}
        mode={mode}
        busy={createPlaceMutation.isPending || createRegionMutation.isPending}
        onSubmit={(value) => {
          if (value.mode === 'place') createPlaceMutation.mutate(value)
          else createRegionMutation.mutate(value)
        }}
      />

      <section className="grid gap-4 lg:grid-cols-2">
        <ObjectList title="Orte" loading={places.isLoading}>
          {(places.data?.places ?? []).map(place => (
            <ObjectRow
              key={place.id}
              href={`/objects/place/${place.id}`}
              name={place.name}
              kind={PLACE_KIND_LABELS[place.place_kind] ?? place.place_kind}
              visibility={VISIBILITY_LABELS[place.visibility] ?? place.visibility}
              canArchive={canArchiveObject(place, me)}
              archiveBusy={archivePlaceMutation.isPending}
              onArchive={() => archivePlaceMutation.mutate(place.id)}
            />
          ))}
        </ObjectList>
        <ObjectList title="Bereiche" loading={regions.isLoading}>
          {(regions.data?.regions ?? []).map(region => (
            <ObjectRow
              key={region.id}
              href={`/objects/region/${region.id}`}
              name={region.name}
              kind={REGION_KIND_LABELS[region.region_kind] ?? region.region_kind}
              visibility={VISIBILITY_LABELS[region.visibility] ?? region.visibility}
              canArchive={canArchiveObject(region, me)}
              archiveBusy={archiveRegionMutation.isPending}
              onArchive={() => archiveRegionMutation.mutate(region.id)}
            />
          ))}
        </ObjectList>
      </section>
    </div>
  )
}

function ObjectList({ title, loading, children }: { title: string; loading: boolean; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      {loading ? <p className="text-sm text-neutral-500">Lädt...</p> : <div className="divide-y divide-neutral-200 rounded-lg border border-neutral-200 dark:divide-neutral-800 dark:border-neutral-800">{children}</div>}
    </section>
  )
}

function ObjectRow({ href, name, kind, visibility, canArchive, archiveBusy, onArchive }: { href: string; name: string; kind: string; visibility: string; canArchive: boolean; archiveBusy: boolean; onArchive: () => void }) {
  return (
    <div className="flex items-center justify-between gap-3 p-3">
      <Link to={href} className="min-w-0 hover:text-neutral-900 dark:hover:text-neutral-100">
        <p className="truncate text-sm font-medium">{name}</p>
        <p className="text-xs text-neutral-500 dark:text-neutral-400">{kind} · {visibility}</p>
      </Link>
      {canArchive && (
        <button
          onClick={onArchive}
          disabled={archiveBusy}
          className="rounded-md border border-neutral-300 px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
        >
          {archiveBusy ? 'Archiviert...' : 'Archivieren'}
        </button>
      )}
    </div>
  )
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Bitte versuche es erneut.'
}

function canArchiveObject(object: TargetPlace | TargetRegion, me?: { user_id?: string; is_admin?: boolean } | null): boolean {
  const permissions = new Set(object.effective_permissions ?? [])
  return Boolean(me?.is_admin || me?.user_id === object.owner_user_id || permissions.has('hide_or_archive'))
}

function tabClass(active: boolean) {
  return active
    ? 'rounded-md bg-neutral-900 px-3 py-2 text-sm font-medium text-white dark:bg-neutral-100 dark:text-neutral-900'
    : 'rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium dark:border-neutral-700'
}
