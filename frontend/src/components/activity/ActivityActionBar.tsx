import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AlertTriangle, Download, Edit3, Map, MoreHorizontal, RotateCcw, SlidersHorizontal } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { Activity } from '@/api/types'
import { apiDownload } from '@/api/client'
import { reprocessActivity, ungroupActivity, type ExportFormat } from '@/api/activities'
import { revertFusion } from '@/api/fusion'
import { getRouteDraftFromActivity } from '@/api/routes'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureAvailable } from '@/lib/deploymentCapabilities'
import { triggerBlobDownload } from '@/lib/utils'
import { useToast } from '@/lib/toast'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useActiveJobs } from '@/lib/useActiveJobs'
import { invalidateAfterFusion } from '@/lib/fusionCacheInvalidation'
import { availableReprocessScopeKeys } from '@/lib/reprocessScopes'
import ActivityQuickEditModal from '@/components/activity/ActivityQuickEditModal'
import ReprocessScopeModal from '@/components/activity/ReprocessScopeModal'
import SportConflictModal from '@/components/SportConflictModal'

interface ActivityActionBarProps {
  activity: Activity
}

export default function ActivityActionBar({ activity }: ActivityActionBarProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { data: capabilities } = useCapabilities()
  const [quickEditOpen, setQuickEditOpen] = useState(false)
  const [conflictOpen, setConflictOpen] = useState(false)
  const [exportOpen, setExportOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const [smoothOpen, setSmoothOpen] = useState(false)
  const [reprocessScopeOpen, setReprocessScopeOpen] = useState(false)
  const [showRevertFusionModal, setShowRevertFusionModal] = useState(false)
  const [ungroupConfirm, setUngroupConfirm] = useState(false)
  const [smoothWindow, setSmoothWindow] = useState(2)
  const [reprocessState, setReprocessState] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [smoothing, setSmoothing] = useState<'idle' | 'smoothing' | 'resetting'>('idle')
  const [routeLoading, setRouteLoading] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const { isJobActiveForEntity } = useActiveJobs()
  const availableScopeKeys = availableReprocessScopeKeys(capabilities)
  const sourceIds = activity.fusion_memberships
    ? Array.from(new Set(activity.fusion_memberships.map(m => m.source_id))).filter(id => id !== activity.id && id !== 'fuse')
    : []
  const isReprocessing = reprocessState === 'loading'
  const hasActiveRecomputeJob = isJobActiveForEntity(activity.id)
  const prevHadJob = useRef(false)
  const menuButtonIcon = isReprocessing ? (
    <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2.5" strokeDasharray="50 20" />
    </svg>
  ) : reprocessState === 'success' ? (
    <svg className="h-4 w-4 text-green-500" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
      <path fillRule="evenodd" d="M16.704 4.153a.75.75 0 0 1 .143 1.052l-8 10.5a.75.75 0 0 1-1.127.075l-4.5-4.5a.75.75 0 0 1 1.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 0 1 1.05-.143z" clipRule="evenodd" />
    </svg>
  ) : reprocessState === 'error' ? (
    <svg className="h-4 w-4 text-red-500" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
      <path fillRule="evenodd" d="M18 10A8 8 0 1 1 2 10a8 8 0 0 1 16 0ZM9.25 5.75a.75.75 0 0 1 1.5 0v4.5a.75.75 0 0 1-1.5 0v-4.5ZM10 15a1 1 0 1 0 0-2 1 1 0 0 0 0 2Z" clipRule="evenodd" />
    </svg>
  ) : (
    <MoreHorizontal className="h-4 w-4" />
  )

  useClickOutside(menuRef, () => {
    setMenuOpen(false)
    setSmoothOpen(false)
    setUngroupConfirm(false)
  }, menuOpen)

  useEffect(() => {
    if (hasActiveRecomputeJob) {
      setReprocessState('loading')
      prevHadJob.current = true
    } else if (prevHadJob.current) {
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
  }, [activity.id, hasActiveRecomputeJob, queryClient])

  const reprocessMutation = useMutation({
    mutationFn: (scope?: string[]) => reprocessActivity(activity.id, scope),
    onMutate: () => setReprocessState('loading'),
    onSuccess: (result) => {
      if (result && 'job_id' in result) return
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

  const reprocessScopedMutation = useMutation({
    mutationFn: (scope: string[]) => reprocessActivity(activity.id, scope),
    onMutate: () => {
      setReprocessState('loading')
      setReprocessScopeOpen(false)
    },
    onSuccess: (result) => {
      if (result && 'job_id' in result) return
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-sensors', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['lift-candidates', activity.id] })
      setReprocessState('success')
      setTimeout(() => setReprocessState('idle'), 2000)
    },
    onError: () => {
      setReprocessState('error')
      setTimeout(() => setReprocessState('idle'), 3000)
    },
  })

  const revertFusionMutation = useMutation({
    mutationFn: () => revertFusion(activity.id),
    onSuccess: () => {
      invalidateAfterFusion(queryClient, {
        recordingGroupId: activity.recording_group_id,
        activityIds: [activity.id, ...sourceIds],
      })
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

  const ungroupMutation = useMutation({
    mutationFn: () => ungroupActivity(activity.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      navigate('/activities')
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Fehler beim Auflösen der Gruppe', message: err instanceof Error ? err.message : 'Unbekannter Fehler', duration: 0 })
    },
  })

  async function handleExport(format: ExportFormat) {
    setExportOpen(false)
    const { blob, filename } = await apiDownload(`/activities/${activity.id}/export?format=${format}`)
    const extMap: Record<string, string> = { original: '', gpx: '.gpx', tcx: '.tcx', fit: '.fit', zip: '.zip' }
    triggerBlobDownload(blob, filename ?? `activity${extMap[format] ?? ''}`)
  }

  async function openInPlanner() {
    setRouteLoading(true)
    try {
      const route = await getRouteDraftFromActivity(activity.id)
      navigate('/map', { state: { plannerRouteDraft: route } })
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Fehler beim Öffnen im Routenplaner',
        message: err instanceof Error ? err.message : 'Unbekannter Fehler',
        duration: 0,
      })
    } finally {
      setRouteLoading(false)
    }
  }

  return (
    <div className="flex flex-wrap items-center justify-end gap-2">
      {activity.sport_conflict_pending && (
        <button
          onClick={() => setConflictOpen(true)}
          className="inline-flex items-center gap-1.5 rounded-lg border border-amber-200 bg-amber-50 px-3 py-1.5 text-sm font-medium text-amber-800 hover:bg-amber-100 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-300"
        >
          <AlertTriangle className="h-4 w-4" />
          Sportkonflikt
        </button>
      )}
      {activity.has_gps && activity.sport_type !== 'multisport' && featureAvailable(capabilities, 'routing') && (
        <button
          onClick={openInPlanner}
          disabled={routeLoading}
          className="inline-flex items-center gap-1.5 rounded-lg bg-neutral-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-neutral-800 disabled:opacity-50 dark:bg-neutral-100 dark:text-neutral-900 dark:hover:bg-neutral-200"
        >
          <Map className="h-4 w-4" />
          Routenplaner
        </button>
      )}
      <button
        onClick={() => setQuickEditOpen(true)}
        className="inline-flex items-center gap-1.5 rounded-lg border border-neutral-200 px-3 py-1.5 text-sm hover:bg-neutral-50 dark:border-neutral-700 dark:hover:bg-neutral-800"
      >
        <Edit3 className="h-4 w-4" />
        Bearbeiten
      </button>
      <div className="relative">
        <button
          onClick={() => setExportOpen(v => !v)}
          className="inline-flex items-center gap-1.5 rounded-lg border border-neutral-200 px-3 py-1.5 text-sm hover:bg-neutral-50 dark:border-neutral-700 dark:hover:bg-neutral-800"
        >
          <Download className="h-4 w-4" />
          Export
        </button>
        {exportOpen && (
          <div className="absolute right-0 z-20 mt-1 w-36 rounded-lg border border-neutral-200 bg-white p-1 shadow-lg dark:border-neutral-700 dark:bg-neutral-900">
            {(['original', 'fit', 'gpx', 'tcx', 'zip'] as ExportFormat[]).map(format => (
              <button key={format} onClick={() => void handleExport(format)} className="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800">
                {format.toUpperCase()}
              </button>
            ))}
          </div>
        )}
      </div>
      <div className="relative">
        <button
          onClick={() => {
            if (isReprocessing) return
            setMenuOpen(v => !v)
          }}
          disabled={isReprocessing}
          className="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-neutral-200 hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-70 dark:border-neutral-700 dark:hover:bg-neutral-800"
          aria-label="Weitere Aktionen"
          title={isReprocessing ? 'Aktivität wird neu berechnet' : 'Weitere Aktionen'}
        >
          {menuButtonIcon}
        </button>
        {menuOpen && (
          <div ref={menuRef} className="absolute right-0 z-20 mt-1 w-72 rounded-lg border border-neutral-200 bg-white p-1 shadow-lg dark:border-neutral-700 dark:bg-neutral-900">
            {activity.sport_type !== 'multisport' && (
              <Link to={`/activities/${activity.id}/edit`} className="block rounded px-2 py-1.5 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800">
                Track bearbeiten
              </Link>
            )}
            {(activity.fusion_memberships?.length ?? 0) > 0 && (
              <button
                onClick={() => {
                  setMenuOpen(false)
                  setShowRevertFusionModal(true)
                }}
                disabled={revertFusionMutation.isPending}
                className="block w-full rounded px-2 py-1.5 text-left text-sm text-red-600 hover:bg-red-50 disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-950/30"
              >
                Fusion rückgängig
              </button>
            )}
            {activity.sport_type === 'multisport' && (
              ungroupConfirm ? (
                <div className="m-1 rounded-lg border border-red-200 bg-red-50 p-2 text-xs dark:border-red-900 dark:bg-red-950/30">
                  <div className="mb-2 flex items-center gap-1.5 text-red-700 dark:text-red-300">
                    <AlertTriangle className="h-3.5 w-3.5" />
                    Gruppe wirklich auflösen?
                  </div>
                  <div className="flex gap-2">
                    <button
                      onClick={() => ungroupMutation.mutate()}
                      disabled={ungroupMutation.isPending}
                      className="flex-1 rounded bg-red-600 px-2 py-1 font-medium text-white hover:bg-red-700 disabled:opacity-50"
                    >
                      {ungroupMutation.isPending ? '...' : 'Ja'}
                    </button>
                    <button
                      onClick={() => setUngroupConfirm(false)}
                      className="flex-1 rounded border border-neutral-200 px-2 py-1 hover:bg-white dark:border-neutral-700 dark:hover:bg-neutral-800"
                    >
                      Nein
                    </button>
                  </div>
                </div>
              ) : (
                <button
                  onClick={() => setUngroupConfirm(true)}
                  className="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800"
                >
                  Gruppe auflösen
                </button>
              )
            )}
            {activity.has_gps && activity.sport_type !== 'multisport' && (
              <>
                <button
                  onClick={() => setSmoothOpen(v => !v)}
                  disabled={smoothing !== 'idle'}
                  className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-neutral-100 disabled:opacity-50 dark:hover:bg-neutral-800"
                >
                  <SlidersHorizontal className="h-4 w-4" />
                  Geschwindigkeit glätten
                </button>
                {smoothOpen && (
                  <div className="mx-2 my-1 space-y-3 rounded-lg border border-neutral-200 p-3 text-xs dark:border-neutral-800">
                    <div className="flex items-center justify-between text-neutral-500 dark:text-neutral-400">
                      <span>Leicht</span>
                      <span className="font-medium text-neutral-800 dark:text-neutral-100">{smoothWindow * 2 + 1}s Fenster</span>
                      <span>Stark</span>
                    </div>
                    <input
                      type="range"
                      min={1}
                      max={3}
                      step={1}
                      value={smoothWindow}
                      onChange={e => setSmoothWindow(Number(e.target.value))}
                      className="w-full accent-accent-DEFAULT"
                    />
                    <button
                      disabled={smoothing !== 'idle'}
                      onClick={async () => {
                        setSmoothing('smoothing')
                        try {
                          await reprocessActivity(activity.id, ['speed_smooth'], smoothWindow)
                          queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
                          queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
                          addToast({ type: 'success', title: `Geglättet (${smoothWindow * 2 + 1}s Fenster)` })
                          setMenuOpen(false)
                          setSmoothOpen(false)
                        } catch {
                          addToast({ type: 'error', title: 'Glätten fehlgeschlagen', duration: 0 })
                        } finally {
                          setSmoothing('idle')
                        }
                      }}
                      className="w-full rounded-lg bg-accent-DEFAULT px-3 py-1.5 font-medium text-white hover:bg-accent-600 disabled:opacity-50"
                    >
                      Anwenden
                    </button>
                  </div>
                )}
              </>
            )}
            <button
              onClick={async () => {
                setMenuOpen(false)
                setSmoothing('resetting')
                try {
                  await reprocessActivity(activity.id, ['streams'])
                  queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
                  queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
                  addToast({ type: 'success', title: 'Trackdaten aus Originaldatei wiederhergestellt' })
                } catch {
                  addToast({ type: 'error', title: 'Zurücksetzen fehlgeschlagen', duration: 0 })
                } finally {
                  setSmoothing('idle')
                }
              }}
              disabled={smoothing !== 'idle'}
              title="Setzt die Track-Streams aus der Originaldatei zurück, inklusive geglätteter Geschwindigkeitsdaten."
              className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-neutral-100 disabled:opacity-50 dark:hover:bg-neutral-800"
            >
              <RotateCcw className="h-4 w-4" />
              Trackdaten aus Originaldatei wiederherstellen
            </button>
            <button
              onClick={() => {
                setMenuOpen(false)
                reprocessMutation.mutate(activity.sport_type === 'multisport' ? undefined : availableScopeKeys)
              }}
              disabled={isReprocessing}
              className="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-neutral-100 disabled:opacity-50 dark:hover:bg-neutral-800"
            >
              {isReprocessing ? 'Berechne neu...' : 'Alles neu berechnen'}
            </button>
            {activity.sport_type !== 'multisport' && (
              <button
                onClick={() => {
                  setMenuOpen(false)
                  setReprocessScopeOpen(true)
                }}
                disabled={isReprocessing}
                className="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-neutral-100 disabled:opacity-50 dark:hover:bg-neutral-800"
              >
                Bereiche neu berechnen...
              </button>
            )}
          </div>
        )}
      </div>
      <ActivityQuickEditModal
        activity={activity}
        open={quickEditOpen}
        onClose={() => setQuickEditOpen(false)}
        onSaved={(updated) => {
          queryClient.setQueryData(['activity', activity.id], updated)
          setQuickEditOpen(false)
        }}
      />
      {showRevertFusionModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={() => setShowRevertFusionModal(false)} />
          <div className="relative w-full max-w-sm space-y-4 rounded-2xl border border-neutral-200 bg-white p-6 shadow-2xl dark:border-neutral-800 dark:bg-neutral-900">
            <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">Fusion rückgängig machen?</h2>
            <p className="rounded-lg border border-red-200 bg-red-50 p-3 text-xs leading-relaxed text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-400">
              Die zusammengeführte Aktivität wird gelöscht. Die Quellaktivitäten werden wiederhergestellt und wieder sichtbar.
            </p>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowRevertFusionModal(false)}
                disabled={revertFusionMutation.isPending}
                className="rounded-lg border border-neutral-200 px-4 py-2 text-sm text-neutral-600 hover:bg-neutral-50 disabled:opacity-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800"
              >
                Abbrechen
              </button>
              <button
                onClick={() => {
                  setShowRevertFusionModal(false)
                  revertFusionMutation.mutate()
                }}
                disabled={revertFusionMutation.isPending}
                className="rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50"
              >
                {revertFusionMutation.isPending ? 'Wird rückgängig gemacht...' : 'Rückgängig machen'}
              </button>
            </div>
          </div>
        </div>
      )}
      {reprocessScopeOpen && (
        <ReprocessScopeModal
          onConfirm={(scope) => reprocessScopedMutation.mutate(scope)}
          onClose={() => setReprocessScopeOpen(false)}
          isPending={reprocessScopedMutation.isPending}
          capabilities={capabilities}
        />
      )}
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
