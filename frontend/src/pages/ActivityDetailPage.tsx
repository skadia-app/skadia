import { useState, useRef, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { fetchActivity, fetchActivityStreams, fetchActivitySensors, updateActivity, reprocessActivity } from '@/api/activities'
import { fetchTags, createTag } from '@/api/tags'
import type { Activity, StreamPoint, Sensor } from '@/api/types'
// Tag type imported via api/tags is used indirectly through fetchTags
import { SportIcon } from '@/components/SportIcon'
import ActivityCharts, { type ZoomRange } from '@/components/ActivityCharts'
import MapFullscreen from '@/components/MapFullscreen'
import {
  formatDuration, formatDistance, formatElevationGain, formatElevationLoss,
  sportLabel, sportColor, showIndoorBadge,
} from '@/lib/utils'
import EnrichmentTags from '@/components/EnrichmentTags'
import WeatherBadge from '@/components/WeatherBadge'
import GeoTagsList from '@/components/GeoTagsList'
import GearSelector from '@/components/GearSelector'

export default function ActivityDetailPage() {
  const { id } = useParams<{ id: string }>()

  const { data: activity, isLoading } = useQuery({
    queryKey: ['activity', id],
    queryFn: () => fetchActivity(id!),
    enabled: !!id,
  })

  const { data: streams } = useQuery({
    queryKey: ['activity-streams', id],
    queryFn: () => fetchActivityStreams(id!),
    enabled: !!id,
  })

  const { data: sensors } = useQuery({
    queryKey: ['activity-sensors', id],
    queryFn: () => fetchActivitySensors(id!),
    enabled: !!id,
  })

  const [hoverIndex, setHoverIndex] = useState<number | null>(null)
  const [zoomRange, setZoomRange] = useState<ZoomRange | null>(null)
  const [mapFullscreen, setMapFullscreen] = useState(false)

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Aktivität...</div>
  }

  if (!activity) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <span className="text-neutral-400">Aktivität nicht gefunden</span>
        <Link to="/activities" className="text-sm text-orange-500 hover:text-orange-600">Zurück zur Liste</Link>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Back link */}
      <Link
        to="/activities"
        className="inline-flex items-center gap-1 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
      >
        <span>&#8592;</span> Alle Aktivitäten
      </Link>

      {/* Header */}
      <ActivityHeader activity={activity} />

      {/* Tags (user + enrichment) in one row */}
      <div className="flex flex-wrap items-center gap-2">
        <TagsBadges tags={activity.tags} activityId={activity.id} />
        <EnrichmentTags
          timeOfDay={activity.time_of_day}
          isRoundTrip={activity.is_round_trip}
          weatherTags={activity.weather_tags}
        />
      </div>

      {/* Sunrise / Sunset */}
      <SunInfo sunriseAt={activity.sunrise_at} sunsetAt={activity.sunset_at} />

      {/* Stats */}
      <StatsGrid activity={activity} />

      {/* Weather */}
      {activity.weather_temp_c != null && (
        <WeatherBadge
          tempC={activity.weather_temp_c}
          condition={activity.weather_condition}
          humidity={activity.weather_humidity}
          windSpeedMS={activity.weather_wind_speed_ms}
        />
      )}

      {/* Geo Tags */}
      <GeoTagsList activityId={activity.id} />

      {/* Map */}
      <TrackMap
        streams={streams ?? []}
        hoverIndex={hoverIndex}
        sportColor={sportColor(activity.sport_type)}
        hasGps={activity.has_gps}
        zoomRange={zoomRange}
        onMaximize={() => setMapFullscreen(true)}
      />

      {/* Map fullscreen overlay */}
      {mapFullscreen && streams && streams.length > 0 && (
        <MapFullscreen
          streams={streams}
          sportColor={sportColor(activity.sport_type)}
          onClose={() => setMapFullscreen(false)}
        />
      )}

      {/* Charts */}
      {streams && streams.length > 0 && (
        <ActivityCharts
          streams={streams}
          onHover={setHoverIndex}
          onZoomChange={setZoomRange}
          sunriseAt={activity.sunrise_at}
          sunsetAt={activity.sunset_at}
        />
      )}

      {/* Gear */}
      <GearSelector activityId={activity.id} currentGearId={activity.gear_id} />

      {/* Visibility & Privacy */}
      <VisibilitySettings activity={activity} />

      {/* Sensors */}
      {sensors && sensors.length > 0 && <SensorList sensors={sensors} />}
    </div>
  )
}

// ── Header ────────────────────────────────────────────────────────────────────

function ActivityHeader({ activity }: { activity: Activity }) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [titleDraft, setTitleDraft] = useState(activity.title ?? '')
  const [reprocessState, setReprocessState] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const inputRef = useRef<HTMLInputElement>(null)

  const mutation = useMutation({
    mutationFn: (title: string) => updateActivity(activity.id, { title }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activity.id], updated)
      setEditing(false)
    },
  })

  const reprocessMutation = useMutation({
    mutationFn: () => reprocessActivity(activity.id),
    onMutate: () => setReprocessState('loading'),
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activity.id], updated)
      queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-sensors', activity.id] })
      setReprocessState('success')
      setTimeout(() => setReprocessState('idle'), 2000)
    },
    onError: () => {
      setReprocessState('error')
      setTimeout(() => setReprocessState('idle'), 3000)
    },
  })

  useEffect(() => {
    if (editing) inputRef.current?.focus()
  }, [editing])

  function commitTitle() {
    const trimmed = titleDraft.trim()
    if (trimmed && trimmed !== activity.title) {
      mutation.mutate(trimmed)
    } else {
      setTitleDraft(activity.title ?? '')
      setEditing(false)
    }
  }

  const label = sportLabel(activity.sport_type, activity.sub_type)
  const date = new Date(activity.started_at)

  return (
    <div className="flex items-start gap-4">
      <div className="mt-1">
        <SportIcon sport={activity.sport_type} subType={activity.sub_type} className="w-8 h-8 text-neutral-500 dark:text-neutral-400" />
      </div>
      <div className="flex-1 min-w-0">
        {editing ? (
          <input
            ref={inputRef}
            value={titleDraft}
            onChange={(e) => setTitleDraft(e.target.value)}
            onBlur={commitTitle}
            onKeyDown={(e) => {
              if (e.key === 'Enter') commitTitle()
              if (e.key === 'Escape') { setTitleDraft(activity.title ?? ''); setEditing(false) }
            }}
            className="w-full text-2xl font-semibold bg-transparent border-b-2 border-orange-400 outline-none
                       text-neutral-900 dark:text-neutral-100 pb-1"
          />
        ) : (
          <h1
            onClick={() => { setTitleDraft(activity.title ?? ''); setEditing(true) }}
            className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100 cursor-pointer
                       hover:text-orange-500 transition-colors truncate"
            title="Klicken zum Bearbeiten"
          >
            {activity.title ?? <span className="text-neutral-400 italic font-normal">Kein Titel</span>}
          </h1>
        )}
        {!activity.title && activity.title_auto && (
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-0.5 truncate">{activity.title_auto}</p>
        )}
        <div className="flex items-center gap-3 mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          <span>{label}</span>
          {showIndoorBadge(activity.is_indoor, activity.sub_type) && (
            <span className="text-xs text-neutral-400 dark:text-neutral-600">indoor</span>
          )}
          <span>{format(date, 'EEEE, dd. MMMM yyyy', { locale: de })}</span>
          <span className="text-neutral-400 dark:text-neutral-600">{format(date, 'HH:mm')} Uhr</span>
          {activity.source_device && (
            <span className="text-neutral-400 dark:text-neutral-600">{activity.source_device}</span>
          )}
          {activity.start_city && (
            <span className="text-neutral-400 dark:text-neutral-600">{activity.start_city}</span>
          )}
        </div>
      </div>

      {/* Reprocess button */}
      <button
        onClick={() => reprocessMutation.mutate()}
        disabled={reprocessState === 'loading'}
        className="mt-1 flex items-center justify-center w-8 h-8 rounded-lg
                   text-neutral-400 dark:text-neutral-500
                   hover:bg-neutral-100 dark:hover:bg-neutral-800
                   hover:text-neutral-600 dark:hover:text-neutral-300
                   disabled:opacity-50 disabled:cursor-not-allowed
                   transition-colors cursor-pointer"
        title="Aktivität neu verarbeiten"
      >
        {reprocessState === 'loading' ? (
          <svg className="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
            <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2.5" strokeDasharray="50 20" />
          </svg>
        ) : reprocessState === 'success' ? (
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4 text-green-500">
            <path fillRule="evenodd" d="M16.704 4.153a.75.75 0 01.143 1.052l-8 10.5a.75.75 0 01-1.127.075l-4.5-4.5a.75.75 0 011.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 011.05-.143z" clipRule="evenodd" />
          </svg>
        ) : reprocessState === 'error' ? (
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4 text-red-500">
            <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-8-5a.75.75 0 01.75.75v4.5a.75.75 0 01-1.5 0v-4.5A.75.75 0 0110 5zm0 10a1 1 0 100-2 1 1 0 000 2z" clipRule="evenodd" />
          </svg>
        ) : (
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
            <path fillRule="evenodd" d="M15.312 11.424a5.5 5.5 0 01-9.201 2.466l-.312-.311h2.451a.75.75 0 000-1.5H4.5a.75.75 0 00-.75.75v3.75a.75.75 0 001.5 0v-2.033a7 7 0 0011.712-3.138.75.75 0 00-1.449-.391zm-10.624-3.85a5.5 5.5 0 019.201-2.465l.312.31H11.75a.75.75 0 000 1.5h3.75a.75.75 0 00.75-.75V2.419a.75.75 0 00-1.5 0v2.034A7 7 0 003.038 7.59a.75.75 0 001.449.392z" clipRule="evenodd" />
          </svg>
        )}
      </button>
    </div>
  )
}

// ── Stats Grid ────────────────────────────────────────────────────────────────

function StatsGrid({ activity }: { activity: Activity }) {
  const gain = formatElevationGain(activity.elevation_gain_m)
  const loss = formatElevationLoss(activity.elevation_loss_m)

  const stats: { label: string; value: string | null }[] = [
    { label: 'Distanz', value: formatDistance(activity.distance_m) },
    { label: 'Dauer', value: formatDuration(activity.elapsed_time_s ?? 0) },
    { label: 'Bewegungszeit', value: activity.moving_time_s ? formatDuration(activity.moving_time_s) : null },
    { label: 'Anstieg', value: gain },
    { label: 'Abstieg', value: loss },
    { label: 'Avg HR', value: activity.avg_heart_rate != null ? `${Math.round(activity.avg_heart_rate)} bpm` : null },
    { label: 'Max HR', value: activity.max_heart_rate != null ? `${activity.max_heart_rate} bpm` : null },
    { label: 'Avg Power', value: activity.avg_power_w != null ? `${Math.round(activity.avg_power_w)} W` : null },
    { label: 'NP', value: activity.normalized_power_w != null ? `${Math.round(activity.normalized_power_w)} W` : null },
    { label: 'Avg Cadence', value: activity.avg_cadence != null ? `${Math.round(activity.avg_cadence)} rpm` : null },
    { label: 'TSS', value: activity.tss != null ? `${Math.round(activity.tss)}` : null },
    { label: 'Avg Temp', value: activity.avg_temperature_c != null ? `${Math.round(activity.avg_temperature_c)} °C` : null },
  ]

  const visible = stats.filter(s => s.value && s.value !== '—')

  if (visible.length === 0) return null

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
      {visible.map(({ label, value }) => (
        <div key={label} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
          <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{label}</div>
          <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">{value}</div>
        </div>
      ))}
    </div>
  )
}

// ── Tags ──────────────────────────────────────────────────────────────────────

function TagsBadges({ tags, activityId }: { tags: string[]; activityId: string }) {
  const queryClient = useQueryClient()
  const [addOpen, setAddOpen] = useState(false)
  const [search, setSearch] = useState('')
  const popoverRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  const colors = [
    'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
    'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
    'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
    'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
  ]

  const removeMut = useMutation({
    mutationFn: (tagToRemove: string) =>
      updateActivity(activityId, { tags: tags.filter(t => t !== tagToRemove) }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activityId], updated)
    },
  })

  const addMut = useMutation({
    mutationFn: (tagToAdd: string) =>
      updateActivity(activityId, { tags: [...tags, tagToAdd] }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activityId], updated)
      setSearch('')
      setAddOpen(false)
    },
  })

  const createAndAddMut = useMutation({
    mutationFn: async (name: string) => {
      await createTag({ name })
      return updateActivity(activityId, { tags: [...tags, name] })
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activityId], updated)
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      setSearch('')
      setAddOpen(false)
    },
  })

  const { data: allTags } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    enabled: addOpen,
  })

  // Close popover on click outside
  useEffect(() => {
    if (!addOpen) return
    function handleClick(e: MouseEvent) {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setAddOpen(false)
        setSearch('')
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [addOpen])

  // Focus search on open
  useEffect(() => {
    if (addOpen) searchRef.current?.focus()
  }, [addOpen])

  const tagsLower = new Set(tags.map(t => t.toLowerCase()))
  const filtered = (allTags ?? [])
    .filter(t => !tagsLower.has(t.name.toLowerCase()))
    .filter(t => !search || t.name.toLowerCase().includes(search.toLowerCase()))
    .sort((a, b) => b.activity_count - a.activity_count)

  const searchTrimmed = search.trim()
  const exactMatch = (allTags ?? []).some(t => t.name.toLowerCase() === searchTrimmed.toLowerCase())
  const showCreate = searchTrimmed.length > 0 && !exactMatch

  return (
    <>
      {/* Add tag button — left of first tag */}
      <div className="relative" ref={popoverRef}>
        <button
          onClick={() => setAddOpen(!addOpen)}
          className="inline-flex items-center justify-center w-7 h-7 rounded-full text-xs font-medium cursor-pointer
                     bg-neutral-100 text-neutral-500 hover:bg-neutral-200 dark:bg-neutral-800 dark:text-neutral-400 dark:hover:bg-neutral-700 transition-colors"
          title="Tag hinzufügen"
        >
          +
        </button>

        {addOpen && (
          <div className="absolute left-0 top-full mt-1 z-50 w-56 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg">
            <div className="p-2">
              <input
                ref={searchRef}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Tag suchen..."
                className="w-full px-2 py-1.5 text-sm rounded border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
              />
            </div>
            <div className="max-h-48 overflow-y-auto">
              {filtered.map((t) => (
                <button
                  key={t.id}
                  onClick={() => addMut.mutate(t.name)}
                  className="w-full text-left px-3 py-1.5 text-sm text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer flex items-center justify-between"
                >
                  <span>{t.name}</span>
                  <span className="text-xs text-neutral-400">{t.activity_count}</span>
                </button>
              ))}
              {showCreate && (
                <button
                  onClick={() => createAndAddMut.mutate(searchTrimmed)}
                  className="w-full text-left px-3 py-1.5 text-sm text-orange-600 dark:text-orange-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
                >
                  &ldquo;{searchTrimmed}&rdquo; erstellen
                </button>
              )}
              {filtered.length === 0 && !showCreate && (
                <div className="px-3 py-2 text-sm text-neutral-400">Keine Tags gefunden</div>
              )}
            </div>
          </div>
        )}
      </div>

      {tags.map((tag, i) => (
        <span
          key={tag}
          className={`inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium ${colors[i % colors.length]}`}
        >
          {tag}
          <button
            onClick={() => removeMut.mutate(tag)}
            className="text-xs opacity-50 hover:opacity-100 ml-1 cursor-pointer"
            title={`Tag "${tag}" entfernen`}
          >
            &times;
          </button>
        </span>
      ))}
    </>
  )
}

// ── Sun Info ──────────────────────────────────────────────────────────────────

function SunInfo({ sunriseAt, sunsetAt }: { sunriseAt?: string | null; sunsetAt?: string | null }) {
  if (!sunriseAt && !sunsetAt) return null

  const fmt = (iso: string) => {
    const d = new Date(iso)
    return d.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
  }

  return (
    <div className="flex items-center gap-4 text-sm text-neutral-600 dark:text-neutral-400">
      {sunriseAt && (
        <span className="flex items-center gap-1.5">
          <span className="text-amber-500">&#9728;</span>
          <span>{fmt(sunriseAt)}</span>
        </span>
      )}
      {sunsetAt && (
        <span className="flex items-center gap-1.5">
          <span className="text-orange-600">&#9790;</span>
          <span>{fmt(sunsetAt)}</span>
        </span>
      )}
    </div>
  )
}

// ── Map ───────────────────────────────────────────────────────────────────────

function TrackMap({ streams, hoverIndex, sportColor, hasGps, zoomRange, onMaximize }: {
  streams: StreamPoint[]
  hoverIndex: number | null
  sportColor: string
  hasGps: boolean
  zoomRange: import('@/components/ActivityCharts').ZoomRange | null
  onMaximize: () => void
}) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)

  // Build GeoJSON from stream lat/lng
  const coordinates = streams
    .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
    .map(s => [s.lng, s.lat] as [number, number])

  const hasTrack = coordinates.length >= 2

  // Initialize map
  useEffect(() => {
    if (!mapContainerRef.current || !hasTrack) return

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: {
        version: 8,
        sources: {
          osm: {
            type: 'raster',
            tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
            tileSize: 256,
            attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
          },
        },
        layers: [
          {
            id: 'osm-tiles',
            type: 'raster',
            source: 'osm',
            minzoom: 0,
            maxzoom: 19,
          },
        ],
      },
      attributionControl: {},
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      // Full track — greyed out when a segment is highlighted
      map.addSource('track', {
        type: 'geojson',
        data: {
          type: 'Feature',
          properties: {},
          geometry: { type: 'LineString', coordinates },
        },
      })
      map.addLayer({
        id: 'track-line',
        type: 'line',
        source: 'track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': sportColor, 'line-width': 3, 'line-opacity': 0.85 },
      })

      // Highlight segment — shown on top in sport color when a zoom range is active
      map.addSource('highlight', {
        type: 'geojson',
        data: { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: [] } },
      })
      map.addLayer({
        id: 'highlight-line',
        type: 'line',
        source: 'highlight',
        layout: { 'line-join': 'round', 'line-cap': 'round', visibility: 'none' },
        paint: { 'line-color': sportColor, 'line-width': 4, 'line-opacity': 1 },
      })

      const bounds = new maplibregl.LngLatBounds()
      for (const c of coordinates) bounds.extend(c)
      map.fitBounds(bounds, { padding: 40, maxZoom: 15 })
    })

    mapRef.current = map
    return () => { map.remove() }
  }, [hasTrack]) // eslint-disable-line react-hooks/exhaustive-deps

  // Hover-Marker: Index bezieht sich auf das volle streams-Array,
  // nicht auf das gefilterte coordinates-Array (Index-Mismatch vermeiden)
  useEffect(() => {
    const map = mapRef.current
    if (!map || !hasTrack) return

    const point = hoverIndex != null ? streams[hoverIndex] : null
    const hasPos = point?.lat != null && point?.lng != null

    if (hasPos) {
      const lngLat: [number, number] = [point.lng!, point.lat!]
      if (!markerRef.current) {
        const el = document.createElement('div')
        el.className = 'track-hover-marker'
        el.style.cssText = 'width:12px;height:12px;border-radius:50%;background:#fff;border:2px solid #f97316;box-shadow:0 0 4px rgba(0,0,0,0.3);'
        markerRef.current = new maplibregl.Marker({ element: el }).setLngLat(lngLat).addTo(map)
      } else {
        markerRef.current.setLngLat(lngLat)
      }
    } else if (markerRef.current) {
      markerRef.current.remove()
      markerRef.current = null
    }
  }, [hoverIndex, streams, hasTrack])

  // Chart brush selection → grey out full track, highlight selected segment, fit bounds
  useEffect(() => {
    const map = mapRef.current
    if (!map || !hasTrack) return

    const applyZoom = () => {
      if (zoomRange) {
        // Use exact stream indices from the chart's X-axis lookup.
        // This is accurate regardless of GPS point density distribution.
        const segCoords = streams
          .slice(zoomRange.startIdx, zoomRange.endIdx + 1)
          .filter((s): s is StreamPoint & { lat: number; lng: number } =>
            s.lat != null && s.lng != null)
          .map(s => [s.lng, s.lat] as [number, number])

        // Grey out the full track
        map.setPaintProperty('track-line', 'line-color', '#999999')
        map.setPaintProperty('track-line', 'line-opacity', 0.3)

        // Update and show the highlight segment
        ;(map.getSource('highlight') as maplibregl.GeoJSONSource | undefined)?.setData({
          type: 'Feature',
          properties: {},
          geometry: { type: 'LineString', coordinates: segCoords },
        })
        map.setLayoutProperty('highlight-line', 'visibility', 'visible')

        // Fit the map to the highlighted segment
        if (segCoords.length >= 2) {
          const bounds = new maplibregl.LngLatBounds()
          for (const c of segCoords) bounds.extend(c)
          map.fitBounds(bounds, { padding: 50, maxZoom: 18, duration: 600 })
        }
      } else {
        // Restore full track
        map.setPaintProperty('track-line', 'line-color', sportColor)
        map.setPaintProperty('track-line', 'line-opacity', 0.85)
        map.setLayoutProperty('highlight-line', 'visibility', 'none')
        ;(map.getSource('highlight') as maplibregl.GeoJSONSource | undefined)?.setData({
          type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: [] },
        })

        const bounds = new maplibregl.LngLatBounds()
        for (const c of coordinates) bounds.extend(c)
        map.fitBounds(bounds, { padding: 40, maxZoom: 15, duration: 600 })
      }
    }

    if (map.isStyleLoaded()) {
      applyZoom()
    } else {
      map.once('load', applyZoom)
    }
  }, [zoomRange]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!hasGps) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 flex items-center justify-center h-48">
        <span className="text-sm text-neutral-400">Keine GPS-Daten</span>
      </div>
    )
  }

  if (!hasTrack && streams.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 flex items-center justify-center h-48">
        <span className="text-sm text-neutral-400">Lade Kartendaten...</span>
      </div>
    )
  }

  if (!hasTrack) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 flex items-center justify-center h-48">
        <span className="text-sm text-neutral-400">Keine GPS-Daten</span>
      </div>
    )
  }

  return (
    <div className="relative rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={mapContainerRef} className="h-[400px] w-full" />
      {/* Maximize button */}
      <button
        onClick={onMaximize}
        className="absolute top-3 left-3 z-10 flex items-center justify-center w-8 h-8
                   rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm
                   text-neutral-600 dark:text-neutral-300 shadow-md cursor-pointer
                   hover:bg-white dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-neutral-100
                   transition-colors"
        title="Karte maximieren"
      >
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
          <path d="M13.28 7.78l3.22-3.22v2.69a.75.75 0 001.5 0v-4.5a.75.75 0 00-.75-.75h-4.5a.75.75 0 000 1.5h2.69l-3.22 3.22a.75.75 0 001.06 1.06zM2 17.25v-4.5a.75.75 0 011.5 0v2.69l3.22-3.22a.75.75 0 011.06 1.06L4.56 16.5h2.69a.75.75 0 010 1.5h-4.5a.75.75 0 01-.75-.75z" />
        </svg>
      </button>
    </div>
  )
}

// ── Visibility & Privacy ──────────────────────────────────────────────────────
// TODO: Backend muss hide_map und data_visibility in Activity-Response aufnehmen
// Aktuell nur UI-Stubs — Felder werden lokal verwaltet bis das Backend sie liefert

function VisibilitySettings({ activity }: { activity: Activity }) {
  const queryClient = useQueryClient()

  const visibilityMut = useMutation({
    mutationFn: (visibility: Activity['visibility']) =>
      updateActivity(activity.id, { visibility }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activity.id], updated)
    },
  })

  // Stub state for fields not yet in API
  const [hideMap, setHideMap] = useState(false)
  const [dataVis, setDataVis] = useState({
    speed: true,
    heart_rate: true,
    power: true,
    cadence: true,
    temperature: true,
    altitude: true,
  })

  function toggleDataVis(key: keyof typeof dataVis) {
    setDataVis((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const dataVisOptions: { key: keyof typeof dataVis; label: string }[] = [
    { key: 'speed', label: 'Geschwindigkeit' },
    { key: 'heart_rate', label: 'Herzfrequenz' },
    { key: 'power', label: 'Leistung' },
    { key: 'cadence', label: 'Kadenz' },
    { key: 'temperature', label: 'Temperatur' },
    { key: 'altitude', label: 'Höhe' },
  ]

  const visibilityOptions: { value: Activity['visibility']; label: string }[] = [
    { value: 'private', label: 'Privat' },
    { value: 'bekannte', label: 'Bekannte' },
    { value: 'instanz', label: 'Instanz' },
    { value: 'federation', label: 'Federation' },
    { value: 'public', label: 'Öffentlich' },
  ]

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Sichtbarkeit</h2>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {/* Visibility dropdown */}
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Wer kann diese Aktivität sehen?
          </label>
          <select
            value={activity.visibility}
            onChange={(e) => visibilityMut.mutate(e.target.value as Activity['visibility'])}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-orange-500 focus:border-transparent"
          >
            {visibilityOptions.map(({ value, label }) => (
              <option key={value} value={value}>{label}</option>
            ))}
          </select>
        </div>

        {/* Hide map toggle */}
        <div>
          <label className="flex items-center gap-3 cursor-pointer mt-6">
            <input
              type="checkbox"
              checked={hideMap}
              onChange={() => setHideMap(!hideMap)}
              className="w-4 h-4 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500 focus:ring-1"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">
              Karte ausblenden
            </span>
          </label>
          <p className="text-xs text-neutral-400 mt-1 ml-7">
            Karte wird für andere Nutzer nicht angezeigt
          </p>
        </div>
      </div>

      {/* Data visibility checkboxes */}
      <div>
        <div className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          Sichtbare Daten
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          {dataVisOptions.map(({ key, label }) => (
            <label key={key} className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={dataVis[key]}
                onChange={() => toggleDataVis(key)}
                className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500 focus:ring-1"
              />
              <span className="text-sm text-neutral-700 dark:text-neutral-300">{label}</span>
            </label>
          ))}
        </div>
        <p className="text-xs text-neutral-400 mt-2">
          Nicht ausgewählte Daten werden für andere Nutzer ausgeblendet.
        </p>
      </div>
    </div>
  )
}

// ── Sensors ───────────────────────────────────────────────────────────────────

function SensorList({ sensors }: { sensors: Sensor[] }) {
  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Sensoren</h2>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {sensors.map(sensor => (
          <div
            key={sensor.id}
            className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 space-y-1"
          >
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                {sensor.product_name || sensor.manufacturer || 'Unbekannt'}
              </span>
              <span className="text-xs px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400">
                {sensor.source_type}
              </span>
            </div>
            {sensor.manufacturer && sensor.product_name && (
              <div className="text-xs text-neutral-500 dark:text-neutral-400">{sensor.manufacturer}</div>
            )}
            <div className="flex flex-wrap gap-1.5">
              {sensor.all_types.map(type_ => (
                <span
                  key={type_}
                  className="text-xs px-1.5 py-0.5 rounded-full bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400"
                >
                  {type_}
                </span>
              ))}
            </div>
            {sensor.battery_status && sensor.battery_status !== 'unknown' && (
              <div className="text-xs text-neutral-400">Batterie: {sensor.battery_status}</div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
