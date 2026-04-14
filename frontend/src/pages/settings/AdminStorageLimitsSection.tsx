import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { getAdminSettings, putAdminSettings, type SettingField } from '@/api/adminSettings'

// ── i18n-ready labels ────────────────────────────────────────────────────────

const LABELS = {
  title: 'Speicher-Limits',
  description: 'Upload- und Speichergrößen sowie Dateiversionierung.',
  maxUpload: {
    label: 'Max. Upload-Größe',
    unit: 'MB',
    hint: 'Maximale Dateigröße pro Upload.',
  },
  maxUserStorage: {
    label: 'Max. Benutzer-Speicher',
    unit: 'GB',
    hint: 'Gesamtspeicher pro Benutzer.',
  },
  keepVersions: {
    label: 'Versions-Aufbewahrung',
    options: {
      '0': 'Keine (sofort löschen)',
      '1': 'Letzte Version behalten',
      '-1': 'Alle Versionen behalten',
    } satisfies Record<string, string>,
    hint: 'Bestimmt, wie viele Dateiversionen aufbewahrt werden.',
    warnDelete: 'Alle bisherigen Dateiversionen werden dauerhaft gelöscht.',
  },
  lockedHint: (envVar: string) =>
    `Gesetzt per Umgebungsvariable — entferne ${envVar} aus .env`,
  save: 'Speichern',
  saving: 'Speichern…',
  saveSuccess: 'Limits gespeichert',
  saveError: 'Speichern fehlgeschlagen',
  loading: 'Lade Einstellungen…',
  loadError: 'Einstellungen konnten nicht geladen werden.',
  validationUpload: {
    title: 'Ungültige Max Upload-Größe',
    message: 'Bitte eine positive Zahl eingeben oder Feld leer lassen für Default.',
  },
  validationStorage: {
    title: 'Ungültiger Max Benutzer-Speicher',
    message: 'Bitte eine positive Zahl eingeben oder Feld leer lassen für Default.',
  },
}

// ── Constants ─────────────────────────────────────────────────────────────────

const MB = 1_048_576
const GB = 1_073_741_824

// Env-var names come from the backend keys.go registration.
const ENV_VARS: Record<string, string> = {
  'limits.max_upload_bytes': 'MAX_UPLOAD_SIZE',
  'limits.max_user_storage_bytes': 'MAX_USER_STORAGE_BYTES',
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
  const envVar = ENV_VARS[settingsKey] ?? settingsKey
  return (
    <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
      {LABELS.lockedHint(envVar)}
    </p>
  )
}

// ── Sub-component: field row ──────────────────────────────────────────────────

interface NumberFieldProps {
  label: string
  unit: string
  hint: string
  value: string
  locked: boolean
  settingsKey: string
  onChange: (v: string) => void
}

function NumberField({ label, unit, hint, value, locked, settingsKey, onChange }: NumberFieldProps) {
  return (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-neutral-800 dark:text-neutral-200">
        {label}
      </label>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">{hint}</p>
      <div className="flex items-center gap-2 mt-1.5">
        <input
          type="number"
          min={1}
          step={1}
          value={value}
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

// ── Main component ────────────────────────────────────────────────────────────

export function AdminStorageLimitsSection() {
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
  const [keepVersions, setKeepVersions] = useState('1')

  // Initialise from loaded settings (only on first successful load)
  useEffect(() => {
    if (!settings) return
    const uploadField = fieldOf(settings, 'limits', 'max_upload_bytes')
    const storageField = fieldOf(settings, 'limits', 'max_user_storage_bytes')
    const keepField = fieldOf(settings, 'library', 'keep_versions')

    if (uploadField) {
      const bytes = parseInt(uploadField.value, 10)
      setMaxUploadMB(isNaN(bytes) ? '50' : String(Math.round(bytes / MB)))
    }
    if (storageField) {
      const bytes = parseInt(storageField.value, 10)
      setMaxUserStorageGB(isNaN(bytes) ? '50' : String(Math.round(bytes / GB)))
    }
    if (keepField) {
      setKeepVersions(keepField.value)
    }
  }, [settings])

  const uploadField = fieldOf(settings, 'limits', 'max_upload_bytes')
  const storageField = fieldOf(settings, 'limits', 'max_user_storage_bytes')
  const keepField = fieldOf(settings, 'library', 'keep_versions')

  const uploadLocked = uploadField?.locked ?? false
  const storageLocked = storageField?.locked ?? false
  const keepLocked = keepField?.locked ?? false

  const saveMut = useMutation({
    mutationFn: (updates: Record<string, string | null>) => putAdminSettings(updates),
    onSuccess: () => {
      addToast({ type: 'success', title: LABELS.saveSuccess })
      queryClient.invalidateQueries({ queryKey: ['admin-settings'] })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: LABELS.saveError, message: err.message, duration: 0 })
    },
  })

  function handleSave() {
    const updates: Record<string, string | null> = {}

    if (!uploadLocked) {
      const mbStr = maxUploadMB.trim()
      if (mbStr === '') {
        updates['limits.max_upload_bytes'] = null
      } else {
        const mb = parseInt(mbStr, 10)
        if (isNaN(mb) || mb <= 0) {
          addToast({ type: 'error', title: LABELS.validationUpload.title, message: LABELS.validationUpload.message, duration: 0 })
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
        const gb = parseInt(gbStr, 10)
        if (isNaN(gb) || gb <= 0) {
          addToast({ type: 'error', title: LABELS.validationStorage.title, message: LABELS.validationStorage.message, duration: 0 })
          return
        }
        updates['limits.max_user_storage_bytes'] = String(gb * GB)
      }
    }

    if (!keepLocked) {
      updates['library.keep_versions'] = keepVersions
    }

    saveMut.mutate(updates)
  }

  function handleKeepVersionsChange(value: string) {
    if (value === '0') {
      addToast({ type: 'warning', title: LABELS.keepVersions.warnDelete })
    }
    setKeepVersions(value)
  }

  const allLocked = uploadLocked && storageLocked && keepLocked

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <p className="text-sm text-neutral-400">{LABELS.loading}</p>
      </div>
    )
  }

  if (isError) {
    return (
      <div className="rounded-xl border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 px-4 py-3">
        <p className="text-sm text-red-600 dark:text-red-400">{LABELS.loadError}</p>
      </div>
    )
  }

  return (
    <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-5">

      {/* Header */}
      <div>
        <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
          {LABELS.title}
        </h3>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          {LABELS.description}
        </p>
      </div>

      {/* Fields */}
      <div className="space-y-5">

        <NumberField
          label={LABELS.maxUpload.label}
          unit={LABELS.maxUpload.unit}
          hint={LABELS.maxUpload.hint}
          value={maxUploadMB}
          locked={uploadLocked}
          settingsKey="limits.max_upload_bytes"
          onChange={setMaxUploadMB}
        />

        <NumberField
          label={LABELS.maxUserStorage.label}
          unit={LABELS.maxUserStorage.unit}
          hint={LABELS.maxUserStorage.hint}
          value={maxUserStorageGB}
          locked={storageLocked}
          settingsKey="limits.max_user_storage_bytes"
          onChange={setMaxUserStorageGB}
        />

        {/* Versions-Aufbewahrung */}
        <div className="space-y-1">
          <label className="block text-sm font-medium text-neutral-800 dark:text-neutral-200">
            {LABELS.keepVersions.label}
          </label>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {LABELS.keepVersions.hint}
          </p>
          <select
            value={keepVersions}
            disabled={keepLocked}
            onChange={e => handleKeepVersionsChange(e.target.value)}
            className="mt-1.5 rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {Object.entries(LABELS.keepVersions.options).map(([val, label]) => (
              <option key={val} value={val}>{label}</option>
            ))}
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
          {saveMut.isPending ? LABELS.saving : LABELS.save}
        </button>
      </div>

    </section>
  )
}
