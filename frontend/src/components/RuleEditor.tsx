import { useRef, useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchGear } from '@/api/gear'
import { fetchTags } from '@/api/tags'
import { fetchDevices } from '@/api/hardware'
import type { Device } from '@/api/hardware'
import type { AutomationRule, RuleCondition, RuleConditionGroup, RuleAction } from '@/api/types'
import { SPORT_DEFS } from '@/lib/sportTaxonomy'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import ZoneEditor, { type ZoneData } from '@/components/ZoneEditor'
import Toggle from '@/components/Toggle'

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

const VISIBILITY_OPTIONS = [
  { value: 'private',    label: 'Privat' },
  { value: 'bekannte',   label: 'Bekannte' },
  { value: 'instanz',    label: 'Instanz' },
  { value: 'federation', label: 'Federation' },
  { value: 'public',     label: 'Öffentlich' },
]

// Field definitions: each entry tells the editor how to render the value input
interface ConditionField {
  field: string
  label: string
  inputType: 'number' | 'text' | 'bool' | 'enum' | 'tag' | 'segment' | 'geofence' | 'geofence_zone' | 'geofence_proximity'
  unit?: string
  options?: { value: string; label: string }[]
  /** Override available operators for this specific field */
  operators?: string[]
}

interface ConditionCategory {
  key: string
  label: string
  fields: ConditionField[]
}

const MONTH_OPTIONS = [
  { value: '1', label: 'Januar' }, { value: '2', label: 'Februar' },
  { value: '3', label: 'März' }, { value: '4', label: 'April' },
  { value: '5', label: 'Mai' }, { value: '6', label: 'Juni' },
  { value: '7', label: 'Juli' }, { value: '8', label: 'August' },
  { value: '9', label: 'September' }, { value: '10', label: 'Oktober' },
  { value: '11', label: 'November' }, { value: '12', label: 'Dezember' },
]
const INTENSITY_OPTIONS = [
  { value: 'locker', label: 'Locker' },
  { value: 'moderat', label: 'Moderat' },
  { value: 'intensiv', label: 'Intensiv' },
]
const TIME_OF_DAY_OPTIONS = [
  { value: 'nacht', label: 'Nacht' },
  { value: 'frueh', label: 'Früh' },
  { value: 'morgen', label: 'Morgen' },
  { value: 'mittag', label: 'Mittag' },
  { value: 'nachmittag', label: 'Nachmittag' },
  { value: 'abend', label: 'Abend' },
]
const WEATHER_CONDITION_OPTIONS = [
  { value: 'clear', label: 'Sonnig' },
  { value: 'cloudy', label: 'Bewölkt' },
  { value: 'rain', label: 'Regen' },
  { value: 'snow', label: 'Schnee' },
  { value: 'fog', label: 'Nebel' },
  { value: 'storm', label: 'Gewitter' },
]

const CONDITION_CATEGORIES: ConditionCategory[] = [
  { key: 'title', label: 'Titel', fields: [{ field: 'title', label: 'Titel', inputType: 'text' }] },
  { key: 'description', label: 'Beschreibung', fields: [{ field: 'description', label: 'Beschreibung', inputType: 'text' }] },
  { key: 'sport', label: 'Sportart', fields: [
    { field: 'sport_type', label: 'Sportart', inputType: 'enum', options: SPORT_OPTIONS },
    { field: 'sub_type', label: 'Sub-Typ', inputType: 'text' },
  ]},
  { key: 'activity', label: 'Aktivität', fields: [
    { field: 'source', label: 'Quelle', inputType: 'enum', options: SOURCE_OPTIONS },
    { field: 'intensity', label: 'Intensität', inputType: 'enum', options: INTENSITY_OPTIONS },
    { field: 'is_indoor', label: 'Indoor', inputType: 'bool' },
    { field: 'has_gps', label: 'Hat GPS', inputType: 'bool' },
    { field: 'visibility', label: 'Sichtbarkeit', inputType: 'enum', options: VISIBILITY_OPTIONS },
  ]},
  { key: 'time', label: 'Zeit', fields: [
    { field: 'elapsed_time_s', label: 'Gesamtzeit', inputType: 'number', unit: 's' },
    { field: 'moving_time_s', label: 'Bewegungszeit', inputType: 'number', unit: 's' },
    { field: 'weekday', label: 'Wochentag', inputType: 'enum', options: WEEKDAY_OPTIONS },
    { field: 'month', label: 'Monat', inputType: 'enum', options: MONTH_OPTIONS },
    { field: 'season', label: 'Jahreszeit', inputType: 'enum', options: SEASON_OPTIONS },
    { field: 'time_of_day', label: 'Tageszeit', inputType: 'enum', options: TIME_OF_DAY_OPTIONS },
  ]},
  { key: 'distance', label: 'Distanz', fields: [
    { field: 'distance_m', label: 'Distanz', inputType: 'number', unit: 'm' },
  ]},
  { key: 'speed', label: 'Geschwindigkeit', fields: [
    { field: 'avg_speed_ms', label: 'Ø Geschwindigkeit', inputType: 'number', unit: 'm/s' },
    { field: 'max_speed_ms', label: 'Max. Geschwindigkeit', inputType: 'number', unit: 'm/s' },
    { field: 'min_speed_ms', label: 'Min. Geschwindigkeit', inputType: 'number', unit: 'm/s' },
  ]},
  { key: 'pace', label: 'Tempo', fields: [
    { field: 'avg_pace_min_km', label: 'Ø Tempo', inputType: 'number', unit: 'min/km' },
    { field: 'min_pace_min_km', label: 'Bestes Tempo', inputType: 'number', unit: 'min/km' },
    { field: 'max_pace_min_km', label: 'Langsamstes Tempo', inputType: 'number', unit: 'min/km' },
  ]},
  { key: 'elevation', label: 'Anstiege', fields: [
    { field: 'elevation_gain_m', label: 'Höhenmeter aufwärts', inputType: 'number', unit: 'm' },
    { field: 'elevation_loss_m', label: 'Höhenmeter abwärts', inputType: 'number', unit: 'm' },
    { field: 'max_elevation_m', label: 'Max. Höhe', inputType: 'number', unit: 'm' },
    { field: 'min_elevation_m', label: 'Min. Höhe', inputType: 'number', unit: 'm' },
    { field: 'climb_count', label: 'Anzahl Anstiege', inputType: 'number' },
    { field: 'climb_category_max', label: 'Härteste Kategorie', inputType: 'number' },
  ]},
  { key: 'heartrate', label: 'Herzfrequenz', fields: [
    { field: 'avg_heart_rate', label: 'Ø HF', inputType: 'number', unit: 'bpm' },
    { field: 'max_heart_rate', label: 'Max. HF', inputType: 'number', unit: 'bpm' },
    { field: 'min_heart_rate', label: 'Min. HF', inputType: 'number', unit: 'bpm' },
  ]},
  { key: 'cadence', label: 'Kadenz', fields: [
    { field: 'avg_cadence', label: 'Ø Kadenz', inputType: 'number', unit: 'rpm' },
    { field: 'max_cadence', label: 'Max. Kadenz', inputType: 'number', unit: 'rpm' },
  ]},
  { key: 'power', label: 'Leistung', fields: [
    { field: 'avg_power_w', label: 'Ø Leistung', inputType: 'number', unit: 'W' },
    { field: 'max_power_w', label: 'Max. Leistung', inputType: 'number', unit: 'W' },
    { field: 'min_power_w', label: 'Min. Leistung', inputType: 'number', unit: 'W' },
    { field: 'normalized_power_w', label: 'Normalisierte Leistung', inputType: 'number', unit: 'W' },
    { field: 'tss', label: 'TSS', inputType: 'number' },
    { field: 'trimp', label: 'TRIMP', inputType: 'number' },
  ]},
  { key: 'sensor_temp', label: 'Sensor-Temperatur', fields: [
    { field: 'avg_temperature_c', label: 'Ø Temperatur', inputType: 'number', unit: '°C' },
    { field: 'min_temperature_c', label: 'Min. Temperatur', inputType: 'number', unit: '°C' },
    { field: 'max_temperature_c', label: 'Max. Temperatur', inputType: 'number', unit: '°C' },
  ]},
  { key: 'weather', label: 'Wetter', fields: [
    { field: 'weather_temp_c', label: 'Wetter-Temperatur', inputType: 'number', unit: '°C' },
    { field: 'weather_temp_min_c', label: 'Min. Wetter-Temp.', inputType: 'number', unit: '°C' },
    { field: 'weather_temp_max_c', label: 'Max. Wetter-Temp.', inputType: 'number', unit: '°C' },
    { field: 'weather_condition', label: 'Wetterlage', inputType: 'enum', options: WEATHER_CONDITION_OPTIONS },
    { field: 'weather_humidity', label: 'Luftfeuchtigkeit', inputType: 'number', unit: '%' },
    { field: 'weather_wind_speed_ms', label: 'Windgeschwindigkeit', inputType: 'number', unit: 'm/s' },
    { field: 'weather_wind_speed_max_ms', label: 'Max. Windgeschwindigkeit', inputType: 'number', unit: 'm/s' },
  ]},
  { key: 'location', label: 'Ort', fields: [
    { field: 'start_city', label: 'Startort', inputType: 'text' },
    { field: 'start_state', label: 'Bundesland/Region', inputType: 'text' },
    { field: 'start_country', label: 'Land', inputType: 'text' },
    { field: 'start_country_code', label: 'Ländercode', inputType: 'text' },
    { field: 'end_city', label: 'Endort', inputType: 'text' },
  ]},
  { key: 'context', label: 'Kontext', fields: [
    { field: 'gear_id', label: 'Ausrüstung', inputType: 'enum', options: [] },
    { field: 'has_tag', label: 'Hat Tag', inputType: 'tag' },
    { field: 'photo_count', label: 'Anzahl Fotos', inputType: 'number' },
    { field: 'detected_lap_count', label: 'Anzahl Runden', inputType: 'number' },
    { field: 'hide_map', label: 'Karte verborgen', inputType: 'bool' },
  ]},
  { key: 'lift', label: 'Lift', fields: [
    { field: 'has_lift', label: 'Lift genutzt', inputType: 'bool' },
    { field: 'lift_count', label: 'Anzahl Lifte', inputType: 'number' },
    { field: 'lift_ascent_m', label: 'Lift-Aufstieg', inputType: 'number', unit: 'm' },
    { field: 'lift_descent_m', label: 'Lift-Abstieg', inputType: 'number', unit: 'm' },
    { field: 'lift_distance_m', label: 'Lift-Distanz', inputType: 'number', unit: 'm' },
    { field: 'lift_duration_s', label: 'Lift-Dauer', inputType: 'number', unit: 's' },
    { field: 'net_ascent_m', label: 'Netto-Aufstieg', inputType: 'number', unit: 'm' },
    { field: 'net_descent_m', label: 'Netto-Abstieg', inputType: 'number', unit: 'm' },
    { field: 'net_distance_m', label: 'Netto-Distanz', inputType: 'number', unit: 'm' },
  ]},
  { key: 'segments', label: 'Segmente', fields: [
    { field: 'has_segment_effort', label: 'Segment gefahren', inputType: 'segment' },
    { field: 'segment_effort_rank', label: 'Segment-Platzierung', inputType: 'segment' },
    { field: 'segment_effort_time_s', label: 'Segment-Zeit', inputType: 'segment' },
  ]},
  { key: 'device', label: 'Geräte & Sensoren', fields: [
    {
      field: 'recording_device_id',
      label: 'Aufzeichnungsgerät',
      inputType: 'enum',
      options: [], // dynamically filled via devices query in RuleEditor
    },
    {
      field: 'has_sensor_type',
      label: 'Sensortyp vorhanden',
      inputType: 'enum',
      options: [
        { value: 'heart_rate', label: 'Herzfrequenz' },
        { value: 'bike_power', label: 'Leistungsmesser' },
        { value: 'bike_cadence', label: 'Trittfrequenz' },
        { value: 'bike_speed', label: 'Geschwindigkeit' },
        { value: 'bike_speed_cadence', label: 'Geschwindigkeit/Trittfrequenz' },
        { value: 'stride_speed_distance', label: 'Laufpod' },
        { value: 'bike_radar', label: 'Rückwärtsradar' },
        { value: 'shifting', label: 'Schaltung (Di2/eTap)' },
        { value: 'env_sensor', label: 'Umgebungssensor' },
        { value: 'muscle_oxygen', label: 'Muskel-Sauerstoff' },
      ],
      operators: ['contains', 'not_contains'],
    },
    {
      field: 'sensor_device_ids',
      label: 'Konkreter Sensor',
      inputType: 'enum',
      options: [], // dynamically filled via devices query in RuleEditor
      operators: ['contains'],
    },
  ]},
]

// Internal-only fields not shown in category dropdown (geofence, round-trip)
const INTERNAL_FIELDS: ConditionField[] = [
  { field: 'geofence', label: 'Geofence', inputType: 'geofence' },
  { field: 'geofence_zone', label: 'In Zone', inputType: 'geofence_zone' },
  { field: 'geofence_proximity', label: 'Nähe zum Pin', inputType: 'geofence_proximity' },
  { field: 'is_round_trip', label: 'Rundtour', inputType: 'bool' },
]

// Flat lookup: find a ConditionField by its field name
const ALL_CONDITION_FIELDS: ConditionField[] = [
  ...CONDITION_CATEGORIES.flatMap(c => c.fields),
  ...INTERNAL_FIELDS,
]

function findFieldDef(field: string): ConditionField | undefined {
  return ALL_CONDITION_FIELDS.find(f => f.field === field)
}

function findCategory(field: string): ConditionCategory | undefined {
  return CONDITION_CATEGORIES.find(cat => cat.fields.some(f => f.field === field))
}

// Operators per input type
function opsForField(cf: ConditionField): string[] {
  // Field-level override takes precedence
  if (cf.operators && cf.operators.length > 0) return cf.operators
  switch (cf.inputType) {
    case 'bool': return ['eq']
    case 'enum': return ['eq', 'neq', 'in']
    case 'text': return ['eq', 'neq', 'contains']
    case 'number': return ['gt', 'lt', 'gte', 'lte', 'eq']
    case 'tag': return ['eq']
    case 'segment': return ['eq']
    case 'geofence': return ['eq']
    case 'geofence_zone': return ['eq']
    case 'geofence_proximity': return ['eq']
  }
}

const OP_LABELS: Record<string, string> = {
  eq: '=', neq: '!=', gt: '>', lt: '<', gte: '>=', lte: '<=',
  contains: 'enthält', not_contains: 'enthält nicht', in: 'in',
}

const ACTION_TYPES = [
  { value: 'set_gear',        label: 'Ausrüstung zuweisen' },
  { value: 'add_tag',         label: 'Tag hinzufügen' },
  { value: 'remove_tag',      label: 'Tag entfernen' },
  { value: 'set_visibility',  label: 'Sichtbarkeit setzen' },
  { value: 'set_sport_type',  label: 'Sportart setzen' },
  { value: 'set_name',        label: 'Titel setzen' },
  { value: 'set_intensity',   label: 'Intensität setzen' },
  { value: 'set_indoor',      label: 'Indoor-Flag setzen' },
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

const inputClass = 'border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent'

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
  // Keep a live ref to the current value so map callbacks always read the latest state
  const valueRef = useRef<GeofenceValue>(value)
  // Synchronously update ref on every render so event handlers stay current
  valueRef.current = value

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
        const current = valueRef.current
        updateCircle(map, lngLat.lat, lngLat.lng, current.radius_m || 1000)
        onChange({ ...current, lat: lngLat.lat, lon: lngLat.lng })
      })

      // Click to relocate marker
      map.on('click', (e) => {
        const { lat, lng } = e.lngLat
        marker.setLngLat([lng, lat])
        const current = valueRef.current
        updateCircle(map, lat, lng, current.radius_m || 1000)
        onChange({ ...current, lat, lon: lng })
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
        <div className="flex items-center gap-2 flex-1 min-w-[260px]">
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
            className="flex-1 accent-accent-DEFAULT"
          />
          <input
            type="number"
            min={50}
            max={100000}
            step={50}
            value={value.radius_m}
            onChange={(e) => {
              const v = Math.max(50, Math.min(100000, Number(e.target.value) || 1000))
              onChange({ ...value, radius_m: v })
            }}
            className={inputClass + ' w-24 py-1 text-xs'}
            title="Radius in Metern"
          />
          <span className="text-xs text-neutral-400">m</span>
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
// GeofenceZoneEditor — draws a circle/polygon zone using ZoneEditor
// ---------------------------------------------------------------------------

interface GeofenceZoneValue {
  mode: 'inside' | 'outside'
  shape: 'circle' | 'polygon'
  lat?: number
  lon?: number
  radius_m?: number
  polygon?: string // GeoJSON Polygon JSON string
}

function GeofenceZoneEditor({
  value,
  onChange,
}: {
  value: GeofenceZoneValue
  onChange: (v: GeofenceZoneValue) => void
}) {
  // Reconstruct ZoneData from the stored geofence_zone value
  const zoneData: ZoneData = (() => {
    if (value.shape === 'polygon' && value.polygon) {
      try {
        const poly = JSON.parse(value.polygon) as { type: 'Polygon'; coordinates: number[][][] }
        return { mode: 'polygon', polygon: poly }
      } catch {
        // fall through to circle default
      }
    }
    return {
      mode: 'circle',
      center: value.lon != null && value.lat != null ? [value.lon, value.lat] : undefined,
      radiusM: value.radius_m ?? 1000,
    }
  })()

  function handleZoneChange(zone: ZoneData) {
    if (zone.mode === 'circle') {
      onChange({
        mode: value.mode,
        shape: 'circle',
        lat: zone.center?.[1],
        lon: zone.center?.[0],
        radius_m: zone.radiusM,
      })
    } else if (zone.polygon) {
      onChange({
        mode: value.mode,
        shape: 'polygon',
        polygon: JSON.stringify(zone.polygon),
      })
    }
  }

  return (
    <div className="space-y-3 border border-neutral-200 dark:border-neutral-700 rounded-lg p-3 bg-neutral-50 dark:bg-neutral-800/30">
      <div className="flex items-center gap-3">
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
      <ZoneEditor
        value={zoneData}
        onChange={handleZoneChange}
        minRadiusM={50}
        maxRadiusM={100000}
        enableHoles={false}
        className="text-sm"
      />
    </div>
  )
}

// ---------------------------------------------------------------------------
// GeofenceProximityEditor — pin + radius, no hard upper limit
// ---------------------------------------------------------------------------

interface GeofenceProximityValue {
  lat: number
  lon: number
  radius_m: number
  mode: 'inside' | 'outside'
}

function GeofenceProximityEditor({
  value,
  onChange,
}: {
  value: GeofenceProximityValue
  onChange: (v: GeofenceProximityValue) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const circleSourceId = 'gfp-circle'
  const valueRef = useRef<GeofenceProximityValue>(value)
  valueRef.current = value

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
    const src = map.getSource(circleSourceId) as maplibregl.GeoJSONSource | undefined
    if (src) src.setData(buildCircleGeoJSON(lat, lon, radiusM) as any)
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
      map.addSource(circleSourceId, {
        type: 'geojson',
        data: buildCircleGeoJSON(value.lat || 51, value.lon || 10, value.radius_m || 1000) as any,
      })
      map.addLayer({
        id: 'gfp-fill',
        type: 'fill',
        source: circleSourceId,
        paint: { 'fill-color': '#3B82F6', 'fill-opacity': 0.12 },
      })
      map.addLayer({
        id: 'gfp-outline',
        type: 'line',
        source: circleSourceId,
        paint: { 'line-color': '#3B82F6', 'line-width': 2 },
      })

      const el = document.createElement('div')
      el.style.cssText = `
        width:14px;height:14px;border-radius:50%;
        background:#3B82F6;border:2px solid white;
        box-shadow:0 1px 4px rgba(0,0,0,.5);cursor:grab;
      `
      const marker = new maplibregl.Marker({ element: el, draggable: true })
        .setLngLat([value.lon || 10.0, value.lat || 51.0])
        .addTo(map)
      markerRef.current = marker

      marker.on('dragend', () => {
        const lngLat = marker.getLngLat()
        const current = valueRef.current
        updateCircle(map, lngLat.lat, lngLat.lng, current.radius_m || 1000)
        onChange({ ...current, lat: lngLat.lat, lon: lngLat.lng })
      })

      map.on('click', (e) => {
        const { lat, lng } = e.lngLat
        marker.setLngLat([lng, lat])
        const current = valueRef.current
        updateCircle(map, lat, lng, current.radius_m || 1000)
        onChange({ ...current, lat, lon: lng })
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

  const radiusDisplay = value.radius_m >= 1000
    ? `${(value.radius_m / 1000).toFixed(1)} km`
    : `${value.radius_m} m`

  return (
    <div className="space-y-3 border border-blue-200 dark:border-blue-800 rounded-lg p-3 bg-blue-50/30 dark:bg-blue-950/20">
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
            Radius: <span className="font-medium text-neutral-900 dark:text-neutral-100">{radiusDisplay}</span>
          </label>
          <input
            type="range"
            min={50}
            max={200000}
            step={50}
            value={value.radius_m}
            onChange={(e) => onChange({ ...value, radius_m: Number(e.target.value) })}
            className="flex-1"
            style={{ accentColor: 'var(--color-accent-DEFAULT, #f97316)' }}
          />
          <input
            type="number"
            min={50}
            step={50}
            value={value.radius_m}
            onChange={(e) => {
              const v = Math.max(50, Number(e.target.value) || 1000)
              onChange({ ...value, radius_m: v })
            }}
            className={inputClass + ' w-24 py-1 text-xs'}
            title="Radius in Metern (kein Limit)"
          />
          <span className="text-xs text-neutral-400">m</span>
        </div>
        <div className="flex items-center gap-1 text-xs text-neutral-500">
          <span>Lat:</span>
          <span className="font-mono tabular-nums">{value.lat ? value.lat.toFixed(5) : '—'}</span>
          <span className="ml-2">Lon:</span>
          <span className="font-mono tabular-nums">{value.lon ? value.lon.toFixed(5) : '—'}</span>
        </div>
      </div>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        Klicke auf die Karte oder ziehe den Pin, um den genauen Standort zu setzen.
      </p>
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
  recorders,
  sensors,
}: {
  fieldDef: ConditionField
  cond: RuleCondition
  onChange: (patch: Partial<RuleCondition>) => void
  tags: Array<{ id: string; name: string; color: string }>
  recorders?: Device[]
  sensors?: Device[]
}) {
  const { inputType } = fieldDef

  // Resolve dynamic options for device fields
  function deviceOption(d: Device) {
    return { value: d.id, label: d.custom_name || d.product_name || d.manufacturer || d.id }
  }
  const resolvedFieldDef: ConditionField = (() => {
    if (fieldDef.field === 'recording_device_id' && recorders) {
      return { ...fieldDef, options: recorders.map(deviceOption) }
    }
    if (fieldDef.field === 'sensor_device_ids' && sensors) {
      return { ...fieldDef, options: sensors.map(deviceOption) }
    }
    return fieldDef
  })()
  const effectiveOptions = resolvedFieldDef.options ?? fieldDef.options

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

  if (inputType === 'enum' && effectiveOptions) {
    if (cond.op === 'in' || cond.op === 'contains' || cond.op === 'not_contains') {
      const selected: string[] = Array.isArray(cond.value) ? cond.value : (cond.value ? [cond.value] : [])
      function toggle(v: string) {
        onChange({ value: selected.includes(v) ? selected.filter(x => x !== v) : [...selected, v] })
      }
      return (
        <div className="flex flex-wrap gap-1 flex-1 min-w-0">
          {effectiveOptions.map(s => (
            <button
              key={s.value}
              type="button"
              onClick={() => toggle(s.value)}
              className={`px-2 py-0.5 rounded text-xs font-medium border transition-colors cursor-pointer ${
                selected.includes(s.value)
                  ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                  : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:border-accent-400'
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
        <option value="">Wählen...</option>
        {effectiveOptions.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
      </select>
    )
  }

  if (inputType === 'number' && cond.op === 'between') {
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
        {fieldDef.unit && <span className="text-xs text-neutral-400">{fieldDef.unit}</span>}
      </div>
    )
  }

  if (inputType === 'number') {
    return (
      <div className="flex items-center gap-1 flex-1 min-w-0">
        <input type="number" value={cond.value ?? ''}
          onChange={(e) => onChange({ value: Number(e.target.value) })}
          placeholder="Wert"
          className={inputClass + ' flex-1 min-w-0'}
        />
        {fieldDef.unit && <span className="text-xs text-neutral-400">{fieldDef.unit}</span>}
      </div>
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

  if (inputType === 'segment') {
    return (
      <input type="text" value={cond.value ?? ''}
        onChange={(e) => onChange({ value: e.target.value })}
        placeholder="Segment-ID"
        className={inputClass + ' flex-1 min-w-0'}
      />
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

  if (inputType === 'geofence_zone') {
    const gzVal: GeofenceZoneValue = cond.geofence_zone ?? { mode: 'inside', shape: 'circle', lat: 51.0, lon: 10.0, radius_m: 1000 }
    return (
      <div className="flex-1 min-w-0 w-full">
        <GeofenceZoneEditor
          value={gzVal}
          onChange={(v) => onChange({
            geofence_zone: v,
            value: v.shape === 'polygon'
              ? `${v.mode}:polygon`
              : `${v.mode}:${v.lat?.toFixed(5)},${v.lon?.toFixed(5)},${v.radius_m}m`,
          })}
        />
      </div>
    )
  }

  if (inputType === 'geofence_proximity') {
    const gpVal: GeofenceProximityValue = cond.geofence_proximity ?? { lat: 51.0, lon: 10.0, radius_m: 1000, mode: 'inside' }
    return (
      <div className="flex-1 min-w-0 w-full">
        <GeofenceProximityEditor
          value={gpVal}
          onChange={(v) => onChange({
            geofence_proximity: v,
            value: `${v.mode}:${v.lat.toFixed(5)},${v.lon.toFixed(5)},${v.radius_m}m`,
          })}
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
// Single condition row — two-level: Category → Field → Operator → Value
// ---------------------------------------------------------------------------

function ConditionRow({
  cond,
  onUpdate,
  onRemove,
  tags,
  recorders,
  sensors,
}: {
  cond: RuleCondition
  onUpdate: (patch: Partial<RuleCondition>) => void
  onRemove: () => void
  tags: Array<{ id: string; name: string; color: string }>
  recorders?: Device[]
  sensors?: Device[]
}) {
  const currentCat = findCategory(cond.field)
  const currentField = findFieldDef(cond.field)
  const isInternalField = !currentCat  // geofence, is_round_trip etc.

  // For internal fields (geofence, round-trip), show a simplified row
  if (isInternalField && currentField) {
    return (
      <div className={['geofence', 'geofence_zone', 'geofence_proximity'].includes(currentField.inputType) ? 'space-y-2' : 'flex items-center gap-2 flex-wrap'}>
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">{currentField.label}</span>
        <ConditionValueEditor fieldDef={currentField} cond={cond} onChange={onUpdate} tags={tags} recorders={recorders} sensors={sensors} />
        <button type="button" onClick={onRemove}
          className="text-neutral-400 hover:text-rose-500 transition-colors flex-shrink-0 cursor-pointer"
          title="Bedingung entfernen"
        >
          <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        </button>
      </div>
    )
  }

  const safeCat = currentCat ?? CONDITION_CATEGORIES[0]
  const safeField = currentField ?? safeCat.fields[0]
  const ops = opsForField(safeField)

  function handleCategoryChange(catKey: string) {
    const cat = CONDITION_CATEGORIES.find(c => c.key === catKey) ?? CONDITION_CATEGORIES[0]
    const firstField = cat.fields[0]
    const firstOps = opsForField(firstField)
    onUpdate({ field: firstField.field, op: firstOps[0], value: firstField.inputType === 'bool' ? true : '' })
  }

  function handleFieldChange(fieldName: string) {
    const fd = safeCat.fields.find(f => f.field === fieldName) ?? safeCat.fields[0]
    const fieldOps = opsForField(fd)
    onUpdate({ field: fd.field, op: fieldOps[0], value: fd.inputType === 'bool' ? true : '' })
  }

  return (
    <div className="flex items-center gap-2 flex-wrap">
      {/* Category selector */}
      <select
        value={safeCat.key}
        onChange={(e) => handleCategoryChange(e.target.value)}
        className={inputClass}
      >
        {CONDITION_CATEGORIES.map(cat => (
          <option key={cat.key} value={cat.key}>{cat.label}</option>
        ))}
      </select>

      {/* Sub-field selector (only when category has > 1 field) */}
      {safeCat.fields.length > 1 && (
        <select
          value={cond.field}
          onChange={(e) => handleFieldChange(e.target.value)}
          className={inputClass}
        >
          {safeCat.fields.map(f => (
            <option key={f.field} value={f.field}>{f.label}</option>
          ))}
        </select>
      )}

      {/* Operator selector */}
      {ops.length > 1 && (
        <select
          value={cond.op}
          onChange={(e) => onUpdate({ op: e.target.value })}
          className={inputClass}
        >
          {ops.map(op => (
            <option key={op} value={op}>{OP_LABELS[op] ?? op}</option>
          ))}
        </select>
      )}

      {/* Value editor */}
      <ConditionValueEditor fieldDef={safeField} cond={cond} onChange={onUpdate} tags={tags} recorders={recorders} sensors={sensors} />

      {/* Remove button */}
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
  recorders,
  sensors,
}: {
  group: RuleConditionGroup
  groupIndex: number
  totalGroups: number
  onChange: (g: RuleConditionGroup) => void
  onRemove: () => void
  tags: Array<{ id: string; name: string; color: string }>
  recorders?: Device[]
  sensors?: Device[]
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
                    ? 'bg-accent-DEFAULT text-white'
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
                <span className="text-xs font-semibold text-accent-DEFAULT uppercase tracking-wide px-1">
                  {group.logic === 'and' ? 'UND' : 'ODER'}
                </span>
              </div>
            )}
            <ConditionRow
              cond={cond}
              onUpdate={(patch) => updateCondition(i, patch)}
              onRemove={() => removeCondition(i)}
              tags={tags}
              recorders={recorders}
              sensors={sensors}
            />
          </div>
        ))}
      </div>

      <button
        type="button"
        onClick={addCondition}
        className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
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
        value={params.tag_id ?? ''}
        onChange={(e) => onChange({ tag_id: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Tag wählen...</option>
        {tags.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
      </select>
    )
  }
  if (action === 'remove_tag') {
    return (
      <select
        value={params.tag_id ?? ''}
        onChange={(e) => onChange({ tag_id: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Tag wählen...</option>
        {tags.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
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
  if (action === 'set_sport_type') {
    return (
      <div className="flex items-center gap-2 flex-1 min-w-0">
        <select
          value={params.sport_type ?? ''}
          onChange={(e) => onChange({ ...params, sport_type: e.target.value })}
          className={inputClass + ' flex-1 min-w-0'}
        >
          <option value="">Sportart wählen...</option>
          {SPORT_OPTIONS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
        </select>
        <input
          type="text"
          value={params.sub_type ?? ''}
          onChange={(e) => onChange({ ...params, sub_type: e.target.value })}
          placeholder="Sub-Typ (optional)"
          className={inputClass + ' w-32'}
        />
      </div>
    )
  }
  if (action === 'set_name') {
    return (
      <div className="flex flex-col gap-1 flex-1 min-w-0">
        <input
          type="text"
          value={params.template ?? ''}
          onChange={(e) => onChange({ template: e.target.value })}
          placeholder="Neuer Titel"
          className={inputClass + ' w-full'}
        />
        <span className="text-xs text-neutral-400">Platzhalter: {'{city}'}, {'{distance_km}'}, {'{sport}'}</span>
      </div>
    )
  }
  if (action === 'set_intensity') {
    return (
      <select
        value={params.intensity ?? ''}
        onChange={(e) => onChange({ intensity: e.target.value })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="">Intensität wählen...</option>
        {INTENSITY_OPTIONS.map(({ value, label }) => (
          <option key={value} value={value}>{label}</option>
        ))}
      </select>
    )
  }
  if (action === 'set_indoor') {
    return (
      <select
        value={String(params.indoor ?? 'true')}
        onChange={(e) => onChange({ indoor: e.target.value === 'true' })}
        className={inputClass + ' flex-1 min-w-0'}
      >
        <option value="true">Ja (Indoor)</option>
        <option value="false">Nein (Outdoor)</option>
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
  /** Pre-fill values for new rules (e.g. from URL params) */
  initialData?: Partial<AutomationRule> | null
  onSave: (data: Partial<AutomationRule>) => void
  onCancel: () => void
  isSaving?: boolean
}

export default function RuleEditor({ rule, initialData, onSave, onCancel, isSaving }: RuleEditorProps) {
  const seed = rule ?? initialData
  const [name, setName] = useState(seed?.name ?? '')
  const [priority, setPriority] = useState(seed?.priority ?? 0)
  const [groups, setGroups] = useState<RuleConditionGroup[]>(() => normaliseGroups(rule))
  const [actions, setActions] = useState<RuleAction[]>(seed?.actions ?? [])

  const { data: gearList } = useQuery({
    queryKey: ['gear-list'],
    queryFn: () => fetchGear(false),
  })
  const { data: tagList } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })
  const { data: recorderList } = useQuery({
    queryKey: ['devices-recorders'],
    queryFn: () => fetchDevices('recorder'),
  })
  const { data: sensorList } = useQuery({
    queryKey: ['devices-sensors'],
    queryFn: () => fetchDevices('sensor'),
  })

  const tags = (tagList ?? []).map(t => ({ id: t.id, name: t.name, color: t.color }))
  const recorders = recorderList ?? []
  const sensors = sensorList ?? []

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
            className="text-sm text-accent-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 cursor-pointer"
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
                recorders={recorders}
                sensors={sensors}
              />
            </div>
          ))}
        </div>

        {/* Geofence block */}
        <div className="rounded-lg border-2 border-blue-200 dark:border-blue-800 bg-blue-50/50 dark:bg-blue-950/20 p-4 space-y-3">
          <div className="flex items-center gap-2">
            <svg className="w-4 h-4 text-blue-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0118 0z"/><circle cx="12" cy="10" r="3"/></svg>
            <span className="font-medium text-sm text-blue-700 dark:text-blue-300">Geofencing</span>
            <span className="text-xs text-blue-500">— ortsbasierte Regeln</span>
          </div>
          {/* Show status for each active geofence condition type */}
          {['geofence', 'geofence_zone', 'geofence_proximity'].some(f =>
            groups.some(g => g.conditions.some(c => c.field === f))
          ) && (
            <p className="text-xs text-blue-600 dark:text-blue-400">
              Geofence-Bedingung ist aktiv (in den Gruppen oben).
            </p>
          )}
          {/* Buttons to add each geofence type */}
          <div className="flex flex-wrap gap-2">
            {!groups.some(g => g.conditions.some(c => c.field === 'geofence')) && (
              <button
                type="button"
                onClick={() => {
                  const newCond: RuleCondition = {
                    field: 'geofence',
                    op: 'eq',
                    value: '',
                    geofence: { lat: 51.0, lon: 10.0, radius_m: 1000, mode: 'inside' },
                  }
                  if (groups.length > 0) {
                    updateGroup(0, { ...groups[0], conditions: [...groups[0].conditions, newCond] })
                  } else {
                    setGroups([{ logic: 'and', conditions: [newCond] }])
                  }
                }}
                className="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 cursor-pointer border border-blue-200 dark:border-blue-700 rounded px-2 py-1 hover:bg-blue-50 dark:hover:bg-blue-900/30 transition-colors"
              >
                + Geofence (Kreis, Legacy)
              </button>
            )}
            {!groups.some(g => g.conditions.some(c => c.field === 'geofence_zone')) && (
              <button
                type="button"
                onClick={() => {
                  const newCond: RuleCondition = {
                    field: 'geofence_zone',
                    op: 'eq',
                    value: '',
                    geofence_zone: { mode: 'inside', shape: 'circle', lat: 51.0, lon: 10.0, radius_m: 1000 },
                  }
                  if (groups.length > 0) {
                    updateGroup(0, { ...groups[0], conditions: [...groups[0].conditions, newCond] })
                  } else {
                    setGroups([{ logic: 'and', conditions: [newCond] }])
                  }
                }}
                className="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 cursor-pointer border border-blue-200 dark:border-blue-700 rounded px-2 py-1 hover:bg-blue-50 dark:hover:bg-blue-900/30 transition-colors"
              >
                + In Zone (Kreis/Polygon)
              </button>
            )}
            {!groups.some(g => g.conditions.some(c => c.field === 'geofence_proximity')) && (
              <button
                type="button"
                onClick={() => {
                  const newCond: RuleCondition = {
                    field: 'geofence_proximity',
                    op: 'eq',
                    value: '',
                    geofence_proximity: { lat: 51.0, lon: 10.0, radius_m: 1000, mode: 'inside' },
                  }
                  if (groups.length > 0) {
                    updateGroup(0, { ...groups[0], conditions: [...groups[0].conditions, newCond] })
                  } else {
                    setGroups([{ logic: 'and', conditions: [newCond] }])
                  }
                }}
                className="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 cursor-pointer border border-blue-200 dark:border-blue-700 rounded px-2 py-1 hover:bg-blue-50 dark:hover:bg-blue-900/30 transition-colors"
              >
                + Nähe zum Pin
              </button>
            )}
          </div>
        </div>

        {/* Round trip block */}
        {(() => {
          const hasRoundTripCondition = groups.some(g => g.conditions.some(c => c.field === 'is_round_trip'))
          const toggleRoundTrip = (checked: boolean) => {
            if (checked) {
              const newCond: RuleCondition = { field: 'is_round_trip', op: 'eq', value: true }
              if (groups.length > 0) {
                updateGroup(0, { ...groups[0], conditions: [...groups[0].conditions, newCond] })
              } else {
                setGroups([{ logic: 'and', conditions: [newCond] }])
              }
            } else {
              setGroups(groups.map(g => ({
                ...g,
                conditions: g.conditions.filter(c => c.field !== 'is_round_trip'),
              })).filter(g => g.conditions.length > 0))
            }
          }
          return (
            <div className="flex items-center gap-3 rounded-lg border border-neutral-200 dark:border-neutral-700 p-3">
              <svg className="w-4 h-4 text-neutral-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 12a9 9 0 109 9"/><path d="M3 12V6h6"/></svg>
              <span className="text-sm font-medium">Rundtour</span>
              <div className="ml-auto">
                <Toggle
                  checked={hasRoundTripCondition}
                  onChange={() => toggleRoundTrip(!hasRoundTripCondition)}
                  aria-label="Rundtour"
                />
              </div>
            </div>
          )
        })()}
      </div>

      {/* Actions */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Aktionen *</h3>
          <button
            type="button"
            onClick={addAction}
            className="text-sm text-accent-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 cursor-pointer"
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
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}
