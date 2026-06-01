import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { fetchSegment, updateSegment, publishSegment, reverseSegment, type Segment, type SegmentVisibility, type UpdateSegmentPayload } from '@/api/segments'
import { getMe } from '@/api/auth'
import { ApiError } from '@/api/client'
import { ModalShell } from '@/components/ui/ModalShell'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureAvailable } from '@/lib/deploymentCapabilities'

// Shared Components
import { DifficultySelects } from './segment-editor/DifficultySelects'
import { SurfaceDropdown } from './segment-editor/SurfaceDropdown'
import { DifficultyColorPicker } from './segment-editor/DifficultyColorPicker'
import { ClimbTypePills } from './segment-editor/ClimbTypePills'
import { MapPreview } from './segment-editor/MapPreview'
import { SportCategoryChips } from './segment-editor/SportCategoryChips'
import { expandCategoryToSportTypes, isCategoryFullyActive, whitelistForSegmentType } from '@/lib/sportCategoryHelpers'
import { BIDIRECTIONAL_DEFAULT, type ClimbType, type SegType } from './segment-editor/constants'
import { Pill } from './ui/Pill'
import { filterSegmentSportSelection, removeSegmentSportSelectionKeys } from './segment-editor/segmentSportSelection'
import { SPORT_DEFS, sportLabel } from '@/lib/sportTaxonomy'
import { isSupportedLanguage } from '@/i18n/languages'
import { buildExistingSegmentSportPatch } from './segment-editor/segmentSave'
import VisibilityAccessField from './target-objects/VisibilityAccessField'

const SEGMENT_TYPES: readonly SegType[] = ['standard', 'trail', 'lap', 'wintersport', 'wassersport', 'lift']
const CLIMB_TYPES: readonly ClimbType[] = ['standard', 'bergauf', 'bergab']

interface Props {
  segment: Segment
  onClose: () => void
}

function isSegType(value: unknown): value is SegType {
  return typeof value === 'string' && SEGMENT_TYPES.includes(value as SegType)
}

function isClimbType(value: unknown): value is ClimbType {
  return typeof value === 'string' && CLIMB_TYPES.includes(value as ClimbType)
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

export default function SegmentEditModal({ segment: initialSegment, onClose }: Props) {
  const qc = useQueryClient()
  const { t, i18n } = useTranslation()
  const language = isSupportedLanguage(i18n.language) ? i18n.language : 'de'
  const isCheckpointCourse = initialSegment.course_kind === 'checkpoint_sequence'
  const allSportTypes = useMemo(
    () => SPORT_DEFS.map(s => ({ key: s.sportType, label: sportLabel(s.sportType, undefined, { language }), category: s.categoryKey })),
    [language],
  )
  
  // Load full segment data, including GeoJSON for MapPreview.
  const { data: segment = initialSegment, isLoading: isLoadingFull } = useQuery({
    queryKey: ['segment', initialSegment.id, 'full'],
    queryFn: () => fetchSegment(initialSegment.id, { includeGeoJSON: true }),
    staleTime: 5 * 60 * 1000,
  })

  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 60_000 })

  const [name, setName] = useState(initialSegment.name)
  const [description, setDescription] = useState(initialSegment.description ?? '')
  const [diffColor, setDiffColor] = useState((initialSegment.segment_type_meta?.difficulty_color as 'blue' | 'red' | 'black' | null) ?? null)
  const [sacScale, setSacScale] = useState((initialSegment.segment_type_meta?.sac_scale as string | null) ?? null)
  const [mtbScale, setMtbScale] = useState((initialSegment.segment_type_meta?.mtb_scale as string | null) ?? null)
  const [surface, setSurface] = useState((initialSegment.segment_type_meta?.surface as string | null) ?? null)
  const [newVersion, setNewVersion] = useState(false)
  
  const rawInitialClimbType = initialSegment.climb_type
  const initialClimbTypeSupported = isClimbType(rawInitialClimbType)
  const initialClimbType: ClimbType = initialClimbTypeSupported ? rawInitialClimbType : 'standard'
  const [climbType, setClimbType] = useState<ClimbType>(initialClimbType)
  const [climbTypeTouched, setClimbTypeTouched] = useState(false)
  
  const [visibility, setVisibility] = useState<SegmentVisibility>(initialSegment.visibility ?? 'public')
  const [accessScopeId, setAccessScopeId] = useState<string | null>(initialSegment.access_scope_id ?? null)
  const [allowedSportTypes, setAllowedSportTypes] = useState<string[] | null>(() => {
    if (initialSegment.allowed_predicates != null) {
      return initialSegment.allowed_predicates.map(p => p.sub_type ? `${p.sport}:${p.sub_type}` : p.sport)
    }
    return null
  })
  const [bidirectional, setBidirectional] = useState<boolean>(initialSegment.bidirectional ?? true)
  const [userEditedBidirectional, setUserEditedBidirectional] = useState(false)
  const rawInitialSegType = initialSegment.segment_type
  const initialSegTypeSupported = isSegType(rawInitialSegType)
  const initialSegType: SegType = initialSegTypeSupported ? rawInitialSegType : 'standard'
  const [segType, setSegType] = useState<SegType>(initialSegType)
  const [segmentTypeTouched, setSegmentTypeTouched] = useState(false)
  const [sportSelectionTouched, setSportSelectionTouched] = useState(false)

  function handleSegTypeChange(newType: SegType) {
    setSegmentTypeTouched(true)
    setSegType(newType)
    if (!userEditedBidirectional) {
      setBidirectional(BIDIRECTIONAL_DEFAULT[newType])
    }
    if (newType === 'wintersport' || newType === 'wassersport' || newType === 'lift') {
      setSacScale(null)
      setMtbScale(null)
      setSurface(null)
      setDiffColor(null)
    }
    const whitelist = whitelistForSegmentType(newType)
    if (whitelist) {
      setAllowedSportTypes(prev => {
        return filterSegmentSportSelection(prev, whitelist)
      })
    }
  }

  const { openModal } = useModal()
  const { addToast } = useToast()
  const { data: capabilities } = useCapabilities()
  const segmentPublishAvailable = featureAvailable(capabilities, 'segment_publish')

  const mutation = useMutation({
    mutationFn: () => {
      const meta: Record<string, unknown> = { ...segment.segment_type_meta }
      if (segType === 'trail') {
        if (sacScale) meta.sac_scale = sacScale; else delete meta.sac_scale
        if (mtbScale) meta.mtb_scale = mtbScale; else delete meta.mtb_scale
        if (surface) meta.surface = surface; else delete meta.surface
      } else {
        delete meta.sac_scale
        delete meta.mtb_scale
        delete meta.surface
      }
      if (segType === 'trail' || segType === 'wintersport') {
        if (diffColor) meta.difficulty_color = diffColor; else delete meta.difficulty_color
      } else {
        delete meta.difficulty_color
      }

      const sportPatch = buildExistingSegmentSportPatch({
        segType,
        initialSegType,
        selection: allowedSportTypes,
        initialAllowedPredicates: initialSegment.allowed_predicates,
        touched: sportSelectionTouched,
      })
      if (isCheckpointCourse) {
        const payload: UpdateSegmentPayload = {
          name: name.trim(),
          description: description.trim() || undefined,
          ...sportPatch,
        }
        if (!(segmentPublishAvailable && initialSegment.visibility === 'private' && visibility === 'public')) {
          payload.visibility = visibility
          payload.access_scope_id = visibility === 'shared' ? accessScopeId : null
        }
        return updateSegment(initialSegment.id, payload)
      }
      const payload: UpdateSegmentPayload = {
        name: name.trim(),
        description: description.trim() || undefined,
        segment_type_meta: Object.keys(meta).length > 0 ? meta : undefined,
        new_version: newVersion || undefined,
        bidirectional,
        ...sportPatch,
      }
      if (initialSegTypeSupported || segmentTypeTouched) {
        payload.segment_type = segType
      }
      if (segType === 'lap') {
        payload.climb_type = 'standard'
      } else if (initialClimbTypeSupported || climbTypeTouched || segmentTypeTouched) {
        payload.climb_type = climbType
      }
      if (!(segmentPublishAvailable && initialSegment.visibility === 'private' && visibility === 'public')) {
        payload.visibility = visibility
        payload.access_scope_id = visibility === 'shared' ? accessScopeId : null
      }
      return updateSegment(initialSegment.id, payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['segments'] })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      addToast({ type: 'success', title: t('segmentEdit.toasts.updated') })
      onClose()
    },
  })

  const publishMut = useMutation({
    mutationFn: (options?: { sourceActivityPublishConfirmed?: boolean }) => (
      options ? publishSegment(initialSegment.id, options) : publishSegment(initialSegment.id)
    ),
    onSuccess: () => {
      addToast({
        type: 'success',
        title: t('segmentDetail.toasts.publishSuccessTitle'),
        message: t('segmentEdit.toasts.publishStarted'),
      })
      qc.invalidateQueries({ queryKey: ['segments'] })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      onClose()
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

  const reverseMutation = useMutation({
    mutationFn: () => reverseSegment(initialSegment.id),
    onSuccess: () => {
      addToast({
        type: 'success',
        title: t('segmentEdit.toasts.reversedTitle'),
        message: t('segmentEdit.toasts.reversedMessage'),
      })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      qc.invalidateQueries({ queryKey: ['segments'] })
      onClose()
    },
    onError: () => {
      addToast({ type: 'error', title: t('segmentEdit.toasts.reverseFailed'), duration: 0 })
    },
  })

  function handleReverse() {
    const effortMsg = segment.effort_count > 0
      ? t('segmentEdit.modals.reverseWithEfforts')
      : t('segmentEdit.modals.reverseMessage')
    openModal({
      title: t('segmentEdit.modals.reverseTitle'),
      message: effortMsg,
      buttons: [
        { label: t('common.cancel'), variant: 'default', onClick: () => {} },
        { label: t('segmentEdit.actions.reverse'), variant: 'destructive', onClick: () => reverseMutation.mutate() },
      ],
    })
  }

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md" className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800 space-y-4">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">{t('segmentEdit.title')}</h2>

        {/* Map Preview */}
        <MapPreview 
          geojsonTrack={segment.track_geojson || ''} 
          height="160px" 
          className="mb-2 rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-800"
        />

        <input
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder={t('segmentEdit.fields.name')}
          className={AUTH_INPUT_CLASS}
        />

        <textarea
          value={description}
          onChange={e => setDescription(e.target.value)}
          placeholder={t('segmentEdit.fields.description')}
          rows={2}
          className={`${AUTH_INPUT_CLASS} resize-none`}
        />

        {isCheckpointCourse && (
          <p className="rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-2 text-xs text-neutral-600 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-300">
            {t('segmentEdit.checkpointGeometryLocked')}
          </p>
        )}

        {/* Segment type picker */}
        {!isCheckpointCourse && (
          <div className="space-y-1.5">
            <label className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">{t('segmentEdit.fields.segmentType')}</label>
            <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label={t('segmentEdit.fields.segmentType')}>
              {SEGMENT_TYPES.map(type => (
                <Pill
                  key={type}
                  active={segType === type}
                  onClick={() => handleSegTypeChange(type)}
                  role="radio"
                  ariaChecked={segType === type}
                  disabled={isLoadingFull}
                >
                  {t(`segmentDetail.types.${type}`)}
                </Pill>
              ))}
            </div>
          </div>
        )}

        {!isCheckpointCourse && (
        <div className="space-y-4">
          <DifficultySelects
            segType={segType}
            sacScale={sacScale}
            mtbScale={mtbScale}
            onChange={(f, v) => f === 'sac' ? setSacScale(v) : setMtbScale(v)}
            disabled={isLoadingFull}
          />

          <SurfaceDropdown
            segType={segType}
            value={surface}
            onChange={setSurface}
            disabled={isLoadingFull}
          />

          <DifficultyColorPicker
            segType={segType}
            value={diffColor}
            onChange={setDiffColor}
            disabled={isLoadingFull}
          />
        </div>
        )}

        <div className="space-y-3">
          {!isCheckpointCourse && (segType === 'standard' || segType === 'trail') && (
            <div className="space-y-1.5">
              <label className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">{t('segmentEdit.fields.climbType')}</label>
              <ClimbTypePills
                segType={segType}
                value={climbType}
                onChange={value => {
                  setClimbTypeTouched(true)
                  setClimbType(value)
                }}
                disabled={isLoadingFull}
              />
            </div>
          )}

          {/* Bidirectional toggle — only shown for non-lap segments (lap = closed loop) */}
          {!isCheckpointCourse && segType !== 'lap' && (
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={bidirectional}
                onChange={e => {
                  const newVal = e.target.checked
                  setBidirectional(newVal)
                  setUserEditedBidirectional(newVal !== BIDIRECTIONAL_DEFAULT[segType])
                }}
                className="rounded border-neutral-400 dark:border-neutral-600 accent-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-700 dark:text-neutral-300">{t('segmentEdit.fields.bidirectional')}</span>
            </label>
          )}
        </div>

        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">{t('segmentEdit.fields.allowedSports')}</label>
          <SportCategoryChips
            segType={segType}
            activeTypes={allowedSportTypes}
            onToggleCategory={(catKey) => {
              setSportSelectionTouched(true)
              const whitelist = whitelistForSegmentType(segType)
              const types = expandCategoryToSportTypes(catKey, whitelist)
              if (types.length === 0) return

              const isFull = isCategoryFullyActive(catKey, allowedSportTypes, whitelist)
              setAllowedSportTypes(prev => {
                const current = prev ?? []
                if (isFull) {
                  return removeSegmentSportSelectionKeys(current, types)
                } else {
                  const next = [...current]
                  types.forEach(t => { if (!next.includes(t)) next.push(t) })
                  return next
                }
              })
            }}
            onSetAll={() => {
              setSportSelectionTouched(true)
              setAllowedSportTypes(null)
            }}
            availableSportTypes={allSportTypes}
            showDetailPicker
            onToggleSportType={(sportType) => {
              setSportSelectionTouched(true)
              setAllowedSportTypes(prev => {
                if (prev === null) return [sportType]
                const next = prev.includes(sportType)
                  ? prev.filter(t => t !== sportType)
                  : [...prev, sportType]
                return next
              })
            }}
          />
        </div>

        {/* Visibility — not for lift segments */}
        {segType !== 'lift' && (
          <div className="space-y-2">
            <VisibilityAccessField
              visibility={visibility}
              accessScopeId={accessScopeId}
              onChange={next => {
                setVisibility(next.visibility as SegmentVisibility)
                setAccessScopeId(next.access_scope_id ?? null)
              }}
            />
            {segmentPublishAvailable && initialSegment.visibility === 'private' && visibility === 'public' && (
              <p className="text-xs text-amber-400">
                {t('segmentEdit.publishHint')}
              </p>
            )}
          </div>
        )}

        {!isCheckpointCourse && (
        <label className="flex items-start gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={newVersion}
            onChange={e => setNewVersion(e.target.checked)}
            className="mt-0.5 rounded border-neutral-400 dark:border-neutral-600 accent-accent-DEFAULT"
          />
          <span className="text-sm text-neutral-700 dark:text-neutral-300">
            {t('segmentEdit.fields.newVersion')}
            <span className="block text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              {t('segmentEdit.fields.newVersionHint')}
            </span>
          </span>
        </label>
        )}

        {mutation.isError && (
          <p className="text-sm text-red-600 dark:text-red-400">
            {(() => {
              const err = mutation.error
              if (err instanceof ApiError) {
                if (err.status === 403) return t('segmentEdit.errors.forbidden')
                if (err.status === 404) return t('segmentDetail.notFound')
                const detail = (err.json?.detail as string | undefined) ?? (err.json?.message as string | undefined)
                if (detail) return t('segmentEdit.errors.saveWithDetail', { detail })
              }
              return t('segmentEdit.errors.save')
            })()}
          </p>
        )}

        <div className="flex gap-2 items-center justify-between pt-2">
          {!isCheckpointCourse && segType !== 'lap' && (
            <button
              type="button"
              onClick={handleReverse}
              disabled={mutation.isPending || publishMut.isPending || reverseMutation.isPending}
              className="cursor-pointer rounded-lg border border-amber-300 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-950/30 px-3 py-1.5 text-sm text-amber-700 dark:text-amber-400 hover:bg-amber-100 dark:hover:bg-amber-900/40 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              ⇄ {t('segmentEdit.actions.reverseDirection')}
            </button>
          )}
          <div className="flex gap-2">
            <button
              onClick={onClose}
              disabled={reverseMutation.isPending}
              className="cursor-pointer px-3 py-1.5 text-sm border border-neutral-300 dark:border-neutral-700 rounded-lg text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {t('common.cancel')}
            </button>
            <button
              onClick={() => {
                if (segmentPublishAvailable && initialSegment.visibility === 'private' && visibility === 'public') {
                  const isForeignAdmin = me?.is_admin && initialSegment.created_by !== me?.user_id
                  openModal({
                    title: t('segmentDetail.modals.publishTitle'),
                    message: isForeignAdmin
                      ? t('segmentDetail.modals.publishAdminMessage')
                      : t('segmentEdit.modals.publishMessage'),
                    buttons: [
                      { label: t('common.cancel'), variant: 'default', onClick: () => {} },
                      { label: t('segmentDetail.actions.publish'), variant: 'destructive', onClick: () => publishMut.mutate(undefined) },
                    ],
                  })
                } else {
                  mutation.mutate()
                }
              }}
              disabled={!name.trim() || (visibility === 'shared' && !accessScopeId) || mutation.isPending || publishMut.isPending || reverseMutation.isPending}
              className="cursor-pointer px-4 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {mutation.isPending || publishMut.isPending ? t('segmentEdit.actions.saving') : t('segmentEdit.actions.save')}
            </button>
          </div>
        </div>
    </ModalShell>
  )
}
