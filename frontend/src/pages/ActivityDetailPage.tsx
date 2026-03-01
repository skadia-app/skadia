import { useState, useRef, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { fetchActivity, fetchActivityStreams, fetchActivitySensors, updateActivity } from '@/api/activities'
import type { Activity, StreamPoint, Sensor } from '@/api/types'
import { SportIcon } from '@/components/SportIcon'
import ActivityCharts, { type ZoomRange } from '@/components/ActivityCharts'
import {
  formatDuration, formatDistance, formatElevationGain, formatElevationLoss,
  sportLabel, sportColor, showIndoorBadge,
} from '@/lib/utils'

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

      {/* Stats */}
      <StatsGrid activity={activity} />

      {/* Tags */}
      {activity.tags.length > 0 && <TagsBadges tags={activity.tags} />}

      {/* Map */}
      <TrackMap
        streams={streams ?? []}
        hoverIndex={hoverIndex}
        sportColor={sportColor(activity.sport_type)}
        hasGps={activity.has_gps}
        zoomRange={zoomRange}
      />

      {/* Charts */}
      {streams && streams.length > 0 && (
        <ActivityCharts streams={streams} onHover={setHoverIndex} onZoomChange={setZoomRange} />
      )}

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
  const inputRef = useRef<HTMLInputElement>(null)

  const mutation = useMutation({
    mutationFn: (title: string) => updateActivity(activity.id, { title }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activity.id], updated)
      setEditing(false)
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
        </div>
      </div>
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

function TagsBadges({ tags }: { tags: string[] }) {
  const colors = [
    'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
    'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
    'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
    'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
  ]

  return (
    <div className="flex flex-wrap gap-2">
      {tags.map((tag, i) => (
        <span
          key={tag}
          className={`px-2.5 py-1 rounded-full text-xs font-medium ${colors[i % colors.length]}`}
        >
          {tag}
        </span>
      ))}
    </div>
  )
}

// ── Map ───────────────────────────────────────────────────────────────────────

function TrackMap({ streams, hoverIndex, sportColor, hasGps, zoomRange }: {
  streams: StreamPoint[]
  hoverIndex: number | null
  sportColor: string
  hasGps: boolean
  zoomRange: import('@/components/ActivityCharts').ZoomRange | null
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
      attributionControl: true,
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

  // Hover marker
  useEffect(() => {
    const map = mapRef.current
    if (!map || !hasTrack) return

    if (hoverIndex != null && hoverIndex >= 0 && hoverIndex < coordinates.length) {
      const lngLat = coordinates[hoverIndex]
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
  }, [hoverIndex, coordinates, hasTrack])

  // Chart brush selection → grey out full track, highlight selected segment, fit bounds
  useEffect(() => {
    const map = mapRef.current
    if (!map || !hasTrack) return

    const applyZoom = () => {
      if (zoomRange) {
        const segCoords = streams
          .filter(s => s.lat != null && s.lng != null && s.distance_m != null)
          .filter(s => {
            const km = s.distance_m! / 1000
            return km >= zoomRange.startKm && km <= zoomRange.endKm
          })
          .map(s => [s.lng!, s.lat!] as [number, number])

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
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={mapContainerRef} className="h-[400px] w-full" />
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
