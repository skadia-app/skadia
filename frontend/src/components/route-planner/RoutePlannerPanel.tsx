import { memo, useState, useCallback, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type { RouteWaypoint, RouteSegment, PlannedRoute } from '@/api/routes'
import {
  createRouteCourseRelationship,
  createRouteRelationship,
  createRouteSnapshot,
  forkRoute,
  publishRouteCourse,
  type CreateRouteCourseRelationshipRequest,
  type CreateRouteRelationshipRequest,
  type PublishRouteCourseResponse,
  type RoutePublicationVisibility,
  type RoutePublishPolicy,
} from '@/api/routes'
import { useToast } from '@/lib/toast'
import { useModal } from '@/lib/modal'
import type { ProfileConfig } from '@/lib/routingProfiles'
import { getProfileDefaults, cacheServerDefaults } from '@/lib/routingProfiles'
import { getProfileDefaults as fetchProfileDefaults } from '@/api/routes'
import { fmtDist, fmtDurationMin } from '@/lib/format'
import { lazyFillMessages } from '@/lib/strings/route-planner'
import ProfileEditor from './ProfileEditor'
import {
  fetchAllProfiles,
  createProfile,
  deleteProfile,
  importProfiles,
  exportProfileUrl,
  type RoutingProfileEntry,
} from '@/api/routingProfiles'
import { apiDownload } from '@/api/client'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

function formatElevation(m: number): string {
  return `${Math.round(m)} m`
}


function GripIcon({ dragging }: { dragging?: boolean }) {
  return (
    <svg
      className={`w-4 h-4 text-neutral-400 dark:text-neutral-500 flex-shrink-0 ${dragging ? 'cursor-grabbing' : 'cursor-grab'}`}
      viewBox="0 0 24 24"
      fill="currentColor"
    >
      <circle cx="9" cy="6" r="1.5" />
      <circle cx="15" cy="6" r="1.5" />
      <circle cx="9" cy="12" r="1.5" />
      <circle cx="15" cy="12" r="1.5" />
      <circle cx="9" cy="18" r="1.5" />
      <circle cx="15" cy="18" r="1.5" />
    </svg>
  )
}

function WaypointIcon({ type }: { type: RouteWaypoint['type'] }) {
  if (type === 'start') {
    return (
      <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-green-500 text-white text-[9px] font-bold flex-shrink-0">
        S
      </span>
    )
  }
  if (type === 'end') {
    return (
      <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-red-500 text-white text-[9px] font-bold flex-shrink-0">
        Z
      </span>
    )
  }
  return (
    <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-accent-DEFAULT text-white text-[9px] font-bold flex-shrink-0">
      •
    </span>
  )
}

interface WaypointItemProps {
  waypoint: RouteWaypoint
  index: number
  total: number
  isDragging: boolean
  isDragOver: boolean
  onRemove: (id: string) => void
  /** Stable handler — receives (event, itemIndex). WaypointItem creates its own closure. */
  onDragStartAt: (e: React.DragEvent, index: number) => void
  /** Stable handler — receives (event, itemIndex). */
  onDragOverAt: (e: React.DragEvent, index: number) => void
  onDragLeave: () => void
  /** Stable handler — receives (event, itemIndex). */
  onDropAt: (e: React.DragEvent, index: number) => void
  onDragEnd: () => void
  segmentAfter?: RouteSegment
}

const WaypointItem = memo(function WaypointItem({
  waypoint,
  index,
  total,
  isDragging,
  isDragOver,
  onRemove,
  onDragStartAt,
  onDragOverAt,
  onDragLeave,
  onDropAt,
  onDragEnd,
  segmentAfter,
}: WaypointItemProps) {
  const label = waypoint.poi_name
    ?? `${waypoint.latlng[0].toFixed(4)}, ${waypoint.latlng[1].toFixed(4)}`

  // Per-item closures built inside the component — stable across parent re-renders
  // because the index-aware parent handlers are stable useCallback references.
  const handleDragStart = useCallback(
    (e: React.DragEvent) => onDragStartAt(e, index),
    [onDragStartAt, index],
  )
  const handleDragOver = useCallback(
    (e: React.DragEvent) => onDragOverAt(e, index),
    [onDragOverAt, index],
  )
  const handleDrop = useCallback(
    (e: React.DragEvent) => onDropAt(e, index),
    [onDropAt, index],
  )

  return (
    <div
      onDragOver={handleDragOver}
      onDragLeave={onDragLeave}
      onDrop={handleDrop}
      className={[
        'flex items-start gap-1.5 py-1 rounded transition-colors',
        isDragging ? 'opacity-40' : '',
        isDragOver ? 'bg-accent-100/40 dark:bg-accent-DEFAULT/10 ring-1 ring-accent-DEFAULT/30' : '',
      ].filter(Boolean).join(' ')}
    >
      {/* Grip handle for drag-to-reorder — always visible */}
      <div
        draggable
        onDragStart={handleDragStart}
        onDragEnd={onDragEnd}
        className="flex-shrink-0 pt-1"
        title="Verschieben"
      >
        <GripIcon dragging={isDragging} />
      </div>

      {/* Waypoint type indicator + connector line */}
      <div className="flex flex-col items-center gap-0.5">
        <WaypointIcon type={waypoint.type} />
        {index < total - 1 && (
          <div className="w-px flex-1 min-h-[12px] bg-neutral-200 dark:bg-neutral-700 my-0.5" />
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1">
          <span className="text-xs text-neutral-700 dark:text-neutral-300 truncate flex-1" title={label}>
            {label}
          </span>
          {/* Remove button — always visible, dezent */}
          {total > 2 && (
            <button
              onClick={() => onRemove(waypoint.id)}
              className="w-5 h-5 flex items-center justify-center text-neutral-300 hover:text-red-500 dark:text-neutral-600 dark:hover:text-red-400 transition-colors cursor-pointer flex-shrink-0"
              title="Wegpunkt entfernen"
            >
              <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
              </svg>
            </button>
          )}
        </div>

        {/* Segment info between waypoints */}
        {segmentAfter && (
          <div className="flex items-center gap-2 mt-0.5 text-[10px] text-neutral-400 dark:text-neutral-500">
            <span>{fmtDist(segmentAfter.distance_m)}</span>
            {segmentAfter.elevation_gain > 0 && (
              <span className="text-green-600 dark:text-green-500">↑{segmentAfter.elevation_estimated ? '~' : ''}{formatElevation(segmentAfter.elevation_gain)}</span>
            )}
            {segmentAfter.elevation_loss > 0 && (
              <span className="text-red-500 dark:text-red-400">↓{segmentAfter.elevation_estimated ? '~' : ''}{formatElevation(segmentAfter.elevation_loss)}</span>
            )}
            <span className={`px-1 rounded text-[9px] font-medium ${
              segmentAfter.mode === 'offroad'
                ? 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400'
                : 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
            }`}>
              {segmentAfter.mode === 'offroad' ? 'Freihand' : 'Route'}
            </span>
          </div>
        )}
      </div>
    </div>
  )
})

// Offline fallback for BRouter profiles (used when API is not available)
const BROUTER_PROFILES_FALLBACK = [
  { value: 'hiking-mountain', label: 'Wandern (Alpin, SAC T1-T3)' },
  { value: 'hiking-sac2',     label: 'Wandern (SAC T2 max)' },
  { value: 'cycling-road',    label: 'Rennrad' },
  { value: 'cycling-mountain', label: 'MTB' },
  { value: 'cycling-gravel',  label: 'Gravel' },
  { value: 'trekking',        label: 'Trekking (Rad)' },
  { value: 'running',         label: 'Laufen' },
  { value: 'walking',         label: 'Spaziergang' },
]

export function buildCreateProfilePayload(
  name: string,
  profileConfig: ProfileConfig,
  currentBadRouterEntry?: RoutingProfileEntry,
) {
  const config = { ...profileConfig } as Record<string, unknown>
  const baseProfile = (profileConfig.profile ?? currentBadRouterEntry?.base_profile ?? 'hiking') as string
  const predicates = profileConfig.predicates?.length
    ? profileConfig.predicates
    : currentBadRouterEntry?.predicates
  const modifierFilters = profileConfig.modifier_filters ?? currentBadRouterEntry?.modifier_filters ?? {}

  if (predicates?.length) {
    return {
      name,
      base_profile: baseProfile,
      predicates,
      modifier_filters: modifierFilters,
      config,
    }
  }

  return {
    name,
    base_profile: baseProfile,
    sport_types: currentBadRouterEntry?.sport_types ?? profileConfig.sport_types ?? [],
    config,
  }
}

export interface RoutePlannerPanelProps {
  // Current planning state (controlled from parent / MapPage)
  waypoints: RouteWaypoint[]
  segments: RouteSegment[]
  profileConfig: ProfileConfig | null
  showHeatmap: boolean
  // Routing engine
  routingEngine: 'badrouter' | 'brouter'
  brouterProfile: string
  badRouterAvailable: boolean
  routeSuggestionAvailable?: boolean
  onRoutingEngineChange: (engine: 'badrouter' | 'brouter') => void
  onBrouterProfileChange: (profile: string) => void
  // Stats (computed by parent from segments)
  totalDistanceM: number
  totalElevationGain: number
  totalElevationLoss: number
  estimatedTimeMin?: number
  // Callbacks
  onProfileChange: (config: ProfileConfig) => void
  onRemoveWaypoint: (id: string) => void
  onReorderWaypoints: (waypoints: RouteWaypoint[]) => void
  onRecalculateAll: () => void
  onToggleSegmentMode: (waypointId: string) => void
  onToggleHeatmap: (enabled: boolean) => void
  onClearRoute: () => void
  onSave: () => void              // opens RouteSaveDialog in parent
  onSuggest: () => void           // opens RouteSuggestionDialog in parent
  onClose: () => void
  // Error state
  calculationError?: string | null
  onClearError?: () => void
  // Optional: existing route being edited
  editingRoute?: PlannedRoute | null
  editingRouteId?: string | null
  editingRouteVersion?: number | null
  isSaving?: boolean
  lazyFillPendingSave?: boolean
}

export default function RoutePlannerPanel({
  waypoints,
  segments,
  profileConfig,
  showHeatmap,
  routingEngine,
  brouterProfile,
  badRouterAvailable,
  routeSuggestionAvailable = badRouterAvailable,
  onRoutingEngineChange,
  onBrouterProfileChange,
  totalDistanceM,
  totalElevationGain,
  totalElevationLoss,
  estimatedTimeMin,
  onProfileChange,
  onRemoveWaypoint,
  onReorderWaypoints,
  onRecalculateAll,
  onToggleSegmentMode,
  onToggleHeatmap,
  onClearRoute,
  onSave,
  onSuggest,
  onClose,
  calculationError,
  onClearError,
  editingRoute,
  editingRouteId = editingRoute?.id ?? null,
  editingRouteVersion = editingRoute?.version ?? null,
  isSaving = false,
  lazyFillPendingSave = false,
}: RoutePlannerPanelProps) {
  const { addToast } = useToast()
  const { openModal } = useModal()
  const queryClient = useQueryClient()
  const dragIndexRef = useRef<number | null>(null)
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null)
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null)

  // ── Profile management state ───────────────────────────────────────────
  const [showSaveDialog, setShowSaveDialog] = useState(false)
  const [saveProfileName, setSaveProfileName] = useState('')
  const [publishVisibility, setPublishVisibility] = useState<RoutePublicationVisibility>('private')
  const [publishPolicy, setPublishPolicy] = useState<RoutePublishPolicy>('new_course')
  const [compatibilityPreset, setCompatibilityPreset] = useState('standard')
  const [publishPrivacyReviewed, setPublishPrivacyReviewed] = useState(false)
  const [publishAccessScopeId, setPublishAccessScopeId] = useState('')
  const [targetCourseSegmentId, setTargetCourseSegmentId] = useState('')
  const [targetCourseVersionId, setTargetCourseVersionId] = useState('')
  const [targetPublicationId, setTargetPublicationId] = useState('')
  const [forkName, setForkName] = useState('')
  const [routeRelationshipTargetId, setRouteRelationshipTargetId] = useState('')
  const [routeRelationshipType, setRouteRelationshipType] = useState<CreateRouteRelationshipRequest['relation_type']>('variant_of')
  const [courseRelationshipTargetId, setCourseRelationshipTargetId] = useState('')
  const [courseRelationshipType, setCourseRelationshipType] = useState<CreateRouteCourseRelationshipRequest['relation_type']>('variant_of')
  const importInputRef = useRef<HTMLInputElement | null>(null)

  // Fetch all profiles from the API
  const { data: profilesData } = useQuery({
    queryKey: ['routing-profiles'],
    queryFn: fetchAllProfiles,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })

  // Derived lists
  const badRouterProfiles = profilesData?.profiles.filter(p => p.engine === 'badrouter') ?? []
  const brouterProfiles = profilesData?.profiles.filter(p => p.engine === 'brouter') ?? []
  const brouterOptions = brouterProfiles.length > 0
    ? brouterProfiles.map(p => ({ value: p.name, label: p.name }))
    : BROUTER_PROFILES_FALLBACK

  // Current BadRouter profile entry from API (for defaults reset and delete)
  const currentBadRouterEntry = badRouterProfiles.find(p => p.name === profileConfig?.profile)

  // Mutations
  const createMutation = useMutation({
    mutationFn: createProfile,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['routing-profiles'] })
      setShowSaveDialog(false)
      setSaveProfileName('')
      addToast({ type: 'success', title: 'Profil gespeichert' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Profil speichern fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteProfile,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['routing-profiles'] })
      addToast({ type: 'success', title: 'Profil gelöscht' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Profil löschen fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  const publishMutation = useMutation<PublishRouteCourseResponse, Error>({
    mutationFn: async () => {
      if (!editingRouteId || !editingRouteVersion) {
        throw new Error('route must be saved before publication')
      }
      if ((publishVisibility === 'shared' || publishVisibility === 'public') && !publishPrivacyReviewed) {
        throw new Error('privacy review is required for shared or public publication')
      }
      const operationBase = crypto.randomUUID()
      const snapshot = await createRouteSnapshot(editingRouteId, `${operationBase}:snapshot`, editingRouteVersion)
      return publishRouteCourse(editingRouteId, {
        route_snapshot_id: snapshot.snapshot_id,
        source_route_version: snapshot.source_route_version,
        publish_policy: publishPolicy,
        equivalence_decision: publishPolicy === 'new_course' ? 'new_group' : 'version_specific',
        visibility: publishVisibility,
        compatibility_preset: compatibilityPreset,
        ...(publishVisibility === 'shared' && publishAccessScopeId.trim() ? { access_scope_id: publishAccessScopeId.trim() } : {}),
        ...(targetPublicationId.trim() ? { target_publication_id: targetPublicationId.trim() } : {}),
        ...(targetCourseSegmentId.trim() ? { target_course_segment_id: targetCourseSegmentId.trim() } : {}),
        ...(targetCourseVersionId.trim() ? { target_course_segment_version_id: targetCourseVersionId.trim() } : {}),
        operation_id: `${operationBase}:publish`,
      })
    },
    onSuccess: () => {
      addToast({ type: 'success', title: 'Route als Kurs veröffentlicht' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Veröffentlichung fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  const forkMutation = useMutation({
    mutationFn: () => {
      if (!editingRouteId || !editingRouteVersion) throw new Error('route must be saved before fork')
      const name = forkName.trim()
      if (!name) throw new Error('fork name is required')
      return forkRoute(editingRouteId, {
        name,
        source_route_version: editingRouteVersion,
        operation_id: `${crypto.randomUUID()}:fork`,
      })
    },
    onSuccess: () => {
      setForkName('')
      addToast({ type: 'success', title: 'Fork erstellt' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Fork fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  const routeRelationshipMutation = useMutation({
    mutationFn: () => {
      if (!editingRouteId) throw new Error('route must be saved before relationships')
      const target = routeRelationshipTargetId.trim()
      if (!target) throw new Error('target route is required')
      return createRouteRelationship(editingRouteId, {
        target_route_id: target,
        relation_type: routeRelationshipType,
        operation_id: `${crypto.randomUUID()}:route-relationship`,
      })
    },
    onSuccess: () => {
      setRouteRelationshipTargetId('')
      addToast({ type: 'success', title: 'Beziehung gespeichert' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Beziehung fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  const courseRelationshipMutation = useMutation({
    mutationFn: () => {
      const publication = targetPublicationId.trim()
      const target = courseRelationshipTargetId.trim()
      if (!publication || !target) throw new Error('publication and target are required')
      return createRouteCourseRelationship(publication, {
        target_publication_id: target,
        relation_type: courseRelationshipType,
        operation_id: `${crypto.randomUUID()}:course-relationship`,
      })
    },
    onSuccess: () => {
      setCourseRelationshipTargetId('')
      addToast({ type: 'success', title: 'Kursbeziehung gespeichert' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Kursbeziehung fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  function handleSaveProfile() {
    if (!badRouterAvailable) return
    const name = saveProfileName.trim()
    if (!name || !profileConfig) return
    createMutation.mutate(buildCreateProfilePayload(name, profileConfig, currentBadRouterEntry))
  }

  async function handleResetToDefault() {
    if (!badRouterAvailable) return
    if (!profileConfig) return
    const profileName = profileConfig.profile ?? 'hiking'
    const entry = badRouterProfiles.find(p => p.name === profileName && p.source === 'system')
    if (entry?.config) {
      const apiConfig = entry.config as Partial<ProfileConfig>
      onProfileChange({ profile: profileName, ...apiConfig } as ProfileConfig)
      return
    }
    try {
      const resp = await fetchProfileDefaults(profileName)
      cacheServerDefaults(profileName, resp)
      const fresh = getProfileDefaults(profileName)
      if (fresh) onProfileChange(fresh)
    } catch {
      addToast({
        type: 'error',
        title: 'Reset fehlgeschlagen',
        message: `Profil "${profileName}" konnte nicht geladen werden.`,
        duration: 0,
      })
    }
  }

  async function handleSelectProfile(p: { base_profile: string; config?: Record<string, unknown> | null }) {
    if (!badRouterAvailable) return
    if (p.config) {
      onProfileChange({ profile: p.base_profile, ...p.config } as ProfileConfig)
      return
    }
    const cached = getProfileDefaults(p.base_profile)
    if (cached) {
      onProfileChange(cached)
      return
    }
    try {
      const resp = await fetchProfileDefaults(p.base_profile)
      cacheServerDefaults(p.base_profile, resp)
      const fresh = getProfileDefaults(p.base_profile)
      if (fresh) onProfileChange(fresh)
    } catch {
      addToast({
        type: 'error',
        title: 'Profil laden fehlgeschlagen',
        message: `Profil "${p.base_profile}" konnte nicht geladen werden.`,
        duration: 0,
      })
    }
  }

  async function handleExportProfile(id: string, name: string) {
    if (!badRouterAvailable) return
    try {
      const { blob, filename } = await apiDownload(exportProfileUrl(id))
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename ?? `${name}.json`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      addToast({ type: 'error', title: 'Export fehlgeschlagen', message: String(err), duration: 0 })
    }
  }

  async function handleImportFiles(files: FileList | null) {
    if (!badRouterAvailable) return
    if (!files || files.length === 0) return
    const fileArr = Array.from(files)
    try {
      const res = await importProfiles(fileArr)
      const failed = res.results.filter(r => !r.success)
      const succeeded = res.results.filter(r => r.success)
      if (succeeded.length > 0) {
        void queryClient.invalidateQueries({ queryKey: ['routing-profiles'] })
        addToast({ type: 'success', title: `${succeeded.length === 1 ? '1 Profil' : `${succeeded.length} Profile`} importiert` })
      }
      if (failed.length > 0) {
        addToast({
          type: 'error',
          title: `${failed.length === 1 ? '1 Profil' : `${failed.length} Profile`} nicht importiert`,
          message: failed.map(f => `${f.name}: ${f.error ?? 'Unbekannter Fehler'}`).join('\n'),
          duration: 0,
        })
      }
    } catch (err) {
      addToast({ type: 'error', title: 'Import fehlgeschlagen', message: String(err), duration: 0 })
    }
  }

  const hasRoute = waypoints.length >= 2

  const handleDragStart = useCallback((e: React.DragEvent, index: number) => {
    dragIndexRef.current = index
    setDraggingIndex(index)
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(index))
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent, index: number) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (dragIndexRef.current !== index) setDragOverIndex(index)
  }, [])

  const handleDragLeave = useCallback(() => {
    setDragOverIndex(null)
  }, [])

  const handleDrop = useCallback((_e: React.DragEvent, dropIndex: number) => {
    const from = dragIndexRef.current
    if (from === null || from === dropIndex) { setDragOverIndex(null); return }
    const reordered = [...waypoints]
    const [moved] = reordered.splice(from, 1)
    reordered.splice(dropIndex, 0, moved)
    const reassigned = reordered.map((wp, i) => ({
      ...wp,
      type: i === 0 ? ('start' as const) : i === reordered.length - 1 ? ('end' as const) : ('via' as const),
    }))
    onReorderWaypoints(reassigned)
    void onRecalculateAll()
    setDragOverIndex(null)
  }, [waypoints, onReorderWaypoints, onRecalculateAll])

  const handleDragEnd = useCallback(() => {
    dragIndexRef.current = null
    setDraggingIndex(null)
    setDragOverIndex(null)
  }, [])

  const handleClear = useCallback(() => {
    if (waypoints.length > 0) {
      onClearRoute()
      addToast({ type: 'info', title: 'Route geleert' })
    }
  }, [waypoints.length, onClearRoute, addToast])

  const anyElevationEstimated = segments.some(s => s.elevation_estimated === true)

  // Find the segment between waypoint[i] and waypoint[i+1]
  function segmentAfterWaypoint(waypoint: RouteWaypoint, index: number): RouteSegment | undefined {
    if (index >= waypoints.length - 1) return undefined
    const nextWaypoint = waypoints[index + 1]
    return segments.find(
      s => s.from_waypoint === waypoint.id && s.to_waypoint === nextWaypoint.id
    )
  }

  return (
    <div className="h-full flex flex-col border-r border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900">
      <div className="flex flex-col overflow-hidden flex-1 min-h-0">

        {/* Header */}
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-neutral-100 dark:border-neutral-800 flex-shrink-0">
          <div className="flex items-center gap-2 flex-1">
            {/* Route icon */}
            <svg className="w-4 h-4 text-accent-DEFAULT flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M12.207 2.232a.75.75 0 00.025 1.06l4.146 3.958H6.375a5.375 5.375 0 000 10.75H9.75a.75.75 0 000-1.5H6.375a3.875 3.875 0 010-7.75h10.003l-4.146 3.957a.75.75 0 001.036 1.085l5.5-5.25a.75.75 0 000-1.085l-5.5-5.25a.75.75 0 00-1.061.025z" clipRule="evenodd" />
            </svg>
            <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              {editingRoute ? `Route: ${editingRoute.name}` : 'Route planen'}
            </span>
          </div>
          <button
            onClick={onClose}
            className="ml-2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer flex-shrink-0"
            title="Schliessen"
          >
            <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        </div>

        <div className="flex flex-col overflow-y-auto min-h-0">

            {/* Routing engine toggle */}
            <div className="px-3 pt-2.5 pb-1.5 border-b border-neutral-100 dark:border-neutral-800">
              <div className="flex items-center gap-1 p-0.5 rounded-lg bg-neutral-100 dark:bg-neutral-800">
                {badRouterAvailable && (
                  <button
                    onClick={() => onRoutingEngineChange('badrouter')}
                    className={`flex-1 px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer ${
                      routingEngine === 'badrouter'
                        ? 'bg-accent-DEFAULT text-white shadow-sm'
                        : 'bg-transparent text-neutral-600 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-neutral-200'
                    }`}
                  >
                    BadRouter
                  </button>
                )}
                <button
                  onClick={() => onRoutingEngineChange('brouter')}
                  className={`flex-1 px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer ${
                    routingEngine === 'brouter'
                      ? 'bg-accent-DEFAULT text-white shadow-sm'
                      : 'bg-transparent text-neutral-600 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-neutral-200'
                  }`}
                >
                  BRouter
                </button>
              </div>
            </div>

            {/* Profile editor (BadRouter) or BRouter profile selector */}
            {routingEngine === 'badrouter' && badRouterAvailable ? (
              <>
                {/* Hidden file input for import */}
                <input
                  ref={importInputRef}
                  type="file"
                  accept=".json"
                  multiple
                  className="hidden"
                  onChange={e => { void handleImportFiles(e.target.files); e.target.value = '' }}
                />

                {/* Gear menu for profile management actions */}
                <div className="px-3 py-1.5 border-b border-neutral-100 dark:border-neutral-800 flex items-center justify-end">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <button
                        title="Profilverwaltung"
                        className="p-1 rounded text-neutral-400 dark:text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors cursor-pointer"
                      >
                        <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                          <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
                          <circle cx="12" cy="12" r="3" />
                        </svg>
                      </button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => handleResetToDefault()}>
                        <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
                          <path fillRule="evenodd" d="M15.312 11.424a5.5 5.5 0 01-9.201 2.466l-.312-.311h2.433a.75.75 0 000-1.5H4.28a.75.75 0 00-.75.75v3.955a.75.75 0 001.5 0v-2.146l.312.31a7 7 0 0011.712-3.138.75.75 0 00-1.449-.386zm-10.624-2.85a5.5 5.5 0 019.201-2.466l.312.311h-2.433a.75.75 0 000 1.5h3.952a.75.75 0 00.75-.75V3.214a.75.75 0 00-1.5 0v2.146l-.312-.31A7 7 0 003.244 8.188a.75.75 0 001.449.386z" clipRule="evenodd" />
                        </svg>
                        Auf Standard zuruecksetzen
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => { setSaveProfileName(''); setShowSaveDialog(true) }}>
                        <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
                          <path d="M3.75 3A1.75 1.75 0 002 4.75v10.5c0 .966.784 1.75 1.75 1.75h12.5A1.75 1.75 0 0018 15.25v-8.5A1.75 1.75 0 0016.25 5H15V3.75A1.75 1.75 0 0013.25 2h-3.5A1.75 1.75 0 008 3.75V5H4.75A1.75 1.75 0 003.75 3zm4.75.75A.25.25 0 018.75 4h2.5a.25.25 0 01.25.25V5h-3.5V3.75z" />
                        </svg>
                        Profil speichern...
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => importInputRef.current?.click()}>
                        <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
                          <path fillRule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-7.707a1 1 0 011.414 0L9 10.586V3a1 1 0 112 0v7.586l1.293-1.293a1 1 0 111.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z" clipRule="evenodd" />
                        </svg>
                        Profil importieren...
                      </DropdownMenuItem>
                      {currentBadRouterEntry && (
                        <DropdownMenuItem onClick={() => void handleExportProfile(currentBadRouterEntry.id, currentBadRouterEntry.name)}>
                          <svg className="w-3.5 h-3.5 flex-shrink-0" viewBox="0 0 20 20" fill="currentColor">
                            <path fillRule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-9.707a1 1 0 000 1.414L9 11.414V3a1 1 0 112 0v8.414l2.707-2.707a1 1 0 111.414 1.414l-4.414 4.414a1 1 0 01-1.414 0L4.293 8.707a1 1 0 010-1.414z" clipRule="evenodd" />
                          </svg>
                          Profil exportieren
                        </DropdownMenuItem>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>

                {/* BadRouter: user/filesystem profiles list (editable entries) */}
                {badRouterProfiles.filter(p => p.editable || p.source === 'filesystem').length > 0 && (
                  <div className="px-3 py-2 border-b border-neutral-100 dark:border-neutral-800">
                    <div className="text-[10px] text-neutral-400 dark:text-neutral-500 uppercase tracking-wide mb-1.5">
                      Eigene Profile
                    </div>
                    <div className="space-y-1">
                      {badRouterProfiles
                        .filter(p => p.editable || p.source === 'filesystem')
                        .map(p => (
                          <div key={p.id} className="flex items-center gap-1.5">
                            <button
                              onClick={() => { void handleSelectProfile(p) }}
                              className={`flex-1 text-left px-2 py-1 rounded text-xs transition-colors cursor-pointer truncate ${
                                profileConfig?.profile === p.base_profile && !badRouterProfiles.find(s => s.source === 'system' && s.name === profileConfig?.profile)
                                  ? 'bg-accent-DEFAULT/10 text-accent-DEFAULT font-medium'
                                  : 'text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                              }`}
                              title={p.name}
                            >
                              {p.name}
                              {p.source === 'filesystem' && (
                                <span className="ml-1 text-[9px] text-neutral-400 dark:text-neutral-500">(fs)</span>
                              )}
                            </button>
                            {/* Export */}
                            <button
                              onClick={() => void handleExportProfile(p.id, p.name)}
                              title="Profil exportieren"
                              className="text-neutral-300 dark:text-neutral-600 hover:text-accent-DEFAULT dark:hover:text-accent-DEFAULT transition-colors cursor-pointer flex-shrink-0"
                            >
                              <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                                <path fillRule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-9.707a1 1 0 000 1.414L9 11.414V3a1 1 0 112 0v8.414l2.707-2.707a1 1 0 111.414 1.414l-4.414 4.414a1 1 0 01-1.414 0L4.293 8.707a1 1 0 010-1.414z" clipRule="evenodd" />
                              </svg>
                            </button>
                            {/* Delete — editable profiles only (not system or filesystem) */}
                            {p.editable && (
                              <button
                                onClick={() => {
                                  openModal({
                                    title: 'Profil löschen?',
                                    message: `Profil "${p.name}" wirklich löschen? Diese Aktion kann nicht rückgängig gemacht werden.`,
                                    buttons: [
                                      { label: 'Abbrechen', variant: 'default', onClick: () => {} },
                                      { label: 'Löschen', variant: 'destructive', onClick: () => deleteMutation.mutate(p.id) },
                                    ],
                                  })
                                }}
                                title="Profil löschen"
                                className="text-neutral-300 dark:text-neutral-600 hover:text-red-500 dark:hover:text-red-400 transition-colors cursor-pointer flex-shrink-0"
                              >
                                <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                                  <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 11.25a.75.75 0 01.75.75v2a.75.75 0 01-1.5 0v-2a.75.75 0 01.75-.75zM7.75 12a.75.75 0 00-1.5 0v2a.75.75 0 001.5 0v-2zm4 .75a.75.75 0 01.75-.75.75.75 0 01.75.75v2a.75.75 0 01-1.5 0v-2z" clipRule="evenodd" />
                                </svg>
                              </button>
                            )}
                          </div>
                        ))}
                    </div>
                  </div>
                )}

                {profileConfig === null ? (
                  <div className="px-3 py-6 text-center text-xs text-neutral-400">
                    <svg className="w-4 h-4 animate-spin mx-auto mb-2" viewBox="0 0 24 24" fill="none">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
                    </svg>
                    Lade Profil-Defaults...
                  </div>
                ) : (
                  <ProfileEditor
                    config={profileConfig}
                    onChange={onProfileChange}
                  />
                )}

                {/* Save-as dialog (inline, below profile tabs) */}
                {showSaveDialog && (
                  <div className="px-3 py-2.5 border-b border-neutral-100 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/50">
                    <div className="text-[10px] text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-1.5">
                      Profil speichern
                    </div>
                    <div className="flex gap-1.5">
                      <input
                        type="text"
                        placeholder="Profilname..."
                        value={saveProfileName}
                        onChange={e => setSaveProfileName(e.target.value)}
                        onKeyDown={e => {
                          if (e.key === 'Enter') handleSaveProfile()
                          else if (e.key === 'Escape') setShowSaveDialog(false)
                        }}
                        autoFocus
                        className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                      />
                      <button
                        onClick={handleSaveProfile}
                        disabled={!saveProfileName.trim() || createMutation.isPending}
                        className="px-2.5 py-1.5 rounded-md bg-accent-DEFAULT text-white text-xs font-medium hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                      >
                        OK
                      </button>
                      <button
                        onClick={() => setShowSaveDialog(false)}
                        className="px-2.5 py-1.5 rounded-md border border-neutral-200 dark:border-neutral-700 text-neutral-500 text-xs hover:bg-neutral-100 dark:hover:bg-neutral-700 cursor-pointer transition-colors"
                      >
                        Abbruch
                      </button>
                    </div>
                  </div>
                )}
                {/* Offroad snap threshold (BadRouter only) */}
                {profileConfig && (
                  <div className="px-3 py-2 border-t border-neutral-100 dark:border-neutral-800">
                    <div className="space-y-1">
                      <div className="flex items-center justify-between">
                        <span className="text-xs text-neutral-700 dark:text-neutral-300">Offroad-Schwelle</span>
                        <span className="text-xs font-mono text-neutral-500 dark:text-neutral-400 tabular-nums">
                          {profileConfig.snap_threshold_m ?? 100} m
                        </span>
                      </div>
                      <input
                        type="range"
                        min={10}
                        max={500}
                        step={10}
                        value={profileConfig.snap_threshold_m ?? 100}
                        onChange={e => onProfileChange({ ...profileConfig, snap_threshold_m: parseInt(e.target.value, 10) })}
                        className="w-full h-1.5 rounded-full appearance-none cursor-pointer accent-[var(--color-accent-DEFAULT,#F97316)] bg-neutral-200 dark:bg-neutral-700"
                      />
                      <p className="text-[10px] text-neutral-400 dark:text-neutral-500 leading-snug">
                        Maximale Distanz zum nächsten Weg beim Snap
                      </p>
                    </div>
                  </div>
                )}
              </>
            ) : (
              <div className="px-3 py-2 border-b border-neutral-100 dark:border-neutral-800">
                <label className="block text-[10px] text-neutral-400 dark:text-neutral-500 uppercase tracking-wide mb-1">
                  Profil
                </label>
                <select
                  value={brouterProfile}
                  onChange={e => onBrouterProfileChange(e.target.value)}
                  className="w-full text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 text-neutral-800 dark:text-neutral-200 px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT cursor-pointer"
                >
                  {brouterOptions.map(p => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>
            )}

            {/* Waypoint list */}
            <div className="px-3 pt-2 pb-1">
              {waypoints.length === 0 ? (
                <div className="text-xs text-neutral-400 dark:text-neutral-500 text-center py-4">
                  Klicke auf die Karte, um Wegpunkte zu setzen
                </div>
              ) : (
                <div className="space-y-0">
                  {waypoints.map((wp, i) => (
                    <WaypointItem
                      key={wp.id}
                      waypoint={wp}
                      index={i}
                      total={waypoints.length}
                      isDragging={draggingIndex === i}
                      isDragOver={dragOverIndex === i}
                      onRemove={onRemoveWaypoint}
                      onDragStartAt={handleDragStart}
                      onDragOverAt={handleDragOver}
                      onDragLeave={handleDragLeave}
                      onDropAt={handleDrop}
                      onDragEnd={handleDragEnd}
                      segmentAfter={segmentAfterWaypoint(wp, i)}
                    />
                  ))}
                </div>
              )}
            </div>

            {/* No-route inline dialog */}
            {calculationError === 'no_route' && (
              <div className="mx-3 mb-2 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 px-3 py-2.5">
                <p className="text-xs text-amber-800 dark:text-amber-300 font-medium mb-2">
                  Keine Route gefunden
                </p>
                <p className="text-[10px] text-amber-600 dark:text-amber-400 mb-2.5">
                  Zwischen den Wegpunkten gibt es keinen Weg im Netzwerk.
                </p>
                <div className="flex gap-2">
                  <button
                    onClick={() => {
                      const lastWpIdx = waypoints.length - 1
                      if (lastWpIdx >= 1) {
                        onToggleSegmentMode(waypoints[lastWpIdx - 1].id)
                      }
                      onClearError?.()
                    }}
                    className="flex-1 px-2 py-1.5 rounded-md bg-amber-500 text-white text-xs font-medium hover:bg-amber-600 transition-colors cursor-pointer"
                  >
                    Offroad-Segment anlegen
                  </button>
                  <button
                    onClick={() => onClearError?.()}
                    className="px-2 py-1.5 rounded-md border border-amber-300 dark:border-amber-700 text-amber-700 dark:text-amber-400 text-xs font-medium hover:bg-amber-100 dark:hover:bg-amber-900/30 transition-colors cursor-pointer"
                  >
                    Abbrechen
                  </button>
                </div>
              </div>
            )}

          </div>

        {/* ── Fixierter unterer Bereich (Stats + Buttons) ──────────────── */}
        <div className="flex-shrink-0 border-t border-neutral-100 dark:border-neutral-800">
            {/* Route statistics */}
            {hasRoute && (
              <div className="mx-3 mt-2 mb-2 rounded-lg bg-neutral-50 dark:bg-neutral-800/60 px-3 py-2 grid grid-cols-2 gap-x-4 gap-y-1">
                <div>
                  <div className="text-[10px] text-neutral-400 dark:text-neutral-500 uppercase tracking-wide">Distanz</div>
                  <div className="text-sm font-semibold text-neutral-800 dark:text-neutral-100">
                    {fmtDist(totalDistanceM)}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] text-neutral-400 dark:text-neutral-500 uppercase tracking-wide">Zeit (est.)</div>
                  <div className="text-sm font-semibold text-neutral-800 dark:text-neutral-100">
                    {estimatedTimeMin != null ? fmtDurationMin(estimatedTimeMin) : '—'}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] text-green-600 dark:text-green-500 uppercase tracking-wide">Aufstieg</div>
                  <div className="text-sm font-semibold text-green-700 dark:text-green-400">
                    {totalElevationGain > 0 ? `↑ ${anyElevationEstimated ? '~' : ''}${formatElevation(totalElevationGain)}` : '—'}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] text-red-500 dark:text-red-400 uppercase tracking-wide">Abstieg</div>
                  <div className="text-sm font-semibold text-red-600 dark:text-red-400">
                    {totalElevationLoss > 0 ? `↓ ${anyElevationEstimated ? '~' : ''}${formatElevation(totalElevationLoss)}` : '—'}
                  </div>
                </div>
              </div>
            )}

            {/* Heatmap toggle */}
            <div className="px-3 py-2 flex items-center justify-between">
              <label className="flex items-center gap-2 cursor-pointer select-none">
                <button
                  role="switch"
                  aria-checked={showHeatmap}
                  onClick={() => onToggleHeatmap(!showHeatmap)}
                  className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none ${
                    showHeatmap ? 'bg-accent-DEFAULT' : 'bg-neutral-300 dark:bg-neutral-600'
                  }`}
                >
                  <span
                    className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ${
                      showHeatmap ? 'translate-x-4' : 'translate-x-0'
                    }`}
                  />
                </button>
                <span className="text-xs text-neutral-600 dark:text-neutral-400">Heatmap anzeigen</span>
              </label>
            </div>

            {/* Action buttons */}
            <div className="px-3 pb-3 pt-1 space-y-2">
              {/* Suggest route button */}
              {routeSuggestionAvailable && (
                <button
                  onClick={onSuggest}
                  className="w-full flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg border border-accent-DEFAULT text-accent-DEFAULT hover:bg-accent-100 dark:hover:bg-accent-DEFAULT/10 text-xs font-medium transition-colors cursor-pointer"
                >
                  <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                    <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
                  </svg>
                  Route vorschlagen
                </button>
              )}

              {editingRouteId && (
                <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/60 p-2 space-y-2">
                  <div className="text-[10px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">Kurs veröffentlichen</div>
                  <div className="grid grid-cols-3 gap-1.5">
                    <select
                      aria-label="Sichtbarkeit der Kursveröffentlichung"
                      value={publishVisibility}
                      onChange={e => setPublishVisibility(e.target.value as RoutePublicationVisibility)}
                      className="min-w-0 text-[11px] rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-700 dark:text-neutral-200 px-1.5 py-1 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                    >
                      <option value="private">Privat</option>
                      <option value="shared">Geteilt</option>
                      <option value="public">Öffentlich</option>
                    </select>
                    <select
                      aria-label="Kompatibilitätsprofil"
                      value={compatibilityPreset}
                      onChange={e => setCompatibilityPreset(e.target.value)}
                      className="min-w-0 text-[11px] rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-700 dark:text-neutral-200 px-1.5 py-1 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                    >
                      <option value="standard">Standard</option>
                      <option value="trail">Trail</option>
                      <option value="lap">Runde</option>
                    </select>
                    <select
                      aria-label="Veröffentlichungsart"
                      value={publishPolicy}
                      onChange={e => setPublishPolicy(e.target.value as RoutePublishPolicy)}
                      className="min-w-0 text-[11px] rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-700 dark:text-neutral-200 px-1.5 py-1 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                    >
                      <option value="new_course">Neu</option>
                      <option value="new_version_same_course">Neue Version</option>
                      <option value="continue_equivalence_group">Äquivalenz fortführen</option>
                      <option value="new_equivalence_group">Neue Äquivalenz</option>
                    </select>
                  </div>
                  <div className="grid grid-cols-3 gap-1.5">
                    <input
                      aria-label="Freigabe-Scope"
                      value={publishAccessScopeId}
                      onChange={event => setPublishAccessScopeId(event.target.value)}
                      placeholder="Freigabe-Scope"
                      disabled={publishVisibility !== 'shared'}
                      className="min-w-0 rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 disabled:opacity-40 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                    />
                    <input
                      aria-label="Ziel-Kurs"
                      value={targetCourseSegmentId}
                      onChange={event => setTargetCourseSegmentId(event.target.value)}
                      placeholder="Ziel-Kurs"
                      className="min-w-0 rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                    />
                    <input
                      aria-label="Ziel-Version"
                      value={targetCourseVersionId}
                      onChange={event => setTargetCourseVersionId(event.target.value)}
                      placeholder="Ziel-Version"
                      className="min-w-0 rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                    />
                  </div>
                  <input
                    aria-label="Ziel-Veröffentlichung"
                    value={targetPublicationId}
                    onChange={event => setTargetPublicationId(event.target.value)}
                    placeholder="Bestehende Veröffentlichung oder Beziehung"
                    className="w-full rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                  />
                  {(publishVisibility === 'shared' || publishVisibility === 'public') && (
                    <label className="flex items-start gap-1.5 rounded-md border border-amber-200 bg-amber-50 p-1.5 text-[10px] leading-snug text-amber-800 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-300">
                      <input
                        type="checkbox"
                        checked={publishPrivacyReviewed}
                        onChange={event => setPublishPrivacyReviewed(event.target.checked)}
                        className="mt-0.5 h-3 w-3 accent-accent-DEFAULT"
                      />
                      <span>Privacy-Review bestätigt: Start-, Ziel- und Zwischenpunkte dürfen in dieser Sichtbarkeit erscheinen.</span>
                    </label>
                  )}
                  <button
                    type="button"
                    onClick={() => publishMutation.mutate()}
                    disabled={!hasRoute || !editingRouteVersion || publishMutation.isPending || ((publishVisibility === 'shared' || publishVisibility === 'public') && !publishPrivacyReviewed)}
                    className="w-full flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-md border border-neutral-300 dark:border-neutral-700 text-neutral-700 dark:text-neutral-200 hover:bg-white dark:hover:bg-neutral-900 disabled:opacity-40 disabled:cursor-not-allowed text-xs font-medium transition-colors cursor-pointer"
                  >
                    {publishMutation.isPending ? 'Veröffentliche...' : 'Als Kurs veröffentlichen'}
                  </button>
                  <div className="border-t border-neutral-200 pt-2 dark:border-neutral-700">
                    <div className="text-[10px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">Fork & Beziehungen</div>
                    <div className="mt-1 grid grid-cols-[1fr_auto] gap-1.5">
                      <input
                        aria-label="Fork-Name"
                        value={forkName}
                        onChange={event => setForkName(event.target.value)}
                        placeholder="Fork-Name"
                        className="min-w-0 rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                      />
                      <button
                        type="button"
                        onClick={() => forkMutation.mutate()}
                        disabled={!editingRouteVersion || !forkName.trim() || forkMutation.isPending}
                        className="rounded-md border border-neutral-300 px-2 py-1 text-[11px] text-neutral-700 disabled:opacity-40 dark:border-neutral-700 dark:text-neutral-200"
                      >
                        Fork erstellen
                      </button>
                    </div>
                    <div className="mt-1 grid grid-cols-[1fr_auto_auto] gap-1.5">
                      <input
                        aria-label="Zielroute"
                        value={routeRelationshipTargetId}
                        onChange={event => setRouteRelationshipTargetId(event.target.value)}
                        placeholder="Zielroute"
                        className="min-w-0 rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                      />
                      <select
                        aria-label="Routenbeziehung"
                        value={routeRelationshipType}
                        onChange={event => setRouteRelationshipType(event.target.value as CreateRouteRelationshipRequest['relation_type'])}
                        className="rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                      >
                        <option value="variant_of">Variante</option>
                        <option value="derived_from">Abgeleitet</option>
                        <option value="fork_of">Fork</option>
                      </select>
                      <button
                        type="button"
                        onClick={() => routeRelationshipMutation.mutate()}
                        disabled={!routeRelationshipTargetId.trim() || routeRelationshipMutation.isPending}
                        className="rounded-md border border-neutral-300 px-2 py-1 text-[11px] text-neutral-700 disabled:opacity-40 dark:border-neutral-700 dark:text-neutral-200"
                      >
                        Beziehung speichern
                      </button>
                    </div>
                    <div className="mt-1 grid grid-cols-[1fr_auto_auto] gap-1.5">
                      <input
                        aria-label="Ziel-Kursveröffentlichung"
                        value={courseRelationshipTargetId}
                        onChange={event => setCourseRelationshipTargetId(event.target.value)}
                        placeholder="Ziel-Kursveröffentlichung"
                        className="min-w-0 rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                      />
                      <select
                        aria-label="Kursbeziehung"
                        value={courseRelationshipType}
                        onChange={event => setCourseRelationshipType(event.target.value as CreateRouteCourseRelationshipRequest['relation_type'])}
                        className="rounded-md border border-neutral-200 bg-white px-1.5 py-1 text-[11px] text-neutral-700 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-200"
                      >
                        <option value="variant_of">Variante</option>
                        <option value="derived_from">Abgeleitet</option>
                      </select>
                      <button
                        type="button"
                        onClick={() => courseRelationshipMutation.mutate()}
                        disabled={!targetPublicationId.trim() || !courseRelationshipTargetId.trim() || courseRelationshipMutation.isPending}
                        className="rounded-md border border-neutral-300 px-2 py-1 text-[11px] text-neutral-700 disabled:opacity-40 dark:border-neutral-700 dark:text-neutral-200"
                      >
                        Kursbeziehung speichern
                      </button>
                    </div>
                  </div>
                </div>
              )}

              {/* Save / Clear row */}
              <div className="flex gap-2">
                <button
                  onClick={onSave}
                  disabled={!hasRoute || isSaving}
                  className={[
                    'flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed text-xs font-medium transition-colors cursor-pointer',
                    lazyFillPendingSave && !isSaving ? 'ring-2 ring-amber-400 animate-pulse' : '',
                  ].join(' ')}
                  title={lazyFillPendingSave ? lazyFillMessages.saveButtonHint : undefined}
                >
                  {isSaving ? (
                    <>
                      <svg className="w-3.5 h-3.5 animate-spin" viewBox="0 0 24 24" fill="none">
                        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                      </svg>
                      Speichern...
                    </>
                  ) : (
                    <>
                      <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                        <path d="M3.75 3A1.75 1.75 0 002 4.75v10.5c0 .966.784 1.75 1.75 1.75h12.5A1.75 1.75 0 0018 15.25v-8.5A1.75 1.75 0 0016.25 5H15V3.75A1.75 1.75 0 0013.25 2h-3.5A1.75 1.75 0 008 3.75V5H4.75A1.75 1.75 0 003.75 3H3.75zm4.75.75A.25.25 0 018.75 4h2.5a.25.25 0 01.25.25V5h-3.5V3.75zm3.25 7.5a.75.75 0 01-.75.75H9.75a.75.75 0 010-1.5H11v-1.25a.75.75 0 011.5 0v1.5z" />
                      </svg>
                      {editingRoute ? 'Aktualisieren' : 'Speichern'}
                      {lazyFillPendingSave && (
                        <span className="ml-0.5 w-1.5 h-1.5 rounded-full bg-amber-300 shrink-0" aria-hidden="true" />
                      )}
                    </>
                  )}
                </button>

                <button
                  onClick={handleClear}
                  disabled={waypoints.length === 0}
                  className="px-3 py-2 rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-500 dark:text-neutral-400 hover:border-red-300 hover:text-red-500 dark:hover:border-red-700 dark:hover:text-red-400 disabled:opacity-40 disabled:cursor-not-allowed text-xs font-medium transition-colors cursor-pointer"
                  title="Route leeren"
                >
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 11.25a.75.75 0 01.75.75v2a.75.75 0 01-1.5 0v-2a.75.75 0 01.75-.75zM7.75 12a.75.75 0 00-1.5 0v2a.75.75 0 001.5 0v-2zm4 .75a.75.75 0 01.75-.75.75.75 0 01.75.75v2a.75.75 0 01-1.5 0v-2z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>

              {/* Hint text when no waypoints */}
              {!hasRoute && waypoints.length === 0 && (
                <p className="text-[10px] text-neutral-400 dark:text-neutral-500 text-center leading-snug">
                  Klicke auf die Karte um Start und Ziel zu setzen.{' '}
                  Oder nutze "Route vorschlagen" für eine KI-generierte Route.
                </p>
              )}
            </div>
          </div>
      </div>
    </div>
  )
}
