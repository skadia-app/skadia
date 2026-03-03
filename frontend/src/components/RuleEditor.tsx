import { useRef, useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchGear } from '@/api/gear'
import { fetchTags } from '@/api/tags'
import type { AutomationRule, RuleCondition, RuleConditionGroup, RuleAction } from '@/api/types'
import { SPORT_DEFS } from '@/lib/sportTaxonomy'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const SPORT_OPTIONS = SPORT_DEFS.map(d => ({
  value: d.sportType,
  label: d.label,
  category: d.categoryKey,
}))



const SOURCE_OPTIONS = [
  { value: 'garmin', label: 'Garmin' },
  { value: 'wahoo', label: 'Wahoo' },
  { value: 'strava', label: 'Strava' },
  { value: 'komoot', label: 'Komoot' },
  { value: 'endomondo', label: 'Endomondo' },
  { value: 'osmtracker', label: 'OSMTracker' },
  { value: 'manual', label: 'Manuell' },
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

// Field definitions: each entry tells the editor how to render the value input
interface FieldDef {
  value: string
  label: string
  ops: string[]
  inputType: 'text' | 'number' | 'bool' | 'sport' | 'source' | 'season' | 'weekday' | 'time_range' | 'between' | 'tag' | 'geofence'
}

const CONDITION_FIELDS: FieldDef[] = [
  // Activity properties
  { value: 'sport_type',         label: 'Sportart',                ops: ['eq', 'neq', 'in'],         inputType: 'sport' },
  { value: 'distance_m',         label: 'Distanz (m)',             ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'elapsed_time_s',     label: 'Dauer (s)',               ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'elevation_gain_m',   label: 'Höhenmeter (m)',          ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'avg_speed_ms',       label: 'Geschwindigkeit avg (m/s)',ops: ['gt', 'lt', 'gte', 'lte'], inputType: 'number' },
  { value: 'avg_heart_rate',     label: 'Puls avg (bpm)',          ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'max_heart_rate',     label: 'Puls max (bpm)',          ops: ['gt', 'lt', 'gte', 'lte'], inputType: 'number' },
  { value: 'avg_power_w',        label: 'Leistung avg (W)',        ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'avg_temperature_c',  label: 'Temperatur avg (°C)',     ops: ['gt', 'lt', 'gte', 'lte', 'between'], inputType: 'between' },
  { value: 'min_temperature_c',  label: 'Temperatur min (°C)',     ops: ['gt', 'lt', 'gte', 'lte'], inputType: 'number' },
  // Boolean
  { value: 'is_indoor',          label: 'Indoor',                  ops: ['eq'],                      inputType: 'bool' },
  { value: 'has_gps',            label: 'Hat GPS',                 ops: ['eq'],                      inputType: 'bool' },
  { value: 'is_round_trip',      label: 'Rundtour',                ops: ['eq'],                      inputType: 'bool' },
  // Location
  { value: 'start_city',         label: 'Startort (Stadt)',        ops: ['eq', 'neq', 'contains'],   inputType: 'text' },
  { value: 'start_country',      label: 'Startland',               ops: ['eq', 'neq'],               inputType: 'text' },
  // Time/date
  { value: 'time_of_day',        label: 'Tageszeit',               ops: ['eq', 'in'],                inputType: 'text' },
  { value: 'time_range',         label: 'Uhrzeit-Bereich',         ops: ['between'],                 inputType: 'time_range' },
  { value: 'weekday',            label: 'Wochentag',               ops: ['in'],                      inputType: 'weekday' },
  { value: 'season',             label: 'Jahreszeit',              ops: ['in'],                      inputType: 'season' },
  // Tag-based
  { value: 'has_tag',            label: 'Tag ist gesetzt',         ops: ['eq'],                      inputType: 'tag' },
  // Source
  { value: 'source',             label: 'Gerät / Quelle',          ops: ['eq', 'neq', 'in'],         inputType: 'source' },
  // Geofence
  { value: 'geofence',           label: 'Gebiet (Geofence)',       ops: ['eq'],                      inputType: 'geofence' },
]

const OP_LABELS: Record<string, string> = {
  eq: '=', neq: '!=', gt: '>', lt: '<', gte: '>=', lte: '<=',
  between: 'zwischen', contains: 'enthält', in: 'in',
}

const ACTION_TYPES = [
  { value: 'set_gear',        label: 'Ausrüstung zuweisen' },
  { value: 'add_tag',         label: 'Tag hinzufügen' },
  { value: 'set_visibility',  label: 'Sichtbarkeit setzen' },
]

const VISIBILITY_OPTIONS = [
  { value: 'private',    label: 'Privat' },
  { value: 'bekannte',   label: 'Bekannte' },
  { value: 'instanz',    label: 'Instanz' },
  { value: 'federation', label: 'Federation' },
  { value: 'public',     label: 'Öffentlich' },
]

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

function defaultCondition(): RuleCondition {
  return { field: 'sport_type', op: 'eq', value: '' }
}

function defaultGroup(): RuleConditionGroup {
  return { logic: 'and', conditions: [defaultCondition()] }
}

/** Normalise a rule to always have condition_groups (migrate flat conditions if needed) */
function normaliseGroups(rule?: AutomationRule | null): RuleConditionGroup[] {
  if (rule?.condition_groups && rule.condition_groups.length > 0) {
    return rule.condition_groups
  }
  if (rule?.conditions && rule.conditions.length > 0) {
    return [{ logic: rule.conditions_logic ?? 'and' as 'and' | 'or', conditions: rule.conditions }]
  }
  return [defaultGroup()]
}

/** Flatten groups to a single condition list + logic for legacy API compat */
function flattenGroups(groups: RuleConditionGroup[]): { conditions: RuleCondition[]; conditions_logic: 'and' | 'or' } {
  const all: RuleCondition[] = groups.flatMap(g => g.conditions)
  return { conditions: all, conditions_logic: groups[0]?.logic ?? 'and' }
}

// ---------------------------------------------------------------------------
// Small UI bits
// ---------------------------------------------------------------------------

const inputClass = 'border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent'

// ---------------------------------------------------------------------------
// Geofence mini-map
// ---------------------------------------------------------------------------

interface GeofenceValue {
  lat: number
  lon: number
  radius_m: number
  mode: 'inside' | 'outside'
}

function GeofenceEditor({
  value,
  onChange,
}: {
  value: GeofenceValue
  onChange: (v: GeofenceValue) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const circleSourceId = 'geofence-circle'

  // Build a GeoJSON circle polygon from lat/lon/radius
  function buildCircleGeoJSON(lat: number, lon: number, radiusM: number) {
    const points = 64
    const coords: [number, number][] = []
    const earthRadius = 6371000
    const angularDist = radiusM / earthRadius
    const latRad = (lat * Math.PI) / 180
    const lonRad = (lon * Math.PI) / 180
    for (let i = 0; i <= points; i++) {
      const bearing = (i * 360) / points * (Math.PI / 180)
      const pLat = Math.asin(
        Math.sin(latRad) * Math.cos(angularDist) +
        Math.cos(latRad) * Math.sin(angularDist) * Math.cos(bearing)
      )
      const pLon = lonRad + Math.atan2(
        Math.sin(bearing) * Math.sin(angularDist) * Math.cos(latRad),
        Math.cos(angularDist) - Math.sin(latRad) * Math.sin(pLat)
      )
      coords.push([(pLon * 180) / Math.PI, (pLat * 180) / Math.PI])
    }
    return {
      type: 'Feature' as const,
      geometry: { type: 'Polygon' as const, coordinates: [coords] },
      properties: {},
    }
  }

  function updateCircle(map: maplibregl.Map, lat: number, lon: number, radiusM: number) {
    const geojson = buildCircleGeoJSON(lat, lon, radiusM)
    const src = map.getSource(circleSourceId) as maplibregl.GeoJSONSource | undefined
    if (src) {
      src.setData(geojson as any)
    }
  }

  useEffect(() => {
    if (!containerRef.current) return
    const map = new maplibregl.Map({
      container: containerRef.current,
      style: {
        version: 8,
        sources: {
          basemap: {
            type: 'raster',
            tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
            tileSize: 256,
            attribution: '&copy; OpenStreetMap',
          },
        },
        layers: [{ id: 'basemap-tiles', type: 'raster', source: 'basemap', minzoom: 0, maxzoom: 19 }],
      },
      center: [value.lon || 10.0, value.lat || 51.0],
      zoom: value.lat ? 11 : 5,
    })
    mapRef.current = map

    map.on('load', () => {
      // Add circle source + layers
      map.addSource(circleSourceId, {
        type: 'geojson',
        data: buildCircleGeoJSON(value.lat || 51, value.lon || 10, value.radius_m || 1000) as any,
      })
      map.addLayer({
        id: 'geofence-fill',
        type: 'fill',
        source: circleSourceId,
        paint: { 'fill-color': '#f97316', 'fill-opacity': 0.2 },
      })
      map.addLayer({
        id: 'geofence-outline',
        type: 'line',
        source: circleSourceId,
        paint: { 'line-color': '#f97316', 'line-width': 2 },
      })

      // Marker
      const marker = new maplibregl.Marker({ color: '#f97316', draggable: true })
        .setLngLat([value.lon || 10.0, value.lat || 51.0])
        .addTo(map)
      markerRef.current = marker

      marker.on('dragend', () => {
        const lngLat = marker.getLngLat()
        updateCircle(map, lngLat.lat, lngLat.lng, value.radius_m || 1000)
        onChange({ ...value, lat: lngLat.lat, lon: lngLat.lng })
      })

      // Click to relocate marker
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

  // Sync radius changes to existing map
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    updateCircle(map, value.lat || 51, value.lon || 10, value.radius_m || 1000)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value.radius_m])

  const radiusKm = value.radius_m >= 1000 ? `${(value.radius_m / 1000).toFixed(1)} km` : `${value.radius_m} m`

  return (
    <div className="space-y-3 border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 bg-neutral-50 dark:bg-neutral-800/30">
      <div className="flex items-center gap-3 flex-wrap">
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
        <div className="flex items-center gap-2 flex-1 min-w-[220px]">
          <label className="text-xs text-neutral-600 dark:text-neutral-400 whitespace-nowrap">
            Radius: <span className="font-medium text-neutral-900 dark:text-neutral-100">{radiusKm}</span>
          </label>
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
        <div className="flex items-center gap-1 text-xs text-neutral-500">
          <span>Lat:</span>
          <span className="font-mono tabular-nums">{value.lat ? value.lat.toFixed(5) : '—'}</span>
          <span className="ml-2">Lon:</span>
          <span className="font-mono tabular-nums">{value.lon ? value.lon.toFixed(5) : '—'}</span>
        </div>
      </div>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">Klicke auf die Karte oder ziehe den Pin um den Mittelpunkt zu setzen.</p>
      <div ref={containerRef} className="w-full rounded-lg overflow-hidden" style={{ height: 260 }} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Condition value editor — renders the right input for each field type
// ---------------------------------------------------------------------------

function ConditionValueEditor({
  fieldDef,
  cond,
  onChange,
  tags,
}: {
  fieldDef: FieldDef
  cond: RuleCondition
  onChange: (patch: Partial<RuleCondition>) => void
  tags: Array<{ id: string; name: string; color: string }>
}) {
  const { inputType } = fieldDef

  if (inputType === 'bool') {
    return (
      <select
        value={String(cond.value ?? 'true')}
        onChange={(e) => onChange({ value: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">Ja</option>
        <option value="false">Nein</option>
      </select>
    )
  }

  if (inputType === 'sport') {
    if (cond.op === 'in') {
      const selected: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
      function toggle(v: string) {
        onChange({ value: selected.includes(v) ? selected.filter(x => x !== v) : [...selected, v] })
      }
      return (
        <div className="flex flex-wrap gap-1 flex-1 min-w-0">
          {SPORT_OPTIONS.map(s => (
            <button
              key={s.value}
              type="button"
              onClick={() => toggle(s.value)}
              className={`px-2 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${
                selected.includes(s.value)
                  ? 'bg-orange-500 text-white border-orange-500'
                  : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-orange-400'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
      )
    }
    return (
      <select
        value={String(cond.value ?? '')}
        onChange={(e) => onChange({ value: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Sportart wählen...</option>
        {SPORT_OPTIONS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
      </select>
    )
  }

  if (inputType === 'source') {
    if (cond.op === 'in') {
      const selected: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
      function toggle(v: string) {
        onChange({ value: selected.includes(v) ? selected.filter(x => x !== v) : [...selected, v] })
      }
      return (
        <div className="flex flex-wrap gap-1 flex-1 min-w-0">
          {SOURCE_OPTIONS.map(s => (
            <button key={s.value} type="button" onClick={() => toggle(s.value)}
              className={`px-2 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${
                selected.includes(s.value)
                  ? 'bg-orange-500 text-white border-orange-500'
                  : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-orange-400'
              }`}
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
    const selected: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
    function toggle(v: string) {
      onChange({ value: selected.includes(v) ? selected.filter(x => x !== v) : [...selected, v] })
    }
    return (
      <div className="flex flex-wrap gap-1 flex-1 min-w-0">
        {SEASON_OPTIONS.map(s => (
          <button key={s.value} type="button" onClick={() => toggle(s.value)}
            className={`px-2 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${
              selected.includes(s.value)
                ? 'bg-orange-500 text-white border-orange-500'
                : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-orange-400'
            }`}
          >{s.label}</button>
        ))}
      </div>
    )
  }

  if (inputType === 'weekday') {
    const selected: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
    function toggle(v: string) {
      onChange({ value: selected.includes(v) ? selected.filter(x => x !== v) : [...selected, v] })
    }
    return (
      <div className="flex flex-wrap gap-1 flex-1 min-w-0">
        {WEEKDAY_OPTIONS.map(d => (
          <button key={d.value} type="button" onClick={() => toggle(d.value)}
            className={`w-9 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${
              selected.includes(d.value)
                ? 'bg-orange-500 text-white border-orange-500'
                : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-orange-400'
            }`}
          >{d.label}</button>
        ))}
      </div>
    )
  }

  if (inputType === 'time_range') {
    const parts = typeof cond.value === 'string' ? cond.value.split(',') : ['', '']
    const from = parts[0] ?? ''
    const to = parts[1] ?? ''
    return (
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <input type="time" value={from}
          onChange={(e) => onChange({ value: `${e.target.value},${to}` })}
          className={inputClass}
        />
        <span className="text-neutral-500 text-xs">bis</span>
        <input type="time" value={to}
          onChange={(e) => onChange({ value: `${from},${e.target.value}` })}
          className={inputClass}
        />
      </div>
    )
  }

  if (inputType === 'between' && cond.op === 'between') {
    const parts = typeof cond.value === 'string' ? cond.value.split(',') : ['', '']
    const from = parts[0] ?? ''
    const to = parts[1] ?? ''
    return (
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <input type="number" placeholder="von" value={from}
          onChange={(e) => onChange({ value: `${e.target.value},${to}` })}
          className={inputClass + ' w-24'}
        />
        <span className="text-neutral-500 text-xs">–</span>
        <input type="number" placeholder="bis" value={to}
          onChange={(e) => onChange({ value: `${from},${e.target.value}` })}
          className={inputClass + ' w-24'}
        />
      </div>
    )
  }

  if (inputType === 'number' || (inputType === 'between' && cond.op !== 'between')) {
    return (
      <input type="number" value={cond.value ?? ''}
        onChange={(e) => onChange({ value: Number(e.target.value) })}
        placeholder="Wert"
        className={inputClass + ' flex-1 min-w-0'}
      />
    )
  }

  if (inputType === 'tag') {
    return (
      <select value={String(cond.value ?? '')} onChange={(e) => onChange({ value: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Tag wählen...</option>
        {tags.map(t => (
          <option key={t.id} value={t.name}>
            {t.name}
          </option>
        ))}
      </select>
    )
  }

  if (inputType === 'geofence') {
    const gfVal: GeofenceValue = cond.geofence ?? { lat: 51.0, lon: 10.0, radius_m: 1000, mode: 'inside' }
    return (
      <div className="flex-1 min-w-0 w-full">
        <GeofenceEditor
          value={gfVal}
          onChange={(v) => onChange({ geofence: v, value: `${v.mode}:${v.lat.toFixed(5)},${v.lon.toFixed(5)},${v.radius_m}m` })}
        />
      </div>
    )
  }

  // Default: free text
  return (
    <input type="text" value={cond.value ?? ''}
      onChange={(e) => onChange({ value: e.target.value })}
      placeholder="Wert"
      className={inputClass + ' flex-1 min-w-0'}
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
  tags,
}: {
  cond: RuleCondition
  onUpdate: (patch: Partial<RuleCondition>) => void
  onRemove: () => void
  tags: Array<{ id: string; name: string; color: string }>
}) {
  const fieldDef = CONDITION_FIELDS.find(f => f.value === cond.field) ?? CONDITION_FIELDS[0]
  const isGeofence = fieldDef.inputType === 'geofence'

  function handleFieldChange(newField: string) {
    const def = CONDITION_FIELDS.find(f => f.value === newField) ?? CONDITION_FIELDS[0]
    onUpdate({ field: newField, op: def.ops[0], value: def.inputType === 'bool' ? true : '' })
  }

  return (
    <div className={`${isGeofence ? 'flex flex-col gap-2' : 'flex items-center gap-2 flex-wrap'}`}>
      <div className="flex items-center gap-2 flex-wrap">
        <select
          value={cond.field}
          onChange={(e) => handleFieldChange(e.target.value)}
          className={inputClass}
        >
          {CONDITION_FIELDS.map(f => <option key={f.value} value={f.value}>{f.label}</option>)}
        </select>

        {/* Op selector — hide for geofence and weekday/season/time_range (implicit) */}
        {!isGeofence && fieldDef.inputType !== 'weekday' && fieldDef.inputType !== 'season' && fieldDef.inputType !== 'time_range' && (
          <select
            value={cond.op}
            onChange={(e) => onUpdate({ op: e.target.value })}
            className={inputClass}
          >
            {fieldDef.ops.map(op => (
              <option key={op} value={op}>{OP_LABELS[op] ?? op}</option>
            ))}
          </select>
        )}

        <button
          type="button"
          onClick={onRemove}
          className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer"
          title="Bedingung entfernen"
        >
          <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        </button>
      </div>

      <ConditionValueEditor fieldDef={fieldDef} cond={cond} onChange={onUpdate} tags={tags} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Condition Group
// ---------------------------------------------------------------------------

function ConditionGroupBlock({
  group,
  groupIndex,
  totalGroups,
  onChange,
  onRemove,
  tags,
}: {
  group: RuleConditionGroup
  groupIndex: number
  totalGroups: number
  onChange: (g: RuleConditionGroup) => void
  onRemove: () => void
  tags: Array<{ id: string; name: string; color: string }>
}) {
  function addCondition() {
    onChange({ ...group, conditions: [...group.conditions, defaultCondition()] })
  }
  function removeCondition(i: number) {
    onChange({ ...group, conditions: group.conditions.filter((_, idx) => idx !== i) })
  }
  function updateCondition(i: number, patch: Partial<RuleCondition>) {
    onChange({
      ...group,
      conditions: group.conditions.map((c, idx) => idx === i ? { ...c, ...patch } : c),
    })
  }

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 space-y-2 bg-white dark:bg-neutral-900/30">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="text-xs text-neutral-500 dark:text-neutral-400 font-medium">Gruppe {groupIndex + 1}</span>
          {/* AND / OR toggle within this group */}
          <div className="flex rounded overflow-hidden border border-neutral-200 dark:border-neutral-700">
            {(['and', 'or'] as const).map(l => (
              <button
                key={l}
                type="button"
                onClick={() => onChange({ ...group, logic: l })}
                className={`px-2 py-0.5 text-xs font-medium transition-colors cursor-pointer ${
                  group.logic === l
                    ? 'bg-orange-500 text-white'
                    : 'text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {l === 'and' ? 'UND' : 'ODER'}
              </button>
            ))}
          </div>
          <span className="text-xs text-neutral-400">(innerhalb Gruppe)</span>
        </div>
        {totalGroups > 1 && (
          <button type="button" onClick={onRemove}
            className="text-xs text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
            title="Gruppe entfernen"
          >
            Gruppe entfernen
          </button>
        )}
      </div>

      <div className="space-y-2">
        {group.conditions.map((cond, i) => (
          <div key={i}>
            {i > 0 && (
              <div className="flex items-center gap-2 my-1">
                <span className="text-xs font-semibold text-orange-500 uppercase tracking-wide px-1">
                  {group.logic === 'and' ? 'UND' : 'ODER'}
                </span>
              </div>
            )}
            <ConditionRow
              cond={cond}
              onUpdate={(patch) => updateCondition(i, patch)}
              onRemove={() => removeCondition(i)}
              tags={tags}
            />
          </div>
        ))}
      </div>

      <button
        type="button"
        onClick={addCondition}
        className="text-xs text-orange-500 hover:text-orange-600 cursor-pointer"
      >
        + Bedingung hinzufügen
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

function ActionParams({
  action,
  params,
  gearList,
  tags,
  onChange,
}: {
  action: string
  params: Record<string, any>
  gearList: any[]
  tags: Array<{ id: string; name: string; color: string }>
  onChange: (params: Record<string, any>) => void
}) {
  if (action === 'set_gear') {
    return (
      <select
        value={params.gear_id ?? ''}
        onChange={(e) => onChange({ gear_id: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Ausrüstung wählen...</option>
        {gearList.filter((g) => !g.is_retired).map((g) => (
          <option key={g.id} value={g.id}>{g.name}</option>
        ))}
      </select>
    )
  }
  if (action === 'add_tag') {
    return (
      <select
        value={params.tag ?? ''}
        onChange={(e) => onChange({ tag: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Tag wählen...</option>
        {tags.map(t => <option key={t.id} value={t.name}>{t.name}</option>)}
      </select>
    )
  }
  if (action === 'set_visibility') {
    return (
      <select
        value={params.visibility ?? ''}
        onChange={(e) => onChange({ visibility: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Sichtbarkeit wählen...</option>
        {VISIBILITY_OPTIONS.map(({ value, label }) => (
          <option key={value} value={value}>{label}</option>
        ))}
      </select>
    )
  }
  return null
}

// ---------------------------------------------------------------------------
// Main RuleEditor component
// ---------------------------------------------------------------------------

interface RuleEditorProps {
  rule?: AutomationRule | null
  onSave: (data: Partial<AutomationRule>) => void
  onCancel: () => void
  isSaving?: boolean
}

export default function RuleEditor({ rule, onSave, onCancel, isSaving }: RuleEditorProps) {
  const [name, setName] = useState(rule?.name ?? '')
  const [priority, setPriority] = useState(rule?.priority ?? 0)
  const [groups, setGroups] = useState<RuleConditionGroup[]>(() => normaliseGroups(rule))
  const [actions, setActions] = useState<RuleAction[]>(rule?.actions ?? [])

  const { data: gearList } = useQuery({
    queryKey: ['gear-list'],
    queryFn: () => fetchGear(false),
  })
  const { data: tagList } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  const tags = (tagList ?? []).map(t => ({ id: t.id, name: t.name, color: t.color }))

  // Groups mutations
  function addGroup() {
    setGroups([...groups, defaultGroup()])
  }
  function updateGroup(i: number, g: RuleConditionGroup) {
    setGroups(groups.map((old, idx) => idx === i ? g : old))
  }
  function removeGroup(i: number) {
    setGroups(groups.filter((_, idx) => idx !== i))
  }

  // Actions mutations
  function addAction() {
    setActions([...actions, { action: 'add_tag', params: {} }])
  }
  function removeAction(idx: number) {
    setActions(actions.filter((_, i) => i !== idx))
  }
  function updateAction(idx: number, action: string, params: Record<string, any>) {
    setActions(actions.map((a, i) => (i === idx ? { action, params } : a)))
  }

  const totalConditions = groups.reduce((s, g) => s + g.conditions.length, 0)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim() || totalConditions === 0 || actions.length === 0) return
    const flat = flattenGroups(groups)
    onSave({
      name: name.trim(),
      priority,
      condition_groups: groups,
      // Also send flat for backward compatibility:
      conditions: flat.conditions,
      conditions_logic: flat.conditions_logic,
      actions,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Name & Priority */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="sm:col-span-2">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Name *</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            className={inputClass + ' w-full'}
            placeholder="z.B. MTB-Rides automatisch taggen"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Priorität</label>
          <input
            type="number"
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
            min={0}
            className={inputClass + ' w-full'}
          />
        </div>
      </div>

      {/* Condition groups */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Bedingungen *</h3>
            {groups.length > 1 && (
              <p className="text-xs text-neutral-400 mt-0.5">Gruppen werden mit UND verknüpft</p>
            )}
          </div>
          <button
            type="button"
            onClick={addGroup}
            className="text-sm text-orange-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 cursor-pointer"
          >
            + Gruppe hinzufügen
          </button>
        </div>
        {totalConditions === 0 && (
          <p className="text-sm text-neutral-400 py-2">Mindestens eine Bedingung erforderlich.</p>
        )}
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
                tags={tags}
              />
            </div>
          ))}
        </div>
      </div>

      {/* Actions */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Aktionen *</h3>
          <button
            type="button"
            onClick={addAction}
            className="text-sm text-orange-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 cursor-pointer"
          >
            + Aktion
          </button>
        </div>
        {actions.length === 0 && (
          <p className="text-sm text-neutral-400 py-2">Mindestens eine Aktion erforderlich.</p>
        )}
        <div className="space-y-2">
          {actions.map((act, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <select
                value={act.action}
                onChange={(e) => updateAction(idx, e.target.value, {})}
                className={inputClass}
              >
                {ACTION_TYPES.map(a => (
                  <option key={a.value} value={a.value}>{a.label}</option>
                ))}
              </select>
              <ActionParams
                action={act.action}
                params={act.params}
                gearList={gearList ?? []}
                tags={tags}
                onChange={(params) => updateAction(idx, act.action, params)}
              />
              <button
                type="button"
                onClick={() => removeAction(idx)}
                className="text-neutral-400 hover:text-rose-500 transition-colors cursor-pointer"
                title="Aktion entfernen"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Submit */}
      <div className="flex gap-3 justify-end pt-2">
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
        >
          Abbrechen
        </button>
        <button
          type="submit"
          disabled={isSaving || !name.trim() || totalConditions === 0 || actions.length === 0}
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}
