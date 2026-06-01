import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useToast } from '@/lib/toast'
import { getAdminSettings, putAdminSettings, type SettingField } from '@/api/adminSettings'

// ── Constants ─────────────────────────────────────────────────────────────────

const MB = 1_048_576
const GB = 1_073_741_824

// Env-var names come from the backend keys.go registration.
const ENV_VARS: Record<string, string> = {
  'limits.max_upload_bytes': 'MAX_UPLOAD_SIZE',
  'limits.max_user_storage_bytes': 'MAX_USER_STORAGE_BYTES',
  'limits.max_activity_photos_per_activity': 'MAX_ACTIVITY_PHOTOS_PER_ACTIVITY',
  'limits.max_route_photos_per_route': 'MAX_ROUTE_PHOTOS_PER_ROUTE',
  'library.keep_versions': 'LIBRARY_KEEP_VERSIONS',
}

// ── Helper ────────────────────────────────────────────────────────────────────

function fieldOf(
  settings: Record<string, Record<string, SettingField>> | undefined,
  group: string,
  key: string,
): SettingField | undefined {
  return settings?.[group]?.[key]
}

// ── Sub-component: locked hint ────────────────────────────────────────────────

function LockedHint({ settingsKey }: { settingsKey: string }) {
  const { t } = useTranslation()
  const envVar = ENV_VARS[settingsKey] ?? settingsKey
  return (
    <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
      {t('settings.adminStorageLimits.lockedHint', { envVar })}
    </p>
  )
}

// ── Sub-component: field row ──────────────────────────────────────────────────

interface NumberFieldProps {
  label: string
  unit: string
  hint: string
  value: string
  placeholder?: string
  locked: boolean
  settingsKey: string
  min?: number
  onChange: (v: string) => void
}

function NumberField({ label, unit, hint, value, placeholder, locked, settingsKey, min = 1, onChange }: NumberFieldProps) {
  return (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-neutral-800 dark:text-neutral-200">
        {label}
      </label>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">{hint}</p>
      <div className="flex items-center gap-2 mt-1.5">
        <input
          type="number"
          min={min}
          step={1}
          value={value}
          placeholder={placeholder}
          disabled={locked}
          onChange={e => onChange(e.target.value)}
          className="w-36 rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed"
        />
        <span className="text-sm text-neutral-500 dark:text-neutral-400">{unit}</span>
      </div>
      {locked && <LockedHint settingsKey={settingsKey} />}
    </div>
  )
}

function draftFromField(field: SettingField | undefined, divisor = 1): string {
  if (!field || field.source === 'default') return ''
  const value = parseInt(field.value, 10)
  if (Number.isNaN(value)) return ''
  if (value === 0) return '0'
  if (value > 0) return String(Math.round(value / divisor))
  return ''
}

function placeholderFromDefault(field: SettingField | undefined, divisor = 1, standardLabel: string): string | undefined {
  if (!field) return undefined
  const value = parseInt(field.value, 10)
  if (Number.isNaN(value) || value < 0) return undefined
  return `${standardLabel}: ${Math.round(value / divisor)}`
}

function parseStrictInteger(value: string): number | null {
  if (!/^(0|[1-9]\d*)$/.test(value)) return null
  const parsed = Number(value)
  if (!Number.isSafeInteger(parsed)) return null
  return parsed
}

// ── Main component ────────────────────────────────────────────────────────────

export function AdminStorageLimitsSection() {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const { data: settings, isLoading, isError } = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getAdminSettings,
    staleTime: 60_000,
  })

  // Local draft state (display units)
  const [maxUploadMB, setMaxUploadMB] = useState('')
  const [maxUserStorageGB, setMaxUserStorageGB] = useState('')
  const [maxActivityPhotos, setMaxActivityPhotos] = useState('')
  const [maxRoutePhotos, setMaxRoutePhotos] = useState('')
  const [keepVersions, setKeepVersions] = useState('1')

  // Initialise from loaded settings (only on first successful load)
  useEffect(() => {
    if (!settings) return
    const uploadField = fieldOf(settings, 'limits', 'max_upload_bytes')
    const storageField = fieldOf(settings, 'limits', 'max_user_storage_bytes')
    const activityPhotosField = fieldOf(settings, 'limits', 'max_activity_photos_per_activity')
    const routePhotosField = fieldOf(settings, 'limits', 'max_route_photos_per_route')
    const keepField = fieldOf(settings, 'library', 'keep_versions')

    setMaxUploadMB(draftFromField(uploadField, MB))
    setMaxUserStorageGB(draftFromField(storageField, GB))
    setMaxActivityPhotos(draftFromField(activityPhotosField))
    setMaxRoutePhotos(draftFromField(routePhotosField))
    if (keepField) {
      setKeepVersions(keepField.value)
    }
  }, [settings])

  const uploadField = fieldOf(settings, 'limits', 'max_upload_bytes')
  const storageField = fieldOf(settings, 'limits', 'max_user_storage_bytes')
  const activityPhotosField = fieldOf(settings, 'limits', 'max_activity_photos_per_activity')
  const routePhotosField = fieldOf(settings, 'limits', 'max_route_photos_per_route')
  const keepField = fieldOf(settings, 'library', 'keep_versions')

  const uploadLocked = uploadField?.locked ?? false
  const storageLocked = storageField?.locked ?? false
  const activityPhotosLocked = activityPhotosField?.locked ?? false
  const routePhotosLocked = routePhotosField?.locked ?? false
  const keepLocked = keepField?.locked ?? false

  const saveMut = useMutation({
    mutationFn: (updates: Record<string, string | null>) => putAdminSettings(updates),
    onSuccess: () => {
      addToast({ type: 'success', title: t('settings.adminStorageLimits.saveSuccess') })
      queryClient.invalidateQueries({ queryKey: ['admin-settings'] })
    },
    onError: () => {
      addToast({
        type: 'error',
        title: t('settings.adminStorageLimits.saveError'),
        duration: 0,
      })
    },
  })

  function handleSave() {
    const updates: Record<string, string | null> = {}

    if (!uploadLocked) {
      const mbStr = maxUploadMB.trim()
      if (mbStr === '') {
        updates['limits.max_upload_bytes'] = null
      } else {
        const mb = parseStrictInteger(mbStr)
        if (mb === null || mb <= 0) {
          addToast({ type: 'error', title: t('settings.adminStorageLimits.validationUpload.title'), message: t('settings.adminStorageLimits.validationUpload.message'), duration: 0 })
          return
        }
        updates['limits.max_upload_bytes'] = String(mb * MB)
      }
    }

    if (!storageLocked) {
      const gbStr = maxUserStorageGB.trim()
      if (gbStr === '') {
        updates['limits.max_user_storage_bytes'] = null
      } else {
        const gb = parseStrictInteger(gbStr)
        if (gb === null || gb < 0) {
          addToast({ type: 'error', title: t('settings.adminStorageLimits.validationStorage.title'), message: t('settings.adminStorageLimits.validationStorage.message'), duration: 0 })
          return
        }
        updates['limits.max_user_storage_bytes'] = String(gb * GB)
      }
    }

    if (!activityPhotosLocked) {
      const countStr = maxActivityPhotos.trim()
      if (countStr === '') {
        updates['limits.max_activity_photos_per_activity'] = null
      } else {
        const count = parseStrictInteger(countStr)
        if (count === null || count < 0) {
          addToast({ type: 'error', title: t('settings.adminStorageLimits.validationMediaCount.title'), message: t('settings.adminStorageLimits.validationMediaCount.message'), duration: 0 })
          return
        }
        updates['limits.max_activity_photos_per_activity'] = String(count)
      }
    }

    if (!routePhotosLocked) {
      const countStr = maxRoutePhotos.trim()
      if (countStr === '') {
        updates['limits.max_route_photos_per_route'] = null
      } else {
        const count = parseStrictInteger(countStr)
        if (count === null || count < 0) {
          addToast({ type: 'error', title: t('settings.adminStorageLimits.validationMediaCount.title'), message: t('settings.adminStorageLimits.validationMediaCount.message'), duration: 0 })
          return
        }
        updates['limits.max_route_photos_per_route'] = String(count)
      }
    }

    if (!keepLocked) {
      updates['library.keep_versions'] = keepVersions
    }

    saveMut.mutate(updates)
  }

  function handleKeepVersionsChange(value: string) {
    if (value === '0') {
      addToast({ type: 'warning', title: t('settings.adminStorageLimits.keepVersions.warnDelete') })
    }
    setKeepVersions(value)
  }

  const allLocked = uploadLocked && storageLocked && activityPhotosLocked && routePhotosLocked && keepLocked

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <p className="text-sm text-neutral-400">{t('settings.adminStorageLimits.loading')}</p>
      </div>
    )
  }

  if (isError) {
    return (
      <div className="rounded-xl border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 px-4 py-3">
        <p className="text-sm text-red-600 dark:text-red-400">{t('settings.adminStorageLimits.loadError')}</p>
      </div>
    )
  }

  return (
    <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-5">

      {/* Header */}
      <div>
        <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
          {t('settings.adminStorageLimits.title')}
        </h3>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          {t('settings.adminStorageLimits.description')}
        </p>
      </div>

      {/* Fields */}
      <div className="space-y-5">

        <NumberField
          label={t('settings.adminStorageLimits.maxUpload.label')}
          unit={t('settings.adminStorageLimits.maxUpload.unit')}
          hint={t('settings.adminStorageLimits.maxUpload.hint')}
          value={maxUploadMB}
          placeholder={placeholderFromDefault(uploadField, MB, t('settings.adminStorageLimits.standard'))}
          locked={uploadLocked}
          settingsKey="limits.max_upload_bytes"
          onChange={setMaxUploadMB}
        />

        <NumberField
          label={t('settings.adminStorageLimits.maxUserStorage.label')}
          unit={t('settings.adminStorageLimits.maxUserStorage.unit')}
          hint={t('settings.adminStorageLimits.maxUserStorage.hint')}
          value={maxUserStorageGB}
          placeholder={placeholderFromDefault(storageField, GB, t('settings.adminStorageLimits.standard'))}
          locked={storageLocked}
          settingsKey="limits.max_user_storage_bytes"
          min={0}
          onChange={setMaxUserStorageGB}
        />

        <NumberField
          label={t('settings.adminStorageLimits.maxActivityPhotos.label')}
          unit={t('settings.adminStorageLimits.maxActivityPhotos.unit')}
          hint={t('settings.adminStorageLimits.maxActivityPhotos.hint')}
          value={maxActivityPhotos}
          placeholder={placeholderFromDefault(activityPhotosField, 1, t('settings.adminStorageLimits.standard'))}
          locked={activityPhotosLocked}
          settingsKey="limits.max_activity_photos_per_activity"
          min={0}
          onChange={setMaxActivityPhotos}
        />

        <NumberField
          label={t('settings.adminStorageLimits.maxRoutePhotos.label')}
          unit={t('settings.adminStorageLimits.maxRoutePhotos.unit')}
          hint={t('settings.adminStorageLimits.maxRoutePhotos.hint')}
          value={maxRoutePhotos}
          placeholder={placeholderFromDefault(routePhotosField, 1, t('settings.adminStorageLimits.standard'))}
          locked={routePhotosLocked}
          settingsKey="limits.max_route_photos_per_route"
          min={0}
          onChange={setMaxRoutePhotos}
        />

        {/* Version retention */}
        <div className="space-y-1">
          <label className="block text-sm font-medium text-neutral-800 dark:text-neutral-200">
            {t('settings.adminStorageLimits.keepVersions.label')}
          </label>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {t('settings.adminStorageLimits.keepVersions.hint')}
          </p>
          <select
            value={keepVersions}
            disabled={keepLocked}
            onChange={e => handleKeepVersionsChange(e.target.value)}
            className="mt-1.5 rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <option value="0">{t('settings.adminStorageLimits.keepVersions.options.0')}</option>
            <option value="1">{t('settings.adminStorageLimits.keepVersions.options.1')}</option>
            <option value="-1">{t('settings.adminStorageLimits.keepVersions.options.-1')}</option>
          </select>
          {keepLocked && <LockedHint settingsKey="library.keep_versions" />}
        </div>

      </div>

      {/* Save */}
      <div className="pt-1 border-t border-neutral-100 dark:border-neutral-800">
        <button
          onClick={handleSave}
          disabled={saveMut.isPending || allLocked}
          className="px-4 py-1.5 rounded-lg text-sm font-medium bg-accent-DEFAULT text-white hover:bg-accent-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
        >
          {saveMut.isPending ? t('settings.adminStorageLimits.saving') : t('settings.adminStorageLimits.save')}
        </button>
      </div>

    </section>
  )
}
