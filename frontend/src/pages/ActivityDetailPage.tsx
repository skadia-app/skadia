import { useState, useRef, useEffect, useCallback, useMemo, type ReactNode, type CSSProperties } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fmtActivityDate, fmtTimeFromDate } from '@/lib/format'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { fetchActivity, fetchActivityStreams, fetchActivitySensors, fetchActivityLaps, fetchActivitySplits, fetchActivityLengths, fetchActivityClimbPro, updateActivity, reprocessActivity, reprocessAllActivities, getZoneDistribution, correctActivityElevation, resetActivityElevation, correctActivityDistance, resetActivityDistance, fetchActivitySets, fetchActivityJumps, fetchActivityEvents, type ExportFormat } from '@/api/activities'
import { fetchWorkout } from '@/api/workouts'
import { getHeatmapSettings, getWeatherSettings } from '@/api/settings'
import { apiDownload } from '@/api/client'
import { fetchTags, createTag, unassignActivitiesFromTag, assignActivitiesToTag } from '@/api/tags'
import ActivityLocationTagSection from '@/components/ActivityLocationTagSection'
import type { Activity, StreamPoint, Sensor, ZoneDistribution, ActivitySet, ActivityJump, ActivityEvent, Workout } from '@/api/types'
// Tag type imported via api/tags is used indirectly through fetchTags
import { getTagBadgeStyle } from '@/lib/tagUtils'
import { SportIcon } from '@/components/SportIcon'
import ActivityCharts, { type ZoomRange, type SplitMarker } from '@/components/ActivityCharts'
import MapFullscreen from '@/components/MapFullscreen'
import {
  formatDuration, formatDistance, formatElevationGain, formatElevationLoss,
  sportColor, showIndoorBadge, triggerBlobDownload,
} from '@/lib/utils'
import { sportDetailLabel, getSegmentGroup, categoryForSport } from '@/lib/sportTaxonomy'
import { defaultSpeedMode } from '@/lib/sportFieldLabels'
import { SpeedPaceToggle } from '@/components/activity/SpeedPaceToggle'
import WeatherBadge from '@/components/WeatherBadge'
import GeoTagsList from '@/components/GeoTagsList'
import GearSelector from '@/components/GearSelector'
import ActivityPhotoSection from '@/components/ActivityPhotoSection'
import { uploadActivityPhoto, listActivityPhotos, type ActivityPhoto } from '@/api/activityPhotos'
import LapTable from '@/components/LapTable'
import SplitTable from '@/components/SplitTable'
import SwimLengthTable from '@/components/SwimLengthTable'
import ClimbProChart from '@/components/ClimbProChart'
import ClimbsSection from '@/components/ClimbsSection'
import IntervalSection from '@/components/IntervalSection'
import StopsSection from '@/components/StopsSection'
import SurfacesSection from '@/components/SurfacesSection'
import EnrichmentStatusSection from '@/components/EnrichmentStatusSection'
import ActivitySegments from '@/components/ActivitySegments'
import DetectedLapsTable from '@/components/DetectedLapsTable'
import LiftSection from '@/components/LiftSection'
import { fetchGeoTags, fetchStops } from '@/api/enrichment'
import { apiFetch } from '@/lib/auth'
import type { GeoTag, ActivityStop } from '@/api/types'
import { GEO_SCOPE_KEYS, GEO_SCOPE_LABELS } from '@/components/RecomputeModal'
import SportConflictBadge from '@/components/SportConflictBadge'
import SportConflictModal from '@/components/SportConflictModal'
import MergeDialog from '@/components/MergeDialog'
import MultisportSection from '@/components/MultisportSection'
import { MultisportMap, type MultisportMapHandle } from '@/components/activity/MultisportMap'
import MultisportEnrichments from '@/components/activity/MultisportEnrichments'
import { PiPMap } from '@/components/activity/PiPMap'
import { ChartTabs } from '@/components/activity/ChartTabs'
import { StatsGrid } from '@/components/activity/StatsGrid'
import { RunningDynamicsStats } from '@/components/activity/RunningDynamicsStats'
import { DiveStats } from '@/components/activity/DiveStats'
import { DataCorrectionsSection } from '@/components/activity/DataCorrections'
import { VisibilitySettings } from '@/components/activity/VisibilitySettings'
import { SunInfo } from '@/components/activity/SunInfo'
import { LocationLine } from '@/components/activity/LocationLine'
import { StrengthSetsTable } from '@/components/activity/StrengthSetsTable'
import { WorkoutOverview } from '@/components/activity/WorkoutOverview'
import { BallSportStats } from '@/components/activity/BallSportStats'
import { JumpSection } from '@/components/activity/JumpSection'
import { RadarSummary } from '@/components/activity/RadarSummary'
import { SensorList } from '@/components/activity/SensorList'
import RouteSimilarityCard from '@/components/analytics/RouteSimilarityCard'
import { revertFusion } from '@/api/fusion'
import { getCapabilities, getWaypointsFromActivity } from '@/api/routes'
import { useToast } from '@/lib/toast'
import { useActiveJobs } from '@/lib/useActiveJobs'

function getSportEmoji(sportType: string): string {
  const map: Record<string, string> = {
    swimming: '🏊', open_water_swimming: '🏊',
    cycling: '🚴', mountain_biking: '🚴',
    running: '🏃', trail_running: '🏃',
    transition: '⚡',
    walking: '🚶', hiking: '🚶',
  }
  return map[sportType] ?? '●'
}


export default function ActivityDetailPage() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data: activity, isLoading } = useQuery({
    queryKey: ['activity', id],
    queryFn: () => fetchActivity(id!),
    enabled: !!id,
  })

  const { data: weatherPrefs } = useQuery({
    queryKey: ['weatherSettings'],
    queryFn: getWeatherSettings,
    staleTime: Infinity,
  })
  const weatherDisplayFields = weatherPrefs?.display_fields

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

  const { data: zoneDistribution } = useQuery({
    queryKey: ['zone-distribution', id],
    queryFn: () => getZoneDistribution(id!),
    enabled: !!id,
  })

  const { data: laps } = useQuery({
    queryKey: ['activity-laps', id],
    queryFn: () => fetchActivityLaps(id!),
    enabled: !!id,
  })

  const { data: splitsData } = useQuery({
    queryKey: ['activity-splits', id],
    queryFn: () => fetchActivitySplits(id!),
    enabled: !!id,
  })

  const { data: lengths } = useQuery({
    queryKey: ['activity-lengths', id],
    queryFn: () => fetchActivityLengths(id!),
    enabled: !!id,
  })

  const { data: climbPro } = useQuery({
    queryKey: ['activity-climbpro', id],
    queryFn: () => fetchActivityClimbPro(id!),
    enabled: !!id,
  })

  const { data: geoTags } = useQuery<GeoTag[]>({
    queryKey: ['activity-geo-tags', id],
    queryFn: () => fetchGeoTags(id!),
    enabled: !!id && (activity?.has_gps ?? false),
  })

  const { data: stops } = useQuery<ActivityStop[]>({
    queryKey: ['activity-stops', id],
    queryFn: () => fetchStops(id!),
    enabled: !!id,
  })

  // Load child activities for multisport parent
  const { data: childActivities } = useQuery<Activity[]>({
    queryKey: ['activity-children', id],
    queryFn: async () => {
      const resp = await apiFetch(`/api/v1/activities/${id}/children`)
      const json = await resp.json()
      return (json as { children: Activity[] }).children ?? []
    },
    enabled: !!id && activity?.sport_type === 'multisport',
  })

  // Speed/Pace toggle — persisted per segment group
  const segGroup = activity ? getSegmentGroup(activity.sport_type) : 'generic'
  const speedStorageKey = `skadia-speed-format:${segGroup}`
  const [speedMode, setSpeedMode] = useState<'speed' | 'pace'>('speed')
  const speedModeInitRef = useRef(false)
  useEffect(() => {
    if (!activity || speedModeInitRef.current) return
    speedModeInitRef.current = true
    const key = `skadia-speed-format:${getSegmentGroup(activity.sport_type)}`
    const stored = localStorage.getItem(key)
    if (stored === 'speed' || stored === 'pace') {
      setSpeedMode(stored)
    } else {
      setSpeedMode(defaultSpeedMode(activity.sport_type))
    }
  }, [activity])
  // Reset speed mode init when navigating to a different activity
  useEffect(() => { speedModeInitRef.current = false }, [id])
  const handleSpeedModeChange = (mode: 'speed' | 'pace') => {
    setSpeedMode(mode)
    localStorage.setItem(speedStorageKey, mode)
  }

  const [hoverIndex, setHoverIndex] = useState<number | null>(null)
  const [zoomRange, setZoomRange] = useState<ZoomRange | null>(null)
  const [hoveredClimbRange, setHoveredClimbRange] = useState<{ startKm: number; endKm: number } | null>(null)
  const [mapFullscreen, setMapFullscreen] = useState(false)
  const [hoveredChildId, setHoveredChildId] = useState<string | null>(null)
  const multisportMapRef = useRef<MultisportMapHandle>(null)
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const [mapInView, setMapInView] = useState(true)

  useEffect(() => {
    const el = mapContainerRef.current
    if (!el) return
    const obs = new IntersectionObserver(
      ([entry]) => setMapInView(entry.isIntersecting),
      { threshold: 0.75 }
    )
    obs.observe(el)
    return () => obs.disconnect()
  }, [activity?.id])

  // Per-activity module visibility (persisted in localStorage)
  const [hiddenModules, setHiddenModules] = useState<Set<string>>(() => {
    try {
      const raw = localStorage.getItem(`skadia-hidden-modules-${id ?? ''}`)
      return raw ? new Set(JSON.parse(raw) as string[]) : new Set()
    } catch { return new Set() }
  })
  const toggleModule = (name: string) => {
    setHiddenModules(prev => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      try { localStorage.setItem(`skadia-hidden-modules-${id ?? ''}`, JSON.stringify([...next])) } catch { /* ignore */ }
      return next
    })
  }

  // Drag & drop photo upload — page-level
  const [isDraggingPhotos, setIsDraggingPhotos] = useState(false)
  const dragCounter = useRef(0)

  const handleDragEnter = useCallback((e: DragEvent) => {
    if (!e.dataTransfer?.types.includes('Files')) return
    dragCounter.current += 1
    if (dragCounter.current === 1) setIsDraggingPhotos(true)
  }, [])

  const handleDragLeave = useCallback((_e: DragEvent) => {
    dragCounter.current -= 1
    if (dragCounter.current === 0) setIsDraggingPhotos(false)
  }, [])

  const handleDragOver = useCallback((e: DragEvent) => {
    e.preventDefault()
  }, [])

  // Capture phase: only resets overlay state — fires before stopPropagation() in child components (e.g. ImportHub FileDropZone)
  const handleDropCapture = useCallback((_e: DragEvent) => {
    dragCounter.current = 0
    setIsDraggingPhotos(false)
  }, [])

  // Bubble phase: handles actual upload — stopped by ImportHub's stopPropagation(), so no double-upload
  const handleDrop = useCallback((e: DragEvent) => {
    e.preventDefault()
    if (!id) return
    const files = e.dataTransfer?.files
    if (!files || files.length === 0) return
    const imageFiles = Array.from(files).filter(f => f.type.startsWith('image/'))
    if (imageFiles.length === 0) return
    Promise.allSettled(imageFiles.map(f => uploadActivityPhoto(id, f))).then((results) => {
      queryClient.invalidateQueries({ queryKey: ['activity-photos', id] })
      const duplicates = results.filter(r => r.status === 'fulfilled' && r.value.status === 'duplicate').length
      if (duplicates > 0) {
        addToast({ type: 'warning', title: `${duplicates} Foto${duplicates > 1 ? 's' : ''} bereits vorhanden`, message: 'Duplikate wurden nicht erneut hochgeladen.' })
      }
    })
  }, [id, queryClient])

  useEffect(() => {
    window.addEventListener('dragenter', handleDragEnter)
    window.addEventListener('dragleave', handleDragLeave)
    window.addEventListener('dragover', handleDragOver)
    window.addEventListener('drop', handleDropCapture, true)
    window.addEventListener('drop', handleDrop)
    return () => {
      window.removeEventListener('dragenter', handleDragEnter)
      window.removeEventListener('dragleave', handleDragLeave)
      window.removeEventListener('dragover', handleDragOver)
      window.removeEventListener('drop', handleDropCapture, true)
      window.removeEventListener('drop', handleDrop)
    }
  }, [handleDragEnter, handleDragLeave, handleDragOver, handleDropCapture, handleDrop])

  // Load child streams for multisport combined chart
  const { data: childStreams } = useQuery({
    queryKey: ['multisport-streams', id, childActivities?.map(c => c.id)],
    queryFn: async () => {
      if (!childActivities || childActivities.length === 0) return null
      const results = await Promise.all(
        childActivities.map(child => fetchActivityStreams(child.id))
      )
      return results // StreamPoint[][] — one array per child
    },
    enabled: !!id && activity?.sport_type === 'multisport' && !!childActivities && childActivities.length > 0,
  })

  // Compute merged streams + split markers for multisport combined chart
  const multisportChartData = useMemo(() => {
    if (!childStreams || !childActivities || childStreams.length === 0) return null

    const pairs = childActivities
      .map((child, i) => ({ child, streams: childStreams[i] ?? [] }))
      .filter(p => p.streams.length > 0)
      .sort((a, b) => new Date(a.child.started_at).getTime() - new Date(b.child.started_at).getTime())

    if (pairs.length === 0) return null

    const merged: StreamPoint[] = []
    let distanceOffset = 0
    const splitMarkers: SplitMarker[] = []

    const t0 = new Date(pairs[0].streams[0].time).getTime()

    for (let i = 0; i < pairs.length; i++) {
      const { streams: childStream } = pairs[i]

      if (i > 0 && merged.length > 0) {
        const prevSport = pairs[i - 1].child.sport_type
        const currSport = pairs[i].child.sport_type
        const elapsedMin = (new Date(childStream[0].time).getTime() - t0) / 60000
        const distKm = distanceOffset / 1000
        const label = `${getSportEmoji(prevSport)}→${getSportEmoji(currSport)}`
        splitMarkers.push({ elapsedMin, distanceKm: distKm, label })
      }

      for (const point of childStream) {
        const adjustedPoint: StreamPoint = { ...point }
        if (point.distance_m != null) {
          adjustedPoint.distance_m = point.distance_m + distanceOffset
        }
        merged.push(adjustedPoint)
      }

      const lastPoint = childStream[childStream.length - 1]
      if (lastPoint?.distance_m != null) {
        distanceOffset += lastPoint.distance_m
      }
    }

    return { merged, splitMarkers }
  }, [childStreams, childActivities])

  // Multisport hover handler — must be unconditional (Rules of Hooks)
  const handleMultisportHover = useCallback((index: number | null) => {
    if (index == null) {
      multisportMapRef.current?.setHoverPoint(null)
      return
    }
    const pt = multisportChartData?.merged[index]
    if (pt?.lat != null && pt?.lng != null) {
      multisportMapRef.current?.setHoverPoint({ lat: pt.lat, lng: pt.lng })
    }
  }, [multisportChartData])

  // Tags für Standort-Sektion bei Indoor-Aktivitäten laden
  const { data: allTagsForLocation = [] } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    enabled: activity != null && !activity.has_gps,
  })
  const locationTagsForActivity = allTagsForLocation.filter(t => t.lat != null && t.lng != null)
  const assignedLocationTag = locationTagsForActivity.find(t => (activity?.tags ?? []).includes(t.name)) ?? null

  const { data: sets } = useQuery<ActivitySet[]>({
    queryKey: ['activity-sets', id],
    queryFn: () => fetchActivitySets(id!),
    enabled: !!activity && ['strength_training', 'training', 'strength'].includes(activity.sport_type),
  })

  const { data: workout } = useQuery<Workout>({
    queryKey: ['workout', activity?.workout_id],
    queryFn: () => fetchWorkout(activity!.workout_id!),
    enabled: !!activity?.workout_id,
  })

  const { data: jumps } = useQuery<ActivityJump[]>({
    queryKey: ['activity-jumps', id],
    queryFn: () => fetchActivityJumps(id!),
    enabled: !!activity && (activity.jump_count ?? 0) > 0,
  })

  const { data: events } = useQuery<ActivityEvent[]>({
    queryKey: ['activity-events', id],
    queryFn: () => fetchActivityEvents(id!),
    enabled: !!activity && activity.has_gps && getSegmentGroup(activity.sport_type) === 'bike',
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Aktivität...</div>
  }

  if (!activity) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <span className="text-neutral-400">Aktivität nicht gefunden</span>
        <Link to="/activities" className="text-sm text-accent-DEFAULT hover:text-accent-600">Zurück zur Liste</Link>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Drag & drop overlay */}
      {isDraggingPhotos && (
        <div className="fixed inset-0 z-50 bg-black/40 backdrop-blur-sm flex items-center justify-center pointer-events-none">
          <div className="flex flex-col items-center gap-4 text-white">
            <svg xmlns="http://www.w3.org/2000/svg" className="h-16 w-16 opacity-90" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909M3 21h18M3.75 3h16.5A.75.75 0 0121 3.75v13.5a.75.75 0 01-.75.75H3.75A.75.75 0 013 17.25V3.75A.75.75 0 013.75 3z" />
            </svg>
            <span className="text-2xl font-semibold tracking-wide">Fotos hier ablegen</span>
          </div>
        </div>
      )}

      {/* Back link */}
      <div className="flex items-center gap-4 flex-wrap">
        <Link
          to="/activities"
          className="inline-flex items-center gap-1 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
        >
          <span>&#8592;</span> Alle Aktivitäten
        </Link>
        {activity.parent_id && (
          <Link
            to={`/activities/${activity.parent_id}`}
            className="inline-flex items-center gap-1 text-sm text-accent-DEFAULT hover:text-accent-600 transition-colors"
          >
            <span>&#8599;</span> Multisport-Aktivität
          </Link>
        )}
      </div>

      {/* Header */}
      <ActivityHeader activity={activity} childCount={childActivities?.length ?? 0} />

      {/* Tags in one row */}
      <div className="flex flex-wrap items-center gap-2">
        <TagsBadges tags={activity.tags} activityId={activity.id} isVirtual={activity.is_virtual} />
      </div>

      {/* Description */}
      {activity.description && (
        <p className="text-sm text-neutral-600 dark:text-neutral-400 whitespace-pre-wrap leading-relaxed">
          {activity.description}
        </p>
      )}

      {/* Sunrise / Sunset */}
      <SunInfo sunriseAt={activity.sunrise_at} sunsetAt={activity.sunset_at} timezone={activity.timezone} />

      {/* Location */}
      <LocationLine activity={activity} />

      {/* Radar Threats */}
      <RadarSummary events={events} />

      {/* Stats — hide the standard grid for multisport parents (we show custom stats below) */}
      {activity.sport_type !== 'multisport' && (
        <div className="space-y-2">
          {activity.avg_speed_ms != null && (
            <div className="flex justify-end">
              <SpeedPaceToggle
                mode={speedMode}
                onChange={handleSpeedModeChange}
              />
            </div>
          )}
          <StatsGrid activity={activity} speedMode={speedMode} />
          {getSegmentGroup(activity.sport_type) === 'foot' && activity.avg_vertical_oscillation_mm != null && (
            <RunningDynamicsStats activity={activity} />
          )}
          {activity.sport_type === 'diving' && (
            <DiveStats activity={activity} />
          )}
        </div>
      )}

      {/* Workout visualization */}
      {workout && <WorkoutOverview workout={workout} />}

      {/* Ballsport Metriken */}
      {categoryForSport(activity.sport_type).key === 'game' && <BallSportStats activity={activity} />}

      {/* Multisport parent — combined view */}
      {activity.sport_type === 'multisport' && (
        <div className="space-y-4">
          {/* Combined map */}
          <MultisportMap
            ref={multisportMapRef}
            activityId={activity.id}
            className="h-80 w-full rounded-xl overflow-hidden border border-neutral-200 dark:border-neutral-800"
            hoveredActivityId={hoveredChildId}
          />

          {/* Aggregated totals */}
          <div className="grid grid-cols-3 gap-3">
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Gesamtdauer</div>
              <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
                {formatDuration(activity.elapsed_time_s ?? 0)}
              </div>
            </div>
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Gesamtdistanz</div>
              <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
                {formatDistance(activity.distance_m)}
              </div>
            </div>
            {(activity.elevation_gain_m ?? 0) > 0 && (
              <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
                <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Höhenmeter</div>
                <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
                  {Math.round(activity.elevation_gain_m ?? 0)} m
                </div>
              </div>
            )}
          </div>

          {/* Discipline list + ungroup */}
          {childActivities && childActivities.length > 0 && (
            <MultisportSectionWithUngroup
              children={childActivities}
              onHoverChild={setHoveredChildId}
            />
          )}

          {/* Kombinierte Zeitreihen-Datenströme */}
          {multisportChartData && multisportChartData.merged.length > 0 && (
            <ActivityCharts
              streams={multisportChartData.merged}
              onHover={handleMultisportHover}
              splitMarkers={multisportChartData.splitMarkers}
              defaultXMode={childActivities?.some(c => c.has_gps) ? 'distance' : 'time'}
            />
          )}

          {/* Enrichments aus Kindaktivitäten */}
          {childActivities && childActivities.length > 0 && (
            <MultisportEnrichments childActivities={childActivities} timezone={activity.timezone} />
          )}
        </div>
      )}

      {/* Erweiterte Metriken */}
      {(activity.variability_index != null || activity.efficiency_factor != null || activity.aerobic_decoupling != null || activity.workout_rpe != null || activity.workout_feel != null || activity.aerobic_training_effect != null) && (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-5 gap-3">
          {activity.variability_index != null && (
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Variabilität</div>
              <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100" title="Variabilitätsindex: NP/Ø-Power. 1.0 = gleichmäßig, >1.05 = unruhig">
                {activity.variability_index.toFixed(2)}
              </div>
            </div>
          )}
          {activity.efficiency_factor != null && (
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">EF</div>
              <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100" title="Effizienzfaktor: Ø-Power/Ø-HR. Steigt bei besserer Fitness">
                {activity.efficiency_factor.toFixed(2)}
              </div>
            </div>
          )}
          {activity.aerobic_decoupling != null && (
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Pw:Hr Drift</div>
              <div className={`text-lg font-semibold ${
                Math.abs(activity.aerobic_decoupling) < 5 ? 'text-green-600' :
                Math.abs(activity.aerobic_decoupling) < 10 ? 'text-yellow-600' : 'text-red-600'
              }`} title="Aerob. Decoupling Pw:Hr. <5% = gute Ausdauer">
                {activity.aerobic_decoupling.toFixed(1)}%
              </div>
            </div>
          )}
          {activity.workout_feel != null && (
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Gefühl</div>
              <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100" title="Subjektives Wohlbefinden beim Training">
                {(['😫', '😕', '😐', '🙂', '😄'] as const)[activity.workout_feel] ?? activity.workout_feel}
              </div>
            </div>
          )}
          {activity.workout_rpe != null && (
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">RPE</div>
              <div className={`text-lg font-semibold ${
                activity.workout_rpe <= 3 ? 'text-green-600 dark:text-green-400' :
                activity.workout_rpe <= 6 ? 'text-yellow-600 dark:text-yellow-400' :
                activity.workout_rpe <= 8 ? 'text-orange-600 dark:text-orange-400' :
                'text-red-600 dark:text-red-400'
              }`} title="Subjektive Belastung (RPE 1-10, aus Garmin-Gerät)">
                {activity.workout_rpe}/10
              </div>
            </div>
          )}
          {activity.aerobic_training_effect != null && (
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Training Effect</div>
              <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100" title="Aerober Trainingseffekt (Garmin Firstbeat, 1-5)">
                {activity.aerobic_training_effect.toFixed(1)}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Weather */}
      {activity.weather_readings && activity.weather_readings.length > 0 && (
        <div className="mt-2">
          <WeatherBadge readings={activity.weather_readings} displayFields={weatherDisplayFields} timezone={activity.timezone} />
        </div>
      )}

      {/* Geo Tags (nicht für Multisport-Parent — Enrichments werden oben in der Multisport-Sektion gezeigt) */}
      {activity.sport_type !== 'multisport' && <GeoTagsList activityId={activity.id} hiddenModules={hiddenModules} />}

      {/* Standort (nur für Indoor-Aktivitäten ohne GPS, nicht für Multisport-Parent) */}
      {!activity.has_gps && activity.sport_type !== 'multisport' && (
        <ActivityLocationTagSection
          activityId={activity.id}
          currentTags={activity.tags ?? []}
          locationTags={locationTagsForActivity}
          onUpdate={() => {}}
        />
      )}

      {/* Map (nicht für Multisport-Parent — die Karte ist oben in der Multisport-Sektion) */}
      {activity.sport_type !== 'multisport' && (
        <div ref={mapContainerRef}>
          <TrackMap
            activityId={activity.id}
            streams={streams ?? []}
            hoverIndex={hoverIndex}
            sportColor={sportColor(activity.sport_type)}
            hasGps={activity.has_gps}
            zoomRange={zoomRange}
            onMaximize={() => setMapFullscreen(true)}
            geoTags={geoTags}
            stops={stops}
            locationPin={assignedLocationTag && assignedLocationTag.lat != null && assignedLocationTag.lng != null
              ? { lat: assignedLocationTag.lat, lng: assignedLocationTag.lng, name: assignedLocationTag.name, color: assignedLocationTag.color }
              : undefined}
          />
        </div>
      )}

      {/* Map fullscreen overlay */}
      {activity.sport_type !== 'multisport' && mapFullscreen && streams && streams.length > 0 && (
        <MapFullscreen
          streams={streams}
          sportColor={sportColor(activity.sport_type)}
          onClose={() => setMapFullscreen(false)}
          activityId={activity.id}
        />
      )}

      {/* Fotos */}
      <ActivityPhotoSection activityId={activity.id} />

      {/* Charts */}
      {streams && streams.length > 0 && (
        <ChartTabs streams={streams}>
          {(activeTab) => (
            <ActivityCharts
              streams={streams}
              onHover={setHoverIndex}
              onZoomChange={setZoomRange}
              sunriseAt={activity.sunrise_at}
              sunsetAt={activity.sunset_at}
              defaultXMode={!activity.has_gps ? 'time' : 'distance'}
              highlightRange={hoveredClimbRange}
              speedMode={speedMode}
              sportType={activity.sport_type}
              activeTab={activeTab}
              gearEvents={events}
            />
          )}
        </ChartTabs>
      )}

      {/* Krafttraining Sets */}
      {sets && sets.length > 0 && <StrengthSetsTable sets={sets} />}

      {/* Sprünge */}
      {(activity.jump_count ?? 0) > 0 && <JumpSection jumpCount={activity.jump_count!} jumps={jumps} />}

      {/* Runden */}
      {laps && laps.length > 0 && <LapTable laps={laps} sportType={activity.sport_type} speedMode={speedMode} />}

      {/* Splits (Klettern zuerst) */}
      {splitsData && splitsData.splits.length > 0 && activity.sport_type === 'rock_climbing' && (
        <SplitTable splits={splitsData.splits} summaries={splitsData.summaries ?? []} sportType="rock_climbing" subType={activity.sub_type ?? undefined} activityId={activity.id} />
      )}

      {/* Bahnen */}
      {lengths && lengths.length > 0 && <SwimLengthTable lengths={lengths} />}

      {/* Splits (alle anderen) */}
      {splitsData && splitsData.splits.length > 0 && activity.sport_type !== 'rock_climbing' && (
        <SplitTable splits={splitsData.splits} summaries={splitsData.summaries ?? []} sportType={activity.sport_type} activityId={activity.id} />
      )}

      {/* ClimbPro */}
      {climbPro && climbPro.length > 0 && <ClimbProChart events={climbPro} />}

      {/* Anstiege (nur bei GPS-Aktivitäten sinnvoll) */}
      {activity.has_gps && !hiddenModules.has('ascents') && <ClimbsSection activityId={activity.id} onClimbHover={setHoveredClimbRange} />}

      {/* Intervalltraining */}
      <IntervalSection activityId={activity.id} sportType={activity.sport_type} />

      {/* Liftfahrten */}
      {(activity.lift_count ?? 0) > 0 ? (
        <LiftSection
          activityId={activity.id}
          liftCount={activity.lift_count ?? 0}
          liftDistanceM={activity.lift_distance_m}
          liftAscentM={activity.lift_ascent_m}
          liftDescentM={activity.lift_descent_m}
          liftDurationS={activity.lift_duration_s}
          distanceM={activity.distance_m ?? 0}
          totalAscentM={activity.elevation_gain_m ?? 0}
          excludeLiftSubtraction={activity.exclude_lift_subtraction ?? false}
          onToggleExclude={async (v) => {
            await apiFetch(`/api/v1/activities/${activity.id}`, {
              method: 'PATCH',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ exclude_lift_subtraction: v }),
            })
            queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
          }}
        />
      ) : null}

      {/* Stopps (nicht für Multisport-Parent) */}
      {activity.sport_type !== 'multisport' && !hiddenModules.has('stops') && <StopsSection activityId={activity.id} timezone={activity.timezone} />}

      {/* Untergrund (nicht für Multisport) */}
      {activity.sport_type !== 'multisport' && !hiddenModules.has('surfaces') && <SurfacesSection activityId={activity.id} />}

      {/* Enrichment-Modul-Status */}
      <EnrichmentStatusSection
        activityId={activity.id}
        enrichmentStatus={activity.enrichment_status}
        hiddenModules={hiddenModules}
        onToggleModule={toggleModule}
      />

      {/* Datenkorrekturen (Höhe + Distanz) */}
      <DataCorrectionsSection activity={activity} />

      {/* Erkannte Runden */}
      <DetectedLapsTable activityId={activity.id} sportType={activity.sport_type} />

      {/* Segmente (nur bei GPS-Aktivitäten) */}
      {activity.has_gps && (
        <ActivitySegments
          activityId={activity.id}
          streams={streams}
          activitySportType={activity.sport_type}
        />
      )}

      {/* Ähnliche Routen (nur bei GPS-Aktivitäten) */}
      {activity.has_gps && <RouteSimilarityCard activityId={activity.id} />}

      {/* Zonen-Verteilung */}
      {zoneDistribution && (zoneDistribution.hr_zones.length > 0 || zoneDistribution.power_zones.length > 0) && (
        <ZoneDistributionChart distribution={zoneDistribution} />
      )}

      {/* Gear */}
      <GearSelector activityId={activity.id} currentGearId={activity.gear_id} />

      {/* Visibility & Privacy */}
      <VisibilitySettings activity={activity} />

      {/* Sensors */}
      {sensors && sensors.length > 0 && <SensorList sensors={sensors} />}

      {/* PiP-Karte: erscheint wenn die Hauptkarte nicht sichtbar ist */}
      {activity.has_gps && activity.sport_type !== 'multisport' && streams && (
        <PiPMap
          streams={streams}
          sportType={activity.sport_type}
          hoverIndex={hoverIndex}
          visible={!mapInView}
          onClickScrollToMap={() => mapContainerRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })}
        />
      )}
    </div>
  )
}

// ── Reprocess Scope Modal ──────────────────────────────────────────────────────

const REPROCESS_SCOPES = [
  ['streams', 'Streams & Track'],
  ['sensors', 'Sensoren'],
  ['metrics', 'Metriken'],
  ['details', 'Runden/Splits'],
  ['segments', 'Segmente & Runden'],
  ['sport_type', 'Sportart'],
  ['tags', 'Tags'],
] as const

type ReprocessScopeKey = typeof REPROCESS_SCOPES[number][0]
type AnyReprocessKey = ReprocessScopeKey | typeof GEO_SCOPE_KEYS[number]

// Geo-Keys in "geo" Backend-Scope übersetzen (Stage 2 läuft immer als Ganzes).
function toBackendScope(sel: Set<AnyReprocessKey>): string[] {
  const result = new Set<string>()
  for (const s of sel) {
    if ((GEO_SCOPE_KEYS as readonly string[]).includes(s)) {
      result.add('geo')
    } else {
      result.add(s)
    }
  }
  return [...result]
}

function ReprocessScopeModal({
  onConfirm,
  onClose,
  isPending,
}: {
  onConfirm: (scope: string[]) => void
  onClose: () => void
  isPending: boolean
}) {
  const [scope, setScope] = useState<Set<AnyReprocessKey>>(
    new Set(['streams', 'sensors', 'metrics', 'details'])
  )

  function toggleScope(key: AnyReprocessKey) {
    setScope(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="bg-white dark:bg-neutral-900 rounded-xl shadow-xl p-6 max-w-sm w-full mx-4 space-y-4">
        <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
          Neu verarbeiten — Details auswählen
        </h3>
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Wähle, welche Teile dieser Aktivität neu verarbeitet werden sollen.
          Nutzerdefinierte Felder (Titel, Beschreibung, Sichtbarkeit) bleiben immer erhalten.
        </p>

        <div className="space-y-2">
          {REPROCESS_SCOPES.map(([key, label]) => (
            <label
              key={key}
              className="flex items-center gap-2.5 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer select-none"
            >
              <input
                type="checkbox"
                checked={scope.has(key)}
                onChange={() => toggleScope(key)}
                className="rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT"
              />
              {label}
            </label>
          ))}
          <p className="text-xs text-neutral-400 dark:text-neutral-500 pt-1">Geo-Anreicherung (ML, asynchron)</p>
          {GEO_SCOPE_KEYS.map(key => (
            <label
              key={key}
              className="flex items-center gap-2.5 text-sm text-neutral-700 dark:text-neutral-300 cursor-pointer select-none"
            >
              <input
                type="checkbox"
                checked={scope.has(key)}
                onChange={() => toggleScope(key)}
                className="rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT"
              />
              {GEO_SCOPE_LABELS[key]}
            </label>
          ))}
        </div>

        <div className="flex justify-end gap-3 pt-1">
          <button
            onClick={onClose}
            disabled={isPending}
            className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400
                       hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer
                       disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Abbrechen
          </button>
          <button
            onClick={() => onConfirm(toBackendScope(scope))}
            disabled={isPending || scope.size === 0}
            className="px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT text-white
                       hover:bg-accent-600 transition-colors cursor-pointer
                       disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isPending ? 'Verarbeite...' : 'Starten'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Header ────────────────────────────────────────────────────────────────────

function ActivityHeader({ activity, childCount }: { activity: Activity; childCount: number }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { addToast } = useToast()
  const [editing, setEditing] = useState(false)
  const [titleDraft, setTitleDraft] = useState(activity.title ?? '')
  const [reprocessState, setReprocessState] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [exportOpen, setExportOpen] = useState(false)
  const [reprocessMenuOpen, setReprocessMenuOpen] = useState(false)

  const [reprocessScopeOpen, setReprocessScopeOpen] = useState(false)
  const [smoothing, setSmoothing] = useState<'idle' | 'smoothing' | 'resetting'>('idle')
  const [smoothPopover, setSmoothPopover] = useState(false)
  // Default matches backend DefaultSpeedSmoothingWindow (half-radius).
  // Slider range 1-3 → total window 3s/5s/7s (always odd for clean median).
  const [smoothWindow, setSmoothWindow] = useState(2)
  const smoothRef = useRef<HTMLDivElement>(null)
  useClickOutside(smoothRef, () => setSmoothPopover(false), smoothPopover)
  const [multisportConfirm, setMultisportConfirm] = useState(false)
  const [ungroupConfirm, setUngroupConfirm] = useState(false)
  const [ungroupLoading, setUngroupLoading] = useState(false)
  const [mergeDialogOpen, setMergeDialogOpen] = useState(false)
  const [openInPlannerLoading, setOpenInPlannerLoading] = useState(false)
  const { data: routingCapabilities } = useQuery({
    queryKey: ['routing-capabilities'],
    queryFn: getCapabilities,
    staleTime: 5 * 60 * 1000,
  })
  const reprocessMenuRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  // Close reprocess dropdown when clicking outside
  useClickOutside(reprocessMenuRef, () => { setReprocessMenuOpen(false); setMultisportConfirm(false) }, reprocessMenuOpen)

  const handleExport = async (format: ExportFormat) => {
    setExportOpen(false)
    try {
      const { blob, filename } = await apiDownload(`/activities/${activity.id}/export?format=${format}`)
      const extMap: Record<string, string> = { original: '', gpx: '.gpx', tcx: '.tcx', fit: '.fit', zip: '.zip' }
      triggerBlobDownload(blob, filename ?? `activity${extMap[format] ?? ''}`)
    } catch (err) {
      console.error('Export fehlgeschlagen', err)
    }
  }

  const mutation = useMutation({
    mutationFn: (title: string) => updateActivity(activity.id, { title }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activity.id], updated)
      setEditing(false)
    },
  })

  async function handleExcludeChange(field: 'exclude_from_heatmap' | 'exclude_from_stats', value: boolean) {
    await apiFetch(`/api/v1/activities/${activity.id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ [field]: value }),
    })
    queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
  }

  // Track background recompute jobs — spinner stays active across page reloads
  const { isJobActiveForEntity } = useActiveJobs()

  // On mount + whenever activeJobs changes: check if a recompute job exists for THIS activity
  const hasActiveRecomputeJob = isJobActiveForEntity(activity.id)
  const prevHadJob = useRef(false)

  useEffect(() => {
    if (hasActiveRecomputeJob) {
      // Job running → show spinner
      setReprocessState('loading')
      prevHadJob.current = true
    } else if (prevHadJob.current) {
      // Job just finished → refresh all enrichment data
      prevHadJob.current = false
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-sensors', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['geo-tags', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-climbs', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-stops', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-intervals', activity.id] })
      setReprocessState('success')
      setTimeout(() => setReprocessState('idle'), 2000)
    }
  }, [hasActiveRecomputeJob])

  const reprocessMutation = useMutation({
    mutationFn: () => reprocessActivity(activity.id),
    onMutate: () => setReprocessState('loading'),
    onSuccess: (result) => {
      if (result && 'job_id' in result) {
        // 202 Accepted: background job started — spinner stays via useEffect/hasActiveRecomputeJob
        setMultisportConfirm(false)
        return
      }
      // Synchronous 200 response (legacy/fallback)
      queryClient.setQueryData(['activity', activity.id], result)
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

  // Scoped reprocess via recompute-all with ids=[activity.id] + scope
  const reprocessScopedMutation = useMutation({
    mutationFn: (scope: string[]) =>
      reprocessAllActivities({ ids: [activity.id], scope }),
    onMutate: () => {
      setReprocessState('loading')
      setReprocessScopeOpen(false)
    },
    onSuccess: () => {
      // Reload full activity data since recompute-all doesn't return the updated record
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
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

  const [showRevertFusionModal, setShowRevertFusionModal] = useState(false)
  const [conflictOpen, setConflictOpen] = useState(false)

  const revertFusionMutation = useMutation({
    mutationFn: () => revertFusion(activity.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      navigate('/activities')
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fusion konnte nicht rückgängig gemacht werden', message: 'Bitte erneut versuchen.' })
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

  const label = sportDetailLabel(activity.sport_type, activity.sub_type)
  const date = new Date(activity.started_at)
  const isReprocessing = reprocessState === 'loading'

  const reprocessIcon = isReprocessing ? (
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
  )

  return (
    <div className="flex items-start gap-4">
      <div className="mt-1">
        <SportIcon sport={activity.sport_type} subType={activity.sub_type} isEbike={activity.is_ebike} className="w-8 h-8 text-neutral-500 dark:text-neutral-400" />
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
            className="w-full text-2xl font-semibold bg-transparent border-b-2 border-accent-400 outline-none
                       text-neutral-900 dark:text-neutral-100 pb-1"
          />
        ) : (
          <h1
            onClick={() => { setTitleDraft(activity.title ?? ''); setEditing(true) }}
            className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100 cursor-pointer
                       hover:text-accent-DEFAULT transition-colors truncate"
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
          {activity.sport_conflict_pending && (
            <SportConflictBadge onClick={() => setConflictOpen(true)} />
          )}
          {showIndoorBadge(activity.is_indoor, activity.sub_type) && (
            <span className="text-xs text-neutral-400 dark:text-neutral-600">indoor</span>
          )}
          <span>{fmtActivityDate(activity.started_at, activity.timezone)}</span>
          <span className="text-neutral-400 dark:text-neutral-600">{fmtTimeFromDate(date, activity.timezone)} Uhr</span>
          {activity.recording_device_id ? (
            <Link
              to={`/hardware/detail/${activity.recording_device_id}`}
              className="text-neutral-400 dark:text-neutral-600 hover:text-accent-DEFAULT transition-colors"
            >
              {activity.recording_device_name || activity.source_device}
            </Link>
          ) : activity.source_device ? (
            <span className="text-neutral-400 dark:text-neutral-600">{activity.source_device}</span>
          ) : null}
        </div>
        {activity.fusion_sources && (
          <div className="flex items-center gap-2 mt-1">
            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-violet-100 dark:bg-violet-950/40 text-violet-700 dark:text-violet-300 border border-violet-200 dark:border-violet-800">
              ⚡ Fusion Ergebnis
            </span>
            <button
              onClick={() => setShowRevertFusionModal(true)}
              disabled={revertFusionMutation.isPending}
              className="text-xs text-neutral-500 hover:text-red-600 dark:hover:text-red-400 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Fusion rückgängig
            </button>
          </div>
        )}
        {(activity.garmin_activity_id || activity.strava_activity_id || activity.komoot_tour_id) && (
          <div className="flex gap-2 items-center text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            <span>Auf:</span>
            {activity.garmin_activity_id && (
              <a
                href={`https://connect.garmin.com/modern/activity/${activity.garmin_activity_id}`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-accent hover:underline"
              >
                Garmin Connect ↗
              </a>
            )}
            {activity.strava_activity_id && (
              <a
                href={`https://www.strava.com/activities/${activity.strava_activity_id}`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-accent hover:underline"
              >
                Strava ↗
              </a>
            )}
            {activity.komoot_tour_id && (
              <a
                href={`https://www.komoot.com/tour/${activity.komoot_tour_id}`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-accent hover:underline"
              >
                Komoot ↗
              </a>
            )}
          </div>
        )}
      </div>

      {/* Ausschluss-Checkboxen */}
      <div className="flex flex-col gap-2 mt-4">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={activity.exclude_from_heatmap ?? false}
            disabled={activity.is_virtual}
            onChange={e => handleExcludeChange('exclude_from_heatmap', e.target.checked)}
            className="rounded border-neutral-300 disabled:opacity-50 disabled:cursor-not-allowed"
          />
          <span className={activity.is_virtual ? 'text-neutral-400' : ''}>
            Von Heatmap ausschließen
          </span>
          {activity.is_virtual && <span className="text-xs text-neutral-400">(virtuell)</span>}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={activity.exclude_from_stats ?? false}
            onChange={e => handleExcludeChange('exclude_from_stats', e.target.checked)}
            className="rounded border-neutral-300"
          />
          Von Statistik ausschließen
        </label>
      </div>

      {/* Speed smoothing + reset — opt-in correction for GPS speed noise */}
      {activity.has_gps && activity.sport_type !== 'multisport' && (
        <div className="relative" ref={smoothRef}>
          <div className="flex items-center gap-1">
            <button
              disabled={smoothing !== 'idle'}
              onClick={() => setSmoothPopover(v => !v)}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors mt-1 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
            >
              {smoothing === 'smoothing' ? '…' : '⌇'} Glätten
            </button>
            <button
              disabled={smoothing !== 'idle'}
              onClick={async () => {
                setSmoothing('resetting')
                try {
                  await reprocessActivity(activity.id, ['streams'])
                  queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
                  queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
                  addToast({ type: 'success', title: 'Originaldaten wiederhergestellt' })
                } catch {
                  addToast({ type: 'error', title: 'Zurücksetzen fehlgeschlagen', duration: 0 })
                } finally {
                  setSmoothing('idle')
                }
              }}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors mt-1 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
              title="Geschwindigkeitsdaten aus Originaldatei wiederherstellen"
            >
              {smoothing === 'resetting' ? '…' : '↺'}
            </button>
          </div>
          {smoothPopover && (
            <div className="absolute top-full left-0 mt-1 z-50 w-72 bg-white dark:bg-neutral-900 rounded-xl shadow-xl border border-neutral-200 dark:border-neutral-700 p-4 space-y-3">
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                Glättet GPS-Geschwindigkeitsrauschen mit einem Median-Filter.
                Höhere Werte entfernen mehr Ausreißer, können aber echte
                Geschwindigkeitsänderungen abschwächen.
              </p>
              <div className="space-y-1.5">
                <div className="flex justify-between text-xs text-neutral-500 dark:text-neutral-400">
                  <span>Leicht</span>
                  <span className="font-medium text-neutral-700 dark:text-neutral-200">{smoothWindow * 2 + 1}s Fenster</span>
                  <span>Stark</span>
                </div>
                <input
                  type="range"
                  min={1} max={3} step={1}
                  value={smoothWindow}
                  onChange={e => setSmoothWindow(Number(e.target.value))}
                  className="w-full accent-accent-DEFAULT"
                />
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => setSmoothPopover(false)}
                  className="flex-1 px-3 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer"
                >
                  Abbrechen
                </button>
                <button
                  disabled={smoothing !== 'idle'}
                  onClick={async () => {
                    setSmoothPopover(false)
                    setSmoothing('smoothing')
                    try {
                      await reprocessActivity(activity.id, ['speed_smooth'], smoothWindow)
                      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
                      queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
                      addToast({ type: 'success', title: `Geglättet (${smoothWindow * 2 + 1}s Fenster)` })
                    } catch {
                      addToast({ type: 'error', title: 'Glätten fehlgeschlagen', duration: 0 })
                    } finally {
                      setSmoothing('idle')
                    }
                  }}
                  className="flex-1 px-3 py-1.5 text-xs rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white font-medium disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
                >
                  Anwenden
                </button>
              </div>
            </div>
          )}
        </div>
      )}
      {/* Edit Button — trim/split/merge not applicable for multisport parents */}
      {activity.sport_type !== 'multisport' && (
        <Link
          to={`/activities/${activity.id}/edit`}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors mt-1"
        >
          &#9998; Bearbeiten
        </Link>
      )}
      {/* Open in Route Planner — only when routing + offline map are available */}
      {activity.has_gps && activity.sport_type !== 'multisport' && routingCapabilities?.routing && routingCapabilities?.offline_map && (
        <button
          onClick={async () => {
            setOpenInPlannerLoading(true)
            try {
              const result = await getWaypointsFromActivity(activity.id)
              navigate('/map', { state: { plannerWaypoints: result.waypoints } })
            } catch (err) {
              addToast({
                type: 'error',
                title: 'Fehler beim Öffnen im Routenplaner',
                message: err instanceof Error ? err.message : 'Unbekannter Fehler',
                duration: 0,
              })
            } finally {
              setOpenInPlannerLoading(false)
            }
          }}
          disabled={openInPlannerLoading}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors mt-1 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          title="Track als Route im Planer öffnen"
        >
          {openInPlannerLoading ? (
            <svg className="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2.5" strokeDasharray="50 20" />
            </svg>
          ) : (
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
              <path fillRule="evenodd" d="M10 2a.75.75 0 0 1 .75.75v.258a33.186 33.186 0 0 1 6.668 1.98A3.25 3.25 0 0 1 19 7.743V12.5a3.25 3.25 0 0 1-3.25 3.25H4.25A3.25 3.25 0 0 1 1 12.5V7.743a3.25 3.25 0 0 1 1.582-2.756 33.186 33.186 0 0 1 6.668-1.98V2.75A.75.75 0 0 1 10 2Zm0 3.539a31.68 31.68 0 0 0-7.543 1.897A1.75 1.75 0 0 0 1.5 9v3.5a1.75 1.75 0 0 0 1.75 1.75h13.5a1.75 1.75 0 0 0 1.75-1.75V9a1.75 1.75 0 0 0-.957-1.564A31.68 31.68 0 0 0 10 5.539Zm-1 4.711a1 1 0 1 1 2 0 1 1 0 0 1-2 0Z" clipRule="evenodd" />
            </svg>
          )}
          Im Routenplaner öffnen
        </button>
      )}
      {/* Rename + Ungroup buttons — only for multisport parents */}
      {activity.sport_type === 'multisport' && (
        <>
          <button
            onClick={() => { setTitleDraft(activity.title ?? ''); setEditing(true) }}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors mt-1 cursor-pointer"
          >
            &#9998; Umbenennen
          </button>
          {ungroupConfirm ? (
            <div className="inline-flex items-center gap-1.5 mt-1">
              <span className="text-xs text-neutral-500 dark:text-neutral-400 whitespace-nowrap">Wirklich auflösen?</span>
              <button
                onClick={async () => {
                  setUngroupLoading(true)
                  try {
                    const resp = await apiFetch(`/api/v1/activities/${activity.id}/group`, { method: 'DELETE' })
                    if (resp.ok) {
                      queryClient.invalidateQueries({ queryKey: ['activities'] })
                      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
                      navigate('/activities')
                    } else {
                      addToast({ type: 'error', title: 'Fehler beim Auflösen der Gruppe', message: `Server antwortete mit Status ${resp.status}`, duration: 0 })
                    }
                  } catch (err) {
                    addToast({ type: 'error', title: 'Fehler beim Auflösen der Gruppe', message: err instanceof Error ? err.message : 'Unbekannter Fehler', duration: 0 })
                  } finally {
                    setUngroupLoading(false)
                    setUngroupConfirm(false)
                  }
                }}
                disabled={ungroupLoading}
                className="px-2 py-1 text-xs rounded-lg bg-red-600 hover:bg-red-700 text-white font-medium disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
              >
                {ungroupLoading ? '…' : 'Ja'}
              </button>
              <button
                onClick={() => setUngroupConfirm(false)}
                className="px-2 py-1 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
              >
                Nein
              </button>
            </div>
          ) : (
            <button
              onClick={() => setUngroupConfirm(true)}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-400 transition-colors mt-1 cursor-pointer"
            >
              Gruppe auflösen
            </button>
          )}
        </>
      )}

      {/* Merge dialog */}
      {mergeDialogOpen && (
        <MergeDialog activity={activity} onClose={() => setMergeDialogOpen(false)} />
      )}
      {/* Fusion-Revert confirmation modal */}
      {showRevertFusionModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={() => setShowRevertFusionModal(false)} />
          <div className="relative bg-white dark:bg-neutral-900 rounded-2xl shadow-2xl border border-neutral-200 dark:border-neutral-800 w-full max-w-sm p-6 space-y-4">
            <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
              Fusion rückgängig machen?
            </h2>
            <div className="flex items-start gap-2 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg p-3">
              <svg className="w-4 h-4 text-red-500 flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
              </svg>
              <p className="text-xs text-red-700 dark:text-red-400 leading-relaxed">
                Die zusammengeführte Aktivität wird dauerhaft gelöscht. Die Quellaktivitäten werden wiederhergestellt und wieder sichtbar.
              </p>
            </div>
            <div className="flex justify-end gap-3 pt-1">
              <button
                onClick={() => setShowRevertFusionModal(false)}
                disabled={revertFusionMutation.isPending}
                className="px-4 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
              >
                Abbrechen
              </button>
              <button
                onClick={() => {
                  setShowRevertFusionModal(false)
                  revertFusionMutation.mutate()
                }}
                disabled={revertFusionMutation.isPending}
                className="px-4 py-2 text-sm rounded-lg bg-red-600 hover:bg-red-700 text-white font-semibold disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer flex items-center gap-2"
              >
                {revertFusionMutation.isPending ? (
                  <>
                    <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Wird rückgängig gemacht…
                  </>
                ) : (
                  'Rückgängig machen'
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Export Dropdown */}
      <div className="relative mt-1">
        <button
          onClick={() => setExportOpen(!exportOpen)}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
        >
          &#8595; Export
        </button>
        {exportOpen && (
          <div className="absolute right-0 mt-1 w-52 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg z-10">
            {activity.sport_type === 'multisport' ? (
              <button onClick={() => handleExport('zip')} className="w-full text-left px-4 py-2 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer rounded-lg">
                ZIP (Originaldateien)
              </button>
            ) : (
              <>
                <button onClick={() => handleExport('original')} className="w-full text-left px-4 py-2 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer rounded-t-lg">
                  Original herunterladen
                </button>
                <button onClick={() => handleExport('gpx')} className="w-full text-left px-4 py-2 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer">
                  Als GPX exportieren
                </button>
                <button onClick={() => handleExport('tcx')} className="w-full text-left px-4 py-2 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer">
                  Als TCX exportieren
                </button>
                <button onClick={() => handleExport('fit')} className="w-full text-left px-4 py-2 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer rounded-b-lg">
                  Als FIT exportieren
                </button>
              </>
            )}
          </div>
        )}
      </div>

      {/* Reprocess button with dropdown */}
      <div className="relative mt-1" ref={reprocessMenuRef}>
        <div className="flex items-center">
          {/* Main icon button — quick "reprocess all" */}
          <button
            onClick={() => {
              if (isReprocessing) return
              setReprocessMenuOpen(prev => !prev)
            }}
            disabled={isReprocessing}
            className="flex items-center justify-center w-8 h-8 rounded-lg
                       text-neutral-400 dark:text-neutral-500
                       hover:bg-neutral-100 dark:hover:bg-neutral-800
                       hover:text-neutral-600 dark:hover:text-neutral-300
                       disabled:opacity-50 disabled:cursor-not-allowed
                       transition-colors cursor-pointer"
            title="Aktivität neu verarbeiten"
          >
            {reprocessIcon}
          </button>
        </div>

        {/* Dropdown menu */}
        {reprocessMenuOpen && !isReprocessing && (
          <div className="absolute right-0 mt-1 w-52 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg z-10">
            {activity.sport_type === 'multisport' ? (
              multisportConfirm ? (
                <div className="px-4 py-3 space-y-2">
                  <p className="text-xs text-neutral-600 dark:text-neutral-400">
                    Alle {childCount} Disziplinen + Zusammenfassung neu berechnen?
                  </p>
                  <div className="flex gap-2">
                    <button
                      onClick={() => {
                        setReprocessMenuOpen(false)
                        setMultisportConfirm(false)
                        reprocessMutation.mutate()
                      }}
                      className="flex-1 px-3 py-1.5 text-xs font-medium rounded-lg bg-accent-DEFAULT text-white
                                 hover:bg-accent-600 transition-colors cursor-pointer"
                    >
                      OK
                    </button>
                    <button
                      onClick={() => {
                        setReprocessMenuOpen(false)
                        setMultisportConfirm(false)
                      }}
                      className="flex-1 px-3 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700
                                 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                    >
                      Abbrechen
                    </button>
                  </div>
                </div>
              ) : (
                <button
                  onClick={() => setMultisportConfirm(true)}
                  className="w-full text-left px-4 py-2.5 text-sm text-neutral-700 dark:text-neutral-300
                             hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer rounded-lg transition-colors"
                >
                  <span className="font-medium">Alles neu berechnen</span>
                  <span className="block text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
                    Alle Disziplinen + Zusammenfassung
                  </span>
                </button>
              )
            ) : (
              <>
                <button
                  onClick={() => {
                    setReprocessMenuOpen(false)
                    reprocessMutation.mutate()
                  }}
                  className="w-full text-left px-4 py-2.5 text-sm text-neutral-700 dark:text-neutral-300
                             hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer rounded-t-lg transition-colors"
                >
                  <span className="font-medium">Alles</span>
                  <span className="block text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
                    Vollständig neu verarbeiten
                  </span>
                </button>
                <div className="border-t border-neutral-100 dark:border-neutral-800" />
                <button
                  onClick={() => {
                    setReprocessMenuOpen(false)
                    setReprocessScopeOpen(true)
                  }}
                  className="w-full text-left px-4 py-2.5 text-sm text-neutral-700 dark:text-neutral-300
                             hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer rounded-b-lg transition-colors"
                >
                  <span className="font-medium">Details auswählen</span>
                  <span className="block text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">
                    Nur bestimmte Bereiche
                  </span>
                </button>
              </>
            )}
          </div>
        )}
      </div>

      {/* Scoped reprocess modal */}
      {reprocessScopeOpen && (
        <ReprocessScopeModal
          onConfirm={(scope) => reprocessScopedMutation.mutate(scope)}
          onClose={() => setReprocessScopeOpen(false)}
          isPending={reprocessScopedMutation.isPending}
        />
      )}

      {/* Sport conflict modal */}
      {conflictOpen && (
        <SportConflictModal
          activity={activity}
          open={conflictOpen}
          onClose={() => setConflictOpen(false)}
          onResolved={() => {
            setConflictOpen(false)
            queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
          }}
        />
      )}
    </div>
  )
}

// ── Tags ──────────────────────────────────────────────────────────────────────

function TagsBadges({ tags, activityId, isVirtual }: { tags: string[]; activityId: string; isVirtual?: boolean }) {
  const queryClient = useQueryClient()
  const [addOpen, setAddOpen] = useState(false)
  const [search, setSearch] = useState('')
  const popoverRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  const { data: allTags } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    staleTime: 60_000, // 1 min — avoids redundant refetches while staying on the page
  })

  const fallbackColors = [
    { bg: '#F97316', text: '#FFFFFF' }, // Orange
    { bg: '#3B82F6', text: '#FFFFFF' }, // Blue
    { bg: '#22C55E', text: '#FFFFFF' }, // Emerald
    { bg: '#A855F7', text: '#FFFFFF' }, // Purple
    { bg: '#F43F5E', text: '#FFFFFF' }, // Rose
    { bg: '#06B6D4', text: '#FFFFFF' }, // Cyan
  ]

  function getTagStyle(tagName: string, index: number): CSSProperties {
    const tagObj = (allTags ?? []).find(t => t.name.toLowerCase() === tagName.toLowerCase())
    if (tagObj?.color) return getTagBadgeStyle(tagObj.color)
    return getTagBadgeStyle(fallbackColors[index % fallbackColors.length].bg)
  }

  const removeMut = useMutation({
    mutationFn: async (tagToRemove: string) => {
      // Find the tag's ID from the loaded allTags list.
      // Tags with id === '' are "orphan" tags (in activities.tags TEXT[] but not in tags table).
      const tagObj = (allTags ?? []).find(t => t.name.toLowerCase() === tagToRemove.toLowerCase())
      if (tagObj && tagObj.id) {
        // Proper removal: delete from activity_tags junction table + sync TEXT[] cache.
        await unassignActivitiesFromTag(tagObj.id, [activityId])
        // Return fresh activity data for cache update.
        return updateActivity(activityId, {})
      } else {
        // Orphan tag (not in tags table): update TEXT[] cache directly.
        return updateActivity(activityId, { tags: tags.filter(t => t !== tagToRemove) })
      }
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(['activity', activityId], updated)
      queryClient.invalidateQueries({ queryKey: ['tags'] })
    },
  })

  const addMut = useMutation({
    mutationFn: async (tagToAdd: string) => {
      const tagObj = (allTags ?? []).find(t => t.name.toLowerCase() === tagToAdd.toLowerCase())
      if (tagObj?.id) {
        // Existing tag → use proper API (clears removal records)
        await assignActivitiesToTag(tagObj.id, [activityId])
        return updateActivity(activityId, {})
      }
      // Orphan tag or edge case → direct TEXT[] update
      return updateActivity(activityId, { tags: [...tags, tagToAdd] })
    },
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

  // Close popover on click outside
  useClickOutside(popoverRef, () => { setAddOpen(false); setSearch('') }, addOpen)

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
                className="w-full px-2 py-1.5 text-sm rounded border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
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
                  className="w-full text-left px-3 py-1.5 text-sm text-accent-600 dark:text-orange-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
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

      {tags.map((tag, i) => {
        const isHardcoded = tag === 'virtuell' && isVirtual
        return (
          <span
            key={tag}
            className="group inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium"
            style={getTagStyle(tag, i)}
          >
            {tag}
            {!isHardcoded && (
              <button
                onClick={() => removeMut.mutate(tag)}
                className="text-xs ml-1.5 opacity-0 group-hover:opacity-100 cursor-pointer transition-opacity"
                title={`Tag "${tag}" entfernen`}
              >
                &times;
              </button>
            )}
          </span>
        )
      })}
    </>
  )
}

// ── Map ───────────────────────────────────────────────────────────────────────

/** Haversine-Distanz in Metern zwischen zwei [lng, lat]-Koordinaten */
const SUMMIT_ICON_SVG = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">' +
  '<path d="M2 20 L8 8 L12 14 L16 8 L22 20 Z" fill="#f97316" stroke="white" stroke-width="1.5"/>' +
  '</svg>'
)}`

function haversineM([lng1, lat1]: [number, number], [lng2, lat2]: [number, number]): number {
  const R = 6371000
  const dLat = (lat2 - lat1) * Math.PI / 180
  const dLng = (lng2 - lng1) * Math.PI / 180
  const a = Math.sin(dLat/2)**2 + Math.cos(lat1*Math.PI/180) * Math.cos(lat2*Math.PI/180) * Math.sin(dLng/2)**2
  return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a))
}

/** Teilt Koordinaten bei Sprüngen > thresholdM in separate Segmente auf */
function splitAtGaps(coords: [number, number][], thresholdM: number): [number, number][][] {
  if (coords.length < 2 || thresholdM <= 0) return [coords]
  const segments: [number, number][][] = []
  let current: [number, number][] = [coords[0]]
  for (let i = 1; i < coords.length; i++) {
    if (haversineM(coords[i-1], coords[i]) > thresholdM) {
      if (current.length >= 2) segments.push(current)
      current = [coords[i]]
    } else {
      current.push(coords[i])
    }
  }
  if (current.length >= 2) segments.push(current)
  return segments.length > 0 ? segments : [coords]
}

function TrackMap({ activityId, streams, hoverIndex, sportColor, hasGps, zoomRange, onMaximize, geoTags, stops, locationPin }: {
  activityId?: string
  streams: StreamPoint[]
  hoverIndex: number | null
  sportColor: string
  hasGps: boolean
  zoomRange: import('@/components/ActivityCharts').ZoomRange | null
  onMaximize: () => void
  geoTags?: GeoTag[]
  stops?: ActivityStop[]
  locationPin?: { lat: number; lng: number; name: string; color: string }
}) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markerRef = useRef<maplibregl.Marker | null>(null)
  const locationPinMapContainerRef = useRef<HTMLDivElement>(null)
  const locationPinMapRef = useRef<maplibregl.Map | null>(null)
  const photoMarkersRef = useRef<maplibregl.Marker[]>([])
  const photoBlobUrlsRef = useRef<string[]>([])

  const [showPhotos, setShowPhotos] = useState(true)
  const [mapLoaded, setMapLoaded] = useState(false)

  const { data: allPhotos = [] } = useQuery({
    queryKey: ['activity-photos', activityId],
    queryFn: () => listActivityPhotos(activityId!),
    enabled: !!activityId,
    staleTime: 1000 * 60 * 5,
  })

  const positionedPhotos = useMemo(
    () => allPhotos.filter((p): p is ActivityPhoto & { lat: number; lng: number } => p.lat != null && p.lng != null),
    [allPhotos],
  )

  const { data: heatmapSettings } = useQuery({
    queryKey: ['heatmap-settings'],
    queryFn: getHeatmapSettings,
    staleTime: Infinity,
  })

  // Build GeoJSON from stream lat/lng
  const coordinates = streams
    .filter((s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null)
    .map(s => [s.lng, s.lat] as [number, number])

  const hasTrack = coordinates.length >= 2

  const teleportThresholdM = heatmapSettings?.teleport_filter_activity
    ? (heatmapSettings.teleport_threshold_m ?? 2000)
    : 0
  const trackSegments = splitAtGaps(coordinates, teleportThresholdM)

  const trackGeometry = trackSegments.length === 1
    ? { type: 'LineString' as const, coordinates: trackSegments[0] }
    : { type: 'MultiLineString' as const, coordinates: trackSegments }

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
          geometry: trackGeometry,
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

      // Gipfel-Pins aus GeoTags (category: summit/pass)
      const summitTags = (geoTags ?? []).filter(
        (t) => (t.category === 'summit' || t.category === 'pass') && t.lat != null
      )
      if (summitTags.length > 0) {
        const summitGeoJSON = {
          type: 'FeatureCollection' as const,
          features: summitTags.map((t) => ({
            type: 'Feature' as const,
            geometry: { type: 'Point' as const, coordinates: [t.lng!, t.lat!] },
            properties: { name: t.name ?? '', ele: t.ele_m ?? t.ele ?? 0 },
          })),
        }
        map.addSource('activity-summits', { type: 'geojson', data: summitGeoJSON })
        const addSummitLayers = () => {
          map.addLayer({
            id: 'activity-summits-layer',
            type: 'symbol',
            source: 'activity-summits',
            layout: {
              'icon-image': 'summit-icon',
              'icon-size': 0.9,
              'icon-allow-overlap': true,
              'text-field': ['concat', ['get', 'name'], '\n', ['to-string', ['get', 'ele']], ' m'] as maplibregl.ExpressionSpecification,
              'text-font': ['Open Sans Regular'],
              'text-size': 11,
              'text-offset': [0, 1.4],
              'text-anchor': 'top',
              'text-optional': true,
            },
            paint: { 'text-color': '#374151', 'text-halo-color': 'white', 'text-halo-width': 1 },
          })
        }
        if (map.hasImage('summit-icon')) {
          addSummitLayers()
        } else {
          const img = new Image(24, 24)
          img.onload = () => { map.addImage('summit-icon', img); addSummitLayers() }
          img.onerror = () => {
            // Fallback auf circle wenn Icon nicht ladbar
            map.addLayer({
              id: 'activity-summits-layer',
              type: 'circle',
              source: 'activity-summits',
              paint: { 'circle-radius': 6, 'circle-color': '#f97316', 'circle-stroke-color': 'white', 'circle-stroke-width': 2 },
            })
          }
          img.src = SUMMIT_ICON_SVG
        }
      }

      // Restaurant-Pins aus Stops (duration >= 900s) — Fallback ohne GeoTags
      const poiStops = (stops ?? []).filter(
        (s) => s.name && (s.duration_s ?? 0) >= 900 && s.lat != null
      )
      if (poiStops.length > 0) {
        const poiGeoJSON = {
          type: 'FeatureCollection' as const,
          features: poiStops.map((s) => ({
            type: 'Feature' as const,
            geometry: { type: 'Point' as const, coordinates: [s.lng!, s.lat!] },
            properties: {
              name: s.poi_name ?? s.name!,
              duration_min: Math.round((s.duration_s ?? 0) / 60),
              poi_type: s.poi_type ?? '',
            },
          })),
        }
        map.addSource('activity-pois', { type: 'geojson', data: poiGeoJSON })
        map.addLayer({
          id: 'activity-pois-layer',
          type: 'circle',
          source: 'activity-pois',
          paint: {
            'circle-radius': 8,
            'circle-color': '#3b82f6',
            'circle-stroke-color': 'white',
            'circle-stroke-width': 2,
          },
        })
        map.on('click', 'activity-pois-layer', (e) => {
          const props = e.features?.[0]?.properties
          if (!props) return
          new maplibregl.Popup()
            .setLngLat(e.lngLat)
            .setHTML(`<strong>${props.name}</strong><br>${props.duration_min} min`)
            .addTo(map)
        })
      }

      // Gastronomy-Pins aus GeoTags (category: gastronomy)
      const gastronomyTags = (geoTags ?? []).filter(
        (t) => t.category === 'gastronomy' && t.map_point_lat != null && t.map_point_lon != null
      )
      if (gastronomyTags.length > 0) {
        const gastronomyGeoJSON = {
          type: 'FeatureCollection' as const,
          features: gastronomyTags.map((t) => ({
            type: 'Feature' as const,
            geometry: { type: 'Point' as const, coordinates: [t.map_point_lon!, t.map_point_lat!] },
            properties: {
              name: t.name,
              duration_min: t.duration_min != null ? Math.round(t.duration_min) : null,
              poi_type: (t.meta?.poi_type as string | undefined) ?? '',
            },
          })),
        }
        map.addSource('activity-gastronomy-source', { type: 'geojson', data: gastronomyGeoJSON })
        map.addLayer({
          id: 'activity-gastronomy-layer',
          type: 'circle',
          source: 'activity-gastronomy-source',
          paint: {
            'circle-color': '#f97316',
            'circle-radius': 7,
            'circle-stroke-width': 2,
            'circle-stroke-color': '#fff',
          },
        })
        map.on('click', 'activity-gastronomy-layer', (e) => {
          const props = e.features?.[0]?.properties
          if (!props) return
          const durationStr = props.duration_min != null ? `<br>${props.duration_min} min` : ''
          const typeStr = props.poi_type ? `<br><span style="color:#6b7280">${props.poi_type}</span>` : ''
          new maplibregl.Popup()
            .setLngLat(e.lngLat)
            .setHTML(`<strong>${props.name}</strong>${typeStr}${durationStr}`)
            .addTo(map)
        })
      }

      const bounds = new maplibregl.LngLatBounds()
      for (const c of coordinates) bounds.extend(c)
      map.fitBounds(bounds, { padding: 40, maxZoom: 15 })

      setMapLoaded(true)
    })

    mapRef.current = map
    return () => {
      photoMarkersRef.current.forEach(m => m.remove())
      photoMarkersRef.current = []
      photoBlobUrlsRef.current.forEach(u => URL.revokeObjectURL(u))
      photoBlobUrlsRef.current = []
      map.remove()
    }
  }, [hasTrack]) // eslint-disable-line react-hooks/exhaustive-deps

  // Location-Pin-Karte für Indoor-Aktivitäten ohne GPS
  useEffect(() => {
    if (!locationPinMapContainerRef.current || !locationPin) return

    const map = new maplibregl.Map({
      container: locationPinMapContainerRef.current,
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
        layers: [{ id: 'osm-tiles', type: 'raster', source: 'osm', minzoom: 0, maxzoom: 19 }],
      },
      center: [locationPin.lng, locationPin.lat],
      zoom: 15,
      attributionControl: {},
    })

    const el = document.createElement('div')
    el.style.cssText = `width:18px;height:18px;border-radius:50%;background:${locationPin.color};border:3px solid white;box-shadow:0 2px 8px rgba(0,0,0,0.4);`
    new maplibregl.Marker({ element: el })
      .setLngLat([locationPin.lng, locationPin.lat])
      .setPopup(new maplibregl.Popup({ offset: 25 }).setHTML(`<strong>${locationPin.name}</strong>`))
      .addTo(map)

    locationPinMapRef.current = map
    return () => { map.remove() }
  }, [locationPin?.lat, locationPin?.lng, locationPin?.name, locationPin?.color]) // eslint-disable-line react-hooks/exhaustive-deps

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

  // Photo thumbnail markers — sync with positionedPhotos + showPhotos toggle
  useEffect(() => {
    const map = mapRef.current
    if (!map || !mapLoaded) return

    // Remove all existing photo markers + revoke blob URLs
    photoMarkersRef.current.forEach(m => m.remove())
    photoMarkersRef.current = []
    photoBlobUrlsRef.current.forEach(u => URL.revokeObjectURL(u))
    photoBlobUrlsRef.current = []

    if (!showPhotos || positionedPhotos.length === 0) return

    for (const photo of positionedPhotos) {
      const el = document.createElement('div')
      el.style.cssText = 'cursor:pointer;filter:drop-shadow(0 1px 4px rgba(0,0,0,0.5));'

      const imgBox = document.createElement('div')
      imgBox.style.cssText =
        'width:40px;height:30px;border-radius:3px;border:2px solid white;overflow:hidden;background:#444;'

      const triangle = document.createElement('div')
      triangle.style.cssText =
        'width:0;height:0;border-left:5px solid transparent;border-right:5px solid transparent;border-top:6px solid white;margin:0 auto;'

      el.appendChild(imgBox)
      el.appendChild(triangle)

      const marker = new maplibregl.Marker({ element: el, anchor: 'bottom' })
        .setLngLat([photo.lng, photo.lat])
        .addTo(map)

      photoMarkersRef.current.push(marker)

      // Load thumbnail with auth
      apiDownload(photo.thumb_url)
        .then(({ blob }) => {
          const blobUrl = URL.createObjectURL(blob)
          photoBlobUrlsRef.current.push(blobUrl)
          const img = document.createElement('img')
          img.src = blobUrl
          img.style.cssText = 'width:100%;height:100%;object-fit:cover;'
          imgBox.innerHTML = ''
          imgBox.appendChild(img)
        })
        .catch(() => {})
    }
  }, [positionedPhotos, mapLoaded, showPhotos]) // eslint-disable-line react-hooks/exhaustive-deps

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
    if (locationPin) {
      return (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden h-48 relative">
          <div ref={locationPinMapContainerRef} className="w-full h-full" />
          <div className="absolute top-2 left-2 flex items-center gap-1.5 bg-white/90 dark:bg-neutral-900/90 rounded-full px-2.5 py-1 text-xs font-medium shadow-sm pointer-events-none">
            <svg className="w-3 h-3 shrink-0" style={{ color: locationPin.color }} viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M9.69 18.933l.003.001C9.89 19.02 10 19 10 19s.11.02.308-.066l.002-.001.006-.003.018-.008a5.741 5.741 0 00.281-.14c.186-.096.446-.24.757-.433.62-.384 1.445-.966 2.274-1.765C15.302 15.957 17 13.514 17 10c0-3.866-3.582-7-8-7S2 6.134 2 10c0 3.514 1.698 5.957 3.355 7.584a13.731 13.731 0 002.273 1.765 11.842 11.842 0 00.757.433 5.766 5.766 0 00.282.14l.018.008.006.003zM10 11.25a1.25 1.25 0 100-2.5 1.25 1.25 0 000 2.5z" clipRule="evenodd" />
            </svg>
            <span className="text-neutral-700 dark:text-neutral-300">{locationPin.name}</span>
          </div>
        </div>
      )
    }
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

  const btnBase = 'flex items-center justify-center w-8 h-8 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md cursor-pointer transition-colors'
  const btnActive = 'text-neutral-600 dark:text-neutral-300 hover:bg-white dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-neutral-100'
  const btnDimmed = 'text-neutral-400 dark:text-neutral-600 hover:bg-white dark:hover:bg-neutral-700 hover:text-neutral-900 dark:hover:text-neutral-100'

  return (
    <div className="relative rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div ref={mapContainerRef} className="h-[400px] w-full" />
      <div className="absolute top-3 left-3 z-10 flex gap-2">
        {/* Maximize */}
        <button
          onClick={onMaximize}
          className={`${btnBase} ${btnActive}`}
          title="Karte maximieren"
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
            <path d="M13.28 7.78l3.22-3.22v2.69a.75.75 0 001.5 0v-4.5a.75.75 0 00-.75-.75h-4.5a.75.75 0 000 1.5h2.69l-3.22 3.22a.75.75 0 001.06 1.06zM2 17.25v-4.5a.75.75 0 011.5 0v2.69l3.22-3.22a.75.75 0 011.06 1.06L4.56 16.5h2.69a.75.75 0 010 1.5h-4.5a.75.75 0 01-.75-.75z" />
          </svg>
        </button>
        {/* Photo toggle — only shown when activity has positioned photos */}
        {positionedPhotos.length > 0 && (
          <button
            onClick={() => setShowPhotos(p => !p)}
            className={`${btnBase} ${showPhotos ? btnActive : btnDimmed}`}
            title={showPhotos ? 'Fotos ausblenden' : 'Fotos einblenden'}
          >
            <svg viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
              <path fillRule="evenodd" d="M1 8a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 018.07 3h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0016.07 6H17a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V8zm13.5 3a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0zM10 14a3 3 0 100-6 3 3 0 000 6z" clipRule="evenodd" />
            </svg>
          </button>
        )}
      </div>
    </div>
  )
}

// ── Zone Distribution Chart ──────────────────────────────────────────────────

function ZoneDistributionChart({ distribution }: { distribution: ZoneDistribution }) {
  const [activeType, setActiveType] = useState<'hr' | 'power'>(
    distribution.hr_zones.length > 0 ? 'hr' : 'power'
  )

  const zones = activeType === 'hr' ? distribution.hr_zones : distribution.power_zones
  if (zones.length === 0) return null

  const totalSeconds = zones.reduce((sum, z) => sum + z.seconds, 0)
  const ZONE_COLORS = ['#94a3b8', '#60a5fa', '#4ade80', '#facc15', '#f97316', '#ef4444', '#a855f7']

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Zonen-Verteilung</h3>
        {distribution.hr_zones.length > 0 && distribution.power_zones.length > 0 && (
          <div className="flex gap-2 text-xs">
            <button
              onClick={() => setActiveType('hr')}
              className={`px-2 py-0.5 rounded cursor-pointer ${activeType === 'hr' ? 'bg-accent-DEFAULT text-white' : 'text-neutral-500 hover:text-neutral-700'}`}
            >HR</button>
            <button
              onClick={() => setActiveType('power')}
              className={`px-2 py-0.5 rounded cursor-pointer ${activeType === 'power' ? 'bg-accent-DEFAULT text-white' : 'text-neutral-500 hover:text-neutral-700'}`}
            >Power</button>
          </div>
        )}
      </div>
      <div className="flex h-8 rounded-full overflow-hidden">
        {zones.map((z, i) => {
          const pct = totalSeconds > 0 ? (z.seconds / totalSeconds) * 100 : 0
          const color = ZONE_COLORS[z.zone_index] ?? ZONE_COLORS[ZONE_COLORS.length - 1]
          return (
            <div
              key={i}
              style={{ width: `${pct}%`, backgroundColor: color }}
              title={`Z${z.zone_index + 1}: ${Math.round(z.seconds / 60)}min (${pct.toFixed(1)}%)`}
            />
          )
        })}
      </div>
      <div className="flex flex-wrap gap-2">
        {zones.map((z, i) => {
          const pct = totalSeconds > 0 ? (z.seconds / totalSeconds) * 100 : 0
          const color = ZONE_COLORS[z.zone_index] ?? ZONE_COLORS[ZONE_COLORS.length - 1]
          return (
            <div key={i} className="flex items-center gap-1 text-xs text-neutral-600 dark:text-neutral-400">
              <span className="inline-block w-3 h-3 rounded-sm" style={{ backgroundColor: color }} />
              Z{z.zone_index + 1}: {Math.round(z.seconds / 60)}min ({pct.toFixed(0)}%)
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ── MultisportSectionWithUngroup ──────────────────────────────────────────────
function MultisportSectionWithUngroup({
  children,
  onHoverChild,
}: {
  children: Activity[]
  onHoverChild?: (id: string | null) => void
}) {
  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400">
        Multisport-Disziplinen
      </h3>
      <MultisportSection children={children} onHoverChild={onHoverChild} />
    </div>
  )
}
