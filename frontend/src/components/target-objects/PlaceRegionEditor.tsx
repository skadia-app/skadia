import { useMemo, useState } from 'react'
import VisibilityAccessField from './VisibilityAccessField'
import type { TargetGeometry, TargetPlaceKind, TargetRegionKind, TargetVisibility } from '@/api/targetObjects'

type EditorMode = 'place' | 'region'

export interface PlaceRegionEditorValue {
  mode: EditorMode
  name: string
  kind: TargetPlaceKind | TargetRegionKind
  geometry: TargetGeometry
  visibility: TargetVisibility
  access_scope_id?: string | null
}

interface PlaceRegionEditorProps {
  mode: EditorMode
  onSubmit: (value: PlaceRegionEditorValue) => void
  initialValue?: Partial<Omit<PlaceRegionEditorValue, 'mode'>>
  busy?: boolean
  submitLabel?: string
  showVisibility?: boolean
}

const PLACE_KIND_OPTIONS: { value: TargetPlaceKind; label: string }[] = [
  { value: 'custom', label: 'Eigener Punkt' },
  { value: 'private_place', label: 'Privater Ort' },
  { value: 'poi', label: 'POI' },
  { value: 'entrance', label: 'Einstieg' },
  { value: 'parking', label: 'Parkplatz' },
  { value: 'summit', label: 'Gipfel' },
  { value: 'pass', label: 'Pass' },
  { value: 'water', label: 'Gewässer' },
  { value: 'settlement', label: 'Ort' },
  { value: 'amenity', label: 'Infrastruktur' },
]

const REGION_KIND_OPTIONS: { value: TargetRegionKind; label: string }[] = [
  { value: 'custom', label: 'Eigener Bereich' },
  { value: 'private_area', label: 'Privater Bereich' },
  { value: 'public_pool', label: 'Öffentlicher Pool' },
  { value: 'venue_area', label: 'Gelände' },
  { value: 'privacy_zone_mirror', label: 'Privacy-Zonen-Referenz' },
  { value: 'admin_area', label: 'Admin-Bereich' },
  { value: 'protected_area', label: 'Schutzgebiet' },
]

const DEFAULT_POINT = '{ "type": "Point", "coordinates": [10.0, 47.0] }'
const DEFAULT_POLYGON = '{ "type": "Polygon", "coordinates": [[[10.0,47.0],[10.01,47.0],[10.01,47.01],[10.0,47.01],[10.0,47.0]]] }'

export default function PlaceRegionEditor({ mode, onSubmit, initialValue, busy = false, submitLabel, showVisibility = true }: PlaceRegionEditorProps) {
  const [name, setName] = useState(initialValue?.name ?? '')
  const [kind, setKind] = useState<TargetPlaceKind | TargetRegionKind>(initialValue?.kind ?? 'custom')
  const [geometryText, setGeometryText] = useState(() => {
    if (initialValue?.geometry) return JSON.stringify(initialValue.geometry, null, 2)
    return mode === 'place' ? DEFAULT_POINT : DEFAULT_POLYGON
  })
  const [visibility, setVisibility] = useState<TargetVisibility>(initialValue?.visibility ?? 'private')
  const [accessScopeId, setAccessScopeId] = useState<string | null>(initialValue?.access_scope_id ?? null)
  const [error, setError] = useState<string | null>(null)

  const kindOptions = useMemo(() => mode === 'place' ? PLACE_KIND_OPTIONS : REGION_KIND_OPTIONS, [mode])

  return (
    <form
      className="space-y-4 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800"
      onSubmit={(event) => {
        event.preventDefault()
        setError(null)
        let geometry: TargetGeometry
        try {
          geometry = JSON.parse(geometryText) as TargetGeometry
        } catch {
          setError('Geometrie ist kein gültiges JSON.')
          return
        }
        const normalized = normalizeGeometryForMode(mode, geometry)
        if (typeof normalized === 'string') {
          setError(normalized)
          return
        }
        onSubmit({ mode, name, kind, geometry: normalized, visibility, access_scope_id: accessScopeId })
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="block">
          <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">Name</span>
          <input
            value={name}
            onChange={event => setName(event.target.value)}
            required
            className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">Typ</span>
          <select
            value={kind}
            onChange={event => setKind(event.target.value as TargetPlaceKind | TargetRegionKind)}
            className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900"
          >
            {kindOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
          </select>
        </label>
      </div>

      {showVisibility && (
        <VisibilityAccessField
          visibility={visibility}
          accessScopeId={accessScopeId}
          onChange={(next) => {
            setVisibility(next.visibility)
            setAccessScopeId(next.access_scope_id ?? null)
          }}
        />
      )}

      <label className="block">
        <span className="mb-1 block text-sm font-medium text-neutral-700 dark:text-neutral-200">Geometrie</span>
        <textarea
          value={geometryText}
          onChange={event => setGeometryText(event.target.value)}
          rows={mode === 'place' ? 3 : 5}
          className="w-full rounded-md border border-neutral-300 bg-white px-3 py-2 font-mono text-xs dark:border-neutral-700 dark:bg-neutral-900"
        />
      </label>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      <button
        type="submit"
        disabled={busy || (showVisibility && visibility === 'shared' && !accessScopeId)}
        className="rounded-md bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? 'Speichert...' : (submitLabel ?? (mode === 'place' ? 'Ort speichern' : 'Bereich speichern'))}
      </button>
    </form>
  )
}

function normalizeGeometryForMode(mode: EditorMode, geometry: TargetGeometry): TargetGeometry | string {
  if (mode === 'place') {
    if (geometry?.type !== 'Point') return 'Orte müssen als GeoJSON Point gespeichert werden.'
    return geometry
  }
  if (geometry?.type === 'MultiPolygon') return geometry
  if (geometry?.type === 'Polygon') {
    return { type: 'MultiPolygon', coordinates: [geometry.coordinates] }
  }
  return 'Bereiche müssen als GeoJSON Polygon oder MultiPolygon gespeichert werden.'
}
