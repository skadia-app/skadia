import { useState, useMemo, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Activity } from '@/api/types'
import { apiFetch } from '@/lib/auth'
import { resolveSportConflict, type ResolveSportConflictPayload } from '@/api/sportConflict'
import { useToast } from '@/lib/toast'
import { CATEGORIES, SPORT_DEFS, normalizeSportTuple, sportDetailLabel } from '@/lib/sportTaxonomy'
import { modifierLabel } from '@/lib/sportModifiers'
import { ModalShell } from '@/components/ui/ModalShell'
import { localizeString, supportedLanguageFrom } from '@/i18n/localizedString'

interface SportConflictModalProps {
  activity: Activity
  open: boolean
  onClose: () => void
  onResolved: () => void
}

type Selection = {
  key: string
  payload: ResolveSportConflictPayload | undefined
  label: string
}

type SportPickerEntry = {
  sportType: string
  subType: string
  label: string
  displayLabel: string
  category: string
}

export default function SportConflictModal({ activity, open, onClose, onResolved }: SportConflictModalProps) {
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [selection, setSelection] = useState<Selection | null>(null)
  const { addToast } = useToast()

  // If opened from ActivitiesPage (list), sport_conflict meta may be null
  // (ListActivities doesn't JOIN import_queue). Fetch full activity if needed.
  const [fullActivity, setFullActivity] = useState<Activity | null>(null)
  const effectiveActivity = fullActivity ?? activity

  useEffect(() => {
    const abortCtrl = new AbortController()
    if (open && !activity.sport_conflict && activity.sport_conflict_pending) {
      apiFetch(`/api/v1/activities/${activity.id}`, { signal: abortCtrl.signal })
        .then(r => r.json())
        .then(a => { if (!abortCtrl.signal.aborted) setFullActivity(a) })
        .catch(() => {})
    }
    if (!open) {
      setFullActivity(null)
      setSelection(null)
      setSearch('')
    }
    return () => abortCtrl.abort()
  }, [open, activity.id, activity.sport_conflict, activity.sport_conflict_pending])

  const conflict = effectiveActivity.sport_conflict
  const hasConflictMeta = conflict && conflict.file_sport && conflict.platform_sport

  const bothGeneric = hasConflictMeta &&
    conflict.file_sport === 'generic' && conflict.platform_sport === 'generic'

  const allSports = useMemo(() => {
    // Deduplicate top-level entries that share label+category (e.g. strength vs strength_training).
    // First occurrence wins; later duplicates are hidden from the picker.
    const seenTopLevel = new Set<string>()
    return SPORT_DEFS.flatMap(sd => {
      const cat = CATEGORIES.find(c => c.key === sd.categoryKey)
      const category = cat ? localizeString(cat.labels, language) : sd.categoryKey
      const entries: SportPickerEntry[] = []
      if (sd.selectable) {
        const topKey = `${sd.label}:${category}`
        if (!seenTopLevel.has(topKey)) {
          seenTopLevel.add(topKey)
          const label = localizeString(sd.labels, language)
          entries.push({ sportType: sd.sportType, subType: '', label, displayLabel: label, category })
        }
      }
      for (const st of sd.subTypes) {
        if (st.selectable) {
          const parentLabel = localizeString(sd.labels, language)
          const subTypeLabel = localizeString(st.labels, language)
          entries.push({ sportType: sd.sportType, subType: st.subType, label: subTypeLabel, displayLabel: `${parentLabel} › ${subTypeLabel}`, category })
        }
      }
      return entries
    })
  }, [language])

  const filtered = useMemo(() => {
    if (!search) return allSports
    const q = search.toLowerCase()
    return allSports.filter(s =>
      s.label.toLowerCase().includes(q) || s.displayLabel.toLowerCase().includes(q) || s.sportType.includes(q) || s.subType.includes(q)
    )
  }, [allSports, search])

  const payloadForSport = (sportType: string, subType = ''): ResolveSportConflictPayload => {
    const tuple = normalizeSportTuple(sportType, subType)
    return {
      sport_type: tuple.sportType,
      sub_type: tuple.subType || undefined,
      is_electric: tuple.modifiers.isElectric,
      is_indoor: tuple.modifiers.isIndoor,
    }
  }
  const platformPayload = (): ResolveSportConflictPayload => ({
    sport_type: conflict!.platform_sport,
    sub_type: conflict!.platform_sub_type || undefined,
    is_electric: conflict!.platform_is_electric,
    is_indoor: conflict!.platform_is_indoor,
    is_virtual: conflict!.platform_is_virtual,
  })
  const filePayload = (): ResolveSportConflictPayload => ({
    sport_type: conflict!.file_sport,
    sub_type: conflict!.file_sub_type || undefined,
    is_electric: conflict!.file_is_electric,
    is_indoor: conflict!.file_is_indoor,
    is_virtual: conflict!.file_is_virtual,
  })

  const conflictLabel = (sportType: string, subType?: string, isElectric?: boolean, isIndoor?: boolean, isVirtual?: boolean) => {
    const modifiers = [
      isElectric ? modifierLabel('is_electric', language) : '',
      isIndoor ? modifierLabel('is_indoor', language) : '',
      isVirtual ? modifierLabel('is_virtual', language) : '',
    ].filter(Boolean)
    return [sportDetailLabel(sportType, subType || null, language), ...modifiers].join(' · ')
  }

  const handleConfirm = async () => {
    if (!selection) return
    setLoading(true)
    try {
      await resolveSportConflict(activity.id, selection.payload)
      addToast({
        type: 'success',
        title: selection.payload
          ? t('activityOperations.sportConflict.toasts.updated')
          : t('activityOperations.sportConflict.toasts.resolved'),
      })
      onResolved()
      onClose()
    } catch {
      addToast({ type: 'error', title: t('activityOperations.sportConflict.toasts.resolveFailed'), duration: 0 })
    } finally {
      setLoading(false)
    }
  }

  if (!open) return null

  const sourceLabel = (src: string) => {
    switch (src) {
      case 'strava': return 'Strava'
      case 'garmin': return 'Garmin'
      case 'komoot': return 'Komoot'
      case 'fit': return t('activityOperations.sportConflict.sources.fit')
      case 'gpx': return t('activityOperations.sportConflict.sources.gpx')
      case 'tcx': return t('activityOperations.sportConflict.sources.tcx')
      default: return t('activityOperations.sportConflict.sources.unknown')
    }
  }

  const selectionButtonClass = (key: string) =>
    selection?.key === key
      ? 'bg-accent-DEFAULT text-white shadow-sm hover:bg-accent-600'
      : 'border border-accent-DEFAULT/35 bg-accent-DEFAULT/10 text-accent-DEFAULT hover:bg-accent-DEFAULT/20 dark:border-accent-DEFAULT/45 dark:bg-accent-DEFAULT/15 dark:text-accent-DEFAULT dark:hover:bg-accent-DEFAULT/25'

  const selectionListClass = (key: string) =>
    selection?.key === key
      ? 'bg-accent-DEFAULT/10 dark:bg-accent-DEFAULT/15 text-neutral-900 dark:text-neutral-100'
      : 'hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-700 dark:text-neutral-300'

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md">
      <div className="space-y-4">
        {/* Header */}
        <div>
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            {t('activityOperations.sportConflict.title')}
          </h2>
          <p className="mt-1 text-sm text-neutral-600 dark:text-neutral-400">
            {bothGeneric
              ? t('activityOperations.sportConflict.noSportDetected')
              : t('activityOperations.sportConflict.description')}
          </p>
          {effectiveActivity.sport_type && (
            <p className="mt-2 text-xs text-neutral-500 dark:text-neutral-500">
              {t('activityOperations.sportConflict.currentPrefix')}{' '}
              <span className="font-medium text-neutral-700 dark:text-neutral-300">
                {sportDetailLabel(effectiveActivity.sport_type, effectiveActivity.sub_type ?? null, language)}
              </span>
            </p>
          )}
        </div>

        {/* Conflict info cards */}
        {hasConflictMeta && !bothGeneric && (
          <div className="grid grid-cols-2 gap-3">
            <div className="bg-neutral-100 dark:bg-neutral-800 rounded-lg p-3 text-center">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">
                {sourceLabel(conflict.platform_source)}
              </div>
              <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                {conflictLabel(conflict.platform_sport, conflict.platform_sub_type, conflict.platform_is_electric, conflict.platform_is_indoor, conflict.platform_is_virtual)}
              </div>
            </div>
            <div className="bg-neutral-100 dark:bg-neutral-800 rounded-lg p-3 text-center">
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">
                {sourceLabel(conflict.file_source)}
              </div>
              <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                {conflictLabel(conflict.file_sport, conflict.file_sub_type, conflict.file_is_electric, conflict.file_is_indoor, conflict.file_is_virtual)}
              </div>
            </div>
          </div>
        )}

        {/* Quick select buttons */}
        {hasConflictMeta && !bothGeneric && (
          <div className="flex gap-2">
            <button
              disabled={loading}
              onClick={() => setSelection({
                key: 'platform',
                payload: platformPayload(),
                label: conflictLabel(conflict.platform_sport, conflict.platform_sub_type, conflict.platform_is_electric, conflict.platform_is_indoor, conflict.platform_is_virtual),
              })}
              className={`flex-1 px-3 py-2 rounded-lg text-sm cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${selectionButtonClass('platform')}`}
            >
              {conflictLabel(conflict.platform_sport, conflict.platform_sub_type, conflict.platform_is_electric, conflict.platform_is_indoor, conflict.platform_is_virtual)}
            </button>
            <button
              disabled={loading}
              onClick={() => setSelection({
                key: 'file',
                payload: filePayload(),
                label: conflictLabel(conflict.file_sport, conflict.file_sub_type, conflict.file_is_electric, conflict.file_is_indoor, conflict.file_is_virtual),
              })}
              className={`flex-1 px-3 py-2 rounded-lg text-sm cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${selectionButtonClass('file')}`}
            >
              {conflictLabel(conflict.file_sport, conflict.file_sub_type, conflict.file_is_electric, conflict.file_is_indoor, conflict.file_is_virtual)}
            </button>
          </div>
        )}

        {/* Search */}
        <input
          type="text"
          placeholder={t('activityOperations.sportConflict.searchPlaceholder')}
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="w-full px-3 py-2 rounded-lg bg-white dark:bg-neutral-800 border border-neutral-300 dark:border-neutral-600 text-sm text-neutral-900 dark:text-neutral-100 placeholder:text-neutral-400 dark:placeholder:text-neutral-500 focus:outline-none focus:border-neutral-400 dark:focus:border-neutral-500"
        />

        {/* Sport list */}
        <div className="space-y-0.5 max-h-48 overflow-y-auto">
          {filtered.map(s => {
            const key = `sport:${s.sportType}:${s.subType}`
            return (
              <button
                key={key}
                disabled={loading}
                onClick={() => setSelection({ key, payload: payloadForSport(s.sportType, s.subType), label: s.displayLabel })}
                className={`w-full text-left px-3 py-1.5 rounded-lg text-sm cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${selectionListClass(key)}`}
              >
                <span>{s.displayLabel}</span>
                <span className="text-xs text-neutral-400 dark:text-neutral-500 ml-2">{s.category}</span>
              </button>
            )
          })}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between gap-3 pt-2 border-t border-neutral-200 dark:border-neutral-800">
          <button
            disabled={loading}
              onClick={() => setSelection({ key: 'keep', payload: undefined, label: t('activityOperations.sportConflict.keepImported') })}
            className={`cursor-pointer px-3 py-2 rounded-lg text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed border ${
              selection?.key === 'keep'
                ? 'ring-2 ring-accent-DEFAULT border-accent-DEFAULT/30 bg-accent-DEFAULT/10 dark:bg-accent-DEFAULT/15 text-neutral-900 dark:text-neutral-100'
                : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
            }`}
          >
            {t('activityOperations.sportConflict.keepImported')}
          </button>
          <div className="flex gap-2 shrink-0">
            <button
              onClick={onClose}
              disabled={loading}
              className="cursor-pointer px-4 py-2 rounded-lg text-sm border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {t('common.cancel')}
            </button>
            <button
              onClick={handleConfirm}
              disabled={loading || !selection}
              className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT hover:bg-accent-600 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {loading ? t('common.saving') : t('activityOperations.sportConflict.apply')}
            </button>
          </div>
        </div>
      </div>
    </ModalShell>
  )
}
