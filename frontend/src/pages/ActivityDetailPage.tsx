import { useState, useRef, useEffect, useCallback, useMemo, type ReactNode, type CSSProperties } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchActivity, fetchActivityStreams, fetchActivitySensors, fetchActivityLaps, fetchActivitySplits, fetchActivityLengths, fetchActivityClimbPro, updateActivity, getZoneDistribution, correctActivityElevation, resetActivityElevation, correctActivityDistance, resetActivityDistance, fetchActivitySets, fetchActivityJumps, fetchActivityEvents } from '@/api/activities'
import { fetchWorkout } from '@/api/workouts'
import { getWeatherSettings } from '@/api/settings'
import { fetchTags, createTag, unassignActivitiesFromTag, assignActivitiesToTag } from '@/api/tags'
import ActivityLocationTagSection from '@/components/ActivityLocationTagSection'
import type { Activity, StreamPoint, Sensor, ZoneDistribution, ActivitySet, ActivityJump, ActivityEvent, Workout } from '@/api/types'
// Tag type imported via api/tags is used indirectly through fetchTags
import { getTagBadgeStyle } from '@/lib/tagUtils'
import ActivityCharts, { type ZoomRange, type SplitMarker } from '@/components/ActivityCharts'
import MapFullscreen from '@/components/MapFullscreen'
import {
  formatDuration, formatDistance, formatElevationGain, formatElevationLoss,
  sportColor,
} from '@/lib/utils'
import { getSegmentGroup, categoryForSport } from '@/lib/sportTaxonomy'
import { defaultSpeedMode } from '@/lib/sportFieldLabels'
import WeatherBadge from '@/components/WeatherBadge'
import GeoTagsList from '@/components/GeoTagsList'
import GearSelector from '@/components/GearSelector'
import ActivityPhotoSection from '@/components/ActivityPhotoSection'
import { uploadActivityPhoto } from '@/api/activityPhotos'
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
import ActivityHeader from '@/components/activity/ActivityHeader'
import TrackMap from '@/components/activity/TrackMap'
import RouteSimilarityCard from '@/components/analytics/RouteSimilarityCard'
import { useToast } from '@/lib/toast'

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
  useEffect(() => {
    if (!activity) return
    const key = `skadia-speed-format:${getSegmentGroup(activity.sport_type)}`
    const stored = localStorage.getItem(key)
    if (stored === 'speed' || stored === 'pace') {
      setSpeedMode(stored)
    } else {
      setSpeedMode(defaultSpeedMode(activity.sport_type))
    }
  }, [activity?.id])
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
          <StatsGrid activity={activity} />
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
              onSpeedModeChange={handleSpeedModeChange}
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
