import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { getWeatherSettings, updateWeatherSettings } from '@/api/settings'
import { apiClient } from '@/api/client'
import type { WeatherPrefs } from '@/api/types'
import { WEATHER_DISPLAY_FIELD_OPTIONS } from '@/api/types'
import Toggle from '@/components/Toggle'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import {
  elevationFromDisplayValue,
  elevationInputUnit,
  elevationToDisplayValue,
  type UnitSystem,
} from '@/lib/units'

function roundWeatherThresholdValue(value: number): number {
  return Number(value.toFixed(3))
}

function elevationDisplayValue(meters: number, units: UnitSystem): number {
  return roundWeatherThresholdValue(elevationToDisplayValue(meters, units))
}

function elevationStoredValue(value: string, units: UnitSystem): number | null {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return null
  return roundWeatherThresholdValue(elevationFromDisplayValue(numeric, units))
}

export function WeatherSettings() {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const queryClient = useQueryClient()
  const { options: unitOptions } = useUnitFormatters()

  const { data: prefs, isLoading } = useQuery({
    queryKey: ['weatherSettings'],
    queryFn: getWeatherSettings,
  })

  const [localPrefs, setLocalPrefs] = useState<WeatherPrefs | null>(null)

  useEffect(() => {
    if (prefs && !localPrefs) {
      setLocalPrefs(prefs)
    }
  }, [prefs, localPrefs])

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (timerRef.current) clearTimeout(timerRef.current)
  }, [])

  const mutation = useMutation({
    mutationFn: updateWeatherSettings,
    onSuccess: (data) => {
      queryClient.setQueryData(['weatherSettings'], data)
      setLocalPrefs(null)
    },
    onError: () => {
      addToast({ type: 'error', title: t('settings.weather.toasts.errorTitle'), message: t('settings.weather.toasts.saveFailed'), duration: 0 })
    },
  })

  const backfillMutation = useMutation({
    mutationFn: () => apiClient<void>('/admin/backfill-weather', { method: 'POST' }),
    onSuccess: () => {
      addToast({ type: 'success', title: t('settings.weather.toasts.backfillStarted'), message: t('settings.weather.toasts.runsInBackground') })
    },
    onError: () => {
      addToast({ type: 'error', title: t('settings.weather.toasts.errorTitle'), message: t('settings.weather.toasts.backfillFailed'), duration: 0 })
    },
  })

  const update = (patch: Partial<WeatherPrefs>) => {
    if (!localPrefs) return
    const next = { ...localPrefs, ...patch }
    setLocalPrefs(next)
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => mutation.mutate(next), 800)
  }
  const elevationUnit = elevationInputUnit(unitOptions.units)
  const elevationUnitText = t('settings.weather.thresholds.elevationUnit').replace(/^m\b/, elevationUnit)

  if (isLoading || !localPrefs) {
    return (
      <SettingsSection title={t('settings.weather.title')}>
        <div className="text-sm text-neutral-500 dark:text-neutral-400">{t('settings.weather.loadingSettings')}</div>
      </SettingsSection>
    )
  }

  return (
    <SettingsSection
      title={t('settings.weather.title')}
      description={t('settings.weather.description')}
      className="space-y-6"
    >

      {/* Section 1: reading count */}
      <div className="space-y-2">
        <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{t('settings.weather.readingMode.title')}</div>
        <div className="space-y-2">
          {[
            {
              value: 'auto' as const,
              label: t('settings.weather.readingMode.auto'),
              hint: t('settings.weather.readingMode.autoHint'),
            },
            {
              value: 'always_one' as const,
              label: t('settings.weather.readingMode.alwaysOne'),
              hint: null,
            },
            {
              value: 'always_two' as const,
              label: t('settings.weather.readingMode.alwaysTwo'),
              hint: t('settings.weather.readingMode.alwaysTwoHint'),
            },
          ].map(({ value, label, hint }) => (
            <label key={value} className="flex items-start gap-2.5 cursor-pointer">
              <input
                type="radio"
                name="reading_mode"
                value={value}
                checked={localPrefs.reading_mode === value}
                onChange={() => update({ reading_mode: value })}
                className="mt-0.5 accent-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-800 dark:text-neutral-200">
                {label}
                {hint && (
                  <span className="text-neutral-500 dark:text-neutral-400 ml-1 text-xs">({hint})</span>
                )}
              </span>
            </label>
          ))}
        </div>
      </div>

      {/* Section 2: thresholds (only for auto or always_two) */}
      {localPrefs.reading_mode !== 'always_one' && (
        <div className="space-y-4">
          <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{t('settings.weather.thresholds.title')}</div>

          <div className="space-y-1">
            <label htmlFor="mountain-threshold" className="text-xs text-neutral-500 dark:text-neutral-400">
              {t('settings.weather.thresholds.elevationLabel')}
            </label>
            <div className="flex items-center gap-2">
              <input
                id="mountain-threshold"
                type="number"
                min={elevationDisplayValue(50, unitOptions.units)}
                max={elevationDisplayValue(3000, unitOptions.units)}
                step={elevationDisplayValue(50, unitOptions.units)}
                value={elevationDisplayValue(localPrefs.mountain_threshold_m, unitOptions.units)}
                onChange={(e) => {
                  const storedValue = elevationStoredValue(e.target.value, unitOptions.units)
                  if (storedValue != null) update({ mountain_threshold_m: storedValue })
                }}
                className="w-24 rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-500 dark:text-neutral-400">{elevationUnitText}</span>
            </div>
            <p className="text-xs text-neutral-400 dark:text-neutral-500">
              {t('settings.weather.thresholds.elevationHint')}
            </p>
          </div>

          <div className="space-y-1">
            <label htmlFor="long-duration" className="text-xs text-neutral-500 dark:text-neutral-400">
              {t('settings.weather.thresholds.durationLabel')}
            </label>
            <div className="flex items-center gap-2">
              <input
                id="long-duration"
                type="number"
                min={30}
                max={480}
                step={30}
                value={Math.round(localPrefs.long_duration_s / 60)}
                onChange={(e) => update({ long_duration_s: Number(e.target.value) * 60 })}
                className="w-24 rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-500 dark:text-neutral-400">{t('settings.weather.thresholds.durationUnit')}</span>
            </div>
            <p className="text-xs text-neutral-400 dark:text-neutral-500">
              {t('settings.weather.thresholds.durationHint')}
            </p>
          </div>
        </div>
      )}

      {/* Section 3: Angezeigte Felder */}
      <div className="space-y-2">
        <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{t('settings.weather.displayFields.title')}</div>
        <div className="grid grid-cols-2 gap-2">
          {WEATHER_DISPLAY_FIELD_OPTIONS.map((opt) => {
            const checked = localPrefs.display_fields.includes(opt.key)
            const isLast = localPrefs.display_fields.length === 1 && checked
            return (
              <label key={opt.key} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={isLast}
                  onChange={(e) => {
                    const next = e.target.checked
                      ? [...localPrefs.display_fields, opt.key]
                      : localPrefs.display_fields.filter((f) => f !== opt.key)
                    if (next.length > 0) update({ display_fields: next })
                  }}
                  className="accent-accent-DEFAULT disabled:opacity-40 disabled:cursor-not-allowed"
                />
                <span className="text-sm text-neutral-800 dark:text-neutral-200">{t(`settings.weather.displayFields.options.${opt.key}`)}</span>
              </label>
            )
          })}
        </div>
      </div>

      {/* Automatic weather tags */}
      <div className="flex items-center justify-between">
        <div>
          <h4 className="text-sm font-medium text-neutral-900 dark:text-white">
            {t('settings.weather.autoTags.title')}
          </h4>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {t('settings.weather.autoTags.description')}
          </p>
        </div>
        <Toggle
          checked={localPrefs?.auto_tags ?? true}
          onChange={() => update({ auto_tags: !(localPrefs?.auto_tags ?? true) })}
          aria-label={t('settings.weather.autoTags.title')}
        />
      </div>

      {/* Backfill-Button */}
      <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60">
        <p className="text-xs text-neutral-400 dark:text-neutral-500 mb-3">
          {t('settings.weather.backfill.description')}
        </p>
        <button
          onClick={() => backfillMutation.mutate()}
          disabled={backfillMutation.isPending}
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {backfillMutation.isPending ? t('settings.weather.backfill.starting') : t('settings.weather.backfill.start')}
        </button>
      </div>
    </SettingsSection>
  )
}
