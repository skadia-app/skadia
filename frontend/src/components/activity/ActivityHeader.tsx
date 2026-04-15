import { useState, useRef, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useActiveJobs } from '@/lib/useActiveJobs'
import { useToast } from '@/lib/toast'
import { updateActivity, reprocessActivity, reprocessAllActivities, ungroupActivity, type ExportFormat } from '@/api/activities'
import { revertFusion } from '@/api/fusion'
import { getCapabilities, getWaypointsFromActivity } from '@/api/routes'
import { apiDownload } from '@/api/client'
import type { Activity } from '@/api/types'
import { SportIcon } from '@/components/SportIcon'
import SportConflictBadge from '@/components/SportConflictBadge'
import SportConflictModal from '@/components/SportConflictModal'
import MergeDialog from '@/components/MergeDialog'
import ReprocessScopeModal from '@/components/activity/ReprocessScopeModal'
import { sportDetailLabel } from '@/lib/sportTaxonomy'
import { triggerBlobDownload, cn } from '@/lib/utils'
import { fmtActivityDate, fmtTimeFromDate } from '@/lib/format'

interface ActivityHeaderProps {
  activity: Activity
  childCount: number
}

function ActivityHeader({ activity, childCount }: ActivityHeaderProps) {
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
    await updateActivity(activity.id, { [field]: value })
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

  const sourceIds = activity.fusion_sources
    ? Array.from(new Set(Object.values(activity.fusion_sources))).filter(id => id !== activity.id && id !== 'fuse')
    : []

  const revertFusionMutation = useMutation({
    mutationFn: () => revertFusion(activity.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      if (sourceIds.length > 0) {
        navigate(`/activities/${sourceIds[0]}`)
      } else {
        navigate('/activities')
      }
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
        <SportIcon sport={activity.sport_type} subType={activity.sub_type} isElectric={activity.is_electric} className="w-8 h-8 text-neutral-500 dark:text-neutral-400" />
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
        {(activity.vo2max_ml_kg_min != null || activity.suunto_vo2max != null || activity.gps_accuracy_m != null) && (
          <div className="flex items-center gap-3 mt-1">
            {(activity.vo2max_ml_kg_min != null || activity.suunto_vo2max != null) && (() => {
              const isSuuntoFallback = activity.vo2max_ml_kg_min == null && activity.suunto_vo2max != null
              const value = isSuuntoFallback ? activity.suunto_vo2max! : activity.vo2max_ml_kg_min!
              const label = isSuuntoFallback ? 'VO₂max (Suunto)' : 'VO₂max'
              return (
                <div className="flex items-center gap-1 text-sm">
                  <span className="font-medium text-emerald-600">
                    {label} {value.toFixed(1)}
                  </span>
                  <span className="text-muted-foreground text-xs">ml/kg/min</span>
                  {!isSuuntoFallback && activity.vo2max_calibrated === false && (
                    <span className="text-amber-500 text-xs" title="Nicht kalibriert">~</span>
                  )}
                </div>
              )
            })()}
            {activity.gps_accuracy_m != null && (
              <span className={cn(
                "text-xs px-1.5 py-0.5 rounded",
                activity.gps_accuracy_m <= 3 ? "bg-emerald-100 text-emerald-700" :
                activity.gps_accuracy_m <= 8 ? "bg-amber-100 text-amber-700" :
                "bg-red-100 text-red-700"
              )}>
                GPS ±{activity.gps_accuracy_m}m
              </span>
            )}
          </div>
        )}
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
                href={`https://connect.garmin.com/app/activity/${activity.garmin_activity_id}`}
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
                    await ungroupActivity(activity.id)
                    queryClient.invalidateQueries({ queryKey: ['activities'] })
                    queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
                    navigate('/activities')
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

export default ActivityHeader
