import { useRef, useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchTags, createTag, updateTag, deleteTag } from '@/api/tags'
import type { Tag, TagCondition, TagConditionGroup } from '@/api/tags'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'

// ---------------------------------------------------------------------------
// Colour palette
// ---------------------------------------------------------------------------

const colorPalette = [
  { color: '#f97316', text: '#ffffff', label: 'Orange' },
  { color: '#3b82f6', text: '#ffffff', label: 'Blau' },
  { color: '#10b981', text: '#ffffff', label: 'Grün' },
  { color: '#8b5cf6', text: '#ffffff', label: 'Lila' },
  { color: '#f43f5e', text: '#ffffff', label: 'Rosa' },
  { color: '#06b6d4', text: '#ffffff', label: 'Cyan' },
  { color: '#eab308', text: '#1a1a1a', label: 'Gelb' },
  { color: '#6b7280', text: '#ffffff', label: 'Grau' },
]

// ---------------------------------------------------------------------------
// Condition field definitions
// ---------------------------------------------------------------------------

interface FieldDef {
  value: string
  label: string
  ops: string[]
  inputType: 'text' | 'number' | 'bool' | 'sport' | 'source' | 'season' | 'weekday' | 'time_range' | 'between' | 'tag' | 'geofence'
}

const SPORT_OPTIONS = [
  { value: 'cycling',         label: 'Radfahren' },
  { value: 'running',         label: 'Laufen' },
  { value: 'hiking',          label: 'Wandern' },
  { value: 'swimming',        label: 'Schwimmen' },
  { value: 'e_biking',        label: 'E-Bike' },
  { value: 'skiing',          label: 'Ski' },
  { value: 'paddling',        label: 'Paddeln' },
  { value: 'walking',         label: 'Gehen' },
  { value: 'strength',        label: 'Kraft' },
  { value: 'rock_climbing',   label: 'Klettern' },
  { value: 'generic',         label: 'Sonstiges' },
]

const SOURCE_OPTIONS = [
  { value: 'garmin',      label: 'Garmin' },
  { value: 'wahoo',       label: 'Wahoo' },
  { value: 'strava',      label: 'Strava' },
  { value: 'komoot',      label: 'Komoot' },
  { value: 'endomondo',   label: 'Endomondo' },
  { value: 'osmtracker',  label: 'OSMTracker' },
  { value: 'manual',      label: 'Manuell' },
]

const SEASON_OPTIONS = [
  { value: 'spring', label: 'Frühling (Mär–Mai)' },
  { value: 'summer', label: 'Sommer (Jun–Aug)' },
  { value: 'autumn', label: 'Herbst (Sep–Nov)' },
  { value: 'winter', label: 'Winter (Dez–Feb)' },
]

const WEEKDAY_OPTIONS = [
  { value: '1', label: 'Mo' },
  { value: '2', label: 'Di' },
  { value: '3', label: 'Mi' },
  { value: '4', label: 'Do' },
  { value: '5', label: 'Fr' },
  { value: '6', label: 'Sa' },
  { value: '0', label: 'So' },
]

const CONDITION_FIELDS: FieldDef[] = [
  { value: 'sport_type',         label: 'Sportart',                  ops: ['eq', 'neq', 'in'],          inputType: 'sport' },
  { value: 'distance_m',         label: 'Distanz (m)',               ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'elapsed_time_s',     label: 'Dauer (s)',                 ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'elevation_gain_m',   label: 'Höhenmeter (m)',            ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'avg_speed_ms',       label: 'Geschwindigkeit avg (m/s)', ops: ['gt', 'lt', 'gte', 'lte'],   inputType: 'number' },
  { value: 'avg_heart_rate',     label: 'Puls avg (bpm)',            ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'max_heart_rate',     label: 'Puls max (bpm)',            ops: ['gt', 'lt', 'gte', 'lte'],   inputType: 'number' },
  { value: 'avg_power_w',        label: 'Leistung avg (W)',          ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'avg_temperature_c',  label: 'Temperatur avg (°C)',       ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'min_temperature_c',  label: 'Temperatur min (°C)',       ops: ['gt', 'lt', 'gte', 'lte'],   inputType: 'number' },
  { value: 'is_indoor',          label: 'Indoor',                    ops: ['eq'],                       inputType: 'bool' },
  { value: 'has_gps',            label: 'Hat GPS',                   ops: ['eq'],                       inputType: 'bool' },
  { value: 'is_round_trip',      label: 'Rundtour',                  ops: ['eq'],                       inputType: 'bool' },
  { value: 'start_city',         label: 'Startort (Stadt)',          ops: ['eq', 'neq', 'contains'],    inputType: 'text' },
  { value: 'start_country',      label: 'Startland',                 ops: ['eq', 'neq'],                inputType: 'text' },
  { value: 'time_of_day',        label: 'Tageszeit',                 ops: ['eq', 'in'],                 inputType: 'text' },
  { value: 'time_range',         label: 'Uhrzeit-Bereich',           ops: ['between'],                  inputType: 'time_range' },
  { value: 'weekday',            label: 'Wochentag',                 ops: ['in'],                       inputType: 'weekday' },
  { value: 'season',             label: 'Jahreszeit',                ops: ['in'],                       inputType: 'season' },
  { value: 'has_tag',            label: 'Tag ist gesetzt',           ops: ['eq'],                       inputType: 'tag' },
  { value: 'source',             label: 'Gerät / Quelle',            ops: ['eq', 'neq', 'in'],          inputType: 'source' },
  { value: 'geofence',           label: 'Gebiet (Geofence)',         ops: ['eq'],                       inputType: 'geofence' },
]

const OP_LABELS: Record<string, string> = {
  eq: '=', neq: '!=', gt: '>', lt: '<', gte: '>=', lte: '<=',
  between: 'zwischen', contains: 'enthält', in: 'in',
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const inputClass = 'border border-neutral-200 dark:border-neutral-700 rounded-lg px-2.5 py-1.5 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400'

function defaultCondition(): TagCondition {
  return { field: 'avg_temperature_c', op: 'lt', value: '' }
}

function defaultGroup(): TagConditionGroup {
  return { logic: 'and', conditions: [defaultCondition()] }
}

/** Normalise tag to always work with condition_groups */
function normaliseGroups(tag?: Tag): TagConditionGroup[] {
  if (tag?.condition_groups && tag.condition_groups.length > 0) {
    return tag.condition_groups
  }
  if (tag?.conditions && tag.conditions.length > 0) {
    return [{ logic: tag.conditions_logic ?? 'and', conditions: tag.conditions }]
  }
  return []
}

/** Check if a tag has any conditions (makes it an "autotag") */
function hasConditions(groups: TagConditionGroup[]): boolean {
  return groups.some(g => g.conditions.length > 0)
}

/** Flatten groups → legacy flat for API */
function flattenGroups(groups: TagConditionGroup[]): { conditions: TagCondition[]; conditions_logic: 'and' | 'or' } {
  return {
    conditions: groups.flatMap(g => g.conditions),
    conditions_logic: groups[0]?.logic ?? 'and',
  }
}

// ---------------------------------------------------------------------------
// Geofence mini-map
// ---------------------------------------------------------------------------

interface GeofenceValue {
  lat: number
  lon: number
  radius_m: number
  mode: 'inside' | 'outside'
}

function GeofenceEditor({ value, onChange }: { value: GeofenceValue; onChange: (v: GeofenceValue) => void }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const circleSourceId = 'geofence-circle'

  function buildCircleGeoJSON(lat: number, lon: number, radiusM: number) {
    const pts = 64
    const coords: [number, number][] = []
    const R = 6371000
    const ang = radiusM / R
    const latR = (lat * Math.PI) / 180
    const lonR = (lon * Math.PI) / 180
    for (let i = 0; i <= pts; i++) {
      const b = (i * 360 / pts) * (Math.PI / 180)
      const pLat = Math.asin(Math.sin(latR) * Math.cos(ang) + Math.cos(latR) * Math.sin(ang) * Math.cos(b))
      const pLon = lonR + Math.atan2(Math.sin(b) * Math.sin(ang) * Math.cos(latR), Math.cos(ang) - Math.sin(latR) * Math.sin(pLat))
      coords.push([(pLon * 180) / Math.PI, (pLat * 180) / Math.PI])
    }
    return { type: 'Feature' as const, geometry: { type: 'Polygon' as const, coordinates: [coords] }, properties: {} }
  }

  function updateCircle(map: maplibregl.Map, lat: number, lon: number, radiusM: number) {
    const src = map.getSource(circleSourceId) as maplibregl.GeoJSONSource | undefined
    if (src) src.setData(buildCircleGeoJSON(lat, lon, radiusM) as any)
  }

  useEffect(() => {
    if (!containerRef.current) return
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: {
        version: 8,
        sources: { basemap: { type: 'raster', tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'], tileSize: 256, attribution: '&copy; OpenStreetMap' } },
        layers: [{ id: 'tiles', type: 'raster', source: 'basemap', minzoom: 0, maxzoom: 19 }],
      },
      center: [value.lon || 10.0, value.lat || 51.0],
      zoom: value.lat ? 11 : 5,
    })
    mapRef.current = map

    map.on('load', () => {
      map.addSource(circleSourceId, {
        type: 'geojson',
        data: buildCircleGeoJSON(value.lat || 51, value.lon || 10, value.radius_m || 1000) as any,
      })
      map.addLayer({ id: 'gf-fill', type: 'fill', source: circleSourceId, paint: { 'fill-color': '#f97316', 'fill-opacity': 0.2 } })
      map.addLayer({ id: 'gf-line', type: 'line', source: circleSourceId, paint: { 'line-color': '#f97316', 'line-width': 2 } })

      const marker = new maplibregl.Marker({ color: '#f97316', draggable: true })
        .setLngLat([value.lon || 10.0, value.lat || 51.0])
        .addTo(map)

      marker.on('dragend', () => {
        const { lat, lng } = marker.getLngLat()
        updateCircle(map, lat, lng, value.radius_m || 1000)
        onChange({ ...value, lat, lon: lng })
      })
      map.on('click', (e) => {
        const { lat, lng } = e.lngLat
        marker.setLngLat([lng, lat])
        updateCircle(map, lat, lng, value.radius_m || 1000)
        onChange({ ...value, lat, lon: lng })
      })
    })

    return () => { map.remove() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    updateCircle(map, value.lat || 51, value.lon || 10, value.radius_m || 1000)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value.radius_m])

  const radiusKm = value.radius_m >= 1000 ? `${(value.radius_m / 1000).toFixed(1)} km` : `${value.radius_m} m`

  return (
    <div className="space-y-2 border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 bg-neutral-50 dark:bg-neutral-800/30">
      <div className="flex items-center gap-3 flex-wrap text-sm">
        <div className="flex items-center gap-2">
          <label className="text-xs text-neutral-600 dark:text-neutral-400">Modus:</label>
          <select
            value={value.mode}
            onChange={(e) => onChange({ ...value, mode: e.target.value as 'inside' | 'outside' })}
            className={inputClass + ' py-1 text-xs'}
          >
            <option value="inside">innerhalb</option>
            <option value="outside">außerhalb</option>
          </select>
        </div>
        <div className="flex items-center gap-2 flex-1 min-w-[200px]">
          <span className="text-xs text-neutral-600 dark:text-neutral-400 whitespace-nowrap">
            Radius: <strong className="text-neutral-900 dark:text-neutral-100">{radiusKm}</strong>
          </span>
          <input
            type="range"
            min={50}
            max={100000}
            step={50}
            value={value.radius_m}
            onChange={(e) => onChange({ ...value, radius_m: Number(e.target.value) })}
            className="flex-1 accent-orange-500"
          />
        </div>
        <span className="text-xs text-neutral-400 font-mono tabular-nums">
          {value.lat ? `${value.lat.toFixed(4)}, ${value.lon.toFixed(4)}` : 'kein Punkt gesetzt'}
        </span>
      </div>
      <p className="text-xs text-neutral-400">Klicke auf die Karte oder ziehe den Pin um den Mittelpunkt zu setzen.</p>
      <div ref={containerRef} className="w-full rounded-lg overflow-hidden" style={{ height: 240 }} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Condition value input (polymorphic)
// ---------------------------------------------------------------------------

function ConditionValueInput({
  fieldDef,
  cond,
  onChange,
  allTags,
}: {
  fieldDef: FieldDef
  cond: TagCondition
  onChange: (patch: Partial<TagCondition>) => void
  allTags: Tag[]
}) {
  const { inputType } = fieldDef

  if (inputType === 'bool') {
    return (
      <select value={String(cond.value ?? 'true')} onChange={(e) => onChange({ value: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">Ja</option>
        <option value="false">Nein</option>
      </select>
    )
  }

  if (inputType === 'sport') {
    if (cond.op === 'in') {
      const sel: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
      const toggle = (v: string) => onChange({ value: sel.includes(v) ? sel.filter(x => x !== v) : [...sel, v] })
      return (
        <div className="flex flex-wrap gap-1 flex-1 min-w-0">
          {SPORT_OPTIONS.map(s => (
            <button key={s.value} type="button" onClick={() => toggle(s.value)}
              className={`px-2 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${sel.includes(s.value) ? 'bg-orange-500 text-white border-orange-500' : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-orange-400'}`}
            >{s.label}</button>
          ))}
        </div>
      )
    }
    return (
      <select value={String(cond.value ?? '')} onChange={(e) => onChange({ value: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Sportart wählen...</option>
        {SPORT_OPTIONS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
      </select>
    )
  }

  if (inputType === 'source') {
    if (cond.op === 'in') {
      const sel: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
      const toggle = (v: string) => onChange({ value: sel.includes(v) ? sel.filter(x => x !== v) : [...sel, v] })
      return (
        <div className="flex flex-wrap gap-1 flex-1 min-w-0">
          {SOURCE_OPTIONS.map(s => (
            <button key={s.value} type="button" onClick={() => toggle(s.value)}
              className={`px-2 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${sel.includes(s.value) ? 'bg-orange-500 text-white border-orange-500' : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-orange-400'}`}
            >{s.label}</button>
          ))}
        </div>
      )
    }
    return (
      <select value={String(cond.value ?? '')} onChange={(e) => onChange({ value: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Quelle wählen...</option>
        {SOURCE_OPTIONS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
      </select>
    )
  }

  if (inputType === 'season') {
    const sel: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
    const toggle = (v: string) => onChange({ value: sel.includes(v) ? sel.filter(x => x !== v) : [...sel, v] })
    return (
      <div className="flex flex-wrap gap-1 flex-1 min-w-0">
        {SEASON_OPTIONS.map(s => (
          <button key={s.value} type="button" onClick={() => toggle(s.value)}
            className={`px-2 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${sel.includes(s.value) ? 'bg-orange-500 text-white border-orange-500' : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-orange-400'}`}
          >{s.label}</button>
        ))}
      </div>
    )
  }

  if (inputType === 'weekday') {
    const sel: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
    const toggle = (v: string) => onChange({ value: sel.includes(v) ? sel.filter(x => x !== v) : [...sel, v] })
    return (
      <div className="flex flex-wrap gap-1 flex-1 min-w-0">
        {WEEKDAY_OPTIONS.map(d => (
          <button key={d.value} type="button" onClick={() => toggle(d.value)}
            className={`w-9 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${sel.includes(d.value) ? 'bg-orange-500 text-white border-orange-500' : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-orange-400'}`}
          >{d.label}</button>
        ))}
      </div>
    )
  }

  if (inputType === 'time_range') {
    const parts = typeof cond.value === 'string' ? cond.value.split(',') : ['', '']
    return (
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <input type="time" value={parts[0] ?? ''}
          onChange={(e) => onChange({ value: `${e.target.value},${parts[1] ?? ''}` })}
          className={inputClass}
        />
        <span className="text-neutral-500 text-xs">bis</span>
        <input type="time" value={parts[1] ?? ''}
          onChange={(e) => onChange({ value: `${parts[0] ?? ''},${e.target.value}` })}
          className={inputClass}
        />
      </div>
    )
  }

  if (inputType === 'between' && cond.op === 'between') {
    const parts = typeof cond.value === 'string' ? cond.value.split(',') : ['', '']
    return (
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <input type="number" placeholder="von" value={parts[0] ?? ''}
          onChange={(e) => onChange({ value: `${e.target.value},${parts[1] ?? ''}` })}
          className={inputClass + ' w-24'}
        />
        <span className="text-neutral-400 text-xs">–</span>
        <input type="number" placeholder="bis" value={parts[1] ?? ''}
          onChange={(e) => onChange({ value: `${parts[0] ?? ''},${e.target.value}` })}
          className={inputClass + ' w-24'}
        />
      </div>
    )
  }

  if (inputType === 'number' || (inputType === 'between' && cond.op !== 'between')) {
    return (
      <input type="number" value={cond.value ?? ''} onChange={(e) => onChange({ value: Number(e.target.value) })}
        placeholder="Wert" className={inputClass + ' flex-1 min-w-0'}
      />
    )
  }

  if (inputType === 'tag') {
    return (
      <select value={String(cond.value ?? '')} onChange={(e) => onChange({ value: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Tag wählen...</option>
        {allTags.map(t => <option key={t.id} value={t.name}>{t.name}</option>)}
      </select>
    )
  }

  if (inputType === 'geofence') {
    const gfVal: GeofenceValue = cond.geofence ?? { lat: 51.0, lon: 10.0, radius_m: 1000, mode: 'inside' }
    return (
      <div className="flex-1 min-w-0 w-full mt-1">
        <GeofenceEditor
          value={gfVal}
          onChange={(v) => onChange({
            geofence: v,
            value: `${v.mode}:${v.lat.toFixed(5)},${v.lon.toFixed(5)},${v.radius_m}m`,
          })}
        />
      </div>
    )
  }

  // Default: text
  return (
    <input type="text" value={cond.value ?? ''} onChange={(e) => onChange({ value: e.target.value })}
      placeholder="Wert" className={inputClass + ' flex-1 min-w-0'}
    />
  )
}

// ---------------------------------------------------------------------------
// Single condition row
// ---------------------------------------------------------------------------

function ConditionRow({
  cond,
  onUpdate,
  onRemove,
  allTags,
}: {
  cond: TagCondition
  onUpdate: (patch: Partial<TagCondition>) => void
  onRemove: () => void
  allTags: Tag[]
}) {
  const fieldDef = CONDITION_FIELDS.find(f => f.value === cond.field) ?? CONDITION_FIELDS[0]
  const isGeofence = fieldDef.inputType === 'geofence'
  const isMultiPicker = ['weekday', 'season', 'time_range'].includes(fieldDef.inputType)

  function handleFieldChange(newField: string) {
    const def = CONDITION_FIELDS.find(f => f.value === newField) ?? CONDITION_FIELDS[0]
    onUpdate({ field: newField, op: def.ops[0], value: def.inputType === 'bool' ? true : '' })
  }

  return (
    <div className={isGeofence ? 'space-y-1' : 'flex items-center gap-2 flex-wrap'}>
      <div className="flex items-center gap-2 flex-wrap">
        <select value={cond.field} onChange={(e) => handleFieldChange(e.target.value)} className={inputClass}>
          {CONDITION_FIELDS.map(f => <option key={f.value} value={f.value}>{f.label}</option>)}
        </select>

        {/* Show op selector when relevant */}
        {!isGeofence && !isMultiPicker && (
          <select value={cond.op} onChange={(e) => onUpdate({ op: e.target.value })} className={inputClass}>
            {fieldDef.ops.map(op => <option key={op} value={op}>{OP_LABELS[op] ?? op}</option>)}
          </select>
        )}

        <button
          type="button"
          onClick={onRemove}
          className="text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer flex-shrink-0"
          title="Bedingung entfernen"
        >
          <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        </button>
      </div>

      <ConditionValueInput fieldDef={fieldDef} cond={cond} onChange={onUpdate} allTags={allTags} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Condition group block
// ---------------------------------------------------------------------------

function ConditionGroupBlock({
  group,
  groupIndex,
  totalGroups,
  onChange,
  onRemove,
  allTags,
}: {
  group: TagConditionGroup
  groupIndex: number
  totalGroups: number
  onChange: (g: TagConditionGroup) => void
  onRemove: () => void
  allTags: Tag[]
}) {
  function addCondition() {
    onChange({ ...group, conditions: [...group.conditions, defaultCondition()] })
  }
  function removeCondition(i: number) {
    onChange({ ...group, conditions: group.conditions.filter((_, idx) => idx !== i) })
  }
  function updateCondition(i: number, patch: Partial<TagCondition>) {
    onChange({ ...group, conditions: group.conditions.map((c, idx) => idx === i ? { ...c, ...patch } : c) })
  }

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 space-y-2 bg-white dark:bg-neutral-900/20">
      {/* Group header */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          {totalGroups > 1 && (
            <span className="text-xs text-neutral-500 dark:text-neutral-400 font-medium">Gruppe {groupIndex + 1}</span>
          )}
          <div className="flex rounded overflow-hidden border border-neutral-200 dark:border-neutral-700">
            {(['and', 'or'] as const).map(l => (
              <button key={l} type="button"
                onClick={() => onChange({ ...group, logic: l })}
                className={`px-2 py-0.5 text-xs font-medium transition-colors cursor-pointer ${group.logic === l ? 'bg-orange-500 text-white' : 'text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'}`}
              >{l === 'and' ? 'UND' : 'ODER'}</button>
            ))}
          </div>
          <span className="text-xs text-neutral-400 dark:text-neutral-500">innerhalb Gruppe</span>
        </div>
        {totalGroups > 1 && (
          <button type="button" onClick={onRemove}
            className="text-xs text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
          >Gruppe entfernen</button>
        )}
      </div>

      {/* Conditions */}
      <div className="space-y-2">
        {group.conditions.map((cond, i) => (
          <div key={i}>
            {i > 0 && (
              <div className="my-1.5">
                <span className="text-xs font-semibold text-orange-500 uppercase tracking-wide">
                  {group.logic === 'and' ? 'UND' : 'ODER'}
                </span>
              </div>
            )}
            <ConditionRow
              cond={cond}
              onUpdate={(patch) => updateCondition(i, patch)}
              onRemove={() => removeCondition(i)}
              allTags={allTags}
            />
          </div>
        ))}
      </div>

      <button type="button" onClick={addCondition}
        className="text-xs text-orange-500 hover:text-orange-600 cursor-pointer"
      >+ Bedingung hinzufügen</button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Condition summary (read-only preview text)
// ---------------------------------------------------------------------------

function conditionSummary(groups: TagConditionGroup[]): string {
  if (!hasConditions(groups)) return ''
  return groups
    .filter(g => g.conditions.length > 0)
    .map(g => {
      const inner = g.conditions.map(c => {
        const fl = CONDITION_FIELDS.find(f => f.value === c.field)?.label ?? c.field
        if (c.field === 'geofence' && c.geofence) {
          return `${c.geofence.mode === 'inside' ? 'innerhalb' : 'außerhalb'} (${c.geofence.radius_m}m)`
        }
        const vStr = Array.isArray(c.value) ? c.value.join(', ') : String(c.value ?? '?')
        return `${fl} ${OP_LABELS[c.op] ?? c.op} ${vStr}`
      }).join(g.logic === 'and' ? ' UND ' : ' ODER ')
      return `(${inner})`
    })
    .join(' UND ')
}

// ---------------------------------------------------------------------------
// Main TagForm component
// ---------------------------------------------------------------------------

function TagForm({
  tag,
  onSave,
  onCancel,
  inline,
  allTags,
}: {
  tag?: Tag
  onSave: () => void
  onCancel: () => void
  inline?: boolean
  allTags: Tag[]
}) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(tag?.name ?? '')
  const [color, setColor] = useState(tag?.color ?? colorPalette[0].color)
  const [textColor, setTextColor] = useState(tag?.text_color ?? colorPalette[0].text)
  const [description, setDescription] = useState(tag?.description ?? '')
  const [groups, setGroups] = useState<TagConditionGroup[]>(() => normaliseGroups(tag))
  const [showConditions, setShowConditions] = useState(() => hasConditions(normaliseGroups(tag)))

  const isAutotag = hasConditions(groups)

  const createMut = useMutation({
    mutationFn: () => {
      const flat = hasConditions(groups) ? flattenGroups(groups) : { conditions: undefined, conditions_logic: undefined }
      return createTag({
        name: name.trim(),
        color,
        text_color: textColor,
        description: description.trim() || undefined,
        is_auto: isAutotag,
        condition_groups: isAutotag ? groups : undefined,
        conditions: isAutotag ? flat.conditions : undefined,
        conditions_logic: isAutotag ? flat.conditions_logic : undefined,
      })
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tags'] }); onSave() },
  })

  const updateMut = useMutation({
    mutationFn: () => {
      const flat = hasConditions(groups) ? flattenGroups(groups) : { conditions: undefined, conditions_logic: undefined }
      return updateTag(tag!.id, {
        name: name.trim(),
        color,
        text_color: textColor,
        description: description.trim() || undefined,
        is_auto: isAutotag,
        condition_groups: isAutotag ? groups : undefined,
        conditions: isAutotag ? flat.conditions : undefined,
        conditions_logic: isAutotag ? flat.conditions_logic : undefined,
      })
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tags'] }); onSave() },
  })

  const isPending = createMut.isPending || updateMut.isPending
  const errorMsg = (createMut.error ?? updateMut.error) instanceof Error
    ? (createMut.error ?? updateMut.error)?.message
    : null

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    tag ? updateMut.mutate() : createMut.mutate()
  }

  function selectColor(c: typeof colorPalette[number]) {
    setColor(c.color)
    setTextColor(c.text)
  }

  // Group management
  function addGroup() { setGroups([...groups, defaultGroup()]) }
  function updateGroup(i: number, g: TagConditionGroup) { setGroups(groups.map((old, idx) => idx === i ? g : old)) }
  function removeGroup(i: number) { setGroups(groups.filter((_, idx) => idx !== i)) }

  // Inline (table row edit) — keep simple: name + colour only
  if (inline) {
    return (
      <form onSubmit={handleSubmit} className="flex items-center gap-3 flex-wrap">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Tag-Name"
          className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400 w-40"
          required
        />
        <div className="flex items-center gap-1.5">
          {colorPalette.map((c) => (
            <button key={c.color} type="button" onClick={() => selectColor(c)}
              className="w-6 h-6 rounded-full border-2 transition-all cursor-pointer"
              style={{ backgroundColor: c.color, borderColor: color === c.color ? '#f97316' : 'transparent', transform: color === c.color ? 'scale(1.2)' : 'scale(1)' }}
              title={c.label}
            />
          ))}
        </div>
        <span className="px-2.5 py-1 rounded-full text-xs font-medium" style={{ backgroundColor: color, color: textColor }}>
          {name || 'Vorschau'}
        </span>
        <div className="flex items-center gap-2">
          <button type="submit" disabled={isPending || !name.trim()}
            className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >{tag ? 'Speichern' : 'Erstellen'}</button>
          <button type="button" onClick={onCancel}
            className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
          >Abbrechen</button>
        </div>
        {errorMsg && <p className="text-sm text-rose-500">Fehler: {errorMsg}</p>}
      </form>
    )
  }

  // Full form
  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {/* Name */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Name</label>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Tag-Name"
          className="w-full px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
          required
        />
      </div>

      {/* Description */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Beschreibung</label>
        <input
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optionale Beschreibung"
          className="w-full px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
        />
      </div>

      {/* Color */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5">Farbe</label>
        <div className="flex items-center gap-2">
          {colorPalette.map((c) => (
            <button key={c.color} type="button" onClick={() => selectColor(c)}
              className="w-6 h-6 rounded-full border-2 transition-all cursor-pointer"
              style={{ backgroundColor: c.color, borderColor: color === c.color ? '#f97316' : 'transparent', transform: color === c.color ? 'scale(1.2)' : 'scale(1)' }}
              title={c.label}
            />
          ))}
          <span className="ml-2 px-2.5 py-1 rounded-full text-xs font-medium" style={{ backgroundColor: color, color: textColor }}>
            {name || 'Vorschau'}
          </span>
          {isAutotag && (
            <span className="text-xs text-neutral-400 dark:text-neutral-500 ml-1" title="Dieser Tag wird automatisch vergeben">
              auto
            </span>
          )}
        </div>
      </div>

      {/* Automation conditions section */}
      <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl overflow-hidden">
        <button
          type="button"
          onClick={() => {
            const next = !showConditions
            setShowConditions(next)
            if (next && groups.length === 0) setGroups([defaultGroup()])
            if (!next) setGroups([])
          }}
          className="w-full flex items-center justify-between px-4 py-3 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors cursor-pointer"
        >
          <span className="flex items-center gap-2">
            <span>Automatisierungsbedingungen</span>
            {isAutotag && (
              <span className="text-xs font-normal text-neutral-400 dark:text-neutral-500">
                Tag wird automatisch vergeben
              </span>
            )}
          </span>
          <svg
            className={`w-4 h-4 transition-transform ${showConditions ? 'rotate-180' : ''}`}
            viewBox="0 0 20 20" fill="currentColor"
          >
            <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
          </svg>
        </button>

        {showConditions && (
          <div className="px-4 py-4 space-y-3 border-t border-neutral-200 dark:border-neutral-800">
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              Wenn Bedingungen gesetzt sind, wird dieser Tag automatisch vergeben — kein manuelles Setzen nötig.
              Mehrere Gruppen werden mit <strong>UND</strong> verknüpft.
            </p>

            {groups.length === 0 ? (
              <button type="button"
                onClick={() => setGroups([defaultGroup()])}
                className="text-sm text-orange-500 hover:text-orange-600 cursor-pointer"
              >
                + Erste Bedingungsgruppe hinzufügen
              </button>
            ) : (
              <>
                <div className="space-y-3">
                  {groups.map((g, i) => (
                    <div key={i}>
                      {i > 0 && (
                        <div className="flex items-center gap-2 my-2">
                          <div className="flex-1 border-t border-dashed border-neutral-300 dark:border-neutral-700" />
                          <span className="text-xs font-semibold text-neutral-500 uppercase tracking-wide px-2">UND</span>
                          <div className="flex-1 border-t border-dashed border-neutral-300 dark:border-neutral-700" />
                        </div>
                      )}
                      <ConditionGroupBlock
                        group={g}
                        groupIndex={i}
                        totalGroups={groups.length}
                        onChange={(updated) => updateGroup(i, updated)}
                        onRemove={() => removeGroup(i)}
                        allTags={allTags.filter(t => t.id !== tag?.id)} // exclude self
                      />
                    </div>
                  ))}
                </div>
                <button type="button" onClick={addGroup}
                  className="text-sm text-orange-500 hover:text-orange-600 cursor-pointer"
                >+ Weitere Gruppe hinzufügen</button>

                {/* Summary preview */}
                {isAutotag && (
                  <div className="text-xs text-neutral-500 dark:text-neutral-400 bg-neutral-50 dark:bg-neutral-800/50 rounded-lg px-3 py-2 leading-relaxed">
                    <span className="font-medium text-neutral-700 dark:text-neutral-300">Vorschau: </span>
                    Tag wird vergeben wenn {conditionSummary(groups)}
                  </div>
                )}
              </>
            )}
          </div>
        )}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2 pt-1">
        <button type="submit" disabled={isPending || !name.trim()}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >{tag ? 'Speichern' : 'Erstellen'}</button>
        <button type="button" onClick={onCancel}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
        >Abbrechen</button>
      </div>

      {errorMsg && <p className="text-sm text-rose-500">Fehler: {errorMsg}</p>}
    </form>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function TagsPage() {
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)

  const { data: tags, isLoading } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  const deleteMut = useMutation({
    mutationFn: deleteTag,
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tags'] }); setDeleteConfirmId(null) },
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Tags...</div>
  }

  const allTags = tags ?? []

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link
            to="/workshop"
            className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
          >
            &#8592; Werkstatt
          </Link>
          <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Tags</h1>
        </div>
        <button
          onClick={() => { setShowForm(true); setEditingId(null) }}
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 transition-colors cursor-pointer"
        >
          Tag erstellen
        </button>
      </div>

      {/* Create form */}
      {showForm && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Neuer Tag</h2>
          <TagForm
            allTags={allTags}
            onSave={() => { setShowForm(false); setEditingId(null) }}
            onCancel={() => { setShowForm(false); setEditingId(null) }}
          />
        </div>
      )}

      {/* Tags list */}
      {allTags.length > 0 ? (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-neutral-50 dark:bg-neutral-900/50 border-b border-neutral-200 dark:border-neutral-800">
                <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Tag</th>
                <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Kategorie</th>
                <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Aktivitäten</th>
                <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Aktionen</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
              {allTags.map((tag) => (
                <tr key={tag.id} className="even:bg-neutral-50/50 dark:even:bg-neutral-800/20">
                  <td className="px-4 py-3">
                    {editingId === tag.id ? (
                      <TagForm
                        tag={tag}
                        allTags={allTags}
                        onSave={() => setEditingId(null)}
                        onCancel={() => setEditingId(null)}
                        inline
                      />
                    ) : (
                      <div className="flex items-center gap-2">
                        <span
                          className="inline-block px-2.5 py-1 rounded-full text-xs font-medium"
                          style={{ backgroundColor: tag.color || '#f3f4f6', color: tag.text_color || '#374151' }}
                        >
                          {tag.name}
                        </span>
                        {tag.is_auto && (
                          <span
                            className="text-xs text-neutral-400 dark:text-neutral-500"
                            title="Automatisch vergeben (Bedingungen gesetzt)"
                          >
                            auto
                          </span>
                        )}
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-3 text-neutral-500 dark:text-neutral-400">
                    {editingId === tag.id ? null : (tag.category || '\u2014')}
                  </td>
                  <td className="px-4 py-3 text-right text-neutral-700 dark:text-neutral-300 tabular-nums">
                    {editingId === tag.id ? null : tag.activity_count}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {editingId !== tag.id && (
                      <div className="flex items-center justify-end gap-2">
                        {tag.is_system && !tag.is_auto ? (
                          <span className="text-xs text-neutral-400">System</span>
                        ) : (
                          <>
                            <button
                              onClick={() => {
                                setEditingId(null)
                                setShowForm(false)
                                // Open full-form editor below the list for non-inline
                                setEditingId(tag.id)
                              }}
                              className="text-xs text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100 cursor-pointer"
                            >
                              Bearbeiten
                            </button>
                            {!tag.is_system && (
                              <>
                                {deleteConfirmId === tag.id ? (
                                  <span className="flex items-center gap-1">
                                    <button
                                      onClick={() => deleteMut.mutate(tag.id)}
                                      className="text-xs text-rose-600 hover:text-rose-800 dark:text-rose-400 dark:hover:text-rose-300 font-medium cursor-pointer"
                                    >
                                      Ja, löschen
                                    </button>
                                    <button
                                      onClick={() => setDeleteConfirmId(null)}
                                      className="text-xs text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100 cursor-pointer"
                                    >
                                      Abbrechen
                                    </button>
                                  </span>
                                ) : (
                                  <button
                                    onClick={() => setDeleteConfirmId(tag.id)}
                                    className="text-xs text-rose-500 hover:text-rose-700 dark:text-rose-400 dark:hover:text-rose-300 cursor-pointer"
                                  >
                                    Löschen
                                  </button>
                                )}
                              </>
                            )}
                          </>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="text-center py-16 text-neutral-400">
          Noch keine Tags vorhanden.
        </div>
      )}

      {/* Full edit form below table (for non-inline edit, i.e. autotags/system-autotags) */}
      {editingId !== null && allTags.find(t => t.id === editingId) && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">
            Tag bearbeiten: <span style={{ color: allTags.find(t => t.id === editingId)?.color }}>
              {allTags.find(t => t.id === editingId)?.name}
            </span>
          </h2>
          <TagForm
            tag={allTags.find(t => t.id === editingId)!}
            allTags={allTags}
            onSave={() => setEditingId(null)}
            onCancel={() => setEditingId(null)}
          />
        </div>
      )}
    </div>
  )
}
