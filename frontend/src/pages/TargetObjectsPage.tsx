import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createTargetPlace,
  createTargetRegion,
  deleteTargetPlace,
  deleteTargetRegion,
  listTargetPlaces,
  listTargetRegions,
  type TargetPlaceKind,
  type TargetRegionKind,
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

const VISIBILITY_LABELS: Record<string, string> = {
  private: 'Privat',
  shared: 'Geteilt',
  public: 'Öffentlich',
}

export default function TargetObjectsPage() {
  const queryClient = useQueryClient()
  const [mode, setMode] = useState<'place' | 'region'>('place')
  const [search, setSearch] = useState('')
  const places = useQuery({ queryKey: ['target-places', search], queryFn: () => listTargetPlaces({ search, limit: 100 }) })
  const regions = useQuery({ queryKey: ['target-regions', search], queryFn: () => listTargetRegions({ search, limit: 100 }) })

  const createPlaceMutation = useMutation({
    mutationFn: (value: PlaceRegionEditorValue) => createTargetPlace({
      name: value.name,
      place_kind: value.kind as TargetPlaceKind,
      geometry: value.geometry,
      visibility: value.visibility,
      access_scope_id: value.access_scope_id,
      origin_key: 'manual',
    }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['target-places'] }),
  })
  const createRegionMutation = useMutation({
    mutationFn: (value: PlaceRegionEditorValue) => createTargetRegion({
      name: value.name,
      region_kind: value.kind as TargetRegionKind,
      geometry: value.geometry as Extract<typeof value.geometry, { type: 'Polygon' | 'MultiPolygon' }>,
      visibility: value.visibility,
      access_scope_id: value.access_scope_id,
      origin_key: 'manual',
    }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['target-regions'] }),
  })
  const archivePlaceMutation = useMutation({
    mutationFn: deleteTargetPlace,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['target-places'] }),
  })
  const archiveRegionMutation = useMutation({
    mutationFn: deleteTargetRegion,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['target-regions'] }),
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
              name={place.name}
              kind={PLACE_KIND_LABELS[place.place_kind] ?? place.place_kind}
              visibility={VISIBILITY_LABELS[place.visibility] ?? place.visibility}
              onArchive={() => archivePlaceMutation.mutate(place.id)}
            />
          ))}
        </ObjectList>
        <ObjectList title="Bereiche" loading={regions.isLoading}>
          {(regions.data?.regions ?? []).map(region => (
            <ObjectRow
              key={region.id}
              name={region.name}
              kind={REGION_KIND_LABELS[region.region_kind] ?? region.region_kind}
              visibility={VISIBILITY_LABELS[region.visibility] ?? region.visibility}
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

function ObjectRow({ name, kind, visibility, onArchive }: { name: string; kind: string; visibility: string; onArchive: () => void }) {
  return (
    <div className="flex items-center justify-between gap-3 p-3">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{name}</p>
        <p className="text-xs text-neutral-500 dark:text-neutral-400">{kind} · {visibility}</p>
      </div>
      <button onClick={onArchive} className="rounded-md border border-neutral-300 px-3 py-1.5 text-sm dark:border-neutral-700">Archivieren</button>
    </div>
  )
}

function tabClass(active: boolean) {
  return active
    ? 'rounded-md bg-neutral-900 px-3 py-2 text-sm font-medium text-white dark:bg-neutral-100 dark:text-neutral-900'
    : 'rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium dark:border-neutral-700'
}
