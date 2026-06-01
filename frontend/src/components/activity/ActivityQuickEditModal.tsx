import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Loader2, Pencil, Save, X } from 'lucide-react'
import { fetchActivity, fetchActivityStreams, updateActivity } from '@/api/activities'
import { assignActivitiesToTag, createTag, fetchTags, unassignActivitiesFromTag, type Tag } from '@/api/tags'
import type { Activity } from '@/api/types'
import { ModalShell } from '@/components/ui/ModalShell'
import Toggle from '@/components/Toggle'
import SportTuplePicker from './SportTuplePicker'
import ActivityQuickEditMiniMap, { trackCoordsFromStreams } from './ActivityQuickEditMiniMap'
import {
  activityQuickEditDraftFromActivity,
  buildActivityQuickEditPatch,
  diffQuickEditTags,
  quickEditTagDraftAfterModifier,
  quickEditTagDraftAfterToggle,
  quickEditModifierSelected,
  quickEditTagRemovalLocked,
  quickEditTagSuggestions,
  quickEditSyncedModifierForTag,
  type ActivityQuickEditDraft,
} from './quickEdit'
import { formatDuration } from '@/lib/utils'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { getTagBadgeStyle } from '@/lib/tagUtils'
import { fmtTime } from '@/lib/format'
import { modifierLabel, type SportModifierKey } from '@/lib/sportModifiers'
import { QuickEditApplicableModifierKeys } from '@/lib/generatedQuickEditApplicableModifiers'
import { isSupportedLanguage, type SupportedLanguage } from '@/i18n/languages'

interface ActivityQuickEditModalProps {
  activity: Activity
  open: boolean
  onClose: () => void
  onSaved: (activity: Activity) => void
  showMoreEdit?: boolean
}

const VISIBILITY_OPTIONS: Activity['visibility'][] = [
  'private',
  'bekannte',
  'instanz',
  'federation',
  'public',
]

const FEEL_OPTIONS = [
  '😫',
  '😕',
  '😐',
  '🙂',
  '😄',
] as const

const FALLBACK_TAG_COLORS = ['#F97316', '#3B82F6', '#22C55E', '#A855F7', '#F43F5E', '#06B6D4']

function tagByName(tags: Tag[], name: string): Tag | undefined {
  return tags.find(tag => tag.name.toLowerCase() === name.toLowerCase())
}

function hasPatchValues(patch: Record<string, unknown>): boolean {
  return Object.keys(patch).length > 0
}

function activityLocationText(activity: Activity): string | null {
  const { start_city, end_city, start_state, start_country } = activity
  if (!start_city) return null
  const region = [start_state, start_country].filter(Boolean).join(', ')
  const place = end_city && end_city !== start_city ? `${start_city} → ${end_city}` : start_city
  return region ? `${place} · ${region}` : place
}

function currentLanguage(language: string): SupportedLanguage {
  const code = language.split('-')[0]
  return isSupportedLanguage(code) ? code : 'de'
}

function visibilityLabel(value: Activity['visibility'], t: TFunction): string {
  return t(`ruleEditor.optionLabels.visibility.${value}`)
}

function feelLabel(index: number, t: TFunction): string {
  return t(`activityQuickEdit.feel.${index}`)
}

function tagRemovalLabel(draft: ActivityQuickEditDraft, tagName: string, t: TFunction, language: SupportedLanguage): string {
  const modifierKey = quickEditSyncedModifierForTag(draft, tagName)
  if (modifierKey && quickEditModifierSelected(draft, modifierKey)) {
    return t('activityQuickEdit.tags.removeDisablesModifier', { tag: tagName, modifier: modifierLabel(modifierKey, language) })
  }
  return t('activityQuickEdit.tags.remove', { tag: tagName })
}

export default function ActivityQuickEditModal({ activity, open, onClose, onSaved, showMoreEdit = true }: ActivityQuickEditModalProps) {
  const { t, i18n } = useTranslation()
  const { formatDistance, formatElevationGain, formatElevationLoss } = useUnitFormatters()
  const language = currentLanguage(i18n.language)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ActivityQuickEditDraft>(() => activityQuickEditDraftFromActivity(activity))
  const [tagSearch, setTagSearch] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [draftDirty, setDraftDirty] = useState(false)
  const [privacyDetailsOpen, setPrivacyDetailsOpen] = useState(false)

  useEffect(() => {
    if (open) {
      setDraft(activityQuickEditDraftFromActivity(activity))
      setDraftDirty(false)
      setTagSearch('')
      setError(null)
      setPrivacyDetailsOpen(false)
    }
  }, [activity.id, open])

  const { data: fullActivity } = useQuery({
    queryKey: ['activity', activity.id],
    queryFn: () => fetchActivity(activity.id),
    enabled: open,
  })

  const baseActivity = fullActivity ?? activity

  useEffect(() => {
    if (open && fullActivity && !draftDirty) {
      setDraft(activityQuickEditDraftFromActivity(fullActivity))
    }
  }, [draftDirty, fullActivity, open])

  const { data: allTags } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    staleTime: 60_000,
    enabled: open,
  })

  const { data: streams = [], isLoading: streamsLoading } = useQuery({
    queryKey: ['activity-streams', activity.id],
    queryFn: () => fetchActivityStreams(activity.id),
    enabled: open,
  })

  const trackCoords = useMemo(() => trackCoordsFromStreams(streams), [streams])
  const hasLoadedGpsTrack = trackCoords.length >= 2
  const visibleSuggestions = useMemo(() => {
    return quickEditTagSuggestions(allTags ?? [], draft.tags, tagSearch)
  }, [allTags, draft.tags, tagSearch])
  const quickEditModifierKeys = useMemo(() => [...QuickEditApplicableModifierKeys], [])

  if (!open) return null
  if (!fullActivity) {
    return (
      <ModalShell onClose={onClose} maxWidth="max-w-5xl" className="w-[calc(100vw-2rem)]">
        <div className="flex min-h-48 items-center justify-center gap-3 text-sm text-neutral-600 dark:text-neutral-300">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>{t('activityQuickEdit.loading')}</span>
        </div>
      </ModalShell>
    )
  }

  function patchDraft(patch: Partial<ActivityQuickEditDraft>) {
    setDraftDirty(true)
    setDraft(prev => ({ ...prev, ...patch }))
  }

  function toggleTag(tagName: string, selected: boolean) {
    setDraftDirty(true)
    setDraft(prev => quickEditTagDraftAfterToggle(prev, tagName, selected))
  }

  function toggleModifier(modifier: SportModifierKey, selected: boolean) {
    setDraftDirty(true)
    setDraft(prev => quickEditTagDraftAfterModifier(prev, modifier, selected))
  }

  function toggleDataVisibility(key: keyof ActivityQuickEditDraft['dataVisibility']) {
    setDraftDirty(true)
    setDraft(prev => ({
      ...prev,
      dataVisibility: { ...prev.dataVisibility, [key]: !prev.dataVisibility[key] },
    }))
  }

  function tagStyle(tagName: string, index: number) {
    const tag = (allTags ?? []).find(t => t.name.toLowerCase() === tagName.toLowerCase())
    return getTagBadgeStyle(tag?.color || FALLBACK_TAG_COLORS[index % FALLBACK_TAG_COLORS.length])
  }

  async function syncTagDiffs(
    currentTags: string[],
    nextTags: string[],
    latestActivity: Activity,
    knownTags: Tag[],
    onMutated?: () => void,
  ): Promise<Activity> {
    const diff = diffQuickEditTags(currentTags, nextTags)
    let updated = latestActivity

    for (const tagName of diff.remove) {
      const tag = tagByName(knownTags, tagName)
      if (tag?.id) {
        await unassignActivitiesFromTag(tag.id, [activity.id], { deferActivityFileRefresh: true })
        onMutated?.()
      } else {
        const remaining = updated.tags.filter(tagValue => tagValue.toLowerCase() !== tagName.toLowerCase())
        updated = await updateActivity(activity.id, { tags: remaining })
        onMutated?.()
      }
    }

    for (const tagName of diff.add) {
      let tag = tagByName(knownTags, tagName)
      if (!tag) {
        tag = await createTag({ name: tagName })
        onMutated?.()
      }
      await assignActivitiesToTag(tag.id, [activity.id], { deferActivityFileRefresh: true })
      onMutated?.()
    }

    if (diff.add.length > 0 || diff.remove.length > 0) {
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      return updateActivity(activity.id, {})
    }
    return updated
  }

  async function handleSave() {
    setSaving(true)
    setError(null)
    let savedAny = false
    try {
      const patch = buildActivityQuickEditPatch(baseActivity, draft)
      const knownTags: Tag[] = allTags ?? await queryClient.fetchQuery<Tag[]>({ queryKey: ['tags'], queryFn: fetchTags })
      const tagDiff = diffQuickEditTags(baseActivity.tags ?? [], draft.tags)
      const tagsChanged = tagDiff.add.length > 0 || tagDiff.remove.length > 0
      let updated = await syncTagDiffs(baseActivity.tags ?? [], draft.tags, baseActivity, knownTags, () => { savedAny = true })
      if (tagsChanged) savedAny = true
      if (hasPatchValues(patch)) {
        updated = await updateActivity(activity.id, patch)
        savedAny = true
      }
      queryClient.setQueryData(['activity', activity.id], updated)
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      onSaved(updated)
      onClose()
    } catch (err) {
      if (savedAny) {
        try {
          const current = await fetchActivity(activity.id)
          queryClient.setQueryData(['activity', activity.id], current)
          queryClient.invalidateQueries({ queryKey: ['activities'] })
          setDraft(activityQuickEditDraftFromActivity(current))
          setDraftDirty(false)
          onSaved(current)
        } catch {
          queryClient.invalidateQueries({ queryKey: ['activities'] })
          queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
        }
        setError(t('activityQuickEdit.errors.partialSave'))
      } else {
        setError(err instanceof Error ? err.message : t('activityQuickEdit.errors.saveFailed'))
      }
    } finally {
      setSaving(false)
    }
  }

  const tagSearchTrimmed = tagSearch.trim()
  const canCreateTag = tagSearchTrimmed.length > 0 &&
    !(allTags ?? []).some(tag => tag.name.toLowerCase() === tagSearchTrimmed.toLowerCase()) &&
    !draft.tags.some(tag => tag.toLowerCase() === tagSearchTrimmed.toLowerCase())
  const recordingDeviceName = baseActivity.recording_device_name || baseActivity.source_device
  const locationText = activityLocationText(baseActivity)
  const hasMapContext = Boolean(baseActivity.sunrise_at || baseActivity.sunset_at || locationText)
  const elevationValue = [formatElevationGain(baseActivity.elevation_gain_m), formatElevationLoss(baseActivity.elevation_loss_m)]
    .filter(Boolean)
    .join(' / ') || '—'
  const heartRateValue = [formatHeartRate(baseActivity.avg_heart_rate), formatHeartRate(baseActivity.max_heart_rate)]
    .filter(Boolean)
    .join(' / ')
  const powerValue = [formatPower(baseActivity.avg_power_w), formatPower(baseActivity.max_power_w)]
    .filter(Boolean)
    .join(' / ')

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-6xl" className="max-h-[92vh] w-[calc(100vw-2rem)] overflow-y-auto">
      <div className="space-y-5">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">{t('activityQuickEdit.title')}</h2>
            <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
              {t('activityQuickEdit.subtitle')}
            </p>
          </div>
          <div className="flex min-w-0 items-start gap-3">
            {recordingDeviceName && (
              <div className="hidden max-w-[18rem] text-right text-sm sm:block">
                <div className="text-[11px] text-neutral-400 dark:text-neutral-500">{t('activityQuickEdit.recordingDevice')}</div>
                {baseActivity.recording_device_id ? (
                  <Link
                    to={`/hardware/detail/${baseActivity.recording_device_id}`}
                    className="block truncate text-neutral-600 transition-colors hover:text-accent-DEFAULT dark:text-neutral-400"
                    title={recordingDeviceName}
                  >
                    {recordingDeviceName}
                  </Link>
                ) : (
                  <span className="block truncate text-neutral-600 dark:text-neutral-400" title={recordingDeviceName}>{recordingDeviceName}</span>
                )}
              </div>
            )}
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg p-1.5 text-neutral-400 hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-neutral-800 dark:hover:text-neutral-200"
              aria-label={t('activityQuickEdit.close')}
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        <div className="grid gap-5 lg:grid-cols-[1.4fr_0.8fr]">
          <div className="space-y-4">
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('activityQuickEdit.fields.title')}</span>
              <input
                value={draft.title}
                onChange={e => patchDraft({ title: e.target.value })}
                className="w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-100"
              />
            </label>

            <QuickEditSection title={t('activityQuickEdit.sections.sportType')}>
              <SportTuplePicker
                sportType={draft.sportType}
                subType={draft.subType}
                onChange={(sportType, subType) => patchDraft({ sportType, subType })}
              />
              <div className="mt-3 grid gap-2 md:grid-cols-3">
                {quickEditModifierKeys.map((key) => {
                  const label = modifierLabel(key, language)
                  return (
                    <ToggleRow key={key} label={label}>
                      <Toggle
                        checked={quickEditModifierSelected(draft, key)}
                        onChange={checked => toggleModifier(key, checked)}
                        size="sm"
                        aria-label={label}
                      />
                    </ToggleRow>
                  )
                })}
              </div>
            </QuickEditSection>

            <QuickEditSection
              title={t('activityQuickEdit.sections.privacy')}
              action={(
                <button
                  type="button"
                  onClick={() => setPrivacyDetailsOpen(open => !open)}
                  className="inline-flex items-center gap-1 rounded-md border border-neutral-200 px-2 py-1 text-xs font-medium text-neutral-600 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800"
                  aria-expanded={privacyDetailsOpen}
                >
                  <Pencil className="h-3.5 w-3.5" />
                  {t('activityQuickEdit.details')}
                </button>
              )}
            >
              <label className="block">
                <span className="sr-only">{t('activityQuickEdit.fields.visibility')}</span>
                <select
                  value={draft.visibility}
                  onChange={e => patchDraft({ visibility: e.target.value as Activity['visibility'] })}
                  className="w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-100"
                >
                  {VISIBILITY_OPTIONS.map(option => (
                    <option key={option} value={option}>{visibilityLabel(option, t)}</option>
                  ))}
                </select>
              </label>
              {privacyDetailsOpen && (
                <div className="mt-3 grid gap-2 md:grid-cols-3">
                  <ToggleRow label={t('activityQuickEdit.privacy.hideMap')}>
                    <Toggle
                      checked={draft.hideMap}
                      onChange={checked => patchDraft({ hideMap: checked })}
                      size="sm"
                      aria-label={t('activityQuickEdit.privacy.hideMap')}
                    />
                  </ToggleRow>
                  <ToggleRow label={t('activityQuickEdit.privacy.hideHeartRate')}>
                    <Toggle
                      checked={!draft.dataVisibility.heart_rate}
                      onChange={() => toggleDataVisibility('heart_rate')}
                      size="sm"
                      aria-label={t('activityQuickEdit.privacy.hideHeartRate')}
                    />
                  </ToggleRow>
                  <ToggleRow label={t('activityQuickEdit.privacy.hidePower')}>
                    <Toggle
                      checked={!draft.dataVisibility.power}
                      onChange={() => toggleDataVisibility('power')}
                      size="sm"
                      aria-label={t('activityQuickEdit.privacy.hidePower')}
                    />
                  </ToggleRow>
                </div>
              )}
            </QuickEditSection>

            <QuickEditSection title={t('activityQuickEdit.sections.statistics')}>
              <div className="grid gap-2 md:grid-cols-2">
                {([
                  ['excludeFromHeatmap', t('activityQuickEdit.statistics.excludeFromHeatmap')],
                  ['excludeFromStats', t('activityQuickEdit.statistics.excludeFromStats')],
                ] as const).map(([key, label]) => (
                  <ToggleRow key={key} label={label}>
                    <Toggle
                      checked={key === 'excludeFromHeatmap' && quickEditModifierSelected(draft, 'is_virtual') ? true : draft[key]}
                      onChange={checked => patchDraft({ [key]: checked })}
                      size="sm"
                      disabled={key === 'excludeFromHeatmap' && quickEditModifierSelected(draft, 'is_virtual')}
                      aria-label={label}
                    />
                  </ToggleRow>
                ))}
              </div>
            </QuickEditSection>

            <div>
              <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('activityQuickEdit.tags.title')}</span>
              <div className="rounded-lg border border-neutral-200 p-3 dark:border-neutral-700">
                <div className="mb-2 flex flex-wrap gap-2">
                  {draft.tags.map(tag => {
                    const locked = quickEditTagRemovalLocked(draft, tag)
                    const removalLabel = tagRemovalLabel(draft, tag, t, language)
                    return (
                      <span key={tag} className="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium" style={tagStyle(tag, draft.tags.indexOf(tag))}>
                        {tag}
                        {!locked && (
                          <button type="button" onClick={() => toggleTag(tag, false)} className="opacity-70 hover:opacity-100" aria-label={removalLabel} title={removalLabel}>
                            <X className="h-3 w-3" />
                          </button>
                        )}
                      </span>
                    )
                  })}
                  {draft.tags.length === 0 && (
                    <span className="text-sm text-neutral-400">{t('activityQuickEdit.tags.empty')}</span>
                  )}
                </div>
                <div className="flex gap-2">
                  <input
                    value={tagSearch}
                    onChange={e => setTagSearch(e.target.value)}
                    placeholder={t('activityQuickEdit.tags.searchPlaceholder')}
                    className="min-w-0 flex-1 rounded border border-neutral-200 bg-transparent px-2 py-1.5 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:text-neutral-100"
                  />
                  {canCreateTag && (
                    <button type="button" onClick={() => { toggleTag(tagSearchTrimmed, true); setTagSearch('') }} className="rounded border border-neutral-200 px-3 py-1.5 text-sm hover:bg-neutral-50 dark:border-neutral-700 dark:hover:bg-neutral-800">
                      {t('activityQuickEdit.tags.create')}
                    </button>
                  )}
                </div>
                {visibleSuggestions.length > 0 && (
                  <div className="mt-2 max-h-28 overflow-y-auto pr-1">
                    <div className="flex flex-wrap gap-1.5">
                      {visibleSuggestions.map(tag => (
                        <button key={tag.name} type="button" onClick={() => { toggleTag(tag.name, true); setTagSearch('') }} className="rounded-full bg-neutral-100 px-2 py-1 text-xs font-medium text-neutral-700 transition-colors hover:bg-neutral-200 dark:bg-neutral-800 dark:text-neutral-300 dark:hover:bg-neutral-700">
                          {tag.name}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </div>

            <label className="block">
              <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('activityQuickEdit.fields.description')}</span>
              <textarea
                value={draft.description}
                onChange={e => patchDraft({ description: e.target.value })}
                rows={4}
                className="w-full resize-y rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-100"
              />
            </label>

            <div className="grid gap-4 md:grid-cols-2">
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">RPE</span>
                <input
                  type="range"
                  min={1}
                  max={10}
                  value={draft.workoutRpe ?? 5}
                  onChange={e => patchDraft({ workoutRpe: Number(e.target.value) })}
                  className="w-full accent-accent-DEFAULT"
                />
                <div className="mt-1 flex items-center justify-between text-xs text-neutral-400">
                  <span>{draft.workoutRpe == null ? t('activityQuickEdit.rpe.notSet') : `${draft.workoutRpe}/10`}</span>
                  <button type="button" onClick={() => patchDraft({ workoutRpe: null })} className="hover:text-neutral-600 dark:hover:text-neutral-200">{t('activityQuickEdit.remove')}</button>
                </div>
                <div className="mt-1 flex justify-between text-[10px] text-neutral-400 dark:text-neutral-500">
                  <span>{t('activityQuickEdit.rpe.light')}</span>
                  <span>{t('activityQuickEdit.rpe.medium')}</span>
                  <span>{t('activityQuickEdit.rpe.maximum')}</span>
                </div>
              </label>
              <div>
                <span className="mb-1 block text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('activityQuickEdit.fields.feel')}</span>
                <div className="grid grid-cols-5 gap-1.5">
                  {FEEL_OPTIONS.map((emoji, index) => (
                    <button
                      key={emoji}
                      type="button"
                      onClick={() => patchDraft({ workoutFeel: draft.workoutFeel === index ? null : index })}
                      title={feelLabel(index, t)}
                      className={`flex min-w-0 flex-col items-center gap-0.5 rounded-lg border px-1.5 py-2 text-xs ${draft.workoutFeel === index
                        ? 'border-accent-DEFAULT bg-accent-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
                        : 'border-neutral-200 text-neutral-600 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800'
                      }`}
                    >
                      <span className="text-lg">{emoji}</span>
                      <span className="w-full truncate text-[10px]">{feelLabel(index, t)}</span>
                    </button>
                  ))}
                </div>
              </div>
            </div>
          </div>

          <aside className="space-y-3">
            <div className="rounded-lg border border-neutral-200 p-3 dark:border-neutral-700">
              <div className="mb-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">{t('activityQuickEdit.minimap')}</div>
              <div className="flex aspect-video items-center justify-center overflow-hidden rounded bg-neutral-100 dark:bg-neutral-800">
                <ActivityQuickEditMiniMap key={activity.id} streams={streams} loading={streamsLoading} />
              </div>
              {hasMapContext && (
                <div className="mt-3 space-y-2 border-t border-neutral-100 pt-3 dark:border-neutral-800">
                  {locationText && (
                    <ActivityContextRow icon={<span className="text-base leading-none text-red-500" aria-hidden>&#x1F4CD;</span>}>
                      {locationText}
                    </ActivityContextRow>
                  )}
                  {(baseActivity.sunrise_at || baseActivity.sunset_at) && (
                    <ActivityContextRow icon={<span className="text-base leading-none text-amber-500" aria-hidden>&#9728;</span>}>
                      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
                        {baseActivity.sunrise_at && <span>{fmtTime(baseActivity.sunrise_at, baseActivity.timezone)}</span>}
                        {baseActivity.sunset_at && (
                          <span className="inline-flex items-center gap-1.5">
                            <span className="text-accent-600" aria-hidden>&#9790;</span>
                            <span>{fmtTime(baseActivity.sunset_at, baseActivity.timezone)}</span>
                          </span>
                        )}
                      </div>
                    </ActivityContextRow>
                  )}
                  <ActivityContextRow icon={<span className="translate-x-0.5 text-base leading-none text-neutral-400" aria-hidden>&#x25B6;</span>}>
                    <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
                      <span>{fmtTime(baseActivity.started_at, baseActivity.timezone)}</span>
                      {baseActivity.ended_at && (
                        <span className="inline-flex items-center gap-1.5">
                          <span className="text-base leading-none text-neutral-400" aria-hidden>&#x25A0;</span>
                          <span>{fmtTime(baseActivity.ended_at, baseActivity.timezone)}</span>
                        </span>
                      )}
                    </div>
                  </ActivityContextRow>
                </div>
              )}
            </div>
            <div className="grid grid-cols-2 gap-2 text-sm">
              <Stat label={t('activityQuickEdit.stats.distance')} value={formatDistance(baseActivity.distance_m)} />
              <Stat label={t('activityQuickEdit.stats.time')} value={formatDuration(baseActivity.elapsed_time_s ?? 0)} />
              <Stat label={t('activityQuickEdit.stats.elevation')} value={elevationValue} />
              <Stat label={t('activityQuickEdit.stats.gps')} value={baseActivity.has_gps ? t('ruleEditor.choices.yes') : hasLoadedGpsTrack ? t('activityQuickEdit.stats.noTrackYes') : streamsLoading ? t('activityQuickEdit.stats.loading') : t('ruleEditor.choices.no')} />
              {heartRateValue && <Stat label={t('activityQuickEdit.stats.heartRate')} value={heartRateValue} />}
              {powerValue && <Stat label={t('activityQuickEdit.stats.power')} value={powerValue} />}
            </div>
          </aside>
        </div>

        {error && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
            {error}
          </div>
        )}

        <div className="flex flex-wrap justify-between gap-3 border-t border-neutral-100 pt-4 dark:border-neutral-800">
          {showMoreEdit ? (
            <button
              type="button"
              onClick={() => navigate(`/activities/${activity.id}/edit`)}
              className="rounded-lg border border-neutral-200 px-4 py-2 text-sm text-neutral-700 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800"
            >
              {t('activityQuickEdit.moreEdit')}
            </button>
          ) : <span />}
          <div className="flex gap-2">
            <button type="button" onClick={onClose} disabled={saving} className="rounded-lg border border-neutral-200 px-4 py-2 text-sm text-neutral-700 hover:bg-neutral-50 disabled:opacity-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800">
              {t('common.cancel')}
            </button>
            <button type="button" onClick={handleSave} disabled={saving} className="inline-flex items-center gap-2 rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:bg-accent-600 disabled:opacity-50">
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
              {t('account.actions.save')}
            </button>
          </div>
        </div>
      </div>
    </ModalShell>
  )
}

function ActivityContextRow({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[1rem_minmax(0,1fr)] items-center gap-2 text-sm text-neutral-600 dark:text-neutral-400">
      <span className="flex h-4 w-4 items-center justify-center">{icon}</span>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

function QuickEditSection({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section>
      <div className="mb-2 flex items-center justify-between gap-3">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">{title}</h3>
        {action}
      </div>
      {children}
    </section>
  )
}

function ToggleRow({ label, description, children }: { label: string; description?: string; children: ReactNode }) {
  return (
    <div className="flex min-h-10 items-center justify-between gap-3 rounded-md border border-neutral-200 bg-white px-3 py-2 dark:border-neutral-700 dark:bg-neutral-900">
      <div className="min-w-0">
        <div className="text-sm text-neutral-700 dark:text-neutral-300">{label}</div>
        {description && <div className="mt-0.5 text-xs text-neutral-400 dark:text-neutral-500">{description}</div>}
      </div>
      {children}
    </div>
  )
}

function formatHeartRate(value: number | null | undefined): string | null {
  if (value == null || !Number.isFinite(value) || value <= 0) return null
  return `${Math.round(value)} bpm`
}

function formatPower(value: number | null | undefined): string | null {
  if (value == null || !Number.isFinite(value) || value <= 0) return null
  return `${Math.round(value)} W`
}

function Stat({ label, value }: { label: string; value: string | null | undefined }) {
  return (
    <div className="rounded-lg border border-neutral-200 px-3 py-2 dark:border-neutral-700">
      <div className="text-[11px] text-neutral-400">{label}</div>
      <div className="mt-0.5 truncate text-sm font-medium text-neutral-800 dark:text-neutral-200">{value ?? '—'}</div>
    </div>
  )
}
