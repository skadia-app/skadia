// frontend/src/components/segment-editor/SegmentSidebar.tsx
import { useMemo, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Check, Loader2, RefreshCw, X } from 'lucide-react'
import { listObjectAccessScopes } from '@/api/accessScopes'
import type { SegmentVisibility } from '@/api/segments'
import {
  BIDIRECTIONAL_DEFAULT,
  type SegType,
  type ClimbType,
} from './constants'
import { Pill } from '../ui/Pill'
import { DifficultySelects } from './DifficultySelects'
import { SurfaceDropdown } from './SurfaceDropdown'
import { ClimbTypePills } from './ClimbTypePills'
import { DifficultyColorPicker } from './DifficultyColorPicker'
import { SportCategoryChips } from './SportCategoryChips'
import { isCategoryFullyActive, expandCategoryToSportTypes } from '@/lib/sportCategoryHelpers'
import { cn } from '@/lib/utils'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { filterSegmentSportSelection, removeSegmentSportSelectionKeys } from './segmentSportSelection'
import { segmentEligibleSelectionKeys } from '@/lib/segmentProjection'

export interface SegmentMetadata {
  name: string
  segType: SegType
  sportTypes: string[] | null
  sacScale: string | null
  mtbScale: string | null
  surface: string | null
  difficultyColor: 'blue' | 'red' | 'black' | null
  bidirectional: boolean
  userEditedBidirectional: boolean
  isPublic: boolean
  visibility: SegmentVisibility
  accessScopeId?: string | null
  climbType?: ClimbType
}

export interface LiveInfo {
  distanceKm: number
  gainM: number
  lossM: number
  elevationMaxM?: number | null
}

export interface SegmentSidebarProps {
  mode: 'track' | 'draw' | 'lap' | 'checkpoint'
  meta: SegmentMetadata
  onMetaChange: (patch: Partial<SegmentMetadata>) => void
  info: LiveInfo | null
  allSportTypes: Array<{ key: string; label: string; category: string }>
  /** Segment group of the source activity (bike/foot/ski/paddle/swim/generic).
   *  Used to auto-expand the matching sport category on mount. */
  defaultSportGroup?: string
  onEnrichmentRequest: () => void
  enrichmentState: { loading: boolean; coverage?: number; lastRequestedAt?: number }
  enrichmentSources: Partial<Record<'surface' | 'sac' | 'mtb', { coverage: number } | null>>
  onClearEnrichmentSource: (field: 'surface' | 'sac' | 'mtb') => void
  enrichmentDisabledReason?: string
  enrichmentStatus?: string
}

function eligibleKeys(segType: SegType): readonly string[] | undefined {
  return segType === 'wintersport' || segType === 'wassersport'
    ? segmentEligibleSelectionKeys(segType)
    : undefined
}

export function SegmentSidebar(props: SegmentSidebarProps) {
  const { t } = useTranslation()
  const { formatDistance, formatElevation } = useUnitFormatters()
  const {
    meta,
    onMetaChange,
    info,
    mode,
    allSportTypes,
    // defaultSportGroup is kept for API compatibility; category chips no longer auto-expand.
    onEnrichmentRequest,
    enrichmentState,
    enrichmentSources,
    onClearEnrichmentSource,
    enrichmentDisabledReason,
    enrichmentStatus,
  } = props

  // difficultyColor is derived centrally in SegmentEditorModal.patchMeta (for segType=trail).
  // Sidebar handlers only set the scale value — parent re-derives color automatically.
  // Previously, sidebar ALSO set difficultyColor, causing a double state-update cycle
  // that triggered React's "Maximum update depth exceeded" when Radix DropdownMenu opened
  // (portal mount → state change → re-render → portal re-mount → infinite loop).
  const handleSacChange = (newSac: string | null) => {
    onMetaChange({ sacScale: newSac })
  }

  const handleMtbChange = (newMtb: string | null) => {
    onMetaChange({ mtbScale: newMtb })
  }

  const handleSegTypeChange = (newType: SegType) => {
    const patch: Partial<SegmentMetadata> = { segType: newType }
    if (!meta.userEditedBidirectional) {
      patch.bidirectional = BIDIRECTIONAL_DEFAULT[newType]
    }
    const whitelist = eligibleKeys(newType)
    if (meta.sportTypes && whitelist) {
      patch.sportTypes = filterSegmentSportSelection(meta.sportTypes, whitelist)
    }
    if (newType === 'wintersport' || newType === 'wassersport' || newType === 'lift') {
      patch.sacScale = null
      patch.mtbScale = null
      patch.surface = null
    }
    onMetaChange(patch)
  }

  const handleBidirectionalToggle = () => {
    const newValue = !meta.bidirectional
    const defaultBidirectional = BIDIRECTIONAL_DEFAULT[meta.segType] ?? true
    const isDefault = newValue === defaultBidirectional
    onMetaChange({
      bidirectional: newValue,
      userEditedBidirectional: !isDefault,
    })
  }

  const toggleSportType = (key: string) => {
    const current = meta.sportTypes
    if (current === null) {
      onMetaChange({ sportTypes: [key] })
      return
    }
    const next = current.includes(key) ? removeSegmentSportSelectionKeys(current, [key]) : [...current, key]
    onMetaChange({ sportTypes: next })
  }

  const setAll = () => onMetaChange({ sportTypes: null })

  const toggleUiCategory = (uiKey: string) => {
    const whitelist = eligibleKeys(meta.segType)
    const categoryTypes = expandCategoryToSportTypes(uiKey, whitelist)
    const current = meta.sportTypes

    if (isCategoryFullyActive(uiKey, current, whitelist)) {
      const next = removeSegmentSportSelectionKeys(current, categoryTypes)
      onMetaChange({ sportTypes: next })
    } else {
      const base = current ?? []
      const next = Array.from(new Set([...base, ...categoryTypes]))
      onMetaChange({ sportTypes: next })
    }
  }

  const availableSportTypes = useMemo(() => {
    const whitelist = eligibleKeys(meta.segType)
    if (whitelist) return allSportTypes.filter(s => whitelist.includes(s.key))
    return allSportTypes
  }, [meta.segType, allSportTypes])

  const { data: accessScopesData } = useQuery({
    queryKey: ['object-access-scopes'],
    queryFn: listObjectAccessScopes,
    enabled: meta.visibility === 'shared',
    staleTime: 60_000,
  })
  const checkpointMode = mode === 'checkpoint'
  const formatSignedElevation = (meters: number | null | undefined, sign: '+' | '−') => {
    if (meters == null || !Number.isFinite(meters) || meters <= 0) return '—'
    return `${sign}${formatElevation(Math.abs(meters))}`
  }

  return (
    <aside className="flex h-full w-full flex-col overflow-y-auto border-r border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900">
      {info && (mode === 'track' || (mode === 'draw' && info.distanceKm > 0)) && (
        <div className="mx-3 mt-3 rounded-lg border border-neutral-200 dark:border-neutral-700/60 bg-neutral-50 dark:bg-neutral-800 p-3">
          <div className="grid grid-cols-3 gap-x-2 text-center">
            <MetricCell label={t('segmentEditor.sidebar.metrics.distance')} value={formatDistance(info.distanceKm * 1000)} />
            <MetricCell label={t('segmentEditor.sidebar.metrics.ascent')} value={formatSignedElevation(info.gainM, '+')} valueClassName="text-red-500" />
            <MetricCell label={t('segmentEditor.sidebar.metrics.descent')}  value={formatSignedElevation(info.lossM, '−')} valueClassName="text-green-500" />
          </div>
          {info.elevationMaxM != null && (
            <div className="mt-2 border-t border-neutral-200 dark:border-neutral-700/60 pt-2 text-center">
              <MetricCell label={t('segmentEditor.sidebar.metrics.highestPoint')} value={formatElevation(info.elevationMaxM)} />
            </div>
          )}
        </div>
      )}

      <div className="flex flex-col gap-4 mx-3 py-3">
        <Field label={t('segmentEditor.sidebar.fields.name')} required>
          <input
            type="text"
            value={meta.name}
            onChange={e => onMetaChange({ name: e.target.value })}
            placeholder={t('segmentEditor.sidebar.namePlaceholder')}
            className={cn(
              'w-full rounded-md border border-neutral-300 dark:border-neutral-700',
              'bg-white dark:bg-neutral-800 px-3 py-1.5',
              'text-sm text-neutral-900 dark:text-neutral-100',
              'placeholder:text-neutral-400 dark:placeholder:text-neutral-500',
              'focus:outline-none focus:border-accent-DEFAULT focus:ring-1 focus:ring-accent-DEFAULT',
            )}
            required
          />
        </Field>

        {!checkpointMode && (
          <Field label={t('segmentEditor.sidebar.fields.type')}>
            <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label={t('segmentEditor.sidebar.segmentTypeAria')}>
              {(['standard', 'trail', 'lap', 'wintersport', 'wassersport', 'lift'] as SegType[]).map(segType => (
                <Pill
                  key={segType}
                  role="radio"
                  ariaChecked={meta.segType === segType}
                  active={meta.segType === segType}
                  onClick={() => handleSegTypeChange(segType)}
                >
                  {t(`segmentDetail.types.${segType}`)}
                </Pill>
              ))}
            </div>
          </Field>
        )}

        <Field label={t('segmentEditor.sidebar.fields.sports')}>
          <SportCategoryChips
            segType={meta.segType}
            activeTypes={meta.sportTypes}
            onToggleCategory={toggleUiCategory}
            onSetAll={setAll}
            availableSportTypes={availableSportTypes}
            showDetailPicker
            onToggleSportType={toggleSportType}
          />
        </Field>

        {!checkpointMode && (
        <div>
          <div className="mb-2 flex items-center justify-between">
            <span className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">
              {t('segmentEditor.sidebar.difficultySurface')}
            </span>
            <button
              type="button"
              onClick={onEnrichmentRequest}
              disabled={!!enrichmentDisabledReason || enrichmentState.loading}
              title={enrichmentDisabledReason ?? t('segmentEditor.enrichment.suggestTitle')}
              aria-pressed={!!enrichmentSources.surface || !!enrichmentSources.sac || !!enrichmentSources.mtb}
              className={cn(
                'flex items-center gap-1 rounded-md px-2 py-1 text-[11px] font-medium',
                'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300',
                'border border-neutral-200 dark:border-neutral-700',
                'transition-colors duration-150',
                'hover:bg-accent-DEFAULT/10 hover:text-accent-DEFAULT hover:border-accent-DEFAULT/30',
                'dark:hover:bg-accent-DEFAULT/10 dark:hover:text-accent-400 dark:hover:border-accent-DEFAULT/30',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1 focus-visible:ring-offset-white dark:focus-visible:ring-offset-neutral-900',
                'disabled:cursor-not-allowed disabled:opacity-40',
              )}
            >
              {enrichmentState.loading
                ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                : <RefreshCw className="h-3.5 w-3.5" />}
              <span className="sr-only">{t('segmentEditor.enrichment.suggest')}</span>
            </button>
          </div>

          <div className="space-y-4 rounded-lg border border-neutral-200 dark:border-neutral-700 p-3">
            {/* DifficultySelects + EnrichmentBadges for SAC and MTB */}
            <DifficultySelects
              segType={meta.segType}
              sacScale={meta.sacScale}
              mtbScale={meta.mtbScale}
              onChange={(f, v) => f === 'sac' ? handleSacChange(v) : handleMtbChange(v)}
            />
            {/* EnrichmentBadges: shown when OSM has suggested a value for SAC or MTB */}
            {(enrichmentSources.sac || enrichmentSources.mtb) && meta.segType === 'trail' && (
              <div className="flex gap-2">
                {enrichmentSources.sac && (
                  <div className="flex items-center gap-1 text-[10px] text-neutral-500">
                    <span>SAC:</span>
                    <EnrichmentBadge source={enrichmentSources.sac} onClear={() => onClearEnrichmentSource('sac')} />
                  </div>
                )}
                {enrichmentSources.mtb && (
                  <div className="flex items-center gap-1 text-[10px] text-neutral-500">
                    <span>MTB:</span>
                    <EnrichmentBadge source={enrichmentSources.mtb} onClear={() => onClearEnrichmentSource('mtb')} />
                  </div>
                )}
              </div>
            )}

            {/* SurfaceDropdown + EnrichmentBadge */}
            <div className="flex items-center gap-1.5">
              <div className="flex-1">
                <SurfaceDropdown
                  segType={meta.segType}
                  value={meta.surface}
                  onChange={(v) => onMetaChange({ surface: v })}
                />
              </div>
              {enrichmentSources.surface && (
                <EnrichmentBadge source={enrichmentSources.surface} onClear={() => onClearEnrichmentSource('surface')} />
              )}
            </div>

            <DifficultyColorPicker
              segType={meta.segType}
              value={meta.difficultyColor}
              onChange={(v) => onMetaChange({ difficultyColor: v })}
            />
            {meta.segType !== 'trail' && meta.segType !== 'wintersport' && (
              <div className="text-xs text-neutral-400 italic">{t('segmentEditor.sidebar.sacMtbTrailOnly')}</div>
            )}
          </div>
        </div>
        )}

        {/* Elevation profile: only visible for standard/trail; ClimbTypePills returns null for others,
            but wrapping <Field> would show an empty label row, so we guard here. */}
        {!checkpointMode && (meta.segType === 'standard' || meta.segType === 'trail') && (
          <Field label={t('segmentEditor.sidebar.fields.elevationProfile')}>
            <ClimbTypePills
              segType={meta.segType}
              value={meta.climbType ?? 'standard'}
              onChange={(v) => onMetaChange({ climbType: v })}
            />
          </Field>
        )}

        {!checkpointMode && (
          <Field label={t('segmentEditor.sidebar.fields.direction')}>
            <ToggleRow
              checked={meta.bidirectional}
              onChange={handleBidirectionalToggle}
              label={t('segmentEditor.sidebar.bidirectional')}
            />
          </Field>
        )}

        <Field label={t('segmentEditor.sidebar.fields.visibility')}>
          <select
            value={meta.visibility}
            onChange={e => {
              const visibility = e.target.value as SegmentVisibility
              onMetaChange({
                visibility,
                isPublic: visibility === 'public',
                accessScopeId: visibility === 'shared' ? meta.accessScopeId ?? null : null,
              })
            }}
            className={cn(
              'w-full rounded-md border border-neutral-300 dark:border-neutral-700',
              'bg-white dark:bg-neutral-800 px-3 py-1.5 text-sm text-neutral-900 dark:text-neutral-100',
              'focus:outline-none focus:border-accent-DEFAULT focus:ring-1 focus:ring-accent-DEFAULT',
            )}
          >
            <option value="private">{t('segmentsList.visibility.private')}</option>
            <option value="shared">{t('segmentsList.visibility.shared')}</option>
            <option value="public">{t('segmentsList.visibility.public')}</option>
          </select>
          {meta.visibility === 'shared' && (
            <select
              aria-label={t('segmentEditor.sidebar.scopeAria')}
              value={meta.accessScopeId ?? ''}
              onChange={e => onMetaChange({ accessScopeId: e.target.value || null })}
              className={cn(
                'mt-2 w-full rounded-md border border-neutral-300 dark:border-neutral-700',
                'bg-white dark:bg-neutral-800 px-3 py-1.5 text-sm text-neutral-900 dark:text-neutral-100',
                'focus:outline-none focus:border-accent-DEFAULT focus:ring-1 focus:ring-accent-DEFAULT',
              )}
            >
              <option value="">{t('segmentEditor.sidebar.chooseScope')}</option>
              {(accessScopesData?.scopes ?? []).map(scope => (
                <option key={scope.id} value={scope.id}>
                  {scope.label?.trim() || t('segmentEditor.sidebar.fallbackScope', { id: scope.id.slice(0, 8) })}
                </option>
              ))}
            </select>
          )}
        </Field>
      </div>

      {/* Live-region for screen-reader enrichment announcements (§4.14) */}
      <div aria-live="polite" aria-atomic="true" className="sr-only">
        {enrichmentStatus ?? (enrichmentState.loading ? t('segmentEditor.enrichment.status.loading') : '')}
      </div>
    </aside>
  )
}

function MetricCell({ label, value, valueClassName }: { label: string; value: string; valueClassName?: string }) {
  return (
    <div>
      <div className="text-[10px] font-medium tracking-wide text-neutral-500 dark:text-neutral-400 uppercase">{label}</div>
      <div className={cn('mt-0.5 text-sm font-semibold', valueClassName ?? 'text-neutral-900 dark:text-neutral-100')}>{value}</div>
    </div>
  )
}

function Field({ label, children, required }: { label: string; children: ReactNode; required?: boolean }) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center gap-1.5 text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">
        <span>{label}</span>
        {required && <span className="text-red-500">*</span>}
      </div>
      {children}
    </div>
  )
}

function ToggleRow({ checked, onChange, label }: { checked: boolean; onChange: React.ChangeEventHandler<HTMLInputElement>; label: string }) {
  return (
    <label className="flex cursor-pointer items-center gap-2.5 text-sm text-neutral-700 dark:text-neutral-300 select-none hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors">
      <input type="checkbox" checked={checked} onChange={onChange} className="h-4 w-4 rounded border-neutral-400 dark:border-neutral-600 accent-accent-DEFAULT" />
      {label}
    </label>
  )
}

function EnrichmentBadge({
  source,
  onClear,
}: {
  source?: { coverage: number } | null
  onClear: () => void
}) {
  const { t } = useTranslation()
  if (!source) return null
  const pct = Math.round(source.coverage * 100)
  return (
    <span className="flex flex-shrink-0 items-center gap-1 rounded-md border border-accent-DEFAULT/30 bg-accent-DEFAULT/10 px-1.5 py-0.5 text-[10px] font-medium text-accent-DEFAULT dark:text-accent-400">
      <Check className="h-2.5 w-2.5 flex-shrink-0" aria-hidden="true" />
      {pct === 100 ? '100%' : `${pct}%`}
      <button
        type="button"
        onClick={onClear}
        aria-label={t('segmentEditor.enrichment.removeSource')}
        className={cn(
          'flex h-3.5 w-3.5 flex-shrink-0 items-center justify-center rounded',
          'text-accent-DEFAULT dark:text-accent-400 transition-colors duration-150',
          'hover:bg-accent-DEFAULT/20 hover:text-accent-600',
          'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent-DEFAULT',
        )}
      >
        <X className="h-2.5 w-2.5" />
      </button>
    </span>
  )
}
