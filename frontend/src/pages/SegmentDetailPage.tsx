import { useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Lock, Map, Pencil, RefreshCw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getMe } from '@/api/auth'
import { fetchSegment, recomputeSegment, publishSegment, deleteSegment, exportSegment } from '@/api/segments'
import { ApiError } from '@/api/client'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import { useActiveJobs } from '@/lib/useActiveJobs'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureAvailable } from '@/lib/deploymentCapabilities'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { triggerBlobDownload } from '@/lib/utils'
import { SegmentMinimap } from '@/components/SegmentDetail/SegmentMinimap'
import { SegmentActivityList } from '@/components/SegmentDetail/SegmentActivityList'
import { SegmentPerformanceChart } from '@/components/SegmentDetail/SegmentPerformanceChart'
import { FavoriteButton } from '@/components/SegmentDetail/FavoriteButton'
import TargetOverlayTabs from '@/components/target-overlays/TargetOverlayTabs'
import SegmentEditModal from '@/components/SegmentEditModal'
import DifficultyBadge from '@/components/DifficultyBadge'
import DateRangePicker from '@/components/DateRangePicker'
import { EMPTY_FILTERS, type ActivityFilterState } from '@/components/ActivityFilters'

const SPORT_GROUPS = ['bike', 'foot', 'swim', 'ski', 'paddle', 'generic'] as const
const SEGMENT_TYPES = ['standard', 'trail', 'lap', 'lift', 'wintersport', 'wassersport'] as const
const CLIMB_TYPES = ['standard', 'bergauf', 'bergab'] as const

function segmentSportGroupLabelKey(value: string): string {
  return SPORT_GROUPS.includes(value as (typeof SPORT_GROUPS)[number])
    ? `segmentDetail.sportGroups.${value}`
    : 'segmentDetail.sportGroups.unknown'
}

function segmentTypeLabelKey(value: string): string {
  return SEGMENT_TYPES.includes(value as (typeof SEGMENT_TYPES)[number])
    ? `segmentDetail.types.${value}`
    : 'segmentDetail.types.unknown'
}

function climbTypeLabelKey(value: string): string {
  return CLIMB_TYPES.includes(value as (typeof CLIMB_TYPES)[number])
    ? `segmentDetail.climbTypes.${value}`
    : 'segmentDetail.climbTypes.unknown'
}

function isPublishPrivacyConfirmationError(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.status !== 409) return false
  const title = typeof error.json?.title === 'string' ? error.json.title : ''
  const code = typeof error.json?.code === 'string' ? error.json.code : ''
  const detail = typeof error.json?.detail === 'string' ? error.json.detail : error.body
  return (
    code === 'segment_publish_privacy_confirmation_required' ||
    title === 'Privacy Confirmation Required' ||
    detail.includes('active privacy zone') ||
    detail.includes('Privacy-Zone')
  )
}

export default function SegmentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { formatDistance, formatElevation } = useUnitFormatters()
  const [showEdit, setShowEdit] = useState(false)
  const [localPending, setLocalPending] = useState(false)
  const [dateFrom, setDateFrom] = useState<string | undefined>(undefined)
  const [dateTo, setDateTo] = useState<string | undefined>(undefined)

  const queryClient = useQueryClient()
  const { isJobActiveForEntity } = useActiveJobs()
  const { openModal } = useModal()
  const { addToast } = useToast()
  const { data: capabilities } = useCapabilities()
  const segmentRecomputeAvailable = featureAvailable(capabilities, 'segment_recompute')
  const segmentPublishAvailable = featureAvailable(capabilities, 'segment_publish')

  const { data: me } = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    staleTime: 60_000,
  })

  const publishMut = useMutation({
    mutationFn: (options?: { sourceActivityPublishConfirmed?: boolean }) => (
      options ? publishSegment(id!, options) : publishSegment(id!)
    ),
    onSuccess: () => {
      addToast({
        type: 'success',
        title: t('segmentDetail.toasts.publishSuccessTitle'),
        message: t('segmentDetail.toasts.publishSuccessMessage'),
      })
      queryClient.invalidateQueries({ queryKey: ['segment', id] })
      queryClient.invalidateQueries({ queryKey: ['segments'] })
    },
    onError: (error) => {
      if (isPublishPrivacyConfirmationError(error)) {
        openModal({
          title: t('segmentDetail.modals.privacyTitle'),
          message: t('segmentDetail.modals.privacyMessage'),
          buttons: [
            { label: t('common.cancel'), variant: 'default', onClick: () => {} },
            {
              label: t('segmentDetail.modals.publishAnyway'),
              variant: 'destructive',
              onClick: () => publishMut.mutate({ sourceActivityPublishConfirmed: true }),
            },
          ],
        })
        return
      }
      addToast({ type: 'error', title: t('segmentDetail.toasts.publishFailed'), duration: 0 })
    },
  })

  const deleteMut = useMutation({
    mutationFn: () => deleteSegment(id!),
    onSuccess: () => navigate('/segments'),
    onError: () => addToast({ type: 'error', title: t('segmentDetail.toasts.deleteFailed'), duration: 0 }),
  })

  const recomputeMut = useMutation({
    mutationFn: () => recomputeSegment(id!),
    onMutate: () => {
      setLocalPending(true)
      queryClient.invalidateQueries({ queryKey: ['active-jobs'] })
    },
    onSuccess: () => {
      setTimeout(() => {
        queryClient.invalidateQueries({ queryKey: ['segment', id] })
        queryClient.invalidateQueries({ queryKey: ['segment-performances', id] })
      }, 3000)
      setTimeout(() => setLocalPending(false), 10000)
    },
    onError: () => {
      setLocalPending(false)
    },
  })

  const isRecomputing = recomputeMut.isPending || localPending || isJobActiveForEntity(id!)

  const handleExport = async (format: 'gpx' | 'geojson') => {
    try {
      const ext = format === 'gpx' ? '.gpx' : '.geojson'
      const { blob, filename } = await exportSegment(id!, format)
      triggerBlobDownload(blob, filename ?? `segment${ext}`)
    } catch (err) {
      console.error('Segment export failed', err)
      addToast({
        type: 'error',
        title: t('segmentDetail.toasts.exportFailedTitle'),
        message: t('segmentDetail.toasts.exportFailedMessage'),
        duration: 0,
      })
    }
  }

  // #1a fix: include_geojson=true so SegmentMinimap receives track_geojson
  const { data: segment, isLoading } = useQuery({
    queryKey: ['segment', id, 'geojson-controls-overlay-summary'],
    queryFn: () => fetchSegment(id!, { includeGeoJSON: true, includeControls: true, includeOverlaySummary: true }),
    enabled: !!id,
  })

  if (isLoading) return <div className="flex items-center justify-center h-64 text-neutral-400">{t('segmentDetail.loading')}</div>
  if (!segment) return <div className="p-6 text-neutral-400">{t('segmentDetail.notFound')}</div>
  const segmentPermissions = new Set(segment.effective_permissions ?? [])
  const canManageSegment = me?.user_id === segment.created_by || me?.is_admin

  // Build a minimal activity filter from the date range for SegmentActivityList
  const dateFilter: ActivityFilterState | undefined =
    dateFrom || dateTo
      ? { ...EMPTY_FILTERS, dateFrom: dateFrom ?? '', dateTo: dateTo ?? '' }
      : undefined

  return (
    <div className="flex flex-col gap-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <Link to="/segments" className="flex items-center gap-2 text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300">
          <ArrowLeft size={16} /> {t('navigation.segments')}
        </Link>
        <div className="flex items-center gap-2">
          {/* #2 fix: only DateRangePicker here — ActivityFilters removed */}
          <DateRangePicker
            dateFrom={dateFrom ?? ''}
            dateTo={dateTo ?? ''}
            onChange={(from, to) => { setDateFrom(from || undefined); setDateTo(to || undefined) }}
          />
          {segment.segment_type !== 'lift' && canManageSegment && (
          <button
            onClick={() => setShowEdit(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
          >
            <Pencil size={14} /> {t('segmentDetail.actions.edit')}
          </button>
          )}
          {segmentRecomputeAvailable && (
            <button
              onClick={() => { if (!isRecomputing) recomputeMut.mutate() }}
              disabled={isRecomputing}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                isRecomputing
                  ? 'border-accent-DEFAULT text-accent-DEFAULT bg-accent-100 dark:bg-accent-100/20'
                  : 'border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
              }`}
              title={t('segmentDetail.actions.recomputeTitle')}
            >
              <RefreshCw size={14} className={isRecomputing ? 'animate-spin' : ''} /> {isRecomputing ? t('segmentDetail.actions.recomputing') : t('segmentDetail.actions.recompute')}
            </button>
          )}
          <button
            onClick={() => handleExport('gpx')}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
          >
            {t('segmentDetail.actions.exportAs', { format: 'GPX' })}
          </button>
          <button
            onClick={() => handleExport('geojson')}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
          >
            {t('segmentDetail.actions.exportAs', { format: 'GeoJSON' })}
          </button>
          <button
            onClick={() => navigate('/map', { state: { activateSegmentsOverlay: true } })}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
          >
            <Map size={14} /> {t('segmentDetail.actions.openMap')}
          </button>
          {/* Publish: shown for creator of private segment OR admin */}
          {segmentPublishAvailable && segment.visibility === 'private' && (segment.created_by === me?.user_id || me?.is_admin) && (
            <button
              onClick={() => {
                const isForeignAdmin = me?.is_admin && segment.created_by !== me?.user_id
                openModal({
                  title: t('segmentDetail.modals.publishTitle'),
                  message: isForeignAdmin
                    ? t('segmentDetail.modals.publishAdminMessage')
                    : t('segmentDetail.modals.publishMessage'),
                  buttons: [
                    { label: t('common.cancel'), variant: 'default', onClick: () => {} },
                    { label: t('segmentDetail.actions.publish'), variant: 'destructive', onClick: () => publishMut.mutate(undefined) },
                  ],
                })
              }}
              disabled={publishMut.isPending}
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-amber-400 text-amber-600 dark:text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-900/20 cursor-pointer transition-colors disabled:opacity-50"
            >
              {t('segmentDetail.actions.publish')}
            </button>
          )}

          {/* Delete: visible for creator or admin, not for OSM lift segments */}
          {segment.segment_type !== 'lift' && canManageSegment && (
            <button
              onClick={() => openModal({
                title: t('segmentDetail.modals.deleteTitle'),
                message: t('segmentDetail.modals.deleteMessage'),
                buttons: [
                  { label: t('common.cancel'), variant: 'default', onClick: () => {} },
                  { label: t('segmentDetail.actions.delete'), variant: 'destructive', onClick: () => deleteMut.mutate() },
                ],
              })}
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 cursor-pointer transition-colors"
            >
              <Trash2 className="w-3.5 h-3.5" />
              {t('segmentDetail.actions.delete')}
            </button>
          )}
          <FavoriteButton segmentId={id!} />
        </div>
      </div>

      {/* Title + Metadata */}
      <div>
        <div className="flex items-center gap-3 mb-2">
          <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{segment.name}</h1>
          {segment.course_kind !== 'checkpoint_sequence' && ['trail', 'wintersport'].includes(segment.segment_type) && (
            <DifficultyBadge
              sac={segment.segment_type_meta?.sac_scale as string}
              mtb={segment.segment_type_meta?.mtb_scale as string}
              color={segment.segment_type_meta?.difficulty_color as string}
            />
          )}
        </div>
        <div className="flex flex-wrap gap-3 text-sm text-neutral-500 dark:text-neutral-400">
          <span className="px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-neutral-800">
            {t(segmentSportGroupLabelKey(segment.sport_group))}
          </span>
          {segment.course_kind === 'checkpoint_sequence' ? (
            <span className="px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-neutral-800">
              {t('segmentDetail.metadata.checkpointCourse')}
            </span>
          ) : segment.segment_type !== 'standard' && (
            <span className="px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-neutral-800">
              {t(segmentTypeLabelKey(segment.segment_type))}
            </span>
          )}
          {segment.distance_m != null && <span>{formatDistance(segment.distance_m)}</span>}
          {segment.elevation_gain_m != null && segment.elevation_gain_m > 0 && (
            <span>+{formatElevation(segment.elevation_gain_m)}</span>
          )}
          {segment.climb_type && segment.climb_type !== 'standard' && (
            <span>{t(climbTypeLabelKey(segment.climb_type))}</span>
          )}
        </div>
        {/* Effort count stats */}
        <div className="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-xs text-neutral-500 dark:text-neutral-400">
          <span>
            {t('segmentDetail.stats.efforts', { count: segment.effort_count })}
          </span>
          {segment.ridden_count > 0 && (
            <span>· {t('segmentDetail.stats.ownEfforts', { count: segment.ridden_count })}</span>
          )}
          {segment.athlete_count > 0 && (
            <span>· {t('segmentDetail.stats.athletes', { count: segment.athlete_count })}</span>
          )}
          {segment.visibility === 'private' && (
            <span className="flex items-center gap-1 text-neutral-400">
              <Lock className="w-3 h-3" />
              {t('segmentDetail.metadata.private')}
            </span>
          )}
        </div>
        {segment.overlay_summary && (
          <div className="mt-3 flex flex-wrap gap-2 text-xs text-neutral-500 dark:text-neutral-400">
            {segment.overlay_summary.public_open_report_count > 0 && (
              <span className="rounded-md bg-neutral-100 px-2 py-1 dark:bg-neutral-800">
                {t('segmentDetail.overlay.openReports', { count: segment.overlay_summary.public_open_report_count })}
              </span>
            )}
            {segment.overlay_summary.current_condition_state && (
              <span className="rounded-md bg-neutral-100 px-2 py-1 dark:bg-neutral-800">
                {t('segmentDetail.overlay.condition', { state: segment.overlay_summary.current_condition_state })}
              </span>
            )}
            {segment.overlay_summary.public_comment_count > 0 && (
              <span className="rounded-md bg-neutral-100 px-2 py-1 dark:bg-neutral-800">
                {t('segmentDetail.overlay.comments', { count: segment.overlay_summary.public_comment_count })}
              </span>
            )}
          </div>
        )}
        {segment.course_kind === 'checkpoint_sequence' && segment.control_summary && (
          <div className="mt-2 flex flex-wrap gap-2 text-xs text-neutral-500 dark:text-neutral-400">
            <span className="px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-neutral-800">
              {t('segmentDetail.controls.checkpoints', { count: segment.control_summary.control_count })}
            </span>
            {segment.controls && segment.controls.length > 0 && (
              <span className="px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-neutral-800">
                {segment.controls.map((control, index) => (
                  control.role === 'start'
                    ? t('segmentDetail.controls.start')
                    : control.role === 'finish'
                      ? t('segmentDetail.controls.finish')
                      : String(index + 1)
                )).join(' -> ')}
              </span>
            )}
          </div>
        )}
        {segment.description && (
          <p className="mt-2 text-sm text-neutral-600 dark:text-neutral-400">{segment.description}</p>
        )}
      </div>

      {/* Minimap + Activity List */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <SegmentMinimap segment={segment} />
        <SegmentActivityList segmentId={id!} segment={segment} filters={dateFilter} />
      </div>

      {/* Performance Chart */}
      <SegmentPerformanceChart segmentId={id!} dateFrom={dateFrom} dateTo={dateTo} />

      <TargetOverlayTabs
        subjectKind="course"
        subjectId={segment.id}
        canWrite={Boolean(me)}
        canManageReports={Boolean(me?.is_admin || me?.user_id === segment.created_by || segmentPermissions.has('manage_reports'))}
      />

      {/* Edit Modal */}
      {showEdit && (
        <SegmentEditModal
          segment={segment}
          onClose={() => {
            setShowEdit(false)
            queryClient.invalidateQueries({ queryKey: ['segment', id] })
          }}
        />
      )}
    </div>
  )
}
